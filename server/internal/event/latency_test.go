package event

import (
	"context"
	"testing"
	"time"
)

// ---- latencySeries: cincin dan persentil ----------------------------------

// TestLatencySeriesEmptySnapshot — seri kosong melaporkan nol untuk KETIGA angka.
// Yang penting di sini adalah Observed == 0 menyertainya: tanpa itu, p50=0 dapat
// dibaca sebagai "latensi nol" alih-alih "belum ada yang diukur".
func TestLatencySeriesEmptySnapshot(t *testing.T) {
	var s latencySeries
	got := s.snapshot()

	if got.Observed != 0 {
		t.Errorf("Observed = %d, mau 0 pada seri kosong", got.Observed)
	}
	if got.P50Ms != 0 || got.P95Ms != 0 {
		t.Errorf("p50/p95 = %d/%d, mau 0/0 pada seri kosong", got.P50Ms, got.P95Ms)
	}
}

// TestLatencySeriesSingleSample — satu sampel: p50 dan p95 keduanya sampel itu.
func TestLatencySeriesSingleSample(t *testing.T) {
	var s latencySeries
	s.observe(42)

	got := s.snapshot()
	if got.Observed != 1 {
		t.Errorf("Observed = %d, mau 1", got.Observed)
	}
	if got.P50Ms != 42 || got.P95Ms != 42 {
		t.Errorf("p50/p95 = %d/%d, mau 42/42", got.P50Ms, got.P95Ms)
	}
}

// TestLatencySeriesPercentilesNearestRank — persentil atas 1..100 ms.
//
// Nearest-rank, jadi nilainya harus sebuah pengamatan yang SUNGGUH terjadi:
// rank = ceil(p/100 * n) memberi p50 = elemen ke-50 = 50, p95 = elemen ke-95 = 95.
// Sebuah nilai seperti 50.5 akan berarti implementasinya beralih ke interpolasi.
func TestLatencySeriesPercentilesNearestRank(t *testing.T) {
	var s latencySeries
	for i := int64(1); i <= 100; i++ {
		s.observe(i)
	}

	got := s.snapshot()
	if got.Observed != 100 {
		t.Errorf("Observed = %d, mau 100", got.Observed)
	}
	if got.P50Ms != 50 {
		t.Errorf("p50 = %d, mau 50 (nearest-rank)", got.P50Ms)
	}
	if got.P95Ms != 95 {
		t.Errorf("p95 = %d, mau 95 (nearest-rank)", got.P95Ms)
	}
}

// TestLatencySeriesUnsortedInput — urutan kedatangan tidak boleh mempengaruhi
// persentil. Sampel masuk terbalik; jawabannya harus sama dengan yang naik.
func TestLatencySeriesUnsortedInput(t *testing.T) {
	var s latencySeries
	for i := int64(100); i >= 1; i-- {
		s.observe(i)
	}

	if got := s.snapshot().P50Ms; got != 50 {
		t.Errorf("p50 = %d, mau 50 terlepas dari urutan kedatangan", got)
	}
}

// TestLatencySeriesRingOverwritesOldest — melewati kapasitas cincin membuang
// sampel TERTUA, bukan yang terbaru, dan tidak menumbuhkan struktur.
//
// Ditulis latencySampleCap sampel bernilai 1, lalu latencySampleCap sampel
// bernilai 9: cincin kini seluruhnya 9, jadi p50 harus 9 dan bukan 1. Observed
// tetap menghitung keduanya — itu perbedaan yang harus dijaga.
func TestLatencySeriesRingOverwritesOldest(t *testing.T) {
	var s latencySeries
	for i := 0; i < latencySampleCap; i++ {
		s.observe(1)
	}
	for i := 0; i < latencySampleCap; i++ {
		s.observe(9)
	}

	got := s.snapshot()
	if got.Observed != int64(2*latencySampleCap) {
		t.Errorf("Observed = %d, mau %d — kumulatif, bukan panjang cincin",
			got.Observed, 2*latencySampleCap)
	}
	if got.P50Ms != 9 || got.P95Ms != 9 {
		t.Errorf("p50/p95 = %d/%d, mau 9/9 — sampel tertua harus tertimpa",
			got.P50Ms, got.P95Ms)
	}
	if got := s.len(); got != latencySampleCap {
		t.Errorf("len() = %d, mau terjepit di %d", got, latencySampleCap)
	}
}

