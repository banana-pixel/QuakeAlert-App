package event

import (
	"testing"
)

// Dimigrasikan dari consensus/phase_test.go dan DIPERKUAT: fase sekarang terlihat
// oleh kode yang diuji, jadi propertinya ditegaskan langsung pada pengunci
// kontributor, bukan tersirat dari struktur.
func TestPrelimAndFinalCountAsOneNode(t *testing.T) {
	h := newHarness(t)
	h.node("N1", baseLat, baseLon)
	h.nodeAt("N2", baseLat, baseLon, 8, 90)

	h.ingest(v2("N1", MinPGAGal+10, onsetBase, PhasePrelim, 1))
	h.ingest(v2("N1", MinPGAGal+40, onsetBase, PhaseFinal, 1))
	h.ingest(v2("N2", MinPGAGal+10, onsetBase+1000, PhasePrelim, 1))

	e := h.only()
	if len(e.Contributors) != 2 {
		t.Fatalf("kontributor = %d, mau 2: PRELIM dan FINAL satu node adalah SATU suara", len(e.Contributors))
	}
	if e.Contributors["N1"].Revisions != 2 {
		t.Errorf("N1 revisions = %d, mau 2", e.Contributors["N1"].Revisions)
	}
	if e.State == StateConfirmed {
		t.Error("dua node tidak boleh CONFIRMED: kuorum tetap 3")
	}
}

func TestPhaseEscalationKeepsPeakPGA(t *testing.T) {
	h := newHarness(t)
	h.node("N1", baseLat, baseLon)

	h.ingest(v2("N1", 120, onsetBase, PhasePrelim, 1))
	h.ingest(v2("N1", 40, onsetBase, PhaseFinal, 1))

	c := h.only().Contributors["N1"]
	if c.PeakPGA != 120 {
		t.Errorf("peak_pga = %.1f, mau 120: puncak tidak pernah menyusut", c.PeakPGA)
	}
	if c.Phase != PhaseFinal {
		t.Errorf("phase = %q, mau FINAL", c.Phase)
	}
	if c.DetriggerTS == nil {
		t.Error("detrigger_ts harus tercatat pada FINAL")
	}
}

// §6.5 aturan 3: PRELIM yang di-retry dapat datang SETELAH FINAL. Fase tetap
// FINAL, dan puncak tetap mengambil maksimum.
func TestPhaseRegressionNeverLowersPeak(t *testing.T) {
	h := newHarness(t)
	h.node("N1", baseLat, baseLon)

	h.ingest(v2("N1", 90, onsetBase, PhaseFinal, 1))
	h.ingest(v2("N1", 30, onsetBase, PhasePrelim, 1))

	c := h.only().Contributors["N1"]
	if c.PeakPGA != 90 {
		t.Errorf("peak_pga = %.1f, mau 90", c.PeakPGA)
	}
	if c.Phase != PhaseFinal {
		t.Errorf("phase = %q, mau tetap FINAL setelah regresi", c.Phase)
	}
	// PRELIM yang datang belakangan membawa PGA lebih besar tetap harus menaikkan
	// puncak: yang dilarang hanya penurunan.
	h.ingest(v2("N1", 150, onsetBase, PhasePrelim, 1))
	if c.PeakPGA != 150 {
		t.Errorf("peak_pga = %.1f, mau 150", c.PeakPGA)
	}
}

// Regresi langsung untuk C1: pencocokan terhadap CENTROID, bukan terhadap anggota
// mana pun, jadi 0/49/98/147 km tidak dapat berantai menjadi satu event.
func TestChainingAtFortyNineKmStepsDoesNotFormOneEvent(t *testing.T) {
	h := newHarness(t)
	for i, km := range []float64{0, 49, 98, 147} {
		h.nodeAt(nodeName(i), baseLat, baseLon, km, 90)
	}
	for i := range 4 {
		h.ingest(v2(nodeName(i), MinPGAGal+10, onsetBase+int64(i)*1000, PhasePrelim, 1))
	}

	if n := len(h.events()); n < 2 {
		t.Fatalf("event = %d, mau > 1: single linkage sepanjang rantai 49 km adalah C1", n)
	}
	// Dan tidak ada satu event pun yang memuat kedua ujung rantai.
	for _, e := range h.events() {
		_, hasFirst := e.Contributors[nodeName(0)]
		_, hasLast := e.Contributors[nodeName(3)]
		if hasFirst && hasLast {
			t.Errorf("event %s memuat node 0 km dan 147 km sekaligus", e.ID[:1])
		}
	}
}

