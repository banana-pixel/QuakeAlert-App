package event

import (
	"math"
	"testing"

	"github.com/banana-pixel/quakealert/server/internal/consensus"
)

// Uji kebenaran spasial GLOBAL, lewat Tracker sungguhan — bukan lewat geometri
// saja. cell_test.go membuktikan invarian cakupan atas sel; yang di sini
// membuktikan konsekuensinya pada hal yang dipedulikan orang: satu gempa
// menghasilkan SATU event_id, di lintang mana pun dan di kedua sisi antimeridian.

// TestCandidateProbeMatchesProbeSet menjaga cermin uji agar tidak menyimpang dari
// candidatesLocked. probeSet di cell_test.go menyalin bentuk loop-nya; bila loop
// yang sungguhan berubah tanpa cerminnya, seluruh uji cakupan berhenti mengukur
// kode yang berjalan.
//
// Dibuktikan secara operasional: sebuah event ditanam pada SETIAP sel yang
// probeSet sebutkan, lalu satu observasi harus melihat seluruhnya sebagai kandidat
// bila jaraknya memenuhi — dan tidak ada sel di luar cermin yang perlu diselidiki
// karena setiap event yang cocok pasti terdaftar di salah satu sel itu.
func TestCandidateProbeMatchesProbeSet(t *testing.T) {
	for _, tc := range []struct{ lat, lon float64 }{
		{-6.9, 107.6}, {45.46, 9.19}, {64.13, -21.9}, {0, 179.9}, {0, -179.9},
	} {
		h := newHarness(t)
		mirror := probeSet(tc.lat, tc.lon, h.trk.opt.AttachRadiusKm)

		// Setiap sel di dalam cermin diselidiki: sebuah event yang terdaftar di sel
		// itu DAN memenuhi matches() harus muncul sebagai kandidat.
		h.node("N1", tc.lat, tc.lon)
		in := v2("N1", MinPGAGal+10, onsetBase, PhasePrelim, 1)
		in.Lat, in.Lon = tc.lat, tc.lon

		planted := 0
		h.trk.mu.Lock()
		b := onsetBucket(onsetBase, h.trk.opt.CorrelationWindowMs)
		for cell := range mirror {
			// Event tiruan pada koordinat observasi itu sendiri, tetapi DIINDEKS pada
			// sel yang sedang diperiksa: yang diuji adalah jangkauan indeks, bukan
			// jarak. matches() akan menerimanya karena jaraknya nol.
			e := &Event{
				ID:             "P" + string(rune('a'+planted%26)) + string(rune('a'+planted/26)) + "-planted",
				State:          StateUnconfirmed,
				OriginTS:       onsetBase,
				Contributors:   map[string]*Contributor{"X": {NodeID: "X", Lat: tc.lat, Lon: tc.lon, PeakPGA: MinPGAGal}},
				minCells:       h.trk.opt.MinIndependentCells,
				minSepKm:       h.trk.opt.IndependenceCellKm,
				LastEvidenceTS: onsetBase,
			}
			h.trk.events[e.ID] = e
			h.trk.indexLocked(e, indexKey{cell: cell, bucket: b})
			planted++
		}
		open, _ := h.trk.candidatesLocked(in)
		h.trk.mu.Unlock()

		if len(open) != planted {
			t.Errorf("%.2f/%.2f: kandidat = %d, mau %d (satu per sel cermin) — probeSet menyimpang dari candidatesLocked",
				tc.lat, tc.lon, len(open), planted)
		}
	}
}

