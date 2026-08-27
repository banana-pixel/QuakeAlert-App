package dispatch

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/banana-pixel/quakealert/server/internal/consensus"
	"github.com/banana-pixel/quakealert/server/internal/ledger"
	"github.com/banana-pixel/quakealert/server/internal/store"
)

// fakeEmissionWriter menangkap baris keputusan tanpa basis data.
type fakeEmissionWriter struct {
	mu   sync.Mutex
	rows []*ledger.Emission
}

func (f *fakeEmissionWriter) RecordEmission(e *ledger.Emission) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows = append(f.rows, e)
}

func (f *fakeEmissionWriter) snapshot() []*ledger.Emission {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*ledger.Emission(nil), f.rows...)
}

func (f *fakeEmissionWriter) audiences() []string {
	out := []string{}
	for _, r := range f.snapshot() {
		out = append(out, r.Audience)
	}
	return out
}

// severeEvent mengembalikan event yang menyala di IsSevere (PGA >= SeverePGAGal)
// dengan jumlah node yang dapat diatur — satu-satunya variabel yang diuji guard.
func severeEvent(nodes int) *consensus.Event {
	ev := confirmedEvent()
	ev.MaxPGA = SeverePGAGal + 100
	ev.MMIScale = "VIII"
	ev.NodeCount = nodes
	return ev
}

func nonSevereEvent(nodes int) *consensus.Event {
	ev := confirmedEvent()
	ev.NodeCount = nodes
	return ev
}

// guardFixture menyiapkan dispatcher + hub + fake, dengan tokenFinder yang
// mengembalikan tokens (nil = tidak ada token dalam radius).
func guardFixture(t *testing.T, tokens []string) (*Dispatcher, *fakeFCM, *fakeEmissionWriter, *client) {
	t.Helper()
	saver := &fakeTargetedSaver{tokens: tokens}
	fcm := &fakeFCM{}
	h := NewHub(testLogger(), func(*http.Request) bool { return true })
	d := NewDispatcher(saver, h, fcm, time.Hour, testLogger())
	w := &fakeEmissionWriter{}
	d.SetLedger(w)
	return d, fcm, w, registerClient(h)
}

// ---------------------------------------------------------------------------
// D6 — guard satu-node (positif)
// ---------------------------------------------------------------------------

// Kluster SATU node pada intensitas severe mengambil jalur token bertarget dan
// TIDAK PERNAH GeoTopic. Satu sensor yang berteriak keras tidak dapat dibedakan
// dari satu sensor yang rusak.
func TestSingleNodeSevereTakesTargetedPathNotGeoTopic(t *testing.T) {
	d, fcm, w, c := guardFixture(t, []string{"token-a", "token-b"})

	d.Dispatch(t.Context(), severeEvent(1))

	if !waitFor(func() bool { return fcm.count() == 2 }) {
		t.Fatalf("want 2 pengiriman bertarget, got %d", fcm.count())
	}
	tokens, topics := fcm.targets()
	if len(topics) != 0 {
		t.Errorf("kluster satu-node menyentuh topic %v — guard tidak berlaku", topics)
	}
	if len(tokens) != 2 {
		t.Errorf("token terkirim = %v, want 2", tokens)
	}

	// WS tetap disiarkan: guard membatasi FCM, bukan peringatan itu sendiri.
	readMessage(t, c.send)

	if !waitFor(func() bool { return len(w.snapshot()) == 1 }) {
		t.Fatal("timeout menunggu baris emisi")
	}
	row := w.snapshot()[0]
	if row.Audience != ledger.AudienceTokensRadius {
		t.Errorf("audience = %q, want %q", row.Audience, ledger.AudienceTokensRadius)
	}
	if !row.IsSevere {
		t.Error("is_severe harus tetap true: guard mengubah audience, bukan intensitas")
	}
	if row.NodeCount != 1 {
		t.Errorf("node_count = %d, want 1", row.NodeCount)
	}
}

