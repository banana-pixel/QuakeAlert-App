package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/banana-pixel/quakealert/server/internal/consensus"
	"github.com/banana-pixel/quakealert/server/internal/store"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// --- Fakes ---

type fakeSaver struct {
	mu         sync.Mutex
	saved      []*store.EarthquakeEvent
	resolved   []string
	saveErr    error
	resolveErr error
	nextID     int
}

func (f *fakeSaver) SaveEvent(_ context.Context, e *store.EarthquakeEvent) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.saveErr != nil {
		return "", f.saveErr
	}
	f.saved = append(f.saved, e)
	f.nextID++
	return fmt.Sprintf("event-%d", f.nextID), nil
}

func (f *fakeSaver) ResolveEvent(_ context.Context, eventID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.resolveErr != nil {
		return f.resolveErr
	}
	f.resolved = append(f.resolved, eventID)
	return nil
}

func (f *fakeSaver) snapshot() ([]*store.EarthquakeEvent, []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*store.EarthquakeEvent(nil), f.saved...), append([]string(nil), f.resolved...)
}

// fakeTargetedSaver menambahkan tokenFinder ke fakeSaver sehingga dispatcher
// memilih jalur token bertarget (type assertion di nearbyTokens).
type fakeTargetedSaver struct {
	fakeSaver
	tokens []string
	err    error

	mu    sync.Mutex
	calls []float64 // radius km yang diminta, untuk memastikan AlertRadiusKm dipakai
}

func (f *fakeTargetedSaver) FCMTokensWithin(_ context.Context, _, _ float64, rangeKm int) ([]string, error) {
	f.mu.Lock()
	f.calls = append(f.calls, float64(rangeKm))
	f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	return f.tokens, nil
}

type fakeFCM struct {
	mu    sync.Mutex
	sends []*FCMMessage
}

func (f *fakeFCM) Send(_ context.Context, m *FCMMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sends = append(f.sends, m)
	return nil
}

// targets mengembalikan token dan topic yang benar-benar dikirimi, terpisah,
// karena perbedaan keduanya adalah inti dari penargetan.
func (f *fakeFCM) targets() (tokens []string, topics []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, m := range f.sends {
		if m.Token != "" {
			tokens = append(tokens, m.Token)
		}
		if m.Topic != "" {
			topics = append(topics, m.Topic)
		}
	}
	return tokens, topics
}

func (f *fakeFCM) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sends)
}

// --- Helpers ---

// registerClient memasukkan client palsu (tanpa koneksi WS nyata) agar
// Broadcast dapat dicegat lewat channel send.
func registerClient(h *Hub) *client {
	c := &client{send: make(chan []byte, clientSendBuffer)}
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	return c
}

func readMessage(t *testing.T, ch chan []byte) []byte {
	t.Helper()
	select {
	case m := <-ch:
		return m
	case <-time.After(2 * time.Second):
		t.Fatal("timeout menunggu pesan WebSocket")
		return nil
	}
}

