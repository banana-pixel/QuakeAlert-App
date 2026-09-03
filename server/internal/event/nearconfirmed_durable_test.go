package event

// --- P4-M2′ (D-012): near-confirmation yang DURABLE, beserta selubung cakupannya ---
//
// Berkas ini menguji sifat yang tidak dimiliki nearconfirmed_test.go, dan
// pemisahannya disengaja: yang lama menguji APA yang dicatat di dalam satu proses,
// yang ini menguji bahwa catatan itu MENYEBERANGI restart dan bahwa sebuah daftar
// kosong dapat menjelaskan dirinya sendiri.
//
// Empat sifat yang seluruhnya tidak dapat diuji lewat NearConfirmedLog():
//
//	persilangan SUNYI    — >= 2 independen tanpa satu pun transisi state
//	                       (UNCONFIRMED -> UNCONFIRMED ilegal, §5.2), jadi tidak
//	                       ada Snapshot dan tidak ada EventUnit yang membawanya.
//	kenaikan PUNCAK      — juga tanpa transisi ketika event sudah CONFIRMED.
//	restart              — entri yang dibaca kembali harus mengaku LOADED.
//	kosong yang EKSPLISIT— fleet satu-node: daftar kosong adalah jawaban yang
//	                       BENAR (S2), jadi ia harus dapat dibedakan dari tidak
//	                       adanya jawaban.

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/banana-pixel/quakealert/server/internal/store"
)

// fakeNearEvents adalah fakeEvents yang JUGA dapat membaca tabel durable
// near-confirmation.
//
// Tipe TERPISAH, dan itu keseluruhan alasannya ada: fakeEvents sengaja TIDAK
// memiliki ListNearConfirmed, sehingga kedua cabang assertion tipe di Reconcile
// benar-benar diuji. Sebuah toko yang dapat memuat event terbuka tetapi belum
// menjalankan migrasi 000009 tetap sah, dan menambahkan metode ini ke fakeEvents
// akan menghapus satu-satunya uji yang membuktikannya.
type fakeNearEvents struct {
	*fakeEvents

	near    []store.NearConfirmedRow
	nearErr error

	// reads adalah jumlah pembacaan tabel durable. Dibaca SEKALI saat boot, bukan
	// per permintaan, dan angka itulah yang menegaskannya.
	reads int
}

func (f *fakeNearEvents) ListNearConfirmed(_ context.Context) ([]store.NearConfirmedRow, error) {
	f.reads++
	if f.nearErr != nil {
		return nil, f.nearErr
	}
	return f.near, nil
}

// restartWithNear mensimulasikan proses yang mati dengan basis data yang selamat —
// termasuk tabel near-confirmation-nya. Sejajar dengan harness.restart, dan
// namanya sama harfiahnya.
func (h *harness) restartWithNear(open []*store.EarthquakeEvent, near ...store.NearConfirmedRow) *fakeNearEvents {
	h.t.Helper()
	return h.attachNear(&fakeNearEvents{
		fakeEvents: &fakeEvents{fakeLoc: h.loc, open: open},
		near:       near,
	})
}

func (h *harness) attachNear(fne *fakeNearEvents) *fakeNearEvents {
	h.trk.loc = fne
	return fne
}

// twoIndependentNodes memasang dua node terpisah 8 km — di atas
// IndependenceCellKm=5, di bawah AttachRadiusKm=50 — yaitu geometri terkecil yang
// dapat melampaui ambang independensi tanpa mencapai kuorum.
func (h *harness) twoIndependentNodes() {
	h.t.Helper()
	h.node("N1", baseLat, baseLon)
	h.nodeAt("N2", baseLat, baseLon, 8, 90)
}

// crossSilently membawa satu event melewati ambang independensi TANPA transisi
// state, dan mengembalikannya.
func (h *harness) crossSilently() *Event {
	h.t.Helper()
	h.twoIndependentNodes()
	h.ingest(v2("N1", MinPGAGal+10, onsetBase, PhasePrelim, 1))
	h.ingest(v2("N2", MinPGAGal+10, onsetBase+500, PhasePrelim, 1))
	e := h.only()
	if e.State != StateUnconfirmed {
		h.t.Fatalf("persiapan: state = %s, mau UNCONFIRMED", e.State)
	}
	return e
}

