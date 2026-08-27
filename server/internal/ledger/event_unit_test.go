package ledger

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/banana-pixel/quakealert/server/internal/store"
)

// fakeEventStore adalah fakeStore ditambah kedua tulisan event, dengan CATATAN
// URUTAN PANGGILAN. Urutan itu yang diuji: ia satu-satunya hal yang berdiri di
// antara jalur tulis asinkron dan pelanggaran foreign key.
type fakeEventStore struct {
	*fakeStore

	mu    sync.Mutex
	calls []string
	evts  []*store.EarthquakeEvent
	logs  []*store.EventStateLog

	upsertErr error
	logErr    error
}

func newFakeEventStore() *fakeEventStore {
	return &fakeEventStore{fakeStore: newFakeStore()}
}

func (f *fakeEventStore) UpsertEvent(_ context.Context, e *store.EarthquakeEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "UpsertEvent")
	if f.upsertErr != nil {
		return f.upsertErr
	}
	f.evts = append(f.evts, e)
	return nil
}

func (f *fakeEventStore) AppendStateLog(_ context.Context, l *store.EventStateLog) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "AppendStateLog")
	if f.logErr != nil {
		return f.logErr
	}
	f.logs = append(f.logs, l)
	return nil
}

func (f *fakeEventStore) callLog() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

// recObs mencatat kabar kegagalan yang dikirim writer ke pemilik counter.
type recObs struct {
	mu                                       sync.Mutex
	dropped, upsertFail, logSkip, logFailure int
}

func (r *recObs) EventPersistDropped()  { r.mu.Lock(); r.dropped++; r.mu.Unlock() }
func (r *recObs) EventUpsertFailed()    { r.mu.Lock(); r.upsertFail++; r.mu.Unlock() }
func (r *recObs) EventStateLogSkipped() { r.mu.Lock(); r.logSkip++; r.mu.Unlock() }
func (r *recObs) EventStateLogFailed()  { r.mu.Lock(); r.logFailure++; r.mu.Unlock() }

func (r *recObs) snapshot() (dropped, upsertFail, logSkip, logFailure int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.dropped, r.upsertFail, r.logSkip, r.logFailure
}

func unit(id string, rev int) *EventUnit {
	from := "DETECTED"
	peak := 413.13
	return &EventUnit{
		Event: &store.EarthquakeEvent{
			EventID: id, Status: "HAPPENING", EventState: "UNCONFIRMED", Revision: rev,
			CentroidLat: -6.9, CentroidLon: 107.6, LocationName: "Bandung",
			MMIScale: "V", IntensityLabel: "moderate", MaxPGA: peak, TriggeredNodes: 1,
			StartedAtMs: 1_700_000_000_000, OriginTS: 1_700_000_000_000,
			OriginTSSource: "SENSOR", IndependentCellCount: 1, AlgoVer: "phase3-1.0/ic=5",
		},
		Log: &store.EventStateLog{
			EventID: id, Revision: rev, FromState: &from, ToState: "UNCONFIRMED",
			Reason: "FLOOR_MET", DecidedAt: 1_700_000_003_000, NodeCount: 1,
			IndependentCells: 1, PeakPGA: &peak, EvidenceSummary: []byte(`{}`),
			AlgoVer: "phase3-1.0/ic=5",
		},
	}
}

// §18.2 R-H3 — urutan FK: bila UpsertEvent gagal, AppendStateLog TIDAK BOLEH
// dipanggil sama sekali. Ditegaskan pada catatan panggilan fake store, karena
// "tidak dipanggil" adalah satu-satunya bentuk pencegahan yang berlaku di sini:
// event_state_log.event_id punya FK ke earthquake_events, jadi mencobanya berarti
// galat kedua yang menutupi galat pertama.
func TestFailedUpsertNeverAttemptsStateLog(t *testing.T) {
	fs := newFakeEventStore()
	fs.upsertErr = errors.New("kehabisan koneksi")
	obsRec := &recObs{}

	w := NewWriter(fs, 16, testLogger())
	w.SetEventObserver(obsRec)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	w.RecordEventUnit(unit("E1", 1))
	w.Stop()

	if got := fs.callLog(); len(got) != 1 || got[0] != "UpsertEvent" {
		t.Fatalf("panggilan store = %v, mau tepat [UpsertEvent]", got)
	}
	dropped, upsertFail, logSkip, logFailure := obsRec.snapshot()
	if upsertFail != 1 || logSkip != 1 {
		t.Errorf("upsert_failures/state_log_skipped = %d/%d, mau 1/1", upsertFail, logSkip)
	}
	if logFailure != 0 {
		t.Errorf("state_log_failures = %d, mau 0: barisnya DILEWATKAN, bukan gagal", logFailure)
	}
	if dropped != 0 {
		t.Errorf("persist_dropped = %d, mau 0", dropped)
	}
}

