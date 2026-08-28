package consensus

import (
	"math"
	"testing"
)

func TestWeightedCentroid(t *testing.T) {
	// Dua node dengan bobot PGA berbeda: centroid harus condong ke node ber-PGA
	// lebih besar. Node A(0,0) PGA 100, Node B(0,10) PGA 300.
	// Lon_c = (0*100 + 10*300)/400 = 3000/400 = 7.5
	rs := []Reading{
		{NodeID: "A", Lat: 0, Lon: 0, PGA: 100},
		{NodeID: "B", Lat: 0, Lon: 10, PGA: 300},
	}
	c := WeightedCentroid(rs)
	if math.Abs(c.Lat-0) > 1e-9 {
		t.Fatalf("Lat = %v, want 0", c.Lat)
	}
	if math.Abs(c.Lon-7.5) > 1e-9 {
		t.Fatalf("Lon = %v, want 7.5", c.Lon)
	}
}

func TestWeightedCentroidEqualWeights(t *testing.T) {
	rs := []Reading{
		{Lat: -6.0, Lon: 106.0, PGA: 50},
		{Lat: -6.4, Lon: 106.8, PGA: 50},
	}
	c := WeightedCentroid(rs)
	if math.Abs(c.Lat-(-6.2)) > 1e-9 || math.Abs(c.Lon-106.4) > 1e-9 {
		t.Fatalf("centroid = %+v, want {-6.2, 106.4}", c)
	}
}

func TestWeightedCentroidZeroWeightFallback(t *testing.T) {
	rs := []Reading{
		{Lat: 2, Lon: 4, PGA: 0},
		{Lat: 4, Lon: 8, PGA: 0},
	}
	c := WeightedCentroid(rs)
	if math.Abs(c.Lat-3) > 1e-9 || math.Abs(c.Lon-6) > 1e-9 {
		t.Fatalf("fallback centroid = %+v, want {3, 6}", c)
	}
}

func TestMMIFromPGA(t *testing.T) {
	// PGA 413.13 gal -> MMI = 3.66*log10(413.13) - 1.66 ~= 7.93
	got := MMIFromPGA(413.13)
	want := 3.66*math.Log10(413.13) - 1.66
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("MMIFromPGA(413.13) = %v, want %v", got, want)
	}
	if MMIFromPGA(0) != 1 {
		t.Fatalf("MMIFromPGA(0) harus 1 (batas bawah)")
	}
}

func TestIntensityThresholds(t *testing.T) {
	cases := []struct {
		pga   float64
		label string
	}{
		{5, "light"},
		{16.6, "moderate"},
		{100, "moderate"},
		{137.2, "strong"},
		{500, "strong"},
	}
	for _, c := range cases {
		_, label := Intensity(c.pga)
		if label != c.label {
			t.Errorf("Intensity(%v) label = %q, want %q", c.pga, label, c.label)
		}
	}
}

func TestMaxPGA(t *testing.T) {
	rs := []Reading{{PGA: 10}, {PGA: 250}, {PGA: 99}}
	if got := MaxPGA(rs); got != 250 {
		t.Fatalf("MaxPGA = %v, want 250", got)
	}
}

// WeightedCentroidGlobal harus menempatkan sentroid DI ANTARA stasiun ketika
// mereka mengangkangi antimeridian. Rata-rata aritmetik pada kolom bujur
// menghasilkan 0,16° — di Afrika, antipode dari tempat gempanya.
func TestWeightedCentroidGlobalAcrossAntimeridian(t *testing.T) {
	rs := []Reading{
		{NodeID: "A", Lat: 0, Lon: 179.9, PGA: 20},
		{NodeID: "B", Lat: 0, Lon: -179.9, PGA: 20},
	}

	c := WeightedCentroidGlobal(rs)
	if d := HaversineKm(c.Lat, c.Lon, 0, 179.9); d > 30 {
		t.Errorf("sentroid global %.4f/%.4f berjarak %.0f km dari A", c.Lat, c.Lon, d)
	}
	if math.Abs(c.Lon) < 179 {
		t.Errorf("bujur sentroid = %.4f, mau di sekitar ±180", c.Lon)
	}
	if c.Lon < -180 || c.Lon >= 180 {
		t.Errorf("bujur sentroid = %.4f, di luar [-180, 180)", c.Lon)
	}

	// Dan yang LAMA memang salah di sini: perbaikan ini harus terbukti perbaikan,
	// bukan penulisan ulang yang setara.
	if old := WeightedCentroid(rs); math.Abs(old.Lon) > 1 {
		t.Fatalf("WeightedCentroid lama = %.4f, seharusnya keliru mendekati 0 — premis uji ini berubah", old.Lon)
	}
}

// Jauh dari antimeridian, keduanya harus SEPAKAT: perilaku Fase 2 tidak boleh
// berubah pada koordinat mana pun yang benar-benar dipakai hari ini.
func TestWeightedCentroidGlobalAgreesAwayFromAntimeridian(t *testing.T) {
	for _, base := range []float64{-107.6, -0.13, 0, 9.19, 107.6, 139.7} {
		rs := []Reading{
			{NodeID: "A", Lat: -6.9, Lon: base, PGA: 20},
			{NodeID: "B", Lat: -6.8, Lon: base + 0.1, PGA: 40},
			{NodeID: "C", Lat: -7.0, Lon: base - 0.1, PGA: 10},
		}
		a, b := WeightedCentroid(rs), WeightedCentroidGlobal(rs)
		if math.Abs(a.Lat-b.Lat) > 1e-9 || math.Abs(a.Lon-b.Lon) > 1e-9 {
			t.Errorf("bujur basis %.2f: lama %.9f/%.9f vs global %.9f/%.9f", base, a.Lat, a.Lon, b.Lat, b.Lon)
		}
	}
	// Himpunan kosong dan bobot nol berperilaku sama pada keduanya.
	if got := WeightedCentroidGlobal(nil); got != (Centroid{}) {
		t.Errorf("himpunan kosong = %v, mau nol", got)
	}
	zero := []Reading{{NodeID: "A", Lat: 1, Lon: 2}, {NodeID: "B", Lat: 3, Lon: 4}}
	if a, b := WeightedCentroid(zero), WeightedCentroidGlobal(zero); a != b {
		t.Errorf("bobot nol: lama %v vs global %v", a, b)
	}
}

// NormalizeLon melipat ke [-180, 180): setengah terbuka, sehingga tidak ada bujur
// yang punya dua representasi kanonik.
func TestNormalizeLonFoldsToHalfOpenRange(t *testing.T) {
	for _, c := range []struct{ in, want float64 }{
		{0, 0}, {179.9, 179.9}, {-179.9, -179.9},
		{180, -180}, {-180, -180}, {181, -179}, {-181, 179},
		{360, 0}, {540, -180}, {-360, 0}, {720.5, 0.5},
	} {
		if got := NormalizeLon(c.in); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("NormalizeLon(%g) = %g, mau %g", c.in, got, c.want)
		}
	}
	for lon := -1080.0; lon <= 1080; lon += 0.7 {
		got := NormalizeLon(lon)
		if got < -180 || got >= 180 {
			t.Fatalf("NormalizeLon(%g) = %g, di luar [-180, 180)", lon, got)
		}
	}
}