// TestNearConfirmedFreshOneNodeIsExplicitlyCovered — pada fleet satu-node,
// jawabannya adalah daftar KOSONG, dan daftar itu menjelaskan dirinya sendiri.
//
// Ini kriteria P4-M2′ yang paling mudah dibaca salah. Kuorum butuh >= 3
// kontributor terverifikasi (S2), jadi satu node tidak akan pernah CONFIRMED dan
// tidak akan pernah melampaui ambang independensi: kosong BUKAN kegagalan, ia
// jawaban yang benar. Yang diuji karena itu bukan "kosong", melainkan bahwa
// selubungnya menyatakan interval dan parameter yang berlaku atas kekosongan itu —
// tanpanya jawaban benar ini terkirim sebagai byte yang identik dengan "tidak ada
// yang dapat dijawab".
func TestNearConfirmedFreshOneNodeIsExplicitlyCovered(t *testing.T) {
	h := newHarness(t)
	h.node("N1", baseLat, baseLon)
	h.ingest(v2("N1", MinPGAGal+10, onsetBase, PhasePrelim, 1))

	if e := h.only(); e.State != StateUnconfirmed {
		t.Fatalf("persiapan: state = %s, mau UNCONFIRMED", e.State)
	}

	rep := h.trk.NearConfirmedReport()
	if len(rep.Entries) != 0 {
		t.Fatalf("entri = %d, mau 0: satu node tidak dapat melampaui ambang 2", len(rep.Entries))
	}

	cov := rep.Coverage
	if cov.ProcessStartedAtMs <= 0 {
		t.Error("process_started_at_ms = 0: kekosongan tanpa awal jendela tidak dapat ditafsirkan")
	}
	if cov.AsOfMs < cov.ProcessStartedAtMs {
		t.Errorf("as_of_ms %d < process_started_at_ms %d: intervalnya terbalik",
			cov.AsOfMs, cov.ProcessStartedAtMs)
	}
	// Tracker uji ini tidak pernah menjalankan Reconcile, jadi tabel durable belum
	// pernah dibaca — dan itu HARUS terlihat. "Belum dicoba" bukan "tabelnya kosong".
	if cov.DurableReadAttempted {
		t.Error("durable_read_attempted = true tanpa Reconcile")
	}
	if cov.DurableRowsLoaded != 0 || cov.EntriesLoadedFromDurable != 0 {
		t.Errorf("baris durable = %d/%d, mau 0/0", cov.DurableRowsLoaded, cov.EntriesLoadedFromDurable)
	}
	if cov.EntriesRecordedInProcess != 0 {
		t.Errorf("entries_recorded_in_process = %d, mau 0", cov.EntriesRecordedInProcess)
	}
	// Parameter yang BERLAKU: keduanya yang membuat "kosong" dapat diperiksa alih-alih
	// dipercaya. Pembaca yang melihat min_independent_cells=2 dapat menyimpulkan
	// sendiri bahwa satu node memang tidak akan pernah muncul di daftar ini.
	if cov.MinIndependentCells != defaultOptions().MinIndependentCells {
		t.Errorf("min_independent_cells = %d, mau %d",
			cov.MinIndependentCells, defaultOptions().MinIndependentCells)
	}
	if cov.AlgoVer != h.trk.algoVer() {
		t.Errorf("algo_ver = %q, mau %q", cov.AlgoVer, h.trk.algoVer())
	}
	// Ketiadaan yang disengaja: ini pengukuran cakupan, bukan penilaian. Tidak ada
	// field complete/healthy/valid untuk diperiksa, dan itu memang intinya.
}

// TestNearConfirmedDurableReadFailureIsNotAnEmptyAnswer — kegagalan pembacaan
// menghasilkan daftar yang juga kosong, dan selubungnya yang membedakannya dari
// kekosongan pada uji di atas.
//
// Pasangan kedua uji inilah keseluruhan alasan B1 ada: dua daftar nol-entri yang
// SAMA panjangnya, satu berarti "tidak ada persilangan yang pernah terjadi" dan
// satu berarti "tidak ada yang dapat dijawab".
func TestNearConfirmedDurableReadFailureIsNotAnEmptyAnswer(t *testing.T) {
	h := newHarness(t)
	fne := h.attachNear(&fakeNearEvents{
		fakeEvents: &fakeEvents{fakeLoc: h.loc},
		nearErr:    errors.New("koneksi ditutup"),
	})

	if err := h.trk.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if fne.reads != 1 {
		t.Errorf("ListNearConfirmed dipanggil %d kali, mau 1", fne.reads)
	}

	rep := h.trk.NearConfirmedReport()
	if len(rep.Entries) != 0 {
		t.Fatalf("entri = %d, mau 0", len(rep.Entries))
	}
	cov := rep.Coverage
	if !cov.DurableReadAttempted {
		t.Error("durable_read_attempted = false: pembacaannya DICOBA dan gagal")
	}
	if cov.DurableReadOK {
		t.Error("durable_read_ok = true padahal pembacaannya gagal")
	}
	if cov.DurableReadError == "" {
		t.Error("durable_read_error kosong: galat yang ditelan menghapus perbedaan " +
			"antara kosong-karena-tidak-ada dan kosong-karena-tak-terjawab")
	}
	if cov.DurableRowsLoaded != 0 {
		t.Errorf("durable_rows_loaded = %d, mau 0", cov.DurableRowsLoaded)
	}
	// Reconcile tetap SELESAI: sebuah server yang menolak menyala karena tidak dapat
	// membaca riwayat forensik adalah server yang tidak dapat memperingatkan gempa
	// yang sedang berlangsung (§15.3).
}

