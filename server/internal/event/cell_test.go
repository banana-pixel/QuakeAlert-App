package event

import (
	"math"
	"testing"

	"github.com/banana-pixel/quakealert/server/internal/consensus"
)

// defaultAttachRadiusKm adalah nilai baku ATTACH_RADIUS_KM (§11.5). Uji cakupan di
// bawah harus lulus untuk nilai ini pada SETIAP lintang, bukan pada sebuah pita.
const defaultAttachRadiusKm = 50.0

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
	return lat2 / deg, consensus.NormalizeLon(lon2 / deg)
}

// probeSet membangun himpunan sel yang candidatesLocked benar-benar selidiki untuk
// sebuah observasi. Sengaja MENCERMINKAN bentuk loop di tracker.go alih-alih
// memanggilnya: candidatesLocked butuh Tracker, indeks, dan Input lengkap, dan uji
// cakupan harus dapat menyatakan invariannya atas geometri saja. Uji
// TestCandidateProbeMatchesProbeSet menjaga agar cermin ini tidak menyimpang.
func probeSet(lat, lon, radiusKm float64) map[cellKey]struct{} {
	c := lookupCell(lat, lon)
	nx, ny, allLon := probeSpan(lat, radiusKm)

	xs := make([]int32, 0, 2*int(nx)+1)
	if allLon {
		half := lonCellCount / 2
		for x := -half; x < half; x++ {
			xs = append(xs, x)
		}
	} else {
		for dx := -nx; dx <= nx; dx++ {
			xs = append(xs, wrapCellX(c.X+dx))
		}
	}

	out := make(map[cellKey]struct{}, len(xs)*(2*int(ny)+1))
	for dy := -ny; dy <= ny; dy++ {
		for _, x := range xs {
			out[cellKey{X: x, Y: c.Y + dy}] = struct{}{}
		}
	}
	return out
}

// INVARIAN CAKUPAN (I-COV): untuk SETIAP lintang, setiap posisi di dalam sel, dan
// setiap arah, sebuah titik pada tepat AttachRadiusKm harus jatuh di dalam
// lingkungan yang diselidiki oleh titik asal.
//
// Lintangnya menyapu seluruh globe, bukan sebuah pita: pita 12° yang dahulu
// dipakai berasal dari kepulauan Indonesia, dan uji yang batas loop-nya SAMA
// dengan asumsi yang diuji tidak dapat pernah menemukan cacatnya. Kota yang
// dahulu gagal disebut namanya di tabel supaya regresinya terlihat sebagai tempat,
// bukan sebagai angka.
func TestLookupProbeCoversAttachRadiusGlobally(t *testing.T) {
	fractions := []float64{0.0, 0.001, 0.25, 0.5, 0.75, 0.999}
	bearings := []float64{0, 30, 45, 60, 90, 120, 135, 150, 180, 210, 225, 240, 270, 300, 315, 330}

	lats := []float64{}
	for l := -89.5; l <= 89.5+1e-9; l += 0.5 {
		lats = append(lats, l)
	}
	// Lintang kota yang dahulu GAGAL pada lingkungan 3x3 tetap, disebut eksplisit.
	lats = append(lats, 44.43, 45.46, 47.60, 49.28, 61.20, 64.13, -41.29, 41.01, 89.9, -89.9)

	radii := []float64{defaultAttachRadiusKm, 1, 10, 120, 300}

	for _, radiusKm := range radii {
		for _, latDeg := range lats {
			baseCell := lookupCell(latDeg, 0)
			for _, fx := range fractions {
				for _, fy := range fractions {
					lat := (float64(baseCell.Y) + fy) * LookupCellDeg
					lon := (float64(baseCell.X) + fx) * LookupCellDeg
					if math.Abs(lat) > 90 {
						continue
					}
					probe := probeSet(lat, lon, radiusKm)

					for _, br := range bearings {
						dLat, dLon := destination(lat, lon, br, radiusKm)
						got := lookupCell(dLat, dLon)
						if _, ok := probe[got]; !ok {
							t.Fatalf("r=%.0f km, basis %.4f/%.4f (bearing %.0f): titik %.4f/%.4f di sel %v, DI LUAR probe (%d sel)",
								radiusKm, lat, lon, br, dLat, dLon, got, len(probe))
						}
					}
				}
			}
		}
	}
}