// E1 — kecocokan sejati di lintang TINGGI tidak terlewat. Dua node berjarak 45 km
// (di dalam AttachRadiusKm 50) di Reykjavík: dahulu sel bujur di sana hanya 29 km
// lebar, jadi node kedua berada dua sel jauhnya, di luar lingkungan 3x3, dan
// membentuk event_id kedua.
func TestHighLatitudeTrueMatchIsNotMissed(t *testing.T) {
	// Kota yang dahulu gagal, plus tepat di ambang lama dan jauh di atasnya.
	for _, lat := range []float64{41.53, 44.43, 45.46, 47.60, 49.28, 61.20, 64.13, 78.22, -49.28} {
		t.Run("", func(t *testing.T) {
			h := newHarness(t)
			// Ke TIMUR: sumbu yang menyusut dengan cos(lat).
			h.node("N1", lat, 0)
			lat2, lon2 := h.nodeAt("N2", lat, 0, 45, 90)
			if d := consensus.HaversineKm(lat, 0, lat2, lon2); d > h.trk.opt.AttachRadiusKm {
				t.Fatalf("premis salah: jarak %.2f km sudah di luar radius attach", d)
			}

			h.ingest(v2("N1", MinPGAGal+20, onsetBase, PhasePrelim, 1))
			h.ingest(v2("N2", MinPGAGal+10, onsetBase+1500, PhasePrelim, 1))

			if got := len(h.events()); got != 1 {
				t.Fatalf("lintang %.2f: event terlacak = %d, mau 1 — PEMBELAHAN", lat, got)
			}
			e := h.only()
			if len(e.Contributors) != 2 {
				t.Errorf("lintang %.2f: kontributor = %d, mau 2", lat, len(e.Contributors))
			}
			if got := h.trk.Created(); got != 1 {
				t.Errorf("lintang %.2f: event_created_total = %d, mau 1", lat, got)
			}
		})
	}
}

// E2 — kecocokan sejati MELINTASI antimeridian tidak terlewat. Dua node terpisah
// 40 km, satu di 179,98°E dan satu di 179,7°W: berdekatan di bumi, dahulu 599 sel
// berjauhan di indeks.
func TestAntimeridianTrueMatchIsNotMissed(t *testing.T) {
	for _, lat := range []float64{-16.5, 0, 51.5} {
		h := newHarness(t)
		h.node("N1", lat, 179.98)
		lat2, lon2 := h.nodeAt("N2", lat, 179.98, 40, 90)
		if lon2 > 0 {
			t.Fatalf("premis salah: node kedua di %.4f, belum menyeberang meridian", lon2)
		}
		if d := consensus.HaversineKm(lat, 179.98, lat2, lon2); d > h.trk.opt.AttachRadiusKm {
			t.Fatalf("premis salah: jarak %.2f km di luar radius attach", d)
		}

		h.ingest(v2("N1", MinPGAGal+20, onsetBase, PhasePrelim, 1))
		h.ingest(v2("N2", MinPGAGal+10, onsetBase+1200, PhasePrelim, 1))

		if got := len(h.events()); got != 1 {
			t.Fatalf("lintang %.2f: event terlacak = %d, mau 1 — antimeridian membelah event", lat, got)
		}
		if got := len(h.only().Contributors); got != 2 {
			t.Errorf("lintang %.2f: kontributor = %d, mau 2", lat, got)
		}
	}
}

// Sentroid sebuah event yang mengangkangi antimeridian harus berada DI ANTARA
// kontributornya, bukan di sisi lain bumi. Ini yang membuat matches() dan keyOf()
// benar di sana: rata-rata aritmetik 179,98 dan -179,66 adalah 0,16 — di Afrika.
func TestCentroidStaysNearContributorsAcrossAntimeridian(t *testing.T) {
	h := newHarness(t)
	h.node("N1", 0, 179.98)
	lat2, lon2 := h.nodeAt("N2", 0, 179.98, 40, 90)
	h.ingest(v2("N1", MinPGAGal+20, onsetBase, PhasePrelim, 1))
	h.ingest(v2("N2", MinPGAGal+20, onsetBase, PhasePrelim, 1))

	c := h.only().centroid()
	if d := consensus.HaversineKm(c.Lat, c.Lon, 0, 179.98); d > 40 {
		t.Errorf("sentroid %.4f/%.4f berjarak %.0f km dari N1 — bujur dirata-rata sebagai garis, bukan lingkaran",
			c.Lat, c.Lon, d)
	}
	if d := consensus.HaversineKm(c.Lat, c.Lon, lat2, lon2); d > 40 {
		t.Errorf("sentroid %.4f/%.4f berjarak %.0f km dari N2", c.Lat, c.Lon, d)
	}
}