// TestNearConfirmedSilentCrossingIsPersisted — persilangan TANPA transisi state
// tetap menjadi baris durable.
//
// Ini kasus yang membuat A2 tidak dapat diganti dengan penurunan ulang dari
// event_state_log. Kontributor kedua tiba saat event sudah UNCONFIRMED, jadi
// classify mengembalikan UNCONFIRMED lagi; UNCONFIRMED -> UNCONFIRMED bukan
// transisi legal (§5.2), sehingga tidak ada revisi, tidak ada baris log, dan tidak
// ada satu pun frame yang keluar. Kalau perubahannya tidak dicatat lewat jalur
// terpisah, ia tidak dapat dicatat di mana pun — dan pada fleet kecil justru
// persilangan seperti inilah yang paling sering terjadi.
func TestNearConfirmedSilentCrossingIsPersisted(t *testing.T) {
	h := newHarness(t)
	p := h.withPersister()

	h.twoIndependentNodes()
	h.ingest(v2("N1", MinPGAGal+10, onsetBase, PhasePrelim, 1))

	// Setelah node pertama: satu transisi (DETECTED -> UNCONFIRMED), satu satuan
	// event. Belum ada persilangan — satu kontributor = satu sel.
	revBefore := h.only().Revision
	unitsBefore := len(p.units)
	if got := len(p.nearFor(h.only().ID)); got != 0 {
		t.Fatalf("catatan durable = %d sebelum node kedua, mau 0", got)
	}

	// Node kedua: persilangan terjadi DI SINI, tanpa transisi.
	h.ingest(v2("N2", MinPGAGal+10, onsetBase+500, PhasePrelim, 1))
	e := h.only()

	if e.State != StateUnconfirmed {
		t.Fatalf("state = %s, mau UNCONFIRMED", e.State)
	}
	// Ketiga hal ini yang membuat persilangannya SUNYI, dan ketiganya ditegaskan
	// supaya uji ini tidak dapat lulus lewat jalur transisi tanpa ada yang sadar.
	if e.Revision != revBefore {
		t.Errorf("revision = %d, mau tetap %d: UNCONFIRMED -> UNCONFIRMED bukan transisi",
			e.Revision, revBefore)
	}
	if len(p.units) != unitsBefore {
		t.Errorf("satuan event = %d, mau tetap %d: tidak ada transisi berarti tidak ada EventUnit",
			len(p.units), unitsBefore)
	}
	if got := h.emit.countFor(StateUnconfirmed); got != 1 {
		t.Errorf("frame UNCONFIRMED = %d, mau tetap 1", got)
	}

	// Dan justru karena itu, catatan durable-nya harus ada.
	row := p.lastNearFor(t, e.ID)
	if row.FirstTwoIndependentAt != h.clock.now().UnixMilli() {
		t.Errorf("first_two_independent_at = %d, mau %d",
			row.FirstTwoIndependentAt, h.clock.now().UnixMilli())
	}
	if row.IndependentCountAtPeak != 2 {
		t.Errorf("independent_count_at_peak = %d, mau 2", row.IndependentCountAtPeak)
	}
	if row.NodeCountAtPeak != 2 {
		t.Errorf("node_count_at_peak = %d, mau 2", row.NodeCountAtPeak)
	}
	// Ambang dan versi algoritma DIREKAM, tidak dihitung ulang oleh pembaca.
	if row.MinIndependentCells != defaultOptions().MinIndependentCells {
		t.Errorf("min_independent_cells = %d, mau %d",
			row.MinIndependentCells, defaultOptions().MinIndependentCells)
	}
	if row.AlgoVer != h.trk.algoVer() {
		t.Errorf("algo_ver = %q, mau %q", row.AlgoVer, h.trk.algoVer())
	}
	// NULL berarti BELUM PERNAH TERJADI, bukan nol. Sebuah event yang tidak pernah
	// CONFIRMED bukan event yang CONFIRMED pada epoch.
	if row.ConfirmedAt != nil {
		t.Errorf("confirmed_at = %d, mau NULL: event ini tidak pernah CONFIRMED", *row.ConfirmedAt)
	}
	if row.TerminalState != nil || row.TerminalAt != nil {
		t.Error("terminal_state/terminal_at bukan NULL padahal event masih terbuka")
	}
}