func nodeName(i int) string { return string(rune('P' + i)) }

// Tutup diameter §6.4: penempelan yang akan mendorong jarak kontributor-ke-centroid
// maksimum melewati MaxEventDiameterKm/2 memulai event baru dan menaikkan counter.
func TestDiameterCapStartsNewEventAndCounts(t *testing.T) {
	// Tutup diameter dibuat LEBIH KETAT dari radius menempel, supaya yang menolak
	// benar-benar tutup itu dan bukan predikat §4.3. Radius menempel dibiarkan pada
	// nilai bawaan: menaikkannya melewati jangkauan 3x3 sel lookup akan melanggar
	// pertidaksamaan §6.3.1 dan membuat kandidatnya tak pernah ditemukan.
	h := newHarness(t, func(o *Options) {
		o.MaxEventDiameterKm = 40 // tutup = 20 km dari centroid
	})
	h.node("A", baseLat, baseLon)
	h.nodeAt("B", baseLat, baseLon, 45, 90)

	h.ingest(v2("A", MinPGAGal+10, onsetBase, PhasePrelim, 1))
	h.ingest(v2("B", MinPGAGal+10, onsetBase+1000, PhasePrelim, 1))

	if n := len(h.events()); n != 2 {
		t.Fatalf("event = %d, mau 2: 45 km melewati tutup diameter 40 km", n)
	}
	if got := h.trk.DiameterRejections(); got != 1 {
		t.Errorf("event_diameter_rejections_total = %d, mau 1", got)
	}
}

// Regresi S8: dua gempa di stasiun yang sama, onset 30 s berjarak (di luar jendela
// 20 s) menghasilkan DUA event dan DUA CONFIRMED. Cooldown 90 s per sel yang lama
// menekan yang kedua sepenuhnya; §6.7 membuangnya, dan uji ini yang membuktikan
// pembuangan itu bekerja.
func TestTwoNearbyEarthquakesThirtySecondsApartBothConfirm(t *testing.T) {
	h := newHarness(t)
	h.threeNodeCluster()

	for i, n := range []string{"N1", "N2", "N3"} {
		h.ingest(v2(n, MinPGAGal+10, onsetBase+int64(i)*1000, PhasePrelim, 1))
	}
	// Gempa kedua: 30 s kemudian, episode baru di setiap node.
	for i, n := range []string{"N1", "N2", "N3"} {
		h.ingest(v2(n, MinPGAGal+10, onsetBase+30000+int64(i)*1000, PhasePrelim, 2))
	}

	if n := len(h.events()); n != 2 {
		t.Fatalf("event = %d, mau 2 (%s)", n, describe(h.events()))
	}
	if got := h.emit.countFor(StateConfirmed); got != 2 {
		t.Errorf("frame CONFIRMED = %d, mau 2 — tidak ada cooldown yang menekan gempa kedua", got)
	}
	if got := len(h.emit.eventIDs()); got != 2 {
		t.Errorf("event_id berbeda pada frame = %d, mau 2", got)
	}
	// Pemisahnya di sini adalah WAKTU (kasus terpisah A §6.6), bukan aturan
	// re-onset: onset kedua 30 s dari origin sudah gagal predikat, jadi ia tidak
	// pernah menjadi kandidat. Aturan 4 diuji sendiri di bawah.
	if got := h.trk.ReonsetSplits(); got != 0 {
		t.Errorf("event_reonset_splits_total = %d, mau 0: pemisahnya jendela, bukan aturan 4", got)
	}
}