// TestLatencySeriesRejectsNegative — sampel negatif DIBUANG, tidak dijepit ke nol.
//
// Latensi negatif berarti jam node mendahului jam server. Menjepitnya ke nol akan
// melaporkan angka terbaik yang mungkin untuk kondisi yang justru kerusakan; jadi
// yang benar adalah Observed tidak naik sama sekali.
func TestLatencySeriesRejectsNegative(t *testing.T) {
	var s latencySeries
	s.observe(-1)
	s.observe(-9999)

	got := s.snapshot()
	if got.Observed != 0 {
		t.Errorf("Observed = %d, mau 0 — sampel negatif harus dibuang", got.Observed)
	}

	// Dan sesudahnya seri tetap dapat dipakai secara normal.
	s.observe(7)
	if got := s.snapshot(); got.Observed != 1 || got.P50Ms != 7 {
		t.Errorf("setelah sampel negatif: Observed/p50 = %d/%d, mau 1/7",
			got.Observed, got.P50Ms)
	}
}

// TestLatencySeriesAcceptsZero — nol adalah pengukuran yang sah (tahap selesai di
// bawah resolusi satu milidetik) dan harus dihitung, tidak seperti nilai negatif.
func TestLatencySeriesAcceptsZero(t *testing.T) {
	var s latencySeries
	s.observe(0)

	if got := s.snapshot().Observed; got != 1 {
		t.Errorf("Observed = %d, mau 1 — nol adalah sampel yang sah", got)
	}
}

// ---- pemisahan provenance --------------------------------------------------

// TestLatencyProvenanceSeparation — SENSOR dan PUBLISH_BOUND masuk seri BERBEDA
// dan tidak saling mencemari.
//
// Ini inti P4-M3′: latensi yang dihitung dari onset publish-bound adalah batas
// atas, bukan pengukuran, jadi menggabungkannya ke dalam satu persentil bersama
// onset sensor akan menyebut sebuah batas sebagai pengukuran.
func TestLatencyProvenanceSeparation(t *testing.T) {
	var l latency
	l.observeOnsetToDecided(OnsetSourceSensor, 100)
	l.observeOnsetToDecided(OnsetSourcePublish, 5000)

	sensor := l.onsetToDecidedSensor.snapshot()
	publish := l.onsetToDecidedPublish.snapshot()

	if sensor.Observed != 1 || sensor.P50Ms != 100 {
		t.Errorf("seri SENSOR: Observed/p50 = %d/%d, mau 1/100", sensor.Observed, sensor.P50Ms)
	}
	if publish.Observed != 1 || publish.P50Ms != 5000 {
		t.Errorf("seri PUBLISH_BOUND: Observed/p50 = %d/%d, mau 1/5000",
			publish.Observed, publish.P50Ms)
	}
	if l.decidedToEmit.snapshot().Observed != 0 {
		t.Error("onset->decided tidak boleh menulis ke seri decided->emit")
	}
}

// TestLatencyUnknownProvenanceDropped — provenance yang tidak dikenali dibuang,
// tidak dimasukkan ke salah satu seri.
//
// Sebuah sumber onset baru yang belum dipahami tidak boleh diam-diam ikut
// dihitung sebagai pengukuran sensor; total kedua seri harus tetap nol.
func TestLatencyUnknownProvenanceDropped(t *testing.T) {
	var l latency
	l.observeOnsetToDecided("", 100)
	l.observeOnsetToDecided("GPS_PPS", 100)
	l.observeOnsetToDecided("sensor", 100) // huruf kecil — bukan konstanta yang sah

	if got := l.onsetToDecidedSensor.snapshot().Observed; got != 0 {
		t.Errorf("seri SENSOR Observed = %d, mau 0 untuk provenance tak dikenal", got)
	}
	if got := l.onsetToDecidedPublish.snapshot().Observed; got != 0 {
		t.Errorf("seri PUBLISH_BOUND Observed = %d, mau 0 untuk provenance tak dikenal", got)
	}
}

