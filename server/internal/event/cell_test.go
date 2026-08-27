package event

import (
	"math"
	"testing"

	"github.com/banana-pixel/quakealert/server/internal/consensus"
)

// defaultAttachRadiusKm adalah nilai baku ATTACH_RADIUS_KM (§11.5). Uji cakupan di
// bawah harus lulus untuk nilai ini pada seluruh pita lintang yang didukung.
const defaultAttachRadiusKm = 50.0

// Pertidaksamaan §6.3.1 dinyatakan LANGSUNG, supaya kedua konstanta tidak dapat
// menyimpang tanpa satu uji gagal. Uji ini harus GAGAL bila LookupCellDeg
// diturunkan kembali ke 0.45.
func TestLookupCellDegSatisfiesCoverageInequality(t *testing.T) {
	cosPhi := math.Cos(MaxFleetLatitudeDeg * math.Pi / 180)

	lonKm := LookupCellDeg * KmPerDegree * cosPhi
	if lonKm < defaultAttachRadiusKm {
		t.Errorf("sumbu bujur: %.2f km < %.2f km — lingkungan 3x3 tidak menutupi radius attach",
			lonKm, defaultAttachRadiusKm)
	}
	latKm := LookupCellDeg * KmPerDegree
	if latKm < defaultAttachRadiusKm {
		t.Errorf("sumbu lintang: %.2f km < %.2f km", latKm, defaultAttachRadiusKm)
	}
}

// destination mengembalikan titik pada jarak distKm dan bearing derajat dari
// (lat, lon), memakai rumus great-circle langsung.
func destination(lat, lon, bearingDeg, distKm float64) (float64, float64) {
	const deg = math.Pi / 180
	d := distKm / consensus.EarthRadiusKm
	br := bearingDeg * deg
	lat1 := lat * deg
	lon1 := lon * deg

	lat2 := math.Asin(math.Sin(lat1)*math.Cos(d) + math.Cos(lat1)*math.Sin(d)*math.Cos(br))
	lon2 := lon1 + math.Atan2(
		math.Sin(br)*math.Sin(d)*math.Cos(lat1),
		math.Cos(d)-math.Sin(lat1)*math.Sin(lat2),
	)
	return lat2 / deg, lon2 / deg
}

// inNeighbourhood melaporkan apakah c berada di lingkungan 3x3 dari origin.
func inNeighbourhood(origin, c cellKey) bool {
	dx := c.X - origin.X
	dy := c.Y - origin.Y
	return dx >= -1 && dx <= 1 && dy >= -1 && dy <= 1
}

// Invarian cakupan (R-H1): untuk setiap lintang yang didukung dan setiap posisi di
// dalam sel, SETIAP titik pada tepat AttachRadiusKm ke delapan arah mata angin
// harus jatuh di dalam lingkungan 3x3 sel titik asal.
//
// Sumbu bujur adalah yang dahulu gagal, jadi kedua sumbu diperiksa terpisah lewat
// bearing yang dipisah, bukan hanya lewat satu jarak agregat.
func TestLookupNeighbourhoodCoversAttachRadius(t *testing.T) {
	// Offset di dalam sel, dinyatakan sebagai fraksi sisi sel — termasuk tepat di
	// tepi, kasus terburuk yang menyisakan tepat satu sel tetangga sebagai margin.
	fractions := []float64{0.0, 0.001, 0.25, 0.5, 0.75, 0.999}
	bearings := []float64{0, 45, 90, 135, 180, 225, 270, 315}

	for latDeg := -MaxFleetLatitudeDeg; latDeg <= MaxFleetLatitudeDeg+1e-9; latDeg += 0.5 {
		baseCell := lookupCell(latDeg, 0)
		for _, fx := range fractions {
			for _, fy := range fractions {
				lat := (float64(baseCell.Y) + fy) * LookupCellDeg
				lon := (float64(baseCell.X) + fx) * LookupCellDeg
				origin := lookupCell(lat, lon)

				for _, br := range bearings {
					dLat, dLon := destination(lat, lon, br, defaultAttachRadiusKm)
					if got := lookupCell(dLat, dLon); !inNeighbourhood(origin, got) {
						t.Fatalf("titik %.4f/%.4f (bearing %.0f, %.0f km) di sel %v, di luar lingkungan 3x3 dari %v (lat basis %.1f)",
							dLat, dLon, br, defaultAttachRadiusKm, got, origin, latDeg)
					}
				}
			}
		}
	}
}