// E3 — dua kontributor yang secara FISIK lebih dekat dari ambang independensi
// tidak boleh terhitung independen hanya karena berada di sel bujur berbeda.
// Inilah D2: di lintang tinggi sel bujur 5 km menyusut menjadi ~2,2 km.
func TestContributorsCloserThanThresholdAreNotIndependent(t *testing.T) {
	for _, lat := range []float64{0, 45.46, 61.20, 64.13, 78.22} {
		for _, sepKm := range []float64{0.5, 2.5, 4.9} {
			h := newHarness(t)
			h.node("N1", lat, 0)
			lat2, lon2 := h.nodeAt("N2", lat, 0, sepKm, 90)
			h.node("N3", lat, 0)

			h.ingest(v2("N1", MinPGAGal+20, onsetBase, PhasePrelim, 1))
			h.ingest(v2("N2", MinPGAGal+10, onsetBase+500, PhasePrelim, 1))
			h.ingest(v2("N3", MinPGAGal+10, onsetBase+900, PhasePrelim, 1))

			e := h.only()
			// Label sel BOLEH berbeda — itu justru cacatnya; yang tidak boleh adalah
			// hitungan independensinya ikut berbeda.
			cellsDiffer := independenceCell(lat, 0, 5) != independenceCell(lat2, lon2, 5)
			if got := e.independentCells(); got != 1 {
				t.Errorf("lintang %.2f, pisah %.1f km: sel independen = %d, mau 1 (label sel berbeda: %v)",
					lat, sepKm, got, cellsDiffer)
			}
			if e.State == StateConfirmed {
				t.Errorf("lintang %.2f, pisah %.1f km: CONFIRMED dari tiga sensor sedekat ini", lat, sepKm)
			}
		}
	}
}

// E4 — dua kontributor yang LEBIH JAUH dari ambang tetap terhitung independen, di
// lintang mana pun. Perbaikan D2 tidak boleh membuat CONFIRMED tak terjangkau.
func TestContributorsFartherThanThresholdRemainIndependent(t *testing.T) {
	for _, lat := range []float64{0, 45.46, 64.13, 78.22, -61.20} {
		for _, sepKm := range []float64{5.1, 8, 16} {
			h := newHarness(t)
			h.node("N1", lat, 0)
			h.nodeAt("N2", lat, 0, sepKm, 90)
			h.nodeAt("N3", lat, 0, sepKm*2, 90)

			h.ingest(v2("N1", MinPGAGal+20, onsetBase, PhasePrelim, 1))
			h.ingest(v2("N2", MinPGAGal+10, onsetBase+500, PhasePrelim, 1))
			h.ingest(v2("N3", MinPGAGal+10, onsetBase+900, PhasePrelim, 1))

			e := h.only()
			if got := e.independentCells(); got != 3 {
				t.Errorf("lintang %.2f, pisah %.1f km: sel independen = %d, mau 3", lat, sepKm, got)
			}
			if e.State != StateConfirmed {
				t.Errorf("lintang %.2f, pisah %.1f km: state = %s, mau CONFIRMED", lat, sepKm, e.State)
			}
		}
	}
}

// Independensi diukur sebagai jarak, jadi ia harus benar juga MELINTASI
// antimeridian: dua node terpisah 1 km yang mengapit meridian 180° adalah satu
// bukti, bukan dua.
func TestIndependenceIsCorrectAcrossAntimeridian(t *testing.T) {
	h := newHarness(t)
	h.node("N1", 0, 179.997)
	h.nodeAt("N2", 0, 179.997, 1, 90)
	h.node("N3", 0, 179.997)

	h.ingest(v2("N1", MinPGAGal+20, onsetBase, PhasePrelim, 1))
	h.ingest(v2("N2", MinPGAGal+10, onsetBase+400, PhasePrelim, 1))
	h.ingest(v2("N3", MinPGAGal+10, onsetBase+800, PhasePrelim, 1))

	e := h.only()
	if got := e.independentCells(); got != 1 {
		t.Errorf("sel independen = %d, mau 1: tiga sensor dalam 1 km adalah satu bukti", got)
	}
}

