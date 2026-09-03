package ledger

// --- P4-M2′ (D-012): jalur tulis catatan near-confirmation durable ---
//
// Yang diuji di sini adalah sifat-sifat yang HANYA berlaku pada jalur ini, dan
// masing-masing berbeda dari jalur satuan event:
//
//	tanpa induk    — sebuah persilangan sunyi tidak punya EventUnit sama sekali,
//	                 jadi catatannya tidak dapat menumpang pada satu pun.
//	tanpa FK       — kegagalan tulis satuan event TIDAK melewatkan catatan ini
//	                 (migrasi 000009 sengaja tidak punya foreign key).
//	akuntansi      — dibuang dan gagal-tulis dihitung TERPISAH dari satuan event.
//	kemampuan      — store pra-000009 hanya melewatkannya, bukan panik.

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/banana-pixel/quakealert/server/internal/store"
)

// fakeNearStore adalah fakeStore ditambah tulisan near-confirmation SAJA.
//
// Sengaja TIDAK mengimplementasikan UpsertEvent/AppendStateLog: kedua kemampuan itu
// dideteksi lewat type assertion yang berbeda, dan sebuah store yang mendukung yang
// satu tanpa yang lain adalah keadaan yang harus tetap sah. fakeEventStore
// melengkapi sisi sebaliknya.
type fakeNearStore struct {
	*fakeStore

	mu    sync.Mutex
	rows  []*store.NearConfirmedRow
	calls int

	upsertErr error
}

func newFakeNearStore() *fakeNearStore {
	return &fakeNearStore{fakeStore: newFakeStore()}
}

func (f *fakeNearStore) UpsertNearConfirmed(_ context.Context, r *store.NearConfirmedRow) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.upsertErr != nil {
		return f.upsertErr
	}
	f.rows = append(f.rows, r)
	return nil
}

func (f *fakeNearStore) snapshot() (calls int, rows []*store.NearConfirmedRow) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls, append([]*store.NearConfirmedRow(nil), f.rows...)
}

// nearRow membangun satu catatan near-confirmation. Kolom NULL-able dibiarkan nil:
// "belum pernah terjadi", bukan nol.
func nearRow(id string) *NearConfirmed {
	return &NearConfirmed{
		EventID:                id,
		FirstTwoIndependentAt:  1_700_000_001_000,
		IndependentCountAtPeak: 2,
		NodeCountAtPeak:        2,
		MinIndependentCells:    2,
		AlgoVer:                "phase3-1.1/ic=5",
	}
}

// Jalan yang bahagia, dan yang membedakannya dari satuan event: TIDAK ADA baris
// induk yang ditulis lebih dulu. Sebuah persilangan sunyi tidak menghasilkan
// transisi, jadi tidak ada earthquake_events yang menyertainya — dan catatannya
// tetap harus sampai.
func TestNearConfirmedWrittenWithoutAnyParentUnit(t *testing.T) {
	fs := newFakeNearStore()
	obsRec := &recObs{}

	w := NewWriter(fs, 16, testLogger())
	w.SetEventObserver(obsRec)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	w.RecordNearConfirmed(nearRow("E1"))
	w.Stop()

	calls, rows := fs.snapshot()
	if calls != 1 || len(rows) != 1 {
		t.Fatalf("UpsertNearConfirmed dipanggil %d kali, %d baris tersimpan, mau 1/1", calls, len(rows))
	}
	if rows[0].EventID != "E1" {
		t.Errorf("event_id = %q, mau E1", rows[0].EventID)
	}
	// NULL bertahan sampai store: nol dan "tidak pernah terjadi" tidak boleh runtuh
	// menjadi satu nilai di perjalanan.
	if rows[0].ConfirmedAt != nil || rows[0].TerminalAt != nil || rows[0].TerminalState != nil {
		t.Error("kolom NULL-able terisi padahal entri tidak pernah CONFIRMED maupun terminal")
	}
	if dropped, upsertFail := obsRec.nearSnapshot(); dropped != 0 || upsertFail != 0 {
		t.Errorf("counter near = %d/%d, mau 0/0", dropped, upsertFail)
	}
}

// Kegagalan upsert dihitung pada counter near-confirmation SENDIRI, dan tidak
// menyentuh satu pun counter satuan event. Sebuah satuan event yang hilang dan
// sebuah jawaban forensik yang hilang bukan kerugian yang sama.
func TestNearConfirmedUpsertFailureCountsSeparately(t *testing.T) {
	fs := newFakeNearStore()
	fs.upsertErr = errors.New("timeout")
	obsRec := &recObs{}

	w := NewWriter(fs, 16, testLogger())
	w.SetEventObserver(obsRec)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	w.RecordNearConfirmed(nearRow("E1"))
	w.Stop()

	if calls, rows := fs.snapshot(); calls != 1 || len(rows) != 0 {
		t.Fatalf("dipanggil %d kali, %d baris tersimpan, mau 1/0", calls, len(rows))
	}
	dropped, upsertFail := obsRec.nearSnapshot()
	if upsertFail != 1 || dropped != 0 {
		t.Errorf("near upsert_failures/dropped = %d/%d, mau 1/0", upsertFail, dropped)
	}
	if d, uf, ls, lf := obsRec.snapshot(); d+uf+ls+lf != 0 {
		t.Errorf("counter satuan event = %d/%d/%d/%d, mau nol semua: kerugiannya bukan yang itu",
			d, uf, ls, lf)
	}
	if got := w.WriteFailures(); got != 1 {
		t.Errorf("ledger_write_failures = %d, mau 1: kegagalan tetap terhitung di sini juga", got)
	}
}