func waitFor(cond func() bool) bool {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

func confirmedEvent() *consensus.Event {
	return &consensus.Event{
		Status:   consensus.StatusConfirmed,
		Centroid: consensus.Centroid{Lat: -6.9, Lon: 107.6},
		// Sengaja di bawah SeverePGAGal dan di bawah MMI VII: event ini menguji
		// jalur bertarget, yang hanya dipakai kalau IsSevere tidak menyala.
		MaxPGA:         120,
		MMIScale:       "VI",
		IntensityLabel: "strong",
		NodeCount:      3,
		LocationName:   "Bandung, West Java, ID",
		CreatedAtMs:    time.Now().UnixMilli(),
	}
}

// --- Tests ---

// Skenario: CONFIRMED -> persist + broadcast ALERT (dengan event_id &
// location_name) -> setelah resolveAfter, broadcast EVENT_RESOLVED ber-event_id
// sama + DB ditandai RESOLVED. FCM terkirim async (alert + resolved).
func TestDispatcherConfirmedResolvesAndBroadcasts(t *testing.T) {
	saver := &fakeSaver{}
	fcm := &fakeFCM{}
	h := NewHub(testLogger(), func(*http.Request) bool { return true })
	d := NewDispatcher(saver, h, fcm, 20*time.Millisecond, testLogger())
	c := registerClient(h)

	d.Dispatch(context.Background(), confirmedEvent())

	// 1. Broadcast ALERT dengan event_id stabil + location_name terisi.
	first := readMessage(t, c.send)
	var alert AlertMessage
	if err := json.Unmarshal(first, &alert); err != nil {
		t.Fatalf("unmarshal alert: %v", err)
	}
	if alert.Type != TypeAlert {
		t.Fatalf("type = %s, want EARTHQUAKE_ALERT", alert.Type)
	}
	if alert.EventID == "" {
		t.Fatal("event_id kosong, want event_id dari persistensi")
	}
	if alert.LocationName != "Bandung, West Java, ID" {
		t.Fatalf("location_name = %q, want Bandung, West Java, ID", alert.LocationName)
	}

	// 2. Event dipersistensikan lengkap dengan location_name.
	saved, resolved := saver.snapshot()
	if len(saved) != 1 {
		t.Fatalf("saved = %d, want 1", len(saved))
	}
	if saved[0].LocationName != "Bandung, West Java, ID" {
		t.Fatalf("persisted location_name = %q", saved[0].LocationName)
	}

	// 3. All-clear: EVENT_RESOLVED dengan event_id SAMA.
	second := readMessage(t, c.send)
	var res AlertMessage
	if err := json.Unmarshal(second, &res); err != nil {
		t.Fatalf("unmarshal resolved: %v", err)
	}
	if res.Type != TypeResolved {
		t.Fatalf("type = %s, want EVENT_RESOLVED", res.Type)
	}
	if res.EventID != alert.EventID {
		t.Fatalf("resolved event_id = %q, want %q (sama)", res.EventID, alert.EventID)
	}

	// 4. DB ditandai RESOLVED.
	saved, resolved = saver.snapshot()
	if len(resolved) != 1 || resolved[0] != alert.EventID {
		t.Fatalf("resolved DB = %v, want [%s]", resolved, alert.EventID)
	}

	// 5. FCM async: minimal 2 pengiriman (alert + resolved).
	if !waitFor(func() bool { return fcm.count() >= 2 }) {
		t.Fatalf("FCM terkirim = %d, want >= 2", fcm.count())
	}
}

// Skenario: ADVISORY -> broadcast yellow banner tanpa persistensi & tanpa
// state machine resolusi. location_name tetap dikirim.
func TestDispatcherAdvisoryNoPersistence(t *testing.T) {
	saver := &fakeSaver{}
	fcm := &fakeFCM{}
	h := NewHub(testLogger(), func(*http.Request) bool { return true })
	d := NewDispatcher(saver, h, fcm, 10*time.Millisecond, testLogger())
	c := registerClient(h)

	ev := confirmedEvent()
	ev.Status = consensus.StatusAdvisory
	ev.NodeCount = 2
	d.Dispatch(context.Background(), ev)

	msg := readMessage(t, c.send)
	var advisory AlertMessage
	if err := json.Unmarshal(msg, &advisory); err != nil {
		t.Fatalf("unmarshal advisory: %v", err)
	}
	if advisory.Type != TypeAdvisory {
		t.Fatalf("type = %s, want EARTHQUAKE_ADVISORY", advisory.Type)
	}
	if advisory.LocationName != "Bandung, West Java, ID" {
		t.Fatalf("location_name = %q", advisory.LocationName)
	}

	saved, resolved := saver.snapshot()
	if len(saved) != 0 || len(resolved) != 0 {
		t.Fatalf("ADVISORY tidak boleh persist/resolve: saved=%d resolved=%d", len(saved), len(resolved))
	}
}

// Skenario: event yang sama tidak boleh membuat timer resolusi ganda (dedup),
// dan timer ganda tidak boleh menulis resolusi berulang.
func TestDispatcherResolutionTimerDedup(t *testing.T) {
	saver := &fakeSaver{}
	fcm := &fakeFCM{}
	h := NewHub(testLogger(), func(*http.Request) bool { return true })
	d := NewDispatcher(saver, h, fcm, 15*time.Millisecond, testLogger())

	d.trackResolution("event-1", &AlertMessage{EventID: "event-1", Type: TypeAlert})
	d.trackResolution("event-1", &AlertMessage{EventID: "event-1", Type: TypeAlert})

	if !waitFor(func() bool {
		_, resolved := saver.snapshot()
		return len(resolved) == 1
	}) {
		t.Fatalf("resolve tidak pernah dipanggil")
	}
	time.Sleep(30 * time.Millisecond)
	_, resolved := saver.snapshot()
	if len(resolved) != 1 {
		t.Fatalf("resolved = %v, want tepat 1 entri", resolved)
	}
}

// Skenario: store yang mendukung pencarian spasial => FCM dikirim per token di
// sekitar centroid, dan topic TIDAK dipakai. Topic tidak bisa dikecualikan per
// pelanggan, jadi mengirim keduanya akan membangunkan seluruh perangkat nasional
// lagi — regresi yang test ini menjaga.
func TestDispatcherFCMTargetsNearbyTokens(t *testing.T) {
	saver := &fakeTargetedSaver{tokens: []string{"tok-a", "tok-b", "tok-c"}}
	fcm := &fakeFCM{}
	h := NewHub(testLogger(), func(*http.Request) bool { return true })
	d := NewDispatcher(saver, h, fcm, time.Hour, testLogger())

	d.Dispatch(context.Background(), confirmedEvent())

	if !waitFor(func() bool { return fcm.count() >= 3 }) {
		t.Fatalf("hanya %d pengiriman FCM, ingin 3 token", fcm.count())
	}
	tokens, topics := fcm.targets()
	if len(tokens) != 3 {
		t.Fatalf("token terkirim = %v, ingin 3", tokens)
	}
	if len(topics) != 0 {
		t.Fatalf("topic tidak boleh dipakai saat ada token bertarget: %v", topics)
	}

	saver.mu.Lock()
	calls := append([]float64(nil), saver.calls...)
	saver.mu.Unlock()
	if len(calls) != 1 || calls[0] != float64(AlertRadiusKm) {
		t.Fatalf("radius pencarian = %v, ingin satu panggilan pada %d km", calls, AlertRadiusKm)
	}
}

// Skenario override intensitas: gempa parah (MMI >= VII atau PGA >= 250 gal)
// disiarkan ke topic TANPA filter jarak, dan pencarian token tidak pernah
// dijalankan. Untuk kejadian sebesar ini, membangunkan orang yang berada di luar
// 200 km adalah jawaban yang benar — dan seorang user yang posisinya belum
// tersinkron tidak boleh terlewat hanya karena ia tak ada di indeks spasial.
func TestDispatcherSevereBypassesDistanceFilter(t *testing.T) {
	cases := map[string]func(*consensus.Event){
		"MMI VII": func(ev *consensus.Event) { ev.MMIScale = "VII"; ev.MaxPGA = 100 },
		"PGA 250": func(ev *consensus.Event) { ev.MMIScale = "V"; ev.MaxPGA = SeverePGAGal },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			// Token TERSEDIA: kalau jalur bertarget yang dipakai, test gagal.
			saver := &fakeTargetedSaver{tokens: []string{"tok-a", "tok-b"}}
			fcm := &fakeFCM{}
			h := NewHub(testLogger(), func(*http.Request) bool { return true })
			d := NewDispatcher(saver, h, fcm, time.Hour, testLogger())

			ev := confirmedEvent()
			mutate(ev)
			d.Dispatch(context.Background(), ev)

			if !waitFor(func() bool { return fcm.count() >= 1 }) {
				t.Fatal("tidak ada pengiriman FCM sama sekali")
			}
			tokens, topics := fcm.targets()
			if len(tokens) != 0 {
				t.Fatalf("event parah tidak boleh dikirim per token: %v", tokens)
			}
			if len(topics) != 1 || topics[0] != GeoTopic {
				t.Fatalf("topic terkirim = %v, ingin [%s]", topics, GeoTopic)
			}

			saver.mu.Lock()
			calls := len(saver.calls)
			saver.mu.Unlock()
			if calls != 0 {
				t.Fatalf("pencarian token dijalankan %d kali, ingin 0 (jarak tidak diperiksa)", calls)
			}
		})
	}
}