// §6.5 aturan 4 — episode kedua di node yang SUDAH menyumbang, dalam jendela
// event tetapi di luar jendela onset kontributornya sendiri. Inilah satu-satunya
// pemisah yang tersedia ketika dua gempa tak terpisahkan secara ruang maupun oleh
// jendela event, dan ia bergantung pada obs_seq — jadi hanya untuk node v2 (§14.2).
func TestSecondEpisodeAtOneNodeSplitsEventAndCounts(t *testing.T) {
	h := newHarness(t)
	h.node("N1", baseLat, baseLon)
	h.nodeAt("N2", baseLat, baseLon, 8, 90)

	// N2 menjangkar event pada origin = onsetBase.
	h.ingest(v2("N2", MinPGAGal+10, onsetBase, PhasePrelim, 1))
	// N1 menempel dengan onset 19 s LEBIH AWAL: masih di dalam jendela origin.
	h.ingest(v2("N1", MinPGAGal+10, onsetBase-19000, PhasePrelim, 1))
	if n := len(h.events()); n != 1 {
		t.Fatalf("persiapan: event = %d, mau 1", n)
	}

	// Episode kedua N1 pada onsetBase+2000: 2 s dari origin (cocok), tetapi 21 s
	// dari onset kontributornya sendiri — guncangan kedua yang berbeda.
	h.ingest(v2("N1", MinPGAGal+10, onsetBase+2000, PhasePrelim, 2))

	if n := len(h.events()); n != 2 {
		t.Fatalf("event = %d, mau 2 (%s)", n, describe(h.events()))
	}
	if got := h.trk.ReonsetSplits(); got != 1 {
		t.Errorf("event_reonset_splits_total = %d, mau 1", got)
	}
}

// KETERBATASAN YANG DITERIMA, bukan bug yang menunggu diperbaiki (§6.6): dua gempa
// yang onsetnya 5 s berjarak dengan jejak yang bertumpang tindih MENYATU menjadi
// satu event. Memisahkannya menuntut inversi waktu tiba dengan phase pick, yang
// perangkat keras ini tidak dapat lakukan (S3/S4). Uji ini ada supaya keterbatasan
// itu bertahan melewati refactor berikutnya.
func TestMergedPairWithinWindowIsAcceptedLimitation(t *testing.T) {
	h := newHarness(t)
	h.threeNodeCluster()

	for i, n := range []string{"N1", "N2", "N3"} {
		h.ingest(v2(n, MinPGAGal+10, onsetBase+int64(i)*500, PhasePrelim, 1))
	}
	// Gempa kedua, 5 s kemudian, episode baru — di dalam jendela, jadi ia MENEMPEL.
	for i, n := range []string{"N1", "N2", "N3"} {
		h.ingest(v2(n, MinPGAGal+60, onsetBase+5000+int64(i)*500, PhasePrelim, 2))
	}

	if n := len(h.events()); n != 1 {
		t.Fatalf("event = %d, mau 1: penyatuan ini adalah perilaku yang diterima", n)
	}
	e := h.only()
	if len(e.Contributors) != 3 {
		t.Errorf("kontributor = %d, mau 3: keduanya satu himpunan node", len(e.Contributors))
	}
	// Yang tersisa sebagai kompensasi: onset per kontributor ada di evidence_summary,
	// jadi pasangan yang menyatu setidaknya dapat didiagnosis dari ledger.
	ev := e.evidence()
	if len(ev.Contributors) != 3 {
		t.Fatalf("evidence contributors = %d, mau 3", len(ev.Contributors))
	}
	for _, c := range ev.Contributors {
		if c.OnsetTS == 0 {
			t.Error("evidence_summary harus membawa onset tiap kontributor untuk diagnosis")
		}
	}
}

// Idempotensi: setiap observasi yang dikirim dua kali harus meninggalkan jumlah dan
// state tak berubah.
func TestEveryObservationDeliveredTwiceIsIdempotent(t *testing.T) {
	obs := func() []Input {
		return []Input{
			v2("N1", MinPGAGal+10, onsetBase, PhasePrelim, 1),
			v2("N1", MinPGAGal+30, onsetBase, PhaseFinal, 1),
			v2("N2", MinPGAGal+12, onsetBase+1000, PhasePrelim, 1),
			v2("N3", MinPGAGal+9, onsetBase+2000, PhasePrelim, 1),
			v1("N4", MinPGAGal+7, onsetBase+8000, 3000),
		}
	}

	single := newHarness(t)
	single.threeNodeCluster()
	single.nodeAt("N4", baseLat, baseLon, 20, 90)
	for _, o := range obs() {
		single.ingest(o)
	}

	doubled := newHarness(t)
	doubled.threeNodeCluster()
	doubled.nodeAt("N4", baseLat, baseLon, 20, 90)
	for _, o := range obs() {
		doubled.ingest(o)
		doubled.ingest(o)
	}

	a, b := single.only(), doubled.only()
	if a.State != b.State {
		t.Errorf("state = %s vs %s", a.State, b.State)
	}
	if a.Revision != b.Revision {
		t.Errorf("revision = %d vs %d", a.Revision, b.Revision)
	}
	if len(a.Contributors) != len(b.Contributors) {
		t.Errorf("kontributor = %d vs %d", len(a.Contributors), len(b.Contributors))
	}
	if a.peakPGA() != b.peakPGA() {
		t.Errorf("peak_pga = %.1f vs %.1f", a.peakPGA(), b.peakPGA())
	}
	if len(single.emit.frames) != len(doubled.emit.frames) {
		t.Errorf("frame = %v vs %v", single.emit.states(), doubled.emit.states())
	}
}