// Jalan yang bahagia: kedua baris ditulis, dalam urutan induk lalu riwayat.
func TestEventUnitWritesParentThenLog(t *testing.T) {
	fs := newFakeEventStore()
	obsRec := &recObs{}

	w := NewWriter(fs, 16, testLogger())
	w.SetEventObserver(obsRec)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	w.RecordEventUnit(unit("E1", 1))
	w.Stop()

	want := []string{"UpsertEvent", "AppendStateLog"}
	got := fs.callLog()
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("panggilan store = %v, mau %v", got, want)
	}
	if len(fs.evts) != 1 || len(fs.logs) != 1 {
		t.Fatalf("baris tersimpan = %d event / %d log, mau 1/1", len(fs.evts), len(fs.logs))
	}
	if fs.logs[0].FromState == nil || *fs.logs[0].FromState != "DETECTED" {
		t.Errorf("from_state = %v, mau DETECTED", fs.logs[0].FromState)
	}
	if _, upsertFail, logSkip, logFailure := obsRec.snapshot(); upsertFail != 0 || logSkip != 0 || logFailure != 0 {
		t.Errorf("counter kegagalan = %d/%d/%d, mau nol semua", upsertFail, logSkip, logFailure)
	}
}

// Baris riwayat yang gagal SETELAH induknya tersimpan adalah kegagalan tersendiri:
// induknya tetap ada, dan itu dibedakan dari pelewatan.
func TestStateLogFailureAfterSuccessfulUpsert(t *testing.T) {
	fs := newFakeEventStore()
	fs.logErr = errors.New("timeout")
	obsRec := &recObs{}

	w := NewWriter(fs, 16, testLogger())
	w.SetEventObserver(obsRec)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	w.RecordEventUnit(unit("E1", 1))
	w.Stop()

	if len(fs.evts) != 1 {
		t.Fatalf("baris event = %d, mau 1: induknya berhasil", len(fs.evts))
	}
	_, upsertFail, logSkip, logFailure := obsRec.snapshot()
	if logFailure != 1 || logSkip != 0 || upsertFail != 0 {
		t.Errorf("failures/skipped/upsert = %d/%d/%d, mau 1/0/0", logFailure, logSkip, upsertFail)
	}
}

// Antrean penuh melaporkan satuan yang dibuang lewat observer, dengan alasan yang
// sama seperti Drops() untuk observasi: sebuah satuan yang tidak pernah masuk
// tetap harus dapat dihitung.
func TestFullQueueReportsDroppedEventUnits(t *testing.T) {
	fs := newFakeEventStore()
	fs.block = make(chan struct{}) // observasi pertama menggantung drain
	obsRec := &recObs{}

	w := NewWriter(fs, 1, testLogger())
	w.SetEventObserver(obsRec)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	// Isi antrean berkapasitas 1, lalu paksa pembuangan.
	const n = 200
	for i := 0; i < n; i++ {
		w.RecordEventUnit(unit("E1", i+1))
	}

	if dropped, _, _, _ := obsRec.snapshot(); dropped == 0 {
		t.Fatal("persist_dropped = 0; satuan yang dibuang harus dilaporkan")
	}
	if got := w.Drops(); got == 0 {
		t.Error("ledger_dropped_total = 0; pembuangan harus tetap terhitung di sini juga")
	}
}

// Store yang TIDAK mendukung persistensi event (jalur Fase 2, atau fake lama)
// tidak boleh menjatuhkan goroutine drain: satuannya hanya lewat.
func TestEventUnitOnStoreWithoutEventSupportIsNoop(t *testing.T) {
	fs := newFakeStore()
	obsRec := &recObs{}

	w := NewWriter(fs, 16, testLogger())
	w.SetEventObserver(obsRec)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	w.RecordEventUnit(unit("E1", 1))
	w.Stop()

	if dropped, upsertFail, logSkip, logFailure := obsRec.snapshot(); dropped+upsertFail+logSkip+logFailure != 0 {
		t.Errorf("counter = %d/%d/%d/%d, mau nol semua", dropped, upsertFail, logSkip, logFailure)
	}
}

// Satuan nil, satuan tanpa baris induk, dan writer yang sudah berhenti tidak
// boleh menghasilkan panggilan store maupun panik.
func TestRecordEventUnitIgnoresUnusableUnits(t *testing.T) {
	fs := newFakeEventStore()
	w := NewWriter(fs, 16, testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	w.RecordEventUnit(nil)
	w.RecordEventUnit(&EventUnit{})
	w.Stop()
	w.RecordEventUnit(unit("E1", 1)) // setelah Stop

	if got := fs.callLog(); len(got) != 0 {
		t.Errorf("panggilan store = %v, mau tidak ada", got)
	}
}