// Kasus yang akan luput dari guard satu baris di cabang severe: satu node, TANPA
// token dalam radius. Tidak ada FCM sama sekali, dan audience tercatat NONE —
// khususnya, fallback GeoTopic tidak boleh tercapai.
func TestSingleNodeNoTokensSendsNothingAndRecordsNone(t *testing.T) {
	d, fcm, w, c := guardFixture(t, nil)

	d.Dispatch(t.Context(), severeEvent(1))

	if !waitFor(func() bool { return len(w.snapshot()) == 1 }) {
		t.Fatal("timeout menunggu baris emisi")
	}
	if fcm.count() != 0 {
		_, topics := fcm.targets()
		t.Fatalf("want 0 pengiriman FCM, got %d (topics=%v)", fcm.count(), topics)
	}
	if got := w.audiences(); len(got) != 1 || got[0] != ledger.AudienceNone {
		t.Errorf("audience = %v, want [NONE]", got)
	}

	// WS tetap jalan: klien foreground di dekat lokasi masih diberi tahu.
	readMessage(t, c.send)
}

// Kluster satu-node non-severe tanpa token: alasannya sama, hasilnya sama.
func TestSingleNodeNonSevereNoTokensRecordsNone(t *testing.T) {
	d, fcm, w, _ := guardFixture(t, nil)

	d.Dispatch(t.Context(), nonSevereEvent(1))

	if !waitFor(func() bool { return len(w.snapshot()) == 1 }) {
		t.Fatal("timeout menunggu baris emisi")
	}
	if fcm.count() != 0 {
		t.Errorf("want 0 pengiriman FCM, got %d", fcm.count())
	}
	if got := w.audiences(); got[0] != ledger.AudienceNone {
		t.Errorf("audience = %v, want NONE", got)
	}
}

// ADVISORY satu node (jalur yang paling sering terjadi di lapangan) juga tidak
// boleh menyiarkan nasional.
func TestSingleNodeAdvisoryNeverReachesGeoTopic(t *testing.T) {
	d, fcm, w, _ := guardFixture(t, nil)

	ev := nonSevereEvent(1)
	ev.Status = consensus.StatusAdvisory
	d.Dispatch(t.Context(), ev)

	if !waitFor(func() bool { return len(w.snapshot()) == 1 }) {
		t.Fatal("timeout menunggu baris emisi")
	}
	if _, topics := fcm.targets(); len(topics) != 0 {
		t.Errorf("ADVISORY satu-node menyentuh topic %v", topics)
	}
	row := w.snapshot()[0]
	if row.AlertType != TypeAdvisory || row.Status != string(consensus.StatusAdvisory) {
		t.Errorf("alert_type/status = %q/%q, want %q/ADVISORY", row.AlertType, row.Status, TypeAdvisory)
	}
	if row.EventID != nil {
		t.Error("ADVISORY tidak dipersistensi, jadi event_id harus NULL")
	}
}

// ---------------------------------------------------------------------------
// D6 — negatif: guard harus tetap sempit
// ---------------------------------------------------------------------------

// DUA node severe tetap mencapai GeoTopic. Guard hanya tentang satu node.
func TestTwoNodeSevereStillReachesGeoTopic(t *testing.T) {
	d, fcm, w, c := guardFixture(t, []string{"token-a"})

	d.Dispatch(t.Context(), severeEvent(2))

	if !waitFor(func() bool { return fcm.count() == 1 }) {
		t.Fatalf("want 1 pengiriman topic, got %d", fcm.count())
	}
	_, topics := fcm.targets()
	if len(topics) != 1 || topics[0] != GeoTopic {
		t.Errorf("topics = %v, want [%s]", topics, GeoTopic)
	}
	readMessage(t, c.send)

	if !waitFor(func() bool { return len(w.snapshot()) == 1 }) {
		t.Fatal("timeout menunggu baris emisi")
	}
	if got := w.audiences(); got[0] != ledger.AudienceGeoTopicAll {
		t.Errorf("audience = %v, want GEO_TOPIC_ALL", got)
	}
}

// DUA node non-severe tanpa token tetap memakai fallback GeoTopic.
func TestTwoNodeNonSevereNoTokensStillFallsBackToGeoTopic(t *testing.T) {
	d, fcm, w, c := guardFixture(t, nil)

	d.Dispatch(t.Context(), nonSevereEvent(2))

	if !waitFor(func() bool { return fcm.count() == 1 }) {
		t.Fatalf("want 1 pengiriman fallback topic, got %d", fcm.count())
	}
	_, topics := fcm.targets()
	if len(topics) != 1 || topics[0] != GeoTopic {
		t.Errorf("topics = %v, want [%s]", topics, GeoTopic)
	}
	readMessage(t, c.send)

	if got := w.audiences(); got[0] != ledger.AudienceGeoTopicAll {
		t.Errorf("audience = %v, want GEO_TOPIC_ALL", got)
	}
}