// TestNearConfirmedPeakRiseIsPersisted — puncak yang NAIK menghasilkan catatan
// durable berikutnya, dan puncak tidak pernah turun kembali.
//
// Kenaikan puncak pada event yang sudah CONFIRMED juga sunyi: CONFIRMED ->
// CONFIRMED bukan transisi, jadi node keempat mengubah angka forensiknya tanpa
// mengubah satu pun state.
func TestNearConfirmedPeakRiseIsPersisted(t *testing.T) {
	h := newHarness(t)
	p := h.withPersister()
	e := h.confirmThreeNodes()

	first := p.lastNearFor(t, e.ID)
	if first.IndependentCountAtPeak != 3 {
		t.Fatalf("puncak awal = %d, mau 3", first.IndependentCountAtPeak)
	}
	revBefore, unitsBefore := e.Revision, len(p.units)

	// Node keempat di sel independensi keempat: 24 km dari acuan, terpisah >= 5 km
	// dari ketiga node lain.
	h.nodeAt("N4", baseLat, baseLon, 24, 90)
	h.clock.advance(1 * time.Second)
	h.ingest(v2("N4", MinPGAGal+10, onsetBase+3000, PhasePrelim, 1))

	if e.State != StateConfirmed {
		t.Fatalf("state = %s, mau tetap CONFIRMED", e.State)
	}
	if e.Revision != revBefore {
		t.Errorf("revision = %d, mau tetap %d: CONFIRMED -> CONFIRMED bukan transisi",
			e.Revision, revBefore)
	}
	if len(p.units) != unitsBefore {
		t.Errorf("satuan event = %d, mau tetap %d", len(p.units), unitsBefore)
	}

	rows := p.nearFor(e.ID)
	if len(rows) < 2 {
		t.Fatalf("catatan durable = %d, mau >= 2: kenaikan puncak harus diantre", len(rows))
	}
	last := rows[len(rows)-1]
	if last.IndependentCountAtPeak != 4 {
		t.Errorf("independent_count_at_peak = %d, mau 4", last.IndependentCountAtPeak)
	}
	if last.NodeCountAtPeak != 4 {
		t.Errorf("node_count_at_peak = %d, mau 4", last.NodeCountAtPeak)
	}
	// first_two_independent_at TIDAK bergerak: "pertama kali" hanya punya satu nilai.
	if last.FirstTwoIndependentAt != first.FirstTwoIndependentAt {
		t.Errorf("first_two_independent_at = %d, mau tetap %d",
			last.FirstTwoIndependentAt, first.FirstTwoIndependentAt)
	}
	// Puncak monoton di setiap catatan berurutan. Diperiksa pada urutan yang
	// benar-benar diantre, karena merge ON CONFLICT di basis data hanya aman bila
	// nilai yang dikirim memang tidak pernah mundur.
	for i := 1; i < len(rows); i++ {
		if rows[i].IndependentCountAtPeak < rows[i-1].IndependentCountAtPeak {
			t.Errorf("puncak turun pada catatan %d: %d < %d",
				i, rows[i].IndependentCountAtPeak, rows[i-1].IndependentCountAtPeak)
		}
	}
}

// TestNearConfirmedTerminalIsPersistedAndSurvivesEviction — event yang sudah
// ditutup DAN tombstone-nya dievakuasi tetap dapat dijawab, di memori maupun
// lewat baris durable-nya.
//
// Inilah pertanyaan yang paling sering diajukan pasca-kejadian, dan ia justru
// menyangkut event yang sudah tidak ada lagi di peta Tracker. dropLocked
// mengeluarkan event dari t.events tanpa menyentuh t.nearConfirmed, dan uji ini
// yang membuat sifat itu tidak dapat hilang secara diam-diam.
func TestNearConfirmedTerminalIsPersistedAndSurvivesEviction(t *testing.T) {
	h := newHarness(t)
	p := h.withPersister()
	e := h.crossSilently()
	eventID := e.ID

	// Sweep pertama: bukti berhenti datang, event ditutup.
	h.clock.advance(time.Duration(defaultOptions().ResolveAfterMs+1) * time.Millisecond)
	h.trk.sweep(context.Background())
	if e.State != StateResolved {
		t.Fatalf("state = %s, mau RESOLVED", e.State)
	}
	closedAt := h.clock.now().UnixMilli()

	// Sweep kedua: retensi tombstone terlampaui, event menghilang dari peta.
	h.clock.advance(time.Duration(defaultOptions().TerminalRetentionMs+1) * time.Millisecond)
	h.trk.sweep(context.Background())
	if n := len(h.events()); n != 0 {
		t.Fatalf("event terlacak = %d, mau 0 setelah tombstone dievakuasi", n)
	}

	// Jawaban di memori tetap ada.
	rep := h.trk.NearConfirmedReport()
	if len(rep.Entries) != 1 {
		t.Fatalf("entri = %d, mau 1 setelah evakuasi", len(rep.Entries))
	}
	if got := rep.Entries[0].TerminalState; got != string(StateResolved) {
		t.Errorf("terminal_state = %q, mau RESOLVED", got)
	}

	// Dan baris durable-nya membawa outcome yang sama, sehingga proses BERIKUTNYA
	// dapat menjawabnya juga.
	row := p.lastNearFor(t, eventID)
	if row.TerminalState == nil || *row.TerminalState != string(StateResolved) {
		t.Fatalf("terminal_state durable = %v, mau RESOLVED", row.TerminalState)
	}
	if row.TerminalAt == nil || *row.TerminalAt != closedAt {
		t.Errorf("terminal_at durable = %v, mau %d", row.TerminalAt, closedAt)
	}
	if row.ConfirmedAt != nil {
		t.Errorf("confirmed_at = %d, mau NULL: event ini mati tanpa konfirmasi", *row.ConfirmedAt)
	}
}