// Kedatangan terbalik harus menghasilkan state akhir dan himpunan kontributor yang
// sama. Yang boleh berbeda hanyalah origin_ts, karena ia adalah onset PEMBUAT.
func TestOutOfOrderReplayReachesSameFinalState(t *testing.T) {
	obs := []Input{
		v2("N1", MinPGAGal+10, onsetBase, PhasePrelim, 1),
		v2("N1", MinPGAGal+30, onsetBase, PhaseFinal, 1),
		v2("N2", MinPGAGal+12, onsetBase+2000, PhasePrelim, 1),
		v2("N3", MinPGAGal+9, onsetBase+4000, PhaseFinal, 1),
	}

	forward := newHarness(t)
	forward.threeNodeCluster()
	for _, o := range obs {
		forward.ingest(o)
	}

	backward := newHarness(t)
	backward.threeNodeCluster()
	for i := len(obs) - 1; i >= 0; i-- {
		backward.ingest(obs[i])
	}

	a, b := forward.only(), backward.only()
	if a.State != b.State {
		t.Errorf("state akhir = %s vs %s", a.State, b.State)
	}
	if len(a.Contributors) != len(b.Contributors) {
		t.Fatalf("kontributor = %d vs %d", len(a.Contributors), len(b.Contributors))
	}
	for id, ca := range a.Contributors {
		cb, ok := b.Contributors[id]
		if !ok {
			t.Fatalf("%s hilang pada pemutaran terbalik", id)
		}
		if ca.PeakPGA != cb.PeakPGA {
			t.Errorf("%s peak_pga = %.1f vs %.1f", id, ca.PeakPGA, cb.PeakPGA)
		}
		if ca.Phase != cb.Phase {
			t.Errorf("%s phase = %q vs %q", id, ca.Phase, cb.Phase)
		}
	}
}

// D29/R-M2: kontributor v1 yang direvisi oleh retry berikutnya MEMPERTAHANKAN
// onset aslinya, sementara peak_pga tetap mengambil maksimum. origin_ts event
// tidak bergerak bersamanya.
func TestFirstBoundWinsForV1Contributor(t *testing.T) {
	h := newHarness(t)
	h.node("N1", baseLat, baseLon)

	first := v1("N1", MinPGAGal+5, onsetBase+3000, 3000) // onset = onsetBase
	h.ingest(first)
	origin := h.only().OriginTS

	// Retry: ts distempel ulang 9 s kemudian, jadi batasnya LEBIH LONGGAR.
	retry := v1("N1", MinPGAGal+80, onsetBase+12000, 3000) // onset = onsetBase+9000
	h.ingest(retry)

	e := h.only()
	c := e.Contributors["N1"]
	if c.OnsetTS != onsetBase {
		t.Errorf("onset kontributor = %d, mau tetap %d: batas pertama adalah yang terketat",
			c.OnsetTS, onsetBase)
	}
	if c.PeakPGA != MinPGAGal+80 {
		t.Errorf("peak_pga = %.1f, mau naik ke %.1f", c.PeakPGA, MinPGAGal+80)
	}
	if e.OriginTS != origin {
		t.Errorf("origin_ts = %d, mau tetap %d", e.OriginTS, origin)
	}
	if len(h.events()) != 1 {
		t.Errorf("event = %d, mau 1: retry v1 bukan episode kedua", len(h.events()))
	}
}