// Uji cakupan di atas harus benar-benar SENSITIF. Bentuk 3x3 yang lama — dan
// setiap lebar yang dipatok — GAGAL di lintang tinggi, dan itu dinyatakan di sini
// supaya "uji cakupan lulus" tidak dapat berarti "uji cakupan tidak mengukur apa
// pun".
func TestFixedThreeByThreeProbeWouldMissAtHighLatitude(t *testing.T) {
	inFixed3x3 := func(origin, c cellKey) bool {
		dx := wrapCellX(c.X - origin.X)
		dy := c.Y - origin.Y
		return dx >= -1 && dx <= 1 && dy >= -1 && dy <= 1
	}

	misses := 0
	for _, lat := range []float64{44.43, 45.46, 47.60, 49.28, 61.20, 64.13} {
		// Posisi di DALAM sel ikut disapu: sebuah titik tepat di batas sel barat
		// masih tertutup oleh 3x3 bahkan ketika sel sudah terlalu sempit, jadi
		// kasus terburuk (dekat tepi timur sel) yang membuktikannya.
		for _, fx := range []float64{0, 0.25, 0.5, 0.75, 0.999} {
			lon := fx * LookupCellDeg
			origin := lookupCell(lat, lon)
			// Tepat ke timur: sumbu bujur adalah yang menyusut dengan cos(lat).
			dLat, dLon := destination(lat, lon, 90, defaultAttachRadiusKm)
			if !inFixed3x3(origin, lookupCell(dLat, dLon)) {
				misses++
			}
			if _, ok := probeSet(lat, lon, defaultAttachRadiusKm)[lookupCell(dLat, dLon)]; !ok {
				t.Fatalf("lintang %.2f (fx %.3f): probe baru juga melewatkan titik 50 km ke timur", lat, fx)
			}
		}
	}
	if misses == 0 {
		t.Fatal("lingkungan 3x3 tetap tidak melewatkan apa pun di sini — uji cakupan tidak membuktikan D1")
	}
}

// Probe di bawah 41,5° harus IDENTIK dengan lingkungan 3x3 yang lama: perbaikan
// ini tidak boleh mengubah perilaku yang sudah benar (requirement D/E6).
func TestProbeIsUnchangedBelowOldLatitudeBound(t *testing.T) {
	for lat := -41.0; lat <= 41.0+1e-9; lat += 0.25 {
		nx, ny, allLon := probeSpan(lat, defaultAttachRadiusKm)
		if allLon || nx != 1 || ny != 1 {
			t.Fatalf("lintang %.2f: probe = %dx%d (allLon=%v), mau 1x1 seperti 3x3 lama", lat, nx, ny, allLon)
		}
	}
}

// Di atas |lat| ~= 41,5° probe HARUS melebar, dan pelebarannya monoton: sebuah
// probe yang tidak melebar di sana adalah cacat D1 yang kembali.
func TestProbeWidensAboveOldLatitudeBound(t *testing.T) {
	prev := int32(0)
	for _, lat := range []float64{42, 45.46, 49.28, 55, 61.2, 64.13, 70, 80} {
		nx, ny, _ := probeSpan(lat, defaultAttachRadiusKm)
		if nx < 2 {
			t.Errorf("lintang %.2f: nx = %d, mau >= 2", lat, nx)
		}
		if nx < prev {
			t.Errorf("lintang %.2f: nx = %d turun dari %d — pelebaran tidak monoton", lat, nx, prev)
		}
		prev = nx
		if ny != 1 {
			t.Errorf("lintang %.2f: ny = %d, mau 1 — sumbu lintang tidak bergantung lintang", lat, ny)
		}
	}
}

// Dekat kutub, lingkaran radius attach MEMUAT kutub, jadi setiap bujur ada di
// dalamnya. Dilaporkan sebagai allLon secara eksplisit — bukan dibiarkan menjadi
// pembagian oleh cos(lat) yang menuju nol, yang akan menghasilkan nx tak hingga
// atau NaN dan sebuah probe yang tidak menyelidiki apa pun.
func TestProbeSpanReportsPolarCaseExplicitly(t *testing.T) {
	for _, lat := range []float64{89.9, -89.9, 90, -90, 89.6} {
		nx, ny, allLon := probeSpan(lat, defaultAttachRadiusKm)
		if !allLon {
			t.Errorf("lintang %.2f: allLon = false, mau true", lat)
		}
		if nx != lonCellCount {
			t.Errorf("lintang %.2f: nx = %d, mau %d", lat, nx, lonCellCount)
		}
		if ny < 1 {
			t.Errorf("lintang %.2f: ny = %d, mau >= 1", lat, ny)
		}
	}
	// Dan nilainya berhingga: NaN atau Inf di sini akan menjadi probe kosong.
	for _, lat := range []float64{90, -90, 89.99999} {
		nx, ny, _ := probeSpan(lat, defaultAttachRadiusKm)
		if nx <= 0 || ny <= 0 {
			t.Errorf("lintang %.2f: probe %dx%d tidak berhingga positif", lat, nx, ny)
		}
	}
}