// Penghitung independensi bersifat KONSERVATIF dan DETERMINISTIK: hasilnya adalah
// himpunan yang sah saling-independen (jadi <= maksimum sejati), dan tidak
// bergantung pada urutan iterasi map.
func TestIndependentCountIsConservativeAndDeterministic(t *testing.T) {
	const sep = 5.0
	// Rantai: 0, 3, 6, 9, 12 km ke timur. Maksimum sejati pada ambang 5 km adalah
	// {0, 6, 12} = 3. Rakus atas urutan id memberi {0, 6, 12} juga; yang wajib
	// adalah hasilnya <= 3 dan setiap pasangan perwakilan benar-benar >= 5 km.
	pts := []geoPoint{}
	for i, km := range []float64{0, 3, 6, 9, 12} {
		lat, lon := destinationKm(0, 0, km, 90)
		pts = append(pts, geoPoint{id: string(rune('a' + i)), lat: lat, lon: lon})
	}

	got := independentCount(pts, sep)
	if got < 1 || got > 3 {
		t.Fatalf("hitungan = %d, mau di dalam [1, 3] (maksimum sejati 3)", got)
	}

	// Deterministik atas permutasi masukan.
	for shift := 0; shift < len(pts); shift++ {
		rot := append(append([]geoPoint{}, pts[shift:]...), pts[:shift]...)
		if n := independentCount(rot, sep); n != got {
			t.Errorf("rotasi %d: hitungan = %d, mau %d — hasil bergantung urutan masukan", shift, n, got)
		}
	}

	// Kasus batas: ambang nol berarti setiap titik dihitung; himpunan kosong nol.
	if n := independentCount(pts, 0); n != len(pts) {
		t.Errorf("ambang 0: hitungan = %d, mau %d", n, len(pts))
	}
	if n := independentCount(nil, sep); n != 0 {
		t.Errorf("himpunan kosong: hitungan = %d, mau 0", n)
	}
	// Satu titik selalu satu bukti, tidak pernah nol: sebuah event dengan satu
	// kontributor harus tetap punya satu.
	if n := independentCount(pts[:1], sep); n != 1 {
		t.Errorf("satu titik: hitungan = %d, mau 1", n)
	}
}

// E5 — pencarian kandidat KONSERVATIF: ia boleh memilih berlebih, tidak pernah
// kurang. Dinyatakan sebagai pernyataan himpunan atas seluruh indeks: setiap event
// yang matches() terima WAJIB muncul di antara kandidat.
func TestCandidateLookupNeverUnderSelects(t *testing.T) {
	for _, tc := range []struct {
		name     string
		lat, lon float64
	}{
		{"ekuator", -6.9, 107.6},
		{"lintang tinggi", 64.13, -21.9},
		{"antimeridian timur", -16.5, 179.95},
		{"antimeridian barat", -16.5, -179.95},
		{"dekat kutub", 84.5, 15.6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			// Sebar event ke delapan arah pada jarak di dalam DAN di luar radius,
			// masing-masing lewat jalur Ingest sungguhan supaya indeksnya asli.
			id := 0
			for _, km := range []float64{5, 20, 45, 49.9, 60, 120, 400} {
				for _, br := range []float64{0, 45, 90, 135, 180, 225, 270, 315} {
					id++
					node := "N" + string(rune('a'+id%26)) + string(rune('a'+id/26))
					h.nodeAt(node, tc.lat, tc.lon, km, br)
					// Onset berjauhan supaya setiap node membentuk event sendiri; yang
					// diuji adalah jangkauan indeks, bukan korelasi.
					h.ingest(v2(node, MinPGAGal+20, onsetBase+int64(id)*int64(1_000_000), PhasePrelim, 1))
				}
			}

			// Observasi penyelidik di titik acuan, pada onset setiap event: untuk
			// masing-masing, kandidat harus memuat SETIAP event yang matches() terima.
			h.node("PROBE", tc.lat, tc.lon)
			h.trk.mu.Lock()
			defer h.trk.mu.Unlock()
			for _, e := range h.trk.events {
				in := v2("PROBE", MinPGAGal+5, e.OriginTS, PhasePrelim, 1)
				in.Lat, in.Lon = tc.lat, tc.lon

				want := map[string]bool{}
				for _, cand := range h.trk.events {
					if matches(cand, in.OnsetTS, in.Lat, in.Lon,
						h.trk.opt.CorrelationWindowMs, h.trk.opt.AttachRadiusKm) {
						want[cand.ID] = true
					}
				}
				open, tomb := h.trk.candidatesLocked(in)
				got := map[string]bool{}
				for _, c := range append(open, tomb...) {
					got[c.ID] = true
				}
				for wid := range want {
					if !got[wid] {
						t.Fatalf("onset %d: event %s cocok tetapi TIDAK muncul sebagai kandidat — I-COV dilanggar",
							in.OnsetTS, wid)
					}
				}
			}
		})
	}
}