// Uji cakupan di atas harus benar-benar SENSITIF: dengan sisi 0.45° yang lama,
// titik 50 km ke timur pada lintang batas keluar dari lingkungan 3x3. Diperiksa di
// sini secara eksplisit supaya "uji cakupan lulus" tidak bisa berarti "uji
// cakupan tidak mengukur apa pun".
func TestCoverageTestWouldFailAtOldCellSize(t *testing.T) {
	const oldCellDeg = 0.45
	cosPhi := math.Cos(MaxFleetLatitudeDeg * math.Pi / 180)
	if oldCellDeg*KmPerDegree*cosPhi >= defaultAttachRadiusKm {
		t.Fatalf("0.45° seharusnya TIDAK mencukupi pada lintang %.1f°, tetapi memberi %.2f km",
			MaxFleetLatitudeDeg, oldCellDeg*KmPerDegree*cosPhi)
	}
}

// Regresi A17: koordinat yang mengapit nol harus berada di sel BERBEDA. Cacat
// aslinya adalah kunci string terformat, di mana -0.0 dan 0.0 dicetak berbeda
// (atau sama, tergantung format) tanpa hubungan dengan posisi.
func TestCellKeysAcrossZeroAreDistinct(t *testing.T) {
	negZero := math.Copysign(0, -1)

	cases := []struct {
		name     string
		lat, lon float64
		want     cellKey
	}{
		{"nol positif", 0, 0, cellKey{X: 0, Y: 0}},
		{"nol negatif", negZero, negZero, cellKey{X: 0, Y: 0}},
		{"tepat di bawah nol", -0.01, -0.01, cellKey{X: -1, Y: -1}},
		{"tepat di atas nol", 0.01, 0.01, cellKey{X: 0, Y: 0}},
		{"kuadran selatan-barat", -6.9, -107.6, cellKey{X: -180, Y: -12}},
	}
	for _, c := range cases {
		if got := lookupCell(c.lat, c.lon); got != c.want {
			t.Errorf("%s: lookupCell(%v, %v) = %v, mau %v", c.name, c.lat, c.lon, got, c.want)
		}
	}

	// -0.0 dan +0.0 adalah posisi yang SAMA, jadi selnya harus sama; yang tidak
	// boleh sama adalah -0.01 dan +0.01, yang mengapit garis nol.
	if lookupCell(negZero, negZero) != lookupCell(0, 0) {
		t.Fatal("-0.0 dan +0.0 adalah titik yang sama dan harus satu sel")
	}
	if lookupCell(-0.01, -0.01) == lookupCell(0.01, 0.01) {
		t.Fatal("titik yang mengapit nol harus di sel berbeda")
	}
}