// TestLatencyDecidedToEmitSeparateFromOnset — decided->emit adalah seri ketiga
// yang berdiri sendiri, tidak dipecah per provenance karena kedua ujungnya jam
// server yang sama.
func TestLatencyDecidedToEmitSeparateFromOnset(t *testing.T) {
	var l latency
	l.observeDecidedToEmit(3)
	l.observeDecidedToEmit(11)

	got := l.decidedToEmit.snapshot()
	if got.Observed != 2 {
		t.Errorf("Observed = %d, mau 2", got.Observed)
	}
	if l.onsetToDecidedSensor.snapshot().Observed != 0 ||
		l.onsetToDecidedPublish.snapshot().Observed != 0 {
		t.Error("decided->emit tidak boleh menulis ke seri onset->decided mana pun")
	}
}

// ---- integrasi dengan Tracker ---------------------------------------------

// TestTrackerLatencyZeroOnFreshTracker — Tracker baru melaporkan ketiga seri nol.
func TestTrackerLatencyZeroOnFreshTracker(t *testing.T) {
	h := newHarness(t)
	s := h.trk.Stats()

	if s.OnsetToDecidedSensor.Observed != 0 {
		t.Errorf("OnsetToDecidedSensor.Observed = %d, mau 0", s.OnsetToDecidedSensor.Observed)
	}
	if s.OnsetToDecidedPublish.Observed != 0 {
		t.Errorf("OnsetToDecidedPublish.Observed = %d, mau 0", s.OnsetToDecidedPublish.Observed)
	}
	if s.DecidedToEmit.Observed != 0 {
		t.Errorf("DecidedToEmit.Observed = %d, mau 0", s.DecidedToEmit.Observed)
	}
}

// TestTrackerLatencyMeasuresOnsetToDecided — observasi v2 (onset TERUKUR sensor)
// mengisi seri SENSOR dengan jarak sesungguhnya antara onset dan keputusan.
//
// Jam palsu disetel ke onset + 800 ms sebelum ingest, jadi jawaban yang benar
// diketahui persis: 800. Angka yang muncul sebagai 0 akan berarti DecidedAt
// dibaca dari onset alih-alih dari jam server.
func TestTrackerLatencyMeasuresOnsetToDecided(t *testing.T) {
	h := newHarness(t)
	h.node("N1", baseLat, baseLon)

	h.clock.t = time.UnixMilli(onsetBase + 800)
	h.ingest(v2("N1", MinPGAGal+1, onsetBase, PhasePrelim, 1))

	s := h.trk.Stats()
	if s.OnsetToDecidedSensor.Observed != 1 {
		t.Fatalf("OnsetToDecidedSensor.Observed = %d, mau 1", s.OnsetToDecidedSensor.Observed)
	}
	if s.OnsetToDecidedSensor.P50Ms != 800 {
		t.Errorf("p50 SENSOR = %d ms, mau 800", s.OnsetToDecidedSensor.P50Ms)
	}
	if s.OnsetToDecidedPublish.Observed != 0 {
		t.Errorf("OnsetToDecidedPublish.Observed = %d, mau 0 untuk observasi v2",
			s.OnsetToDecidedPublish.Observed)
	}
}

