package event

// --- Integrasi Postgres NYATA untuk near-confirmation durable (P4-M2′, D-012) ---
//
// Berkas ini menguji hal yang TIDAK dapat diuji oleh fake, dan daftarnya pendek
// tetapi justru itu inti klaim M2′:
//
//	restart proses  — sebuah Tracker mati, sebuah Tracker lain menyala terhadap
//	                  basis data yang SAMA, dan jawabannya masih ada. Fake
//	                  restartWithNear membuktikan jalur Go-nya; hanya Postgres yang
//	                  membuktikan barisnya benar-benar selamat dari proses yang
//	                  memilikinya.
//	parameter lampau — algo_ver dan min_independent_cells baris lampau tidak
//	                  ditulis ulang oleh biner yang konfigurasinya sudah berbeda.
//	                  Yang menjaganya adalah ON CONFLICT di SQL, jadi hanya SQL
//	                  yang dapat membuktikannya.
//	yatim           — baris near-confirmation tetap terbaca setelah baris induknya
//	                  DIHAPUS. Tanpa FK itu benar; dengan FK ia mustahil. Hanya
//	                  Postgres yang tahu bedanya.
//	S1              — persistensi yang RUSAK SUNGGUHAN (kolam koneksi tertutup)
//	                  tidak menahan satu pun frame.
//
//	TEST_DATABASE_URL=postgres://... go test ./internal/event -run TestPGNearConfirmed
//
// Tanpa env itu seluruh uji di berkas ini skip, pola yang sama dengan
// internal/store.

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/banana-pixel/quakealert/server/internal/consensus"
	"github.com/banana-pixel/quakealert/server/internal/ledger"
	"github.com/banana-pixel/quakealert/server/internal/store"
)

// pgTestDBURL membaca TEST_DATABASE_URL; kosong berarti lingkungan tidak
// menyediakan database integrasi.
func pgTestDBURL(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL tidak diset — lewati integrasi Postgres")
	}
	return dsn
}

func pgLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// pgVerifyPool membuka kolam KETIGA, terpisah dari kedua proses yang diuji.
//
// Terpisah karena ia harus tetap hidup ketika proses A sengaja ditutup — dan
// karena verifikasi lewat kolam yang sama dengan yang diuji tidak dapat
// membedakan "barisnya ada" dari "kolamnya masih ingat". Ia juga yang membersihkan
// baris uji, dengan alasan yang sama.
func pgVerifyPool(t *testing.T, dsn string) *pgxpool.Pool {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	// Basis data uji dibatasi max_connections; tiga kolam sekaligus harus muat.
	cfg.MaxConns = 2
	cfg.MinConns = 0
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("kolam verifikasi: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// pgSeedNode menyisipkan node lewat penulis PRODUKSI (CreateNode). verified
// dibiarkan default FALSE: GetNodeLocation sengaja tidak menyaringnya, dan uji ini
// menguji jalur koordinat, bukan gerbang verifikasi ingest.
func pgSeedNode(t *testing.T, st *store.Store, pool *pgxpool.Pool, id string, lat, lon float64) {
	t.Helper()
	err := st.CreateNode(context.Background(), &store.NewNode{
		StationID: id, SensorModel: "MPU 6050", LocationName: id + " site",
		Lat: lat, Lon: consensus.NormalizeLon(lon),
		SecretEnc: []byte{0}, SecretNonce: []byte{0},
	})
	if err != nil {
		t.Fatalf("seed node %s: %v", id, err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM iot_nodes WHERE station_id = $1`, id)
	})
}

// pgCleanupEvent mendaftarkan pembersihan ketiga tabel milik satu event uji.
// event_state_log LEBIH DULU: FK-nya menunjuk earthquake_events.
func pgCleanupEvent(t *testing.T, pool *pgxpool.Pool, eventID string) {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, `DELETE FROM event_state_log WHERE event_id = $1`, eventID)
		_, _ = pool.Exec(ctx, `DELETE FROM earthquake_events WHERE event_id = $1`, eventID)
		_, _ = pool.Exec(ctx, `DELETE FROM event_near_confirmed WHERE event_id = $1`, eventID)
	})
}

// ---- rig: Tracker + ledger.Writer + *store.Store yang SUNGGUHAN --------------

// pgRig adalah satu "proses": satu kolam koneksi, satu antrean tulis, satu
// Tracker. Sengaja dibangun tanpa harness: harness memasang fakeLoc, dan seluruh
// nilai berkas ini justru terletak pada loc yang merupakan *store.Store — satu
// nilai yang memenuhi nodeSource, eventStore, DAN nearConfirmedReader sekaligus,
// sehingga kedua cabang assertion tipe di Reconcile menempuh basis data nyata.
//
// Jamnya tetap palsu. Yang diuji di sini adalah durabilitas, bukan penjadwalan,
// dan stempel waktu yang deterministik membuat "nilai yang direkam dipertahankan"
// dapat dinyatakan sebagai kesamaan angka alih-alih sebagai toleransi.
type pgRig struct {
	t     *testing.T
	st    *store.Store
	w     *ledger.Writer
	trk   *Tracker
	emit  *recEmitter
	clock *fakeClock

	cancel  context.CancelFunc
	stopped bool
}

// newPGRig membuka proses baru terhadap dsn. clock boleh dibagi dengan proses
// sebelumnya: proses kedua yang memulai jamnya sendiri akan terlihat jauh di masa
// depan dan setiap event yang dimuat akan langsung kedaluwarsa, yang bukan hal yang
// ingin diuji di sini.
func newPGRig(t *testing.T, dsn string, clock *fakeClock, mutate ...func(*Options)) *pgRig {
	t.Helper()

	opt := defaultOptions()
	for _, m := range mutate {
		m(&opt)
	}

	st, err := store.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}

	log := pgLogger()
	r := &pgRig{t: t, st: st, clock: clock, emit: &recEmitter{}}
	r.w = ledger.NewWriter(st, 64, log)
	r.trk = NewTracker(st, opt, log)
	r.trk.now = clock.now
	// Sejajar dengan newHarness: NewTracker menstempel startedAtMs dari jam yang
	// terpasang PADA saat itu (time.Now), dan jam palsu baru menggantikannya sebaris
	// di atas. Tanpa penyelarasan ini selubung cakupan melaporkan awal jendela dari
	// jam nyata dan ujungnya dari jam palsu.
	r.trk.startedAtMs = clock.now().UnixMilli()
	r.trk.SetEmitter(r.emit)
	r.trk.SetLedger(r.w)
	r.w.SetEventObserver(r.trk)

	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	go r.w.Run(ctx)

	t.Cleanup(r.stop)
	return r
}

// stop mematikan proses ini seperti shutdown sungguhan: antrean dibilas lebih dulu
// (Writer.Stop menunggu finalDrain), baru kolam koneksinya ditutup. Idempoten.
func (r *pgRig) stop() {
	if r.stopped {
		return
	}
	r.stopped = true
	r.w.Stop()
	r.cancel()
	r.st.Close()
}

// ingest menyuntikkan satu observasi lewat jalur masuk yang sungguhan.
func (r *pgRig) ingest(in Input) {
	r.t.Helper()
	r.trk.Ingest(context.Background(), in)
}

// only mengembalikan satu-satunya event terlacak.
func (r *pgRig) only() *Event {
	r.t.Helper()
	r.trk.mu.Lock()
	defer r.trk.mu.Unlock()
	if len(r.trk.events) != 1 {
		r.t.Fatalf("event terlacak = %d, mau tepat 1", len(r.trk.events))
	}
	for _, e := range r.trk.events {
		return e
	}
	return nil
}

// tracked adalah jumlah event yang masih dipegang Tracker (terbuka + tombstone).
func (r *pgRig) tracked() int {
	r.trk.mu.Lock()
	defer r.trk.mu.Unlock()
	return len(r.trk.events)
}

// crossSilently membawa satu event melewati ambang independensi TANPA transisi
// state — persilangan sunyi §5.2, kasus yang seluruh tabel durable ini ada untuk
// menangkap. Kedua node harus sudah tersemai di iot_nodes.
func (r *pgRig) crossSilently(n1, n2 string) *Event {
	r.t.Helper()
	r.ingest(v2(n1, MinPGAGal+10, onsetBase, PhasePrelim, 1))
	r.ingest(v2(n2, MinPGAGal+10, onsetBase+500, PhasePrelim, 1))
	e := r.only()
	if e.State != StateUnconfirmed {
		r.t.Fatalf("persiapan: state = %s, mau UNCONFIRMED", e.State)
	}
	return e
}

// pgNearRow adalah satu baris event_near_confirmed yang dibaca lewat SQL LANGSUNG,
// bukan lewat ListNearConfirmed.
//
// SQL langsung dengan sengaja: yang diuji item ini adalah bahwa BARISNYA ada di
// dalam Postgres, dan membacanya lewat pembaca yang sama yang dipakai jalur boot
// akan membuat sebuah bug pemetaan kolom lolos dua kali — sekali saat menulis,
// sekali saat memeriksa.
type pgNearRow struct {
	EventID       string
	FirstTwoAt    int64
	IndepAtPeak   int
	NodesAtPeak   int
	MinCells      int
	ConfirmedAt   *int64
	TerminalState *string
	TerminalAt    *int64
	AlgoVer       string
}

func pgReadNearRow(t *testing.T, pool *pgxpool.Pool, eventID string) *pgNearRow {
	t.Helper()
	var r pgNearRow
	err := pool.QueryRow(context.Background(), `
		SELECT event_id::text, first_two_independent_at, independent_count_at_peak,
		       node_count_at_peak, min_independent_cells, confirmed_at,
		       terminal_state, terminal_at, algo_ver
		FROM event_near_confirmed WHERE event_id = $1::uuid`, eventID).
		Scan(&r.EventID, &r.FirstTwoAt, &r.IndepAtPeak, &r.NodesAtPeak, &r.MinCells,
			&r.ConfirmedAt, &r.TerminalState, &r.TerminalAt, &r.AlgoVer)
	if err != nil {
		t.Fatalf("baca event_near_confirmed untuk %s: %v", eventID, err)
	}
	return &r
}

// pgTwoIndependentNodes menyemai dua node terpisah 8 km — di atas
// IndependenceCellKm=5, di bawah AttachRadiusKm=50 — geometri terkecil yang dapat
// melampaui ambang independensi tanpa mencapai kuorum (S2: kuorum butuh 3).
func pgTwoIndependentNodes(t *testing.T, st *store.Store, pool *pgxpool.Pool, n1, n2 string) {
	t.Helper()
	pgSeedNode(t, st, pool, n1, baseLat, baseLon)
	lat2, lon2 := destinationKm(baseLat, baseLon, 8, 90)
	pgSeedNode(t, st, pool, n2, lat2, lon2)
}

// TestPGNearConfirmedSurvivesProcessRestart — item validasi 5, terhadap Postgres
// yang sungguhan.
//
// Proses A menyaksikan sebuah persilangan sunyi dan mati: antrean dibilas, kolam
// koneksinya ditutup, Tracker-nya beserta seluruh peta di memorinya hilang. Proses
// B menyala terhadap basis data yang SAMA dengan konfigurasi yang BERBEDA — ic=9,
// ambang 3 — dan harus dapat menjawab pertanyaan yang tidak pernah disaksikannya.
//
// Konfigurasinya dibuat berbeda dengan sengaja, dan itu inti item ini: kalau ada
// SATU saja jalur yang menghitung ulang, entri yang dimuat akan mengaku ic=9 dan
// ambang 3, yaitu menilai keputusan lampau dengan parameter yang tidak
// menghasilkannya (U-007, yang belum diputuskan dan tidak boleh dijawab oleh
// implementasi).
func TestPGNearConfirmedSurvivesProcessRestart(t *testing.T) {
	dsn := pgTestDBURL(t)
	pool := pgVerifyPool(t, dsn)
	clock := newFakeClock()

	// ---- proses A: menyaksikan persilangannya ----
	a := newPGRig(t, dsn, clock)
	pgTwoIndependentNodes(t, a.st, pool, "PG5N1", "PG5N2")

	e := a.crossSilently("PG5N1", "PG5N2")
	eventID := e.ID
	pgCleanupEvent(t, pool, eventID)

	repA := a.trk.NearConfirmedReport()
	if len(repA.Entries) != 1 {
		t.Fatalf("entri proses A = %d, mau 1", len(repA.Entries))
	}
	want := repA.Entries[0]
	if want.Source != NearConfirmedSourceProcess {
		t.Fatalf("source proses A = %q, mau %q", want.Source, NearConfirmedSourceProcess)
	}
	if want.AlgoVer != "phase3-1.1/ic=5" {
		t.Fatalf("algo_ver proses A = %q, mau phase3-1.1/ic=5", want.AlgoVer)
	}

	// Mati: antrean dibilas oleh Writer.Stop, lalu kolamnya ditutup.
	a.stop()

	// ---- barisnya ada di Postgres, dibaca lewat kolam KETIGA ----
	row := pgReadNearRow(t, pool, eventID)
	if row.FirstTwoAt != want.FirstTwoIndependentAt {
		t.Errorf("first_two_independent_at durable = %d, mau %d", row.FirstTwoAt, want.FirstTwoIndependentAt)
	}
	if row.IndepAtPeak != want.IndependentCountAtPeak {
		t.Errorf("independent_count_at_peak durable = %d, mau %d", row.IndepAtPeak, want.IndependentCountAtPeak)
	}
	if row.NodesAtPeak != want.NodeCountAtPeak {
		t.Errorf("node_count_at_peak durable = %d, mau %d", row.NodesAtPeak, want.NodeCountAtPeak)
	}
	if row.MinCells != want.MinIndependentCells {
		t.Errorf("min_independent_cells durable = %d, mau %d", row.MinCells, want.MinIndependentCells)
	}
	if row.AlgoVer != want.AlgoVer {
		t.Errorf("algo_ver durable = %q, mau %q", row.AlgoVer, want.AlgoVer)
	}
	if row.ConfirmedAt != nil {
		t.Errorf("confirmed_at = %d, mau NULL: kuorum tak terjangkau pada 2 node (S2)", *row.ConfirmedAt)
	}
	if row.TerminalState != nil || row.TerminalAt != nil {
		t.Errorf("terminal_state/at = %v/%v, mau NULL/NULL: event masih terbuka",
			row.TerminalState, row.TerminalAt)
	}

	// ---- proses B: konfigurasi berbeda, basis data yang sama ----
	b := newPGRig(t, dsn, clock, func(o *Options) {
		o.IndependenceCellKm = 9
		o.MinIndependentCells = 3
	})
	if err := b.trk.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile proses B: %v", err)
	}

	repB := b.trk.NearConfirmedReport()
	if len(repB.Entries) != 1 {
		t.Fatalf("entri proses B = %d, mau 1: inilah kriteria M2′", len(repB.Entries))
	}
	got := repB.Entries[0]

	if got.EventID != eventID {
		t.Errorf("event_id = %q, mau %q", got.EventID, eventID)
	}
	if got.Source != NearConfirmedSourceDurable {
		t.Errorf("source = %q, mau %q: proses ini tidak menyaksikan persilangannya",
			got.Source, NearConfirmedSourceDurable)
	}
	if got.UpdatedInProcess {
		t.Error("updated_in_process = true padahal entri belum berubah di proses ini")
	}
	if got.FirstTwoIndependentAt != want.FirstTwoIndependentAt {
		t.Errorf("first_two_independent_at = %d, mau %d", got.FirstTwoIndependentAt, want.FirstTwoIndependentAt)
	}
	if got.IndependentCountAtPeak != want.IndependentCountAtPeak {
		t.Errorf("independent_count_at_peak = %d, mau %d", got.IndependentCountAtPeak, want.IndependentCountAtPeak)
	}
	if got.NodeCountAtPeak != want.NodeCountAtPeak {
		t.Errorf("node_count_at_peak = %d, mau %d", got.NodeCountAtPeak, want.NodeCountAtPeak)
	}

	// Parameter yang DIREKAM, bukan yang berlaku sekarang.
	if got.AlgoVer != want.AlgoVer {
		t.Errorf("algo_ver = %q, mau %q: baris lampau TIDAK ditulis ulang oleh biner baru",
			got.AlgoVer, want.AlgoVer)
	}
	if got.AlgoVer == b.trk.algoVer() {
		t.Errorf("algo_ver entri = algo_ver proses (%q): entri yang dimuat dihitung ulang", got.AlgoVer)
	}
	if got.MinIndependentCells != want.MinIndependentCells {
		t.Errorf("min_independent_cells = %d, mau %d: ambang lampau TIDAK disegarkan",
			got.MinIndependentCells, want.MinIndependentCells)
	}
	if got.MinIndependentCells == b.trk.opt.MinIndependentCells {
		t.Errorf("min_independent_cells entri = ambang proses (%d): entri yang dimuat dihitung ulang",
			got.MinIndependentCells)
	}

	// Selubung cakupan menyatakan parameter yang berlaku SEKARANG, terpisah dari
	// parameter entri, sehingga pembaca dapat melihat keduanya berbeda tanpa
	// membandingkan entri satu per satu (B1).
	cov := repB.Coverage
	if !cov.DurableReadAttempted || !cov.DurableReadOK {
		t.Errorf("durable_read attempted/ok = %v/%v, mau true/true",
			cov.DurableReadAttempted, cov.DurableReadOK)
	}
	if cov.DurableReadError != "" {
		t.Errorf("durable_read_error = %q, mau kosong", cov.DurableReadError)
	}
	if cov.DurableRowsLoaded != 1 || cov.EntriesLoadedFromDurable != 1 {
		t.Errorf("baris dimuat = %d, entri LOADED = %d, mau 1/1",
			cov.DurableRowsLoaded, cov.EntriesLoadedFromDurable)
	}
	if cov.EntriesRecordedInProcess != 0 {
		t.Errorf("entri RECORDED = %d, mau 0: proses ini tidak menyaksikan satu pun persilangan",
			cov.EntriesRecordedInProcess)
	}
	if cov.AlgoVer != b.trk.algoVer() || cov.MinIndependentCells != 3 {
		t.Errorf("cakupan algo_ver/ambang = %q/%d, mau %q/3",
			cov.AlgoVer, cov.MinIndependentCells, b.trk.algoVer())
	}

	// Dan tabelnya TIDAK berubah karena proses B menyala: pembacaan boot murni baca.
	after := pgReadNearRow(t, pool, eventID)
	if after.AlgoVer != row.AlgoVer || after.MinCells != row.MinCells ||
		after.FirstTwoAt != row.FirstTwoAt || after.IndepAtPeak != row.IndepAtPeak {
		t.Errorf("baris berubah setelah boot proses B: %+v, mau %+v", after, row)
	}
}

// pgWaitFor menunggu sebuah kondisi asinkron sampai batas waktu.
//
// Diperlukan karena penulisan durable SENGAJA asinkron (§9.5): Tracker tidak punya
// cara untuk mengetahui apakah pencatatannya berhasil, jadi uji pun tidak boleh
// punya jalur sinkron untuk menanyakannya. Menunggu dengan batas adalah bentuk yang
// benar; memanggil Stop() akan menutup antrean dan membuat langkah berikutnya
// menguji proses yang sudah mati.
func pgWaitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("batas waktu menunggu: %s", what)
}

// TestPGNearConfirmedTerminalOutlivesItsParentRow — item validasi 6.
//
// Pertanyaan pasca-kejadian yang paling sering diajukan justru menyangkut event
// yang sudah TIDAK ADA lagi: tombstone-nya dievakuasi dari peta Tracker, dan pada
// retensi yang cukup panjang baris induknya pun dapat sudah hilang. Uji ini
// menghapus baris induk itu SUNGGUHAN dan menuntut jawabannya tetap ada.
//
// Hanya Postgres yang dapat membuktikan bagian terakhirnya: tanpa FK baris yatim
// itu sah (migrasi 000009 menjelaskan mengapa FK-nya sengaja tidak ada); DENGAN FK
// penghapusan induk akan menghapus jawabannya juga, atau gagal. Fake tidak
// mengetahui bedanya.
func TestPGNearConfirmedTerminalOutlivesItsParentRow(t *testing.T) {
	dsn := pgTestDBURL(t)
	pool := pgVerifyPool(t, dsn)
	clock := newFakeClock()
	opt := defaultOptions()

	a := newPGRig(t, dsn, clock)
	pgTwoIndependentNodes(t, a.st, pool, "PG6N1", "PG6N2")

	e := a.crossSilently("PG6N1", "PG6N2")
	eventID := e.ID
	pgCleanupEvent(t, pool, eventID)

	// Sweep pertama: bukti berhenti datang, event ditutup.
	clock.advance(time.Duration(opt.ResolveAfterMs+1) * time.Millisecond)
	a.trk.sweep(context.Background())
	if e.State != StateResolved {
		t.Fatalf("state = %s, mau RESOLVED", e.State)
	}
	closedAt := clock.now().UnixMilli()

	// Sweep kedua: retensi tombstone terlampaui, event menghilang dari peta.
	clock.advance(time.Duration(opt.TerminalRetentionMs+1) * time.Millisecond)
	a.trk.sweep(context.Background())
	if n := a.tracked(); n != 0 {
		t.Fatalf("event terlacak = %d, mau 0 setelah tombstone dievakuasi", n)
	}
	// Evakuasi karena USIA, bukan karena tekanan langit-langit (§6.8, §18.2).
	if got := a.trk.TombstoneEvictions(); got != 0 {
		t.Errorf("event_tombstone_evictions_total = %d, mau 0: evakuasi ini karena usia", got)
	}

	// Jawaban di memori proses ini tetap ada meski event-nya tidak.
	repA := a.trk.NearConfirmedReport()
	if len(repA.Entries) != 1 {
		t.Fatalf("entri = %d, mau 1 setelah evakuasi", len(repA.Entries))
	}
	if got := repA.Entries[0].TerminalState; got != string(StateResolved) {
		t.Errorf("terminal_state di memori = %q, mau RESOLVED", got)
	}

	// Bilas antrean, lalu periksa barisnya di Postgres.
	a.stop()

	row := pgReadNearRow(t, pool, eventID)
	if row.TerminalState == nil || *row.TerminalState != string(StateResolved) {
		t.Fatalf("terminal_state durable = %v, mau RESOLVED", row.TerminalState)
	}
	if row.TerminalAt == nil || *row.TerminalAt != closedAt {
		t.Errorf("terminal_at durable = %v, mau %d", row.TerminalAt, closedAt)
	}
	if row.ConfirmedAt != nil {
		t.Errorf("confirmed_at = %d, mau NULL: event ini mati tanpa konfirmasi", *row.ConfirmedAt)
	}

	// ---- baris INDUK dihapus: event-nya benar-benar tidak ada lagi ----
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `DELETE FROM event_state_log WHERE event_id = $1::uuid`, eventID); err != nil {
		t.Fatalf("hapus event_state_log: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM earthquake_events WHERE event_id = $1::uuid`, eventID); err != nil {
		t.Fatalf("hapus earthquake_events: %v", err)
	}
	var parents int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM earthquake_events WHERE event_id = $1::uuid`, eventID).Scan(&parents); err != nil {
		t.Fatalf("hitung baris induk: %v", err)
	}
	if parents != 0 {
		t.Fatalf("baris induk = %d, mau 0", parents)
	}

	// Barisnya YATIM dan tetap ada — inilah yang tidak akan berlaku bila
	// event_near_confirmed punya FK ke earthquake_events.
	orphan := pgReadNearRow(t, pool, eventID)
	if orphan.TerminalState == nil || *orphan.TerminalState != string(StateResolved) {
		t.Fatalf("terminal_state yatim = %v, mau RESOLVED", orphan.TerminalState)
	}

	// ---- dan proses BERIKUTNYA masih dapat menjawabnya ----
	c := newPGRig(t, dsn, clock)
	if err := c.trk.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile proses C: %v", err)
	}
	if n := c.tracked(); n != 0 {
		t.Errorf("event terlacak proses C = %d, mau 0: induknya sudah tidak ada", n)
	}

	repC := c.trk.NearConfirmedReport()
	var got *NearConfirmedEntry
	for i := range repC.Entries {
		if repC.Entries[i].EventID == eventID {
			got = &repC.Entries[i]
		}
	}
	if got == nil {
		t.Fatalf("entri %s tidak ada di proses C: state terminal harus tetap dapat ditanyakan", eventID)
	}
	if got.Source != NearConfirmedSourceDurable {
		t.Errorf("source = %q, mau %q", got.Source, NearConfirmedSourceDurable)
	}
	if got.TerminalState != string(StateResolved) || got.TerminalAt != closedAt {
		t.Errorf("terminal proses C = %q/%d, mau RESOLVED/%d",
			got.TerminalState, got.TerminalAt, closedAt)
	}
	if got.ConfirmedAt != 0 {
		t.Errorf("confirmed_at = %d, mau 0: event ini mati tanpa konfirmasi", got.ConfirmedAt)
	}
}

// pgBreakNearConfirmedTable membuat setiap penulisan ke event_near_confirmed GAGAL
// SUNGGUHAN, dan memulihkannya saat uji selesai.
//
// Caranya mengganti nama tabel, bukan mencabut hak: peran uji ini superuser dan
// PEMILIK tabelnya, jadi REVOKE tidak menghasilkan kegagalan apa pun. Yang
// dibutuhkan item ini adalah galat basis data yang nyata pada satu jalur tulis
// saja — bukan kolam yang tertutup, yang akan mematikan pencarian koordinat node
// dan karenanya menghapus justru emisi yang sedang diuji.
//
// Hanya tabel ini yang dirusak, dan itu bagian dari yang diuji: satuan event tetap
// tersimpan, sehingga akuntansi near-confirmation terbukti TERPISAH dan bukan satu
// angka yang menelan keduanya.
func pgBreakNearConfirmedTable(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	const hidden = "event_near_confirmed_p4m2_broken"

	if _, err := pool.Exec(ctx,
		`ALTER TABLE event_near_confirmed RENAME TO `+hidden); err != nil {
		t.Fatalf("sembunyikan event_near_confirmed: %v", err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(),
			`ALTER TABLE IF EXISTS `+hidden+` RENAME TO event_near_confirmed`); err != nil {
			t.Errorf("pulihkan event_near_confirmed: %v", err)
		}
	})
}

// TestPGNearConfirmedPersistenceFailureDoesNotBlockEmission — item validasi 7, dan
// S1 secara harfiah: "pencatatan boleh gagal, jalur peringatan tidak."
//
// Tabel durable-nya dibuat benar-benar tidak dapat ditulis, lalu jalur peringatan
// dijalankan seperti biasa. Yang harus tetap utuh: setiap frame keluar, peta di
// memori tetap menjawab, dan satuan event tetap tersimpan. Yang boleh hilang:
// barisnya — dan kehilangan itu DIHITUNG, bukan sunyi.
func TestPGNearConfirmedPersistenceFailureDoesNotBlockEmission(t *testing.T) {
	dsn := pgTestDBURL(t)
	pool := pgVerifyPool(t, dsn)
	clock := newFakeClock()
	opt := defaultOptions()

	a := newPGRig(t, dsn, clock)
	pgTwoIndependentNodes(t, a.st, pool, "PG7N1", "PG7N2")

	// Rusak SEBELUM satu pun persilangan: tidak ada baris yang sempat tertulis, jadi
	// setiap kehilangan di bawah ini benar-benar disebabkan kegagalan.
	pgBreakNearConfirmedTable(t, pool)

	e := a.crossSilently("PG7N1", "PG7N2")
	eventID := e.ID
	pgCleanupEvent(t, pool, eventID)

	// EMISI: frame DETECTED->UNCONFIRMED sudah keluar pada saat Ingest kembali.
	// Diperiksa di sini, sebelum menunggu apa pun: penulisannya asinkron, jadi
	// sebuah frame yang menunggu penulisannya akan terlihat sebagai frame yang
	// belum ada di titik ini.
	if got := a.emit.countFor(StateUnconfirmed); got != 1 {
		t.Fatalf("frame UNCONFIRMED = %d, mau 1: emisi tidak boleh menunggu pencatatan (S1)", got)
	}

	// Peta di memori tetap otoritasnya (§9.5): jawabannya ada meski barisnya tidak.
	repBroken := a.trk.NearConfirmedReport()
	if len(repBroken.Entries) != 1 {
		t.Fatalf("entri di memori = %d, mau 1: kegagalan tulis tidak menghapus jawabannya",
			len(repBroken.Entries))
	}

	pgWaitFor(t, "kegagalan upsert near-confirmed tercatat", func() bool {
		return a.trk.NearConfirmedUpsertFailures() >= 1
	})

	// Sweep menutup event: frame kedua keluar, dan kegagalan kedua tercatat.
	clock.advance(time.Duration(opt.ResolveAfterMs+1) * time.Millisecond)
	a.trk.sweep(context.Background())
	if got := a.emit.countFor(StateResolved); got != 1 {
		t.Fatalf("frame RESOLVED = %d, mau 1: all-clear pun tidak menunggu pencatatan", got)
	}
	pgWaitFor(t, "kegagalan upsert kedua tercatat", func() bool {
		return a.trk.NearConfirmedUpsertFailures() >= 2
	})

	// Akuntansinya TERPISAH: jalur satuan event tidak tersentuh oleh kegagalan ini.
	if got := a.trk.NearConfirmedDropped(); got != 0 {
		t.Errorf("event_near_confirmed_persist_dropped_total = %d, mau 0: ini kegagalan tulis, bukan pembuangan antrean", got)
	}
	if got := a.trk.UpsertFailures(); got != 0 {
		t.Errorf("event_upsert_failures_total = %d, mau 0: hanya tabel near-confirmed yang rusak", got)
	}
	if got := a.trk.StateLogFailures(); got != 0 {
		t.Errorf("event_state_log_failures_total = %d, mau 0", got)
	}
	if got := a.trk.StateLogSkipped(); got != 0 {
		t.Errorf("event_state_log_skipped_total = %d, mau 0", got)
	}
	if got := a.trk.PersistDropped(); got != 0 {
		t.Errorf("event_persist_dropped_total = %d, mau 0", got)
	}

	// Bilas antrean, lalu periksa apa yang benar-benar tersimpan.
	a.stop()

	ctx := context.Background()
	var parents int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM earthquake_events WHERE event_id = $1::uuid`, eventID).Scan(&parents); err != nil {
		t.Fatalf("hitung baris induk: %v", err)
	}
	if parents != 1 {
		t.Errorf("baris earthquake_events = %d, mau 1: kegagalan near-confirmed tidak boleh menjatuhkan satuan event", parents)
	}
	var logs int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM event_state_log WHERE event_id = $1::uuid`, eventID).Scan(&logs); err != nil {
		t.Fatalf("hitung baris riwayat: %v", err)
	}
	if logs != 2 {
		t.Errorf("baris event_state_log = %d, mau 2 (UNCONFIRMED, RESOLVED)", logs)
	}

	// Kegagalannya juga terhitung di akuntansi antrean, bukan hanya di Tracker.
	if got := a.w.WriteFailures(); got < 2 {
		t.Errorf("ledger_write_failures_total = %d, mau >= 2", got)
	}
	if got := a.w.Drops(); got != 0 {
		t.Errorf("ledger_drops_total = %d, mau 0: antreannya tidak pernah penuh", got)
	}
}