// TestNearConfirmedSurvivesTrackerRestart — persilangan yang dicatat satu proses
// dapat dijawab oleh proses BERIKUTNYA, dengan provenance yang mengaku dimuat.
//
// Ini kriteria M2′ itu sendiri. Yang dimuat ulang adalah angka yang benar-benar
// DIREKAM saat persilangan, beserta ambang dan algo_ver-nya sendiri — bukan hasil
// hitung ulang dari koordinat node sekarang. Karena itu uji ini sengaja memberi
// proses kedua Options dengan IndependenceCellKm yang BERBEDA: kalau ada satu saja
// jalur yang menghitung ulang, algo_ver entri yang dimuat akan berubah mengikuti
// biner baru, dan keputusan lampau akan dinilai dengan parameter yang tidak
// menghasilkannya (U-007, yang belum diputuskan dan tidak boleh dijawab oleh
// implementasi).
func TestNearConfirmedSurvivesTrackerRestart(t *testing.T) {
	h := newHarness(t)
	p := h.withPersister()
	before := h.crossSilently()
	wantID := before.ID

	row := p.lastNearFor(t, wantID)
	wantAlgoVer := row.AlgoVer

	// Proses kedua: Tracker baru, ambang pemisahan berbeda, basis data yang selamat.
	h2 := newHarness(t, func(o *Options) { o.IndependenceCellKm = 9 })
	h2.loc = h.loc
	fne := h2.restartWithNear(nil, *row)

	if err := h2.trk.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if fne.reads != 1 {
		t.Errorf("ListNearConfirmed dipanggil %d kali, mau 1: tabelnya dibaca sekali saat boot", fne.reads)
	}

	rep := h2.trk.NearConfirmedReport()
	if len(rep.Entries) != 1 {
		t.Fatalf("entri = %d setelah restart, mau 1: inilah kriteria M2′", len(rep.Entries))
	}
	got := rep.Entries[0]

	if got.EventID != wantID {
		t.Errorf("event_id = %q, mau %q", got.EventID, wantID)
	}
	if got.FirstTwoIndependentAt != row.FirstTwoIndependentAt {
		t.Errorf("first_two_independent_at = %d, mau %d",
			got.FirstTwoIndependentAt, row.FirstTwoIndependentAt)
	}
	if got.IndependentCountAtPeak != row.IndependentCountAtPeak {
		t.Errorf("independent_count_at_peak = %d, mau %d",
			got.IndependentCountAtPeak, row.IndependentCountAtPeak)
	}
	if got.NodeCountAtPeak != row.NodeCountAtPeak {
		t.Errorf("node_count_at_peak = %d, mau %d", got.NodeCountAtPeak, row.NodeCountAtPeak)
	}
	// Parameter yang DIREKAM, bukan yang berlaku sekarang. Proses ini berjalan pada
	// ic=9; entri yang dimuat harus tetap berkata ic=5.
	if got.AlgoVer != wantAlgoVer {
		t.Errorf("algo_ver = %q, mau %q: baris lampau TIDAK ditulis ulang oleh biner baru",
			got.AlgoVer, wantAlgoVer)
	}
	if got.AlgoVer == h2.trk.algoVer() {
		t.Errorf("algo_ver entri = algo_ver proses (%q): entri yang dimuat dihitung ulang", got.AlgoVer)
	}
	if got.MinIndependentCells != row.MinIndependentCells {
		t.Errorf("min_independent_cells = %d, mau %d",
			got.MinIndependentCells, row.MinIndependentCells)
	}
	// Provenance: proses ini TIDAK menyaksikan persilangannya, dan jawabannya
	// mengatakannya.
	if got.Source != NearConfirmedSourceDurable {
		t.Errorf("source = %q, mau %q", got.Source, NearConfirmedSourceDurable)
	}
	if got.UpdatedInProcess {
		t.Error("updated_in_process = true padahal entri belum berubah di proses ini")
	}

	cov := rep.Coverage
	if !cov.DurableReadAttempted || !cov.DurableReadOK {
		t.Errorf("durable_read attempted/ok = %v/%v, mau true/true",
			cov.DurableReadAttempted, cov.DurableReadOK)
	}
	if cov.DurableRowsLoaded != 1 || cov.EntriesLoadedFromDurable != 1 {
		t.Errorf("baris dimuat = %d, entri LOADED = %d, mau 1/1",
			cov.DurableRowsLoaded, cov.EntriesLoadedFromDurable)
	}
	if cov.EntriesRecordedInProcess != 0 {
		t.Errorf("entries_recorded_in_process = %d, mau 0: proses ini belum mencatat apa pun",
			cov.EntriesRecordedInProcess)
	}
	// Selubungnya membawa parameter proses BARU, dan itu memang gunanya: pembaca
	// dapat melihat bahwa entri dan proses tidak lagi memakai ambang yang sama tanpa
	// membandingkan entri satu per satu.
	if cov.AlgoVer != h2.trk.algoVer() {
		t.Errorf("coverage.algo_ver = %q, mau %q", cov.AlgoVer, h2.trk.algoVer())
	}
}