// TestTrackerLatencyPublishBoundSeriesSeparate — observasi v1 legacy mengisi seri
// PUBLISH_BOUND, dan seri SENSOR tetap kosong.
func TestTrackerLatencyPublishBoundSeriesSeparate(t *testing.T) {
	h := newHarness(t)
	h.node("N1", baseLat, baseLon)

	publishTS := onsetBase + 4000
	h.clock.t = time.UnixMilli(publishTS + 200)
	h.ingest(v1("N1", MinPGAGal+1, publishTS, 3000))

	s := h.trk.Stats()
	if s.OnsetToDecidedPublish.Observed != 1 {
		t.Fatalf("OnsetToDecidedPublish.Observed = %d, mau 1", s.OnsetToDecidedPublish.Observed)
	}
	// onset yang disimpulkan = publishTS - 3000; keputusan pada publishTS + 200.
	if want := int64(3200); s.OnsetToDecidedPublish.P50Ms != want {
		t.Errorf("p50 PUBLISH_BOUND = %d ms, mau %d", s.OnsetToDecidedPublish.P50Ms, want)
	}
	if s.OnsetToDecidedSensor.Observed != 0 {
		t.Errorf("OnsetToDecidedSensor.Observed = %d, mau 0 untuk observasi v1 — "+
			"batas atas tidak boleh masuk seri pengukuran", s.OnsetToDecidedSensor.Observed)
	}
}

// TestTrackerLatencyDecidedToEmitObserved — setiap transisi yang diemisikan
// menyumbang satu sampel decided->emit.
func TestTrackerLatencyDecidedToEmitObserved(t *testing.T) {
	h := newHarness(t)
	h.node("N1", baseLat, baseLon)

	h.ingest(v2("N1", MinPGAGal+1, onsetBase, PhasePrelim, 1))

	frames := len(h.emit.frames)
	if frames == 0 {
		t.Fatal("tidak ada frame diemisikan — prasyarat uji tidak terpenuhi")
	}
	if got := h.trk.Stats().DecidedToEmit.Observed; got != int64(frames) {
		t.Errorf("DecidedToEmit.Observed = %d, mau %d (satu per frame)", got, frames)
	}
}

// TestTrackerLatencyExcludesTerminalFromOnsetSeries — transisi terminal MASUK
// decided->emit tetapi TIDAK masuk onset->decided.
//
// Ini yang menjaga angkanya tidak menyesatkan: RESOLVED terjadi ResolveAfterMs
// setelah bukti terakhir, secara konfigurasi. Bila ia dihitung sebagai latensi
// deteksi, p95 akan melaporkan ~90 detik dan menyebutnya latensi sistem.
func TestTrackerLatencyExcludesTerminalFromOnsetSeries(t *testing.T) {
	h := newHarness(t)
	h.node("N1", baseLat, baseLon)

	h.clock.t = time.UnixMilli(onsetBase + 800)
	h.ingest(v2("N1", MinPGAGal+1, onsetBase, PhasePrelim, 1))

	before := h.trk.Stats()
	if before.OnsetToDecidedSensor.Observed != 1 {
		t.Fatalf("prasyarat: OnsetToDecidedSensor.Observed = %d, mau 1",
			before.OnsetToDecidedSensor.Observed)
	}

	// Sweep menutup event jauh sesudahnya: satu transisi RESOLVED.
	h.clock.advance(time.Duration(defaultOptions().ResolveAfterMs+1) * time.Millisecond)
	h.trk.sweep(context.Background())

	after := h.trk.Stats()
	if after.TransitionToResolved != 1 {
		t.Fatalf("prasyarat: TransitionToResolved = %d, mau 1", after.TransitionToResolved)
	}
	if after.OnsetToDecidedSensor.Observed != before.OnsetToDecidedSensor.Observed {
		t.Errorf("OnsetToDecidedSensor.Observed naik %d -> %d pada RESOLVED: "+
			"transisi digerakkan timer tidak boleh dihitung sebagai latensi deteksi",
			before.OnsetToDecidedSensor.Observed, after.OnsetToDecidedSensor.Observed)
	}
	if after.DecidedToEmit.Observed <= before.DecidedToEmit.Observed {
		t.Errorf("DecidedToEmit.Observed tetap %d pada RESOLVED: biaya penyerahan "+
			"frame berlaku untuk setiap transisi", after.DecidedToEmit.Observed)
	}
}