// Guard dapat dimatikan: instalasi uji satu-sensor yang menerima konsekuensinya.
func TestGuardCanBeDisabled(t *testing.T) {
	d, fcm, w, _ := guardFixture(t, nil)
	d.SetSingleNodeGeoTopicGuard(false)

	d.Dispatch(t.Context(), severeEvent(1))

	if !waitFor(func() bool { return fcm.count() == 1 }) {
		t.Fatalf("want 1 pengiriman topic saat guard dimatikan, got %d", fcm.count())
	}
	if _, topics := fcm.targets(); topics[0] != GeoTopic {
		t.Errorf("topics = %v, want [%s]", topics, GeoTopic)
	}
	if got := w.audiences(); got[0] != ledger.AudienceGeoTopicAll {
		t.Errorf("audience = %v, want GEO_TOPIC_ALL", got)
	}
}

// Guard aktif secara default: sebuah instalasi yang tidak menyetel apa pun harus
// mendapat perilaku yang aman.
func TestGuardDefaultsOn(t *testing.T) {
	d := NewDispatcher(&fakeSaver{}, NewHub(testLogger(), nil), nil, time.Hour, testLogger())
	if !d.singleNodeGeoTopicGuard {
		t.Error("guard satu-node harus aktif secara default")
	}
}

// ---------------------------------------------------------------------------
// Pencatatan emisi
// ---------------------------------------------------------------------------

// Kegagalan ledger tidak boleh menghentikan kanal mana pun, dan tanpa FCM pun
// keputusan tetap tercatat (audience NONE).
func TestEmissionRecordedWithoutFCMSender(t *testing.T) {
	saver := &fakeSaver{}
	h := NewHub(testLogger(), func(*http.Request) bool { return true })
	d := NewDispatcher(saver, h, nil, time.Hour, testLogger()) // fcm nil
	w := &fakeEmissionWriter{}
	d.SetLedger(w)
	c := registerClient(h)

	d.Dispatch(t.Context(), confirmedEvent())

	readMessage(t, c.send) // WS tetap jalan tanpa FCM
	rows := w.snapshot()
	if len(rows) != 1 {
		t.Fatalf("baris emisi = %d, want 1", len(rows))
	}
	if rows[0].Audience != ledger.AudienceNone {
		t.Errorf("audience = %q, want NONE saat FCM tidak dikonfigurasi", rows[0].Audience)
	}
	if rows[0].EventID == nil {
		t.Error("CONFIRMED dipersistensi, jadi event_id harus terisi")
	}
	if rows[0].AlgoVer != ledger.AlgoVer {
		t.Errorf("algo_ver = %q, want %q", rows[0].AlgoVer, ledger.AlgoVer)
	}
}

// Dispatcher tanpa ledger berjalan seperti sebelumnya: pencatatan opsional.
func TestDispatchWorksWithoutLedger(t *testing.T) {
	saver := &fakeTargetedSaver{tokens: []string{"token-a"}}
	fcm := &fakeFCM{}
	h := NewHub(testLogger(), func(*http.Request) bool { return true })
	d := NewDispatcher(saver, h, fcm, time.Hour, testLogger()) // tanpa SetLedger
	c := registerClient(h)

	d.Dispatch(t.Context(), confirmedEvent())

	readMessage(t, c.send)
	if !waitFor(func() bool { return fcm.count() == 1 }) {
		t.Errorf("want 1 pengiriman, got %d", fcm.count())
	}
}

// EVENT_RESOLVED juga melewati guard yang sama (dispatchFCM adalah jalur
// bersama), dan tercatat sebagai keputusan tersendiri.
func TestResolvedEmissionRecorded(t *testing.T) {
	d, fcm, w, c := guardFixture(t, []string{"token-a"})
	d.resolveAfter = 20 * time.Millisecond

	d.Dispatch(t.Context(), nonSevereEvent(3))
	readMessage(t, c.send) // ALERT

	if !waitFor(func() bool {
		for _, r := range w.snapshot() {
			if r.AlertType == TypeResolved {
				return true
			}
		}
		return false
	}) {
		t.Fatal("timeout menunggu baris emisi EVENT_RESOLVED")
	}
	if fcm.count() < 2 {
		t.Errorf("want >= 2 pengiriman (alert + resolved), got %d", fcm.count())
	}
}