// Antrean penuh melaporkan catatan yang dibuang lewat callback near-confirmation
// sendiri. Tidak ada target nol untuk angka ini (D-011 batasan 1): antrean ini
// sengaja boleh membuang, dan sebuah SLO nol-buangan hanya dapat ditepati dengan
// memblokir jalur peringatan.
func TestFullQueueReportsDroppedNearConfirmed(t *testing.T) {
	fs := newFakeNearStore()
	fs.block = make(chan struct{}) // observasi pertama menggantung drain
	obsRec := &recObs{}

	w := NewWriter(fs, 1, testLogger())
	w.SetEventObserver(obsRec)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	// Satu observasi menahan goroutine drain, lalu antrean berkapasitas 1 diisi
	// sampai membuang.
	w.RecordObservation(obs("NODE-0A1B2C3D", VerifyResultOK))
	const n = 200
	for i := 0; i < n; i++ {
		w.RecordNearConfirmed(nearRow("E1"))
	}

	if dropped, _ := obsRec.nearSnapshot(); dropped == 0 {
		t.Fatal("near dropped = 0: catatan yang dibuang harus dilaporkan")
	}
	if d, _, _, _ := obsRec.snapshot(); d != 0 {
		t.Errorf("event_persist_dropped = %d, mau 0: yang dibuang bukan satuan event", d)
	}
	if got := w.Drops(); got == 0 {
		t.Error("ledger_dropped_total = 0: pembuangan harus tetap terhitung di sini juga")
	}
	close(fs.block)
}

// Store yang belum menjalankan migrasi 000009 — dan setiap fake yang tidak peduli
// pada P4-M2′ — hanya melewatkan catatannya. Bukan galat, dan bukan pula sesuatu
// yang boleh menjatuhkan goroutine drain.
func TestNearConfirmedOnStoreWithoutSupportIsNoop(t *testing.T) {
	fs := newFakeEventStore() // punya UpsertEvent, TIDAK punya UpsertNearConfirmed
	obsRec := &recObs{}

	w := NewWriter(fs, 16, testLogger())
	w.SetEventObserver(obsRec)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	w.RecordNearConfirmed(nearRow("E1"))
	w.Stop()

	if got := fs.callLog(); len(got) != 0 {
		t.Errorf("panggilan store = %v, mau tidak ada", got)
	}
	if dropped, upsertFail := obsRec.nearSnapshot(); dropped != 0 || upsertFail != 0 {
		t.Errorf("counter near = %d/%d, mau 0/0: dilewatkan bukan gagal", dropped, upsertFail)
	}
}

// Catatan nil, catatan tanpa event_id, dan writer yang sudah berhenti tidak boleh
// menghasilkan panggilan store maupun panik.
func TestRecordNearConfirmedIgnoresUnusableRecords(t *testing.T) {
	fs := newFakeNearStore()
	w := NewWriter(fs, 16, testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	w.RecordNearConfirmed(nil)
	w.RecordNearConfirmed(&NearConfirmed{}) // event_id kosong
	w.Stop()
	w.RecordNearConfirmed(nearRow("E1")) // setelah Stop

	if calls, _ := fs.snapshot(); calls != 0 {
		t.Errorf("UpsertNearConfirmed dipanggil %d kali, mau 0", calls)
	}
}

// Kegagalan tulis satuan event TIDAK melewatkan catatan near-confirmation, dan itu
// kebalikan yang tepat dari aturan event_state_log.
//
// Baris log dilewatkan setelah upsert induk gagal karena FK-nya mensyaratkan
// induknya ada. event_near_confirmed sengaja TIDAK punya FK (migrasi 000009), justru
// supaya sebuah satuan induk yang gagal atau dibuang tidak membuat persilangannya
// menjadi tidak pernah terjadi. Uji ini yang membuat kedua aturan itu tidak dapat
// tertukar.
func TestNearConfirmedUnaffectedByEventUnitFailure(t *testing.T) {
	fs := &bothStore{fakeNearStore: newFakeNearStore()}
	fs.upsertEventErr = errors.New("kehabisan koneksi")
	obsRec := &recObs{}

	w := NewWriter(fs, 16, testLogger())
	w.SetEventObserver(obsRec)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	w.RecordEventUnit(unit("E1", 1))
	w.RecordNearConfirmed(nearRow("E1"))
	w.Stop()

	_, rows := fs.snapshot()
	if len(rows) != 1 {
		t.Fatalf("baris near tersimpan = %d, mau 1: catatan ini tidak punya FK ke induk mana pun",
			len(rows))
	}
	if _, upsertFail, logSkip, _ := obsRec.snapshot(); upsertFail != 1 || logSkip != 1 {
		t.Errorf("upsert_failures/state_log_skipped = %d/%d, mau 1/1", upsertFail, logSkip)
	}
	if dropped, nearFail := obsRec.nearSnapshot(); dropped != 0 || nearFail != 0 {
		t.Errorf("counter near = %d/%d, mau 0/0: kegagalan induk bukan kegagalannya", dropped, nearFail)
	}
}

// bothStore mendukung KEDUA kemampuan, dipakai hanya oleh uji di atas: yang diuji
// di sana adalah kemandirian kedua jalur, dan itu hanya dapat diuji pada store yang
// mengenal keduanya.
type bothStore struct {
	*fakeNearStore
	upsertEventErr error
}

func (b *bothStore) UpsertEvent(_ context.Context, e *store.EarthquakeEvent) error {
	if b.upsertEventErr != nil {
		return b.upsertEventErr
	}
	return nil
}

func (b *bothStore) AppendStateLog(_ context.Context, _ *store.EventStateLog) error { return nil }