// TestNearConfirmedLoadedEntryUpdatedInProcessKeepsProvenance — entri yang DIMUAT
// lalu berubah di proses ini menandai dirinya, tanpa mengklaim kesaksian yang
// tidak dimilikinya.
//
// Skenarionya justru event yang paling menarik: sebuah event yang masih terbuka
// saat proses lama mati, lalu ditutup oleh proses baru pada rekonsiliasi. Awal
// entri tetap berasal dari basis data — proses ini tidak menyaksikan
// persilangannya — sementara state terminalnya memang disaksikan di sini. Dua
// fakta berbeda, dan karena itu dua field.
func TestNearConfirmedLoadedEntryUpdatedInProcessKeepsProvenance(t *testing.T) {
	h := newHarness(t)
	p := h.withPersister()
	before := h.crossSilently()
	wantID := before.ID
	row := p.lastNearFor(t, wantID)
	lastUnit := p.units[len(p.units)-1]

	// Proses kedua: baris event masih HAPPENING, baris near-confirmed ikut selamat.
	h2 := newHarness(t)
	h2.loc = h.loc
	p2 := h2.withPersister()
	fne := h2.restartWithNear(
		[]*store.EarthquakeEvent{rowFrom(t, lastUnit, h2.clock.now().UnixMilli())},
		*row,
	)

	// Bukti berhenti datang selama proses mati: rekonsiliasi menutup event-nya.
	h2.clock.advance(time.Duration(defaultOptions().ResolveAfterMs+1) * time.Millisecond)
	if err := h2.trk.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if fne.loads != 1 || fne.reads != 1 {
		t.Fatalf("LoadOpenEvents/ListNearConfirmed = %d/%d, mau 1/1", fne.loads, fne.reads)
	}

	rep := h2.trk.NearConfirmedReport()
	if len(rep.Entries) != 1 {
		t.Fatalf("entri = %d, mau 1", len(rep.Entries))
	}
	got := rep.Entries[0]

	if got.Source != NearConfirmedSourceDurable {
		t.Errorf("source = %q, mau %q: awalnya tetap dari basis data",
			got.Source, NearConfirmedSourceDurable)
	}
	if !got.UpdatedInProcess {
		t.Error("updated_in_process = false padahal entri berubah di proses ini")
	}
	if got.TerminalState != string(StateResolved) {
		t.Errorf("terminal_state = %q, mau RESOLVED", got.TerminalState)
	}
	if got.FirstTwoIndependentAt != row.FirstTwoIndependentAt {
		t.Errorf("first_two_independent_at = %d, mau tetap %d dari baris durable",
			got.FirstTwoIndependentAt, row.FirstTwoIndependentAt)
	}
	// Provenance dihitung menurut Source, bukan menurut siapa yang terakhir
	// menyentuhnya: satu entri LOADED, nol yang disaksikan di sini.
	if rep.Coverage.EntriesLoadedFromDurable != 1 || rep.Coverage.EntriesRecordedInProcess != 0 {
		t.Errorf("provenance = %d LOADED / %d RECORDED, mau 1/0",
			rep.Coverage.EntriesLoadedFromDurable, rep.Coverage.EntriesRecordedInProcess)
	}

	// Dan perubahannya diantre kembali: kalau tidak, penutupan event ini akan hilang
	// pada restart BERIKUTNYA.
	rewritten := p2.lastNearFor(t, wantID)
	if rewritten.TerminalState == nil || *rewritten.TerminalState != string(StateResolved) {
		t.Fatalf("terminal_state yang diantre = %v, mau RESOLVED", rewritten.TerminalState)
	}
}