// ---------------------------------------------------------------------------
// §20.3 — basis data yang macet tidak boleh menjadi peringatan yang tertunda
// ---------------------------------------------------------------------------

// blockingLedgerStore adalah store ledger yang setiap INSERT-nya menggantung
// selamanya: simulasi pool pgx yang habis atau Postgres yang tidak merespons.
type blockingLedgerStore struct{ block chan struct{} }

func (b *blockingLedgerStore) InsertObservation(_ context.Context, _ *store.Observation) error {
	<-b.block
	return nil
}

func (b *blockingLedgerStore) InsertAlertEmission(_ context.Context, _ *store.AlertEmission) error {
	<-b.block
	return nil
}

func (b *blockingLedgerStore) GetNodeLocation(_ context.Context, _ string) (*store.NodeLocation, error) {
	<-b.block
	return nil, nil
}

// Dengan ledger NYATA (bukan fake) yang penulisannya macet total, Dispatch harus
// tetap selesai seketika dan WS tetap tersiar. Inilah properti yang memisahkan
// "pencatatan gagal" dari "peringatan gagal", dan yang paling mungkin dirusak
// oleh refactor yang membuat pencatatan menjadi sinkron.
func TestBlockedLedgerDoesNotDelayDispatch(t *testing.T) {
	blocked := &blockingLedgerStore{block: make(chan struct{})}
	w := ledger.NewWriter(blocked, 4, testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	saver := &fakeTargetedSaver{tokens: []string{"token-a"}}
	fcm := &fakeFCM{}
	h := NewHub(testLogger(), func(*http.Request) bool { return true })
	d := NewDispatcher(saver, h, fcm, time.Hour, testLogger())
	d.SetLedger(w)
	c := registerClient(h)

	start := time.Now()
	for i := 0; i < 200; i++ {
		d.Dispatch(t.Context(), nonSevereEvent(3))
	}
	elapsed := time.Since(start)

	// 200 dispatch dengan ledger yang macet total. Ambangnya longgar dengan
	// sengaja: yang diuji adalah selisih antara "tidak menunggu" dan "menunggu
	// selamanya", bukan performa mikro.
	if elapsed > 2*time.Second {
		t.Fatalf("dispatch tertahan ledger yang macet: %v untuk 200 dispatch", elapsed)
	}
	readMessage(t, c.send) // WS tetap tersiar

	// Antrean penuh berarti membuang, bukan memblokir.
	if w.Drops() == 0 {
		t.Error("ledger yang macet dengan antrean penuh seharusnya menghasilkan drop, bukan tekanan balik")
	}
}

// Shutdown di tengah drain tidak boleh membuat jalur dispatch deadlock: Stop
// menunggu drain, dan drain tidak pernah menunggu dispatch.
func TestLedgerShutdownMidDrainDoesNotDeadlockDispatch(t *testing.T) {
	blocked := &blockingLedgerStore{block: make(chan struct{})}
	w := ledger.NewWriter(blocked, 2, testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	go w.Run(ctx)

	saver := &fakeTargetedSaver{tokens: []string{"token-a"}}
	h := NewHub(testLogger(), func(*http.Request) bool { return true })
	d := NewDispatcher(saver, h, &fakeFCM{}, time.Hour, testLogger())
	d.SetLedger(w)
	registerClient(h)

	d.Dispatch(t.Context(), nonSevereEvent(3))

	// Batalkan context saat drain masih menggantung pada INSERT, lalu dispatch
	// lagi. Tidak ada satu pun langkah di bawah ini yang boleh menggantung.
	cancel()
	close(blocked.block) // lepaskan drain yang tertahan agar ia dapat keluar

	done := make(chan struct{})
	go func() {
		d.Dispatch(t.Context(), nonSevereEvent(3))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("dispatch deadlock setelah shutdown ledger")
	}

	stopped := make(chan struct{})
	go func() { w.Stop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop tidak kembali setelah context dibatalkan")
	}
}