// Radius di luar keberlakuan bound TIDAK boleh menghasilkan probe yang sempit:
// asin(sin(r/R)/cos φ) berhenti menjadi batas atas di r > pi*R/2, dan config
// menolak nilai seperti itu — ini jaring untuk Tracker yang dibangun langsung
// oleh uji.
func TestProbeSpanClampsRadiusBeyondFormulaValidity(t *testing.T) {
	nx, ny, allLon := probeSpan(0, MaxAttachRadiusKm*2)
	if !allLon || nx != lonCellCount {
		t.Errorf("radius berlebih: nx=%d allLon=%v, mau seluruh cincin bujur", nx, allLon)
	}
	if ny*2+1 < int32(math.Ceil(180/LookupCellDeg)) {
		t.Errorf("radius berlebih: ny=%d, mau menutupi seluruh sumbu lintang", ny)
	}
}

// Antimeridian: sel 179,9° dan -179,9° BERTETANGGA, dan probe salah satunya harus
// memuat yang lain. Cacat aslinya adalah X = 299 lawan X = -300, yang tidak pernah
// dijembatani oleh probe berapa pun lebarnya tanpa pelipatan.
func TestProbeBridgesAntimeridian(t *testing.T) {
	east := lookupCell(0, 179.9)
	west := lookupCell(0, -179.9)
	if east == west {
		t.Fatal("179,9 dan -179,9 tidak boleh satu sel")
	}
	// Ke arah TIMUR dari sel 179,4°–180° adalah sel -180°–(-179,4°): selisih satu,
	// dan hanya terlihat satu setelah pelipatan. Tanpa pelipatan ia 599.
	if d := wrapCellX(west.X - east.X); d != 1 {
		t.Fatalf("selisih sel terlipat = %d, mau 1: keduanya bertetangga", d)
	}
	if raw := west.X - east.X; raw == 1 {
		t.Fatal("selisih mentah kebetulan 1 — uji ini tidak membuktikan pelipatan")
	}
	if _, ok := probeSet(0, 179.9, defaultAttachRadiusKm)[west]; !ok {
		t.Error("probe di 179,9 tidak memuat sel -179,9")
	}
	if _, ok := probeSet(0, -179.9, defaultAttachRadiusKm)[east]; !ok {
		t.Error("probe di -179,9 tidak memuat sel 179,9")
	}
}

// Sumbu bujur adalah LINGKARAN, jadi indeks sel harus terlipat: 360/0.60 = 600
// sel tepat, dan pembagian yang tidak bulat akan membuat dua sel bertumpang di
// meridian 180°.
func TestLonCellCountTilesTheCircle(t *testing.T) {
	if got := 360.0 / LookupCellDeg; got != float64(lonCellCount) {
		t.Fatalf("360/%.2f = %v, mau %d sel bujur tepat", LookupCellDeg, got, lonCellCount)
	}
	half := lonCellCount / 2
	for _, c := range []struct {
		in   int32
		want int32
	}{
		{0, 0}, {half - 1, half - 1}, {half, -half}, {-half, -half},
		{-half - 1, half - 1}, {lonCellCount, 0}, {-lonCellCount, 0},
	} {
		if got := wrapCellX(c.in); got != c.want {
			t.Errorf("wrapCellX(%d) = %d, mau %d", c.in, got, c.want)
		}
	}
	// Setiap bujur sah harus memetakan ke rentang terlipat kanonik.
	for lon := -180.0; lon < 180; lon += 0.37 {
		x := lookupCell(0, lon).X
		if x < -half || x >= half {
			t.Fatalf("bujur %.2f -> X = %d, di luar [%d, %d)", lon, x, -half, half)
		}
	}
}

// maxAttachRadiusKm di paket config MENCERMINKAN event.MaxAttachRadiusKm. Idiom
// mirror-plus-drift-test yang sama dengan TestMaxAcceptedTriggerAgeMirrorsIngest:
// paket config tidak boleh mengimpor internal/event, jadi yang menjaga keduanya
// adalah uji ini.
func TestMaxAttachRadiusMirrorsConfig(t *testing.T) {
	const configMirror = 10007.543398010286
	if math.Abs(MaxAttachRadiusKm-configMirror) > 1e-9 {
		t.Fatalf("event.MaxAttachRadiusKm = %v, config mirror = %v — perbarui keduanya",
			MaxAttachRadiusKm, configMirror)
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

// Label sel independensi (kini DESKRIPTIF, lihat independence.go) tetap dihitung
// dengan rumus yang sama, dan aritmetikanya dikunci di dalam uji supaya
// evidence_summary.cell_ids yang sudah tersimpan tetap berarti hal yang sama:
// pasangan
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