// TestNearConfirmedDurableLoadNeverOverwritesMemory — memori adalah otoritasnya
// (§9.5), dan baris durable adalah pengikut.
//
// Bila keduanya berbicara tentang event yang sama, yang di memori menang: ia
// dibangun dari bukti yang benar-benar dilihat proses ini, sementara barisnya
// mungkin tertinggal satu pembaruan karena antreannya boleh membuang.
func TestNearConfirmedDurableLoadNeverOverwritesMemory(t *testing.T) {
	h := newHarness(t)
	e := h.crossSilently()

	live := h.trk.NearConfirmedReport().Entries[0]

	// Baris yang MEMBANTAH memori: puncak lebih rendah, waktu lebih awal, ambang
	// lain, dan mengaku sudah terminal.
	stale := string(StateCancelled)
	staleAt := live.FirstTwoIndependentAt - 60_000
	h.trk.LoadNearConfirmed(context.Background(), &fakeNearEvents{near: []store.NearConfirmedRow{{
		EventID:                e.ID,
		FirstTwoIndependentAt:  staleAt,
		IndependentCountAtPeak: 1,
		NodeCountAtPeak:        1,
		MinIndependentCells:    7,
		TerminalState:          &stale,
		TerminalAt:             &staleAt,
		AlgoVer:                "phase3-1.0/ic=50",
	}}})

	rep := h.trk.NearConfirmedReport()
	if len(rep.Entries) != 1 {
		t.Fatalf("entri = %d, mau 1: baris durable tidak boleh menambah entri kedua", len(rep.Entries))
	}
	if rep.Entries[0] != live {
		t.Errorf("entri = %+v, mau tetap %+v: memori adalah otoritasnya", rep.Entries[0], live)
	}
	// Barisnya DIBACA — pembacaannya berhasil — tetapi tidak DITANAM. Kedua angka itu
	// karena itu tidak boleh sama.
	if !rep.Coverage.DurableReadOK {
		t.Error("durable_read_ok = false padahal pembacaannya berhasil")
	}
	if rep.Coverage.DurableRowsLoaded != 0 {
		t.Errorf("durable_rows_loaded = %d, mau 0: barisnya dilewati, bukan ditanam",
			rep.Coverage.DurableRowsLoaded)
	}
	if rep.Coverage.EntriesRecordedInProcess != 1 {
		t.Errorf("entries_recorded_in_process = %d, mau 1", rep.Coverage.EntriesRecordedInProcess)
	}
}