// E6 — perilaku Fase 3 yang sudah ada TIDAK berubah di bawah 41,5°: himpunan sel
// yang diselidiki identik dengan lingkungan 3x3 yang lama, dan armada acuan
// Bandung berperilaku sama persis.
func TestExistingBehaviourUnchangedBelowOldBound(t *testing.T) {
	old3x3 := func(lat, lon float64) map[cellKey]struct{} {
		c := lookupCell(lat, lon)
		out := make(map[cellKey]struct{}, 9)
		for dx := int32(-1); dx <= 1; dx++ {
			for dy := int32(-1); dy <= 1; dy++ {
				out[cellKey{X: wrapCellX(c.X + dx), Y: c.Y + dy}] = struct{}{}
			}
		}
		return out
	}

	for lat := -41.0; lat <= 41.0+1e-9; lat += 0.5 {
		for _, lon := range []float64{-179.9, -107.6, -0.1, 0, 0.1, 107.6, 179.9} {
			want := old3x3(lat, lon)
			got := probeSet(lat, lon, defaultAttachRadiusKm)
			if len(got) != len(want) {
				t.Fatalf("%.2f/%.2f: probe %d sel, 3x3 lama %d sel", lat, lon, len(got), len(want))
			}
			for k := range want {
				if _, ok := got[k]; !ok {
					t.Fatalf("%.2f/%.2f: sel %v ada di 3x3 lama tetapi tidak di probe baru", lat, lon, k)
				}
			}
		}
	}

	// Armada acuan: tiga node 0/8/16 km ke timur Bandung -> CONFIRMED, satu event.
	h := newHarness(t)
	h.threeNodeCluster()
	h.ingest(v2("N1", MinPGAGal+30, onsetBase, PhasePrelim, 1))
	h.ingest(v2("N2", MinPGAGal+20, onsetBase+1000, PhasePrelim, 1))
	h.ingest(v2("N3", MinPGAGal+10, onsetBase+2000, PhasePrelim, 1))

	e := h.only()
	if e.State != StateConfirmed {
		t.Errorf("armada acuan: state = %s, mau CONFIRMED", e.State)
	}
	if got := e.independentCells(); got != 3 {
		t.Errorf("armada acuan: sel independen = %d, mau 3", got)
	}
	if got := h.trk.Created(); got != 1 {
		t.Errorf("armada acuan: event_created_total = %d, mau 1", got)
	}
}

// Biaya probe yang melebar TERBATAS: indeks memegang paling banyak
// MaxOpen + MaxTombstones event apa pun lebar probe-nya, jadi yang bertambah
// adalah kunci map kosong yang dibaca. Dinyatakan sebagai batas atas eksplisit
// supaya sebuah pelebaran yang tak terduga terlihat sebagai kegagalan uji.
func TestProbeWidthStaysBounded(t *testing.T) {
	for _, tc := range []struct {
		lat     float64
		maxCell int
	}{
		{0, 9}, {41, 9}, {45.46, 15}, {64.13, 21}, {80, 45}, {89.9, 1800},
	} {
		n := len(probeSet(tc.lat, 0, defaultAttachRadiusKm))
		if n > tc.maxCell {
			t.Errorf("lintang %.2f: probe %d sel, batas %d", tc.lat, n, tc.maxCell)
		}
	}
	// Sumbu lintang tidak pernah melebihi seluruh globe.
	_, ny, _ := probeSpan(0, MaxAttachRadiusKm)
	if int(ny) > int(math.Ceil(180/LookupCellDeg)) {
		t.Errorf("ny = %d melebihi tinggi globe", ny)
	}
}
