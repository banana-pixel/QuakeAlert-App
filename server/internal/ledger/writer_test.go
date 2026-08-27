package ledger

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/banana-pixel/quakealert/server/internal/store"
)

// fakeStore adalah store ledger tanpa Postgres. block, bila diisi, membuat
// setiap INSERT menggantung sampai kanalnya ditutup — itulah cara menguji bahwa
// basis data yang macet tidak dapat memperlambat pemanggil.
type fakeStore struct {
	mu        sync.Mutex
	obs       []*store.Observation
	emis      []*store.AlertEmission
	locs      map[string]*store.NodeLocation
	insertErr error
	block     chan struct{}
}

func newFakeStore() *fakeStore {
	return &fakeStore{locs: map[string]*store.NodeLocation{
		"NODE-0A1B2C3D": {StationID: "NODE-0A1B2C3D", Lat: -6.9, Lon: 107.6, LocationName: "Bandung"},
	}}
}

func (f *fakeStore) InsertObservation(_ context.Context, o *store.Observation) error {
	if f.block != nil {
		<-f.block
	}
	if f.insertErr != nil {
		return f.insertErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.obs = append(f.obs, o)
	return nil
}

func (f *fakeStore) InsertAlertEmission(_ context.Context, e *store.AlertEmission) error {
	if f.block != nil {
		<-f.block
	}
	if f.insertErr != nil {
		return f.insertErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.emis = append(f.emis, e)
	return nil
}

func (f *fakeStore) GetNodeLocation(_ context.Context, id string) (*store.NodeLocation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	loc, ok := f.locs[id]
	if !ok {
		return nil, store.ErrNodeNotFound
	}
	return loc, nil
}

func (f *fakeStore) observations() []*store.Observation {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*store.Observation(nil), f.obs...)
}

func (f *fakeStore) emissions() []*store.AlertEmission {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*store.AlertEmission(nil), f.emis...)
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func obs(nodeID, verifyResult string) *Observation {
	return &Observation{
		NodeID:       nodeID,
		SourceClass:  SourceClassFixedESP32,
		Phase:        PhaseFinal,
		PGAGal:       413.13,
		DurMs:        8000,
		PublishTS:    1_700_000_005_000,
		ReceivedTS:   1_700_000_005_010,
		VerifyResult: verifyResult,
	}
}

// ---------------------------------------------------------------------------
// §20.3 — invarian: pencatatan tidak pernah menahan jalur peringatan
// ---------------------------------------------------------------------------

// Sebuah writer yang MENGGANTUNG SELAMANYA tidak boleh menunda pemanggil sama
// sekali. Ini properti asinkron itu sendiri, dan yang paling mungkin dirusak
// oleh refactor di masa depan.
func TestRecordObservation_BlockedWriterDoesNotDelayCaller(t *testing.T) {
	fs := newFakeStore()
	fs.block = make(chan struct{}) // tidak pernah ditutup: INSERT menggantung selamanya
	w := NewWriter(fs, 8, testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	const n = 10_000
	start := time.Now()
	for i := 0; i < n; i++ {
		w.RecordObservation(obs("NODE-0A1B2C3D", VerifyResultOK))
	}
	elapsed := time.Since(start)

	// 10.000 enqueue ke antrean berkapasitas 8 dengan penulis yang macet: bila
	// ada satu saja jalur yang memblokir, ini tidak akan pernah selesai.
	if elapsed > 500*time.Millisecond {
		t.Fatalf("enqueue tertahan penulis yang macet: %v untuk %d panggilan", elapsed, n)
	}
	if w.Drops() == 0 {
		t.Error("antrean penuh dengan penulis macet seharusnya menghasilkan drop")
	}
}

// Antrean penuh MEMBUANG, tidak memblokir: kapasitas 1, tanpa drain sama sekali.
func TestFullQueueDropsRatherThanBlocks(t *testing.T) {
	fs := newFakeStore()
	w := NewWriter(fs, 1, testLogger()) // Run TIDAK dijalankan: tidak ada yang menguras

	const n = 50
	done := make(chan struct{})
	go func() {
		for i := 0; i < n; i++ {
			w.RecordObservation(obs("NODE-0A1B2C3D", VerifyResultOK))
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("enqueue terparkir pada antrean penuh — seharusnya membuang, bukan memblokir")
	}

	if got, want := w.Drops(), int64(n-1); got != want {
		t.Errorf("ledger_drops_total = %d, want %d (kapasitas 1, %d enqueue)", got, want, n)
	}
	if len(w.queue) != 1 {
		t.Errorf("kedalaman antrean = %d, want 1", len(w.queue))
	}
}

// Counter drop tidak boleh melaporkan kurang dari kenyataan, termasuk saat
// banyak produsen bersamaan (dijalankan juga di bawah -race).
func TestDropCounterNeverUnderReports(t *testing.T) {
	fs := newFakeStore()
	const capacity = 4
	w := NewWriter(fs, capacity, testLogger()) // tanpa drain

	const goroutines, each = 16, 200
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < each; i++ {
				w.RecordObservation(obs("NODE-0A1B2C3D", VerifyResultOK))
			}
		}()
	}
	wg.Wait()

	total := int64(goroutines * each)
	queued := int64(len(w.queue))
	if got := w.Drops(); got != total-queued {
		t.Errorf("drops = %d, want %d (total %d - tersisa di antrean %d)", got, total-queued, total, queued)
	}
}

// Kegagalan INSERT dicatat sebagai counter, bukan dikembalikan ke pemanggil:
// jalur peringatan tidak punya tindakan benar apa pun untuk meresponsnya.
func TestWriteFailureIsCountedNotPropagated(t *testing.T) {
	fs := newFakeStore()
	fs.insertErr = errors.New("relation tidak ada")
	w := NewWriter(fs, 8, testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	w.RecordObservation(obs("NODE-0A1B2C3D", VerifyResultOK))
	w.Stop()

	if w.WriteFailures() != 1 {
		t.Errorf("write failures = %d, want 1", w.WriteFailures())
	}
	if w.Written() != 0 {
		t.Errorf("written = %d, want 0", w.Written())
	}
}

// ---------------------------------------------------------------------------
// §20.2 / D22 — pembatasan baris penolakan
// ---------------------------------------------------------------------------

// 100 penolakan dari satu node dalam satu menit menghasilkan SATU baris, dan
// baris berikutnya yang diterima membawa 99 yang ditekan. Barisnya hilang,
// angkanya tidak.
func TestRejectionRateLimitCarriesSuppressedCount(t *testing.T) {
	fs := newFakeStore()
	w := NewWriter(fs, 256, testLogger())

	now := time.UnixMilli(1_700_000_000_000)
	w.now = func() time.Time { return now }

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	for i := 0; i < 100; i++ {
		w.RecordObservation(obs("NODE-0A1B2C3D", "ErrBadSignature"))
	}

	// Melewati jendela: penolakan berikutnya membawa yang tertekan.
	now = now.Add(rejectionInterval + time.Second)
	w.RecordObservation(obs("NODE-0A1B2C3D", "ErrBadSignature"))
	w.Stop()

	rows := fs.observations()
	if len(rows) != 2 {
		t.Fatalf("baris tersimpan = %d, want 2 (satu per jendela)", len(rows))
	}
	if rows[0].SuppressedRejections != 0 {
		t.Errorf("baris pertama suppressed = %d, want 0", rows[0].SuppressedRejections)
	}
	if rows[1].SuppressedRejections != 99 {
		t.Errorf("baris kedua suppressed = %d, want 99", rows[1].SuppressedRejections)
	}
}

// Penolakan dari node yang TIDAK ADA di iot_nodes tidak menjadi baris — hanya
// counter. Kredensial broker berlaku fleet-wide, jadi nama node sembarang pun
// dapat mencapai server; menyimpannya berarti amplifikasi penulisan yang dipicu
// pihak tak terotentikasi.
func TestUnknownNodeRejectionIsCountedNotStored(t *testing.T) {
	fs := newFakeStore()
	w := NewWriter(fs, 16, testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	w.RecordObservation(obs("NODE-DEADBEEF", "ErrBadSignature"))
	w.Stop()

	if rows := fs.observations(); len(rows) != 0 {
		t.Fatalf("baris tersimpan = %d, want 0 untuk node tak dikenal", len(rows))
	}
	if got := w.UnknownNodeRejections(); got != 1 {
		t.Errorf("ledger_unknown_node_rejections_total = %d, want 1", got)
	}
}

// A16: observasi yang LOLOS verifikasi namun lokasinya tidak dapat dicari tetap
// tersimpan, dengan verify_result 'OK' dan node_location NULL. Hari ini kasus
// itu hanya menjadi satu baris log lalu hilang.
func TestVerifiedObservationWithoutLocationIsStored(t *testing.T) {
	fs := newFakeStore()
	w := NewWriter(fs, 16, testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	w.RecordObservation(obs("NODE-DEADBEEF", VerifyResultOK))
	w.Stop()

	rows := fs.observations()
	if len(rows) != 1 {
		t.Fatalf("baris tersimpan = %d, want 1", len(rows))
	}
	if rows[0].VerifyResult != VerifyResultOK {
		t.Errorf("verify_result = %q, want OK", rows[0].VerifyResult)
	}
	if rows[0].Lat != nil || rows[0].Lon != nil {
		t.Error("node_location harus NULL bila lokasi tidak diketahui")
	}
}

// Snapshot lokasi diambil oleh writer, di luar jalur peringatan.
func TestObservationTakesLocationSnapshot(t *testing.T) {
	fs := newFakeStore()
	w := NewWriter(fs, 16, testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	w.RecordObservation(obs("NODE-0A1B2C3D", VerifyResultOK))
	w.Stop()

	rows := fs.observations()
	if len(rows) != 1 {
		t.Fatalf("baris tersimpan = %d, want 1", len(rows))
	}
	if rows[0].Lat == nil || rows[0].Lon == nil {
		t.Fatal("node_location harus terisi untuk node yang dikenal")
	}
	if *rows[0].Lat != -6.9 || *rows[0].Lon != 107.6 {
		t.Errorf("snapshot lokasi = (%f, %f), want (-6.9, 107.6)", *rows[0].Lat, *rows[0].Lon)
	}
}

// ---------------------------------------------------------------------------
// §20.7 — shutdown
// ---------------------------------------------------------------------------

// Stop menuliskan sisa antrean: shutdown yang tertib tidak boleh membuang baris
// yang sudah diantre.
func TestStopDrainsQueuedRows(t *testing.T) {
	fs := newFakeStore()
	w := NewWriter(fs, 64, testLogger())

	// Enqueue SEBELUM drain dijalankan, agar antrean benar-benar berisi saat Stop.
	for i := 0; i < 20; i++ {
		w.RecordObservation(obs("NODE-0A1B2C3D", VerifyResultOK))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)
	w.Stop()

	if got := len(fs.observations()); got != 20 {
		t.Errorf("baris tersimpan = %d, want 20 (shutdown tertib tidak membuang)", got)
	}
	if w.Drops() != 0 {
		t.Errorf("drops = %d, want 0", w.Drops())
	}
}

// Stop idempoten, dan enqueue setelah Stop tidak panik (mis. pesan MQTT yang
// tiba saat shutdown).
func TestStopIsIdempotentAndPostStopEnqueueIsSafe(t *testing.T) {
	fs := newFakeStore()
	w := NewWriter(fs, 8, testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	w.Stop()
	w.Stop()
	w.RecordObservation(obs("NODE-0A1B2C3D", VerifyResultOK))
	w.RecordEmission(&Emission{AlertType: "EARTHQUAKE_ALERT", Status: "CONFIRMED",
		NodeCount: 3, Audience: AudienceGeoTopicAll, DecidedAt: 1, AlgoVer: AlgoVer})
}

// Drain berakhir saat context dibatalkan, tanpa menggantung.
func TestRunExitsOnContextCancel(t *testing.T) {
	fs := newFakeStore()
	w := NewWriter(fs, 8, testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	go w.Run(ctx)
	cancel()

	select {
	case <-w.drained:
	case <-time.After(2 * time.Second):
		t.Fatal("goroutine drain tidak berhenti setelah context dibatalkan")
	}
}

// Writer nil = ledger nonaktif: setiap metode aman dan tidak melakukan apa pun.
func TestNilWriterIsNoop(t *testing.T) {
	var w *Writer
	w.RecordObservation(obs("NODE-0A1B2C3D", VerifyResultOK))
	w.RecordEmission(&Emission{})
	w.Stop()
	if w.Drops() != 0 || w.Written() != 0 || w.UnknownNodeRejections() != 0 || w.WriteFailures() != 0 {
		t.Error("writer nil harus melaporkan counter nol")
	}
}

// Baris emisi ditulis apa adanya: keputusan, bukan hasil pengiriman.
func TestEmissionRoundTripsThroughQueue(t *testing.T) {
	fs := newFakeStore()
	w := NewWriter(fs, 8, testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	w.RecordEmission(&Emission{
		AlertType: "EARTHQUAKE_ADVISORY", Status: "ADVISORY",
		NodeCount: 1, IsSevere: false, Audience: AudienceNone,
		DecidedAt: 1_700_000_005_000, AlgoVer: AlgoVer,
	})
	w.Stop()

	rows := fs.emissions()
	if len(rows) != 1 {
		t.Fatalf("baris emisi = %d, want 1", len(rows))
	}
	if rows[0].EventID != nil {
		t.Error("ADVISORY tidak punya identitas event; event_id harus NULL")
	}
	if rows[0].Audience != AudienceNone {
		t.Errorf("audience = %q, want NONE", rows[0].Audience)
	}
	if rows[0].AlgoVer != AlgoVer {
		t.Errorf("algo_ver = %q, want %q", rows[0].AlgoVer, AlgoVer)
	}
}