// TestNearConfirmedPersistenceFailureNeverBlocksEmission — antrean yang membuang
// dan tulisan yang gagal tidak menahan, tidak mengubah, dan tidak membatalkan satu
// pun keputusan (S1).
//
// Ini kontrak Fase 1 yang sudah tertulis hitam di atas putih: pencatatan boleh
// gagal, jalur peringatan tidak. Kedua mode kegagalan diuji berpasangan supaya
// akuntansinya juga terbukti terpisah — sebuah catatan yang tidak pernah masuk
// antrean dan sebuah catatan yang masuk lalu gagal ditulis bukan kerugian yang
// sama.
//
// Yang TIDAK diuji di sini, dan sengaja: bahwa angka-angka ini nol. Tidak ada
// target nol untuk keduanya (D-011 batasan 1) — sebuah SLO nol-buangan pada
// antrean yang sengaja boleh membuang hanya dapat ditepati dengan memblokir jalur
// peringatan, yaitu tepat hal yang dilarang.
func TestNearConfirmedPersistenceFailureNeverBlocksEmission(t *testing.T) {
	t.Run("antrean penuh: dibuang, dihitung, emisi utuh", func(t *testing.T) {
		h := newHarness(t)
		p := h.withPersister(func(p *recPersister) { p.dropNear = true })

		e := h.confirmThreeNodes()

		if e.State != StateConfirmed {
			t.Errorf("state = %s, mau CONFIRMED: pembuangan tidak boleh mengubah keputusan", e.State)
		}
		if got := h.emit.countFor(StateConfirmed); got != 1 {
			t.Errorf("frame CONFIRMED = %d, mau 1: peringatannya tetap keluar", got)
		}
		if len(p.units) == 0 {
			t.Error("satuan event = 0: jalur persistensi event tidak boleh terpengaruh")
		}
		if len(p.nearRows) != 0 {
			t.Errorf("catatan tersimpan = %d, mau 0: seluruhnya dibuang", len(p.nearRows))
		}
		if got := h.trk.NearConfirmedDropped(); got == 0 {
			t.Error("event_near_confirmed_persist_dropped_total = 0: pembuangan harus terhitung")
		}
		if got := h.trk.NearConfirmedUpsertFailures(); got != 0 {
			t.Errorf("upsert_failures = %d, mau 0: barisnya tidak pernah dicoba", got)
		}
		// Akuntansi event unit TIDAK ikut bergerak.
		if got := h.trk.PersistDropped(); got != 0 {
			t.Errorf("event_persist_dropped_total = %d, mau 0: dua kerugian berbeda, dua counter", got)
		}
		// Dan jawaban di memori tetap utuh: yang hilang adalah durabilitasnya, bukan
		// catatannya.
		if n := len(h.trk.NearConfirmedReport().Entries); n != 1 {
			t.Errorf("entri di memori = %d, mau 1", n)
		}
	})

	t.Run("tulisan gagal: dicoba, dihitung terpisah, emisi utuh", func(t *testing.T) {
		h := newHarness(t)
		p := h.withPersister(func(p *recPersister) { p.failNear = true })

		e := h.confirmThreeNodes()

		if e.State != StateConfirmed {
			t.Errorf("state = %s, mau CONFIRMED", e.State)
		}
		if got := h.emit.countFor(StateConfirmed); got != 1 {
			t.Errorf("frame CONFIRMED = %d, mau 1", got)
		}
		if p.nearAttempts == 0 {
			t.Error("percobaan tulis = 0: barisnya masuk antrean dan DICOBA")
		}
		if len(p.nearRows) != 0 {
			t.Errorf("catatan tersimpan = %d, mau 0: semuanya gagal", len(p.nearRows))
		}
		if got := h.trk.NearConfirmedUpsertFailures(); got == 0 {
			t.Error("event_near_confirmed_upsert_failures_total = 0: kegagalan harus terhitung")
		}
		if got := h.trk.NearConfirmedDropped(); got != 0 {
			t.Errorf("dropped = %d, mau 0: barisnya tidak dibuang, ia gagal ditulis", got)
		}
	})

	t.Run("tanpa antrean sama sekali: tetap melacak", func(t *testing.T) {
		// Tracker tanpa SetLedger. nearPersist nil adalah keadaan yang SAH — jalur
		// EVENT_TRACKER_ENABLED tanpa ledger — dan ia tidak boleh panik maupun
		// mengubah satu pun keputusan.
		h := newHarness(t)
		e := h.crossSilently()

		if e.State != StateUnconfirmed {
			t.Errorf("state = %s, mau UNCONFIRMED", e.State)
		}
		if n := len(h.trk.NearConfirmedReport().Entries); n != 1 {
			t.Errorf("entri = %d, mau 1: pelacakan tidak bergantung pada durabilitas", n)
		}
	})
}

// TestNearConfirmedFlushLeavesNothingPending — setiap perubahan yang dikumpulkan di
// bawah kunci benar-benar keluar, termasuk pada jalur TANPA transisi.
//
// Jalur len(ts) == 0 di publish adalah satu-satunya yang membawa persilangan
// sunyi. Kalau ia kembali lebih awal, entri yang berubah akan tertahan di
// nearPending sampai transisi BERIKUTNYA — dan pada event yang tidak pernah
// bertransisi lagi, sampai selamanya.
func TestNearConfirmedFlushLeavesNothingPending(t *testing.T) {
	h := newHarness(t)
	p := h.withPersister()
	e := h.crossSilently()

	h.trk.mu.Lock()
	pending := len(h.trk.nearPending)
	h.trk.mu.Unlock()

	if pending != 0 {
		t.Errorf("nearPending = %d setelah persilangan sunyi, mau 0: jalur ts kosong "+
			"harus tetap membilas", pending)
	}
	if got := len(p.nearFor(e.ID)); got == 0 {
		t.Error("catatan durable = 0: perubahannya tertahan di bawah kunci")
	}
}

// TestNewTrackerStampsProcessStartFromRealClock — startedAtMs diisi oleh
// NewTracker, dan harness sengaja menimpanya dengan jam palsu. Sifat itu karena
// itu diuji di sini, terhadap jam nyata, alih-alih tidak diuji di mana pun.
func TestNewTrackerStampsProcessStartFromRealClock(t *testing.T) {
	before := time.Now().UnixMilli()
	trk := NewTracker(&fakeLoc{}, defaultOptions(),
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	after := time.Now().UnixMilli()

	if trk.startedAtMs < before || trk.startedAtMs > after {
		t.Errorf("startedAtMs = %d, mau di dalam [%d, %d]", trk.startedAtMs, before, after)
	}
	if got := trk.NearConfirmedReport().Coverage.ProcessStartedAtMs; got != trk.startedAtMs {
		t.Errorf("coverage.process_started_at_ms = %d, mau %d", got, trk.startedAtMs)
	}
}