// Radius peringatan TETAP 200 km. Test ini mengunci angkanya sebagai kontrak
// lintas-komponen: nilai yang sama dipakai gate Haversine di klien
// (domain/SafetyPolicy.kt), jadi mengubahnya di satu sisi saja membuat server dan
// perangkat berbeda pendapat tentang siapa yang dibangunkan.
func TestAlertRadiusIsFixedAt200Km(t *testing.T) {
	if AlertRadiusKm != 200 {
		t.Fatalf("AlertRadiusKm = %d, mau 200", AlertRadiusKm)
	}
}

// Skenario fallback: tidak ada token dalam radius (belum ada user yang
// menyinkronkan posisi) => broadcast topic tetap dikirim, supaya event tidak
// hilang sama sekali dari jalur background.
func TestDispatcherFCMFallsBackToTopic(t *testing.T) {
	cases := map[string]*fakeTargetedSaver{
		"tanpa token dalam radius": {tokens: nil},
		"query token gagal":        {err: errors.New("db down")},
	}
	for name, saver := range cases {
		t.Run(name, func(t *testing.T) {
			fcm := &fakeFCM{}
			h := NewHub(testLogger(), func(*http.Request) bool { return true })
			d := NewDispatcher(saver, h, fcm, time.Hour, testLogger())

			d.Dispatch(context.Background(), confirmedEvent())

			if !waitFor(func() bool { return fcm.count() >= 1 }) {
				t.Fatal("tidak ada pengiriman FCM sama sekali")
			}
			tokens, topics := fcm.targets()
			if len(tokens) != 0 {
				t.Fatalf("tidak boleh ada pengiriman per token: %v", tokens)
			}
			if len(topics) != 1 || topics[0] != GeoTopic {
				t.Fatalf("topic terkirim = %v, ingin [%s]", topics, GeoTopic)
			}
		})
	}
}

// Skenario kompatibilitas: store lama (tanpa FCMTokensWithin) harus tetap
// menempuh broadcast topic seperti sebelum penargetan ada.
func TestDispatcherFCMTopicWhenStoreHasNoTokenLookup(t *testing.T) {
	fcm := &fakeFCM{}
	h := NewHub(testLogger(), func(*http.Request) bool { return true })
	d := NewDispatcher(&fakeSaver{}, h, fcm, time.Hour, testLogger())

	d.Dispatch(context.Background(), confirmedEvent())

	if !waitFor(func() bool { return fcm.count() >= 1 }) {
		t.Fatal("tidak ada pengiriman FCM sama sekali")
	}
	_, topics := fcm.targets()
	if len(topics) != 1 || topics[0] != GeoTopic {
		t.Fatalf("topic terkirim = %v, ingin [%s]", topics, GeoTopic)
	}
}