// TestTrackerLatencySkipsMissingOnsetAnchor — OriginTS == 0 dilewati alih-alih
// diukur terhadap epoch.
//
// Dipanggil pada observeLatency langsung karena jalur Ingest selalu memberi onset:
// snapshot tanpa jangkar berasal dari baris pra-Fase-3 yang direkonsiliasi.
// Latensi terhadap jangkar yang tidak ada tidak terdefinisi, bukan 1,7 triliun ms.
func TestTrackerLatencySkipsMissingOnsetAnchor(t *testing.T) {
	h := newHarness(t)

	h.trk.observeLatency([]Snapshot{{
		To:             StateUnconfirmed,
		OriginTS:       0,
		OriginTSSource: OnsetSourceSensor,
		DecidedAt:      h.clock.t.UnixMilli(),
	}})

	s := h.trk.Stats()
	if s.OnsetToDecidedSensor.Observed != 0 {
		t.Errorf("OnsetToDecidedSensor.Observed = %d, mau 0 tanpa jangkar onset",
			s.OnsetToDecidedSensor.Observed)
	}
	if s.DecidedToEmit.Observed != 1 {
		t.Errorf("DecidedToEmit.Observed = %d, mau 1 — tahap ini tidak butuh jangkar onset",
			s.DecidedToEmit.Observed)
	}
}

// TestTrackerLatencyClockSkewNotClamped — onset di MASA DEPAN relatif keputusan
// (jam node mendahului jam server) tidak dihitung, jadi tidak ada "0 ms" palsu.
func TestTrackerLatencyClockSkewNotClamped(t *testing.T) {
	h := newHarness(t)
	decidedAt := h.clock.t.UnixMilli()

	h.trk.observeLatency([]Snapshot{{
		To:             StateConfirmed,
		OriginTS:       decidedAt + 5000, // onset 5 s setelah keputusan: mustahil
		OriginTSSource: OnsetSourceSensor,
		DecidedAt:      decidedAt,
	}})

	if got := h.trk.Stats().OnsetToDecidedSensor.Observed; got != 0 {
		t.Errorf("OnsetToDecidedSensor.Observed = %d, mau 0 saat jam menyimpang: "+
			"sampel negatif dibuang, tidak dijepit ke nol", got)
	}
}

// TestTrackerLatencyDoesNotDelayEmission — S1: pengukuran terjadi SETELAH emisi.
//
// Emitter mencatat jumlah sampel latensi yang ada pada saat frame diserahkan
// kepadanya. Bila instrumentasi berjalan lebih dulu, angka itu tidak akan nol.
func TestTrackerLatencyDoesNotDelayEmission(t *testing.T) {
	h := newHarness(t)
	h.node("N1", baseLat, baseLon)

	probe := &latencyOrderProbe{trk: h.trk}
	h.trk.SetEmitter(probe)

	h.clock.t = time.UnixMilli(onsetBase + 800)
	h.ingest(v2("N1", MinPGAGal+1, onsetBase, PhasePrelim, 1))

	if probe.frames == 0 {
		t.Fatal("tidak ada frame diemisikan — prasyarat uji tidak terpenuhi")
	}
	if probe.observedAtEmit != 0 {
		t.Errorf("sampel latensi = %d saat frame diemisikan, mau 0: emisi tidak boleh "+
			"menunggu instrumentasinya sendiri (S1)", probe.observedAtEmit)
	}
	if got := h.trk.Stats().DecidedToEmit.Observed; got != int64(probe.frames) {
		t.Errorf("DecidedToEmit.Observed = %d setelah publish, mau %d — pengukuran "+
			"harus tetap terjadi, hanya sesudahnya", got, probe.frames)
	}
}

// latencyOrderProbe adalah emitter yang MELIHAT counter latensi dari dalam
// EmitTransition. Membaca Stats() di sini akan menemui t.mu terbuka — publish
// berjalan di luar kunci — jadi probe ini juga membuktikan tidak ada deadlock
// pada urutan emisi-lalu-ukur.
type latencyOrderProbe struct {
	trk            *Tracker
	frames         int
	observedAtEmit int64
}

func (p *latencyOrderProbe) EmitTransition(_ context.Context, _ Snapshot) {
	p.frames++
	s := p.trk.Stats()
	p.observedAtEmit += s.OnsetToDecidedSensor.Observed +
		s.OnsetToDecidedPublish.Observed + s.DecidedToEmit.Observed
}