// Aritmetika §6.3 dikunci di dalam uji, bukan hanya di dalam prosa: pasangan
// referensi Cimahi–Bandung (~9,4 km) harus SELALU berada di sel berbeda pada 5 km,
// dan pada 10 km dapat jatuh di SATU sel — yang persisnya adalah alasan
// IndependenceCellKm dikonfigurasi dan bukan dipatok 10.
//
// Klaim "pada 10 km keduanya satu sel" dinyatakan sebagai KEMUNGKINAN, bukan
// sebagai fakta untuk satu koordinat, karena aritmetika grid mengatakan demikian:
// pada 10 km pasangan ini kebetulan melewati batas sel hanya 56 m (sumbu lintang)
// dan 210 m (sumbu bujur), jadi berada di sel yang sama atau tidak diputuskan oleh
// beberapa ratus meter penyelarasan grid di atas pemisahan 9,4 km. Sebuah ambang
// keselamatan yang keterjangkauannya diputuskan oleh angka itu adalah tepat cacat
// yang dijelaskan §7.3.
func TestIndependenceCellSeparatesReferenceFleetAtFiveKm(t *testing.T) {
	const (
		cimahiLat, cimahiLon   = -6.8721, 107.5422
		bandungLat, bandungLon = -6.9175, 107.6191
	)

	if d := consensus.HaversineKm(cimahiLat, cimahiLon, bandungLat, bandungLon); d < 9 || d > 10 {
		t.Fatalf("jarak referensi %.2f km di luar ~9,4 km — koordinat uji berubah", d)
	}

	// Geseran kecil dibanding pemisahan pasangan (<= ~1 km). Geometri relatif kedua
	// node tidak berubah; yang berubah hanya di mana batas sel jatuh.
	shifts := []float64{-0.009, -0.005, -0.002, 0, 0.002, 0.005, 0.009}

	sameAt10 := 0
	for _, dLat := range shifts {
		for _, dLon := range shifts {
			a5 := independenceCell(cimahiLat+dLat, cimahiLon+dLon, 5)
			b5 := independenceCell(bandungLat+dLat, bandungLon+dLon, 5)
			if a5 == b5 {
				t.Errorf("pada 5 km (geseran %+.3f/%+.3f) keduanya jatuh di sel %v — independensi tidak terukur",
					dLat, dLon, a5)
			}

			a10 := independenceCell(cimahiLat+dLat, cimahiLon+dLon, 10)
			b10 := independenceCell(bandungLat+dLat, bandungLon+dLon, 10)
			if a10 == b10 {
				sameAt10++
			}
		}
	}

	if sameAt10 == 0 {
		t.Fatal("pada 10 km tidak ada satu pun penyelarasan grid yang menyatukan pasangan ini; " +
			"argumen §7.3 bahwa 10 km membuat CONFIRMED bergantung keberuntungan tidak terbukti di sini")
	}
}

// Ember waktu (R-C1): untuk onset apa pun, {b-1, b, b+1} harus memuat
// bucket(origin) bagi SETIAP origin di dalam [onset-W, onset+W]; dan {b-1, b}
// harus TIDAK memuatnya — bentuk dua ember wajib terbukti tidak cukup, bukan
// sekadar diganti.
func TestBucketProbeCoversWindow(t *testing.T) {
	const w = int64(20000)

	twoBucketMisses := 0
	for _, onset := range []int64{
		1_700_000_000_000,
		1_700_000_000_001,
		1_700_000_000_000 - 1,
		20000, 19999, 20001, 0, 1,
	} {
		b := onsetBucket(onset, w)
		three := map[int64]bool{b - 1: true, b: true, b + 1: true}
		two := map[int64]bool{b - 1: true, b: true}

		for origin := onset - w; origin <= onset+w; origin++ {
			ob := onsetBucket(origin, w)
			if !three[ob] {
				t.Fatalf("onset %d: origin %d ada di ember %d, di luar {%d,%d,%d}",
					onset, origin, ob, b-1, b, b+1)
			}
			if !two[ob] {
				twoBucketMisses++
			}
			// Loop di atas berjalan 40001 kali per onset; cukup untuk membuktikan
			// cakupan tanpa menyapu seluruh rentang epoch.
			origin += 499
		}
	}

	if twoBucketMisses == 0 {
		t.Fatal("bentuk dua ember tidak pernah melewatkan apa pun di sini — uji ini tidak membuktikan R-C1")
	}
}

// Ember harus floor, bukan pemotongan ke arah nol: dua instan yang mengapit epoch
// nol tidak boleh melipat menjadi satu ember.
func TestOnsetBucketFloorsTowardNegativeInfinity(t *testing.T) {
	const w = int64(20000)
	cases := []struct {
		ms   int64
		want int64
	}{
		{0, 0}, {1, 0}, {19999, 0}, {20000, 1},
		{-1, -1}, {-20000, -1}, {-20001, -2},
	}
	for _, c := range cases {
		if got := onsetBucket(c.ms, w); got != c.want {
			t.Errorf("onsetBucket(%d) = %d, mau %d", c.ms, got, c.want)
		}
	}
	if onsetBucket(123, 0) != 0 {
		t.Fatal("window nol harus mengembalikan 0, bukan panik pembagian nol")
	}
}
