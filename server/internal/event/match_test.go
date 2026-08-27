package event

import (
	"math"
	"testing"

	"github.com/banana-pixel/quakealert/server/internal/consensus"
)

const (
	testWindowMs = int64(20000)
	testRadiusKm = 50.0
)

// eventAt membangun event dengan satu kontributor di (lat,lon) dan origin_ts
// tertentu, sehingga centroid()-nya tepat titik itu.
func eventAt(originTS int64, lat, lon float64) *Event {
	return &Event{
		ID:       "e1",
		State:    StateUnconfirmed,
		OriginTS: originTS,
		Contributors: map[string]*Contributor{
			"NODE-00000001": {NodeID: "NODE-00000001", Lat: lat, Lon: lon, PeakPGA: 100, OnsetTS: originTS},
		},
		minCells: 2,
	}
}

// Kedua batas predikat §4.3 TERTUTUP. Diuji terpisah dari isi karena tepat di
// batas itulah satu gempa terbelah menjadi dua event bila tandanya salah.
func TestMatchWindowBoundsAreClosed(t *testing.T) {
	const origin = int64(1_700_000_000_000)
	e := eventAt(origin, -6.9, 107.6)

	cases := []struct {
		name    string
		onsetTS int64
		want    bool
	}{
		{"tepat di batas awal", origin - testWindowMs, true},
		{"tepat di batas akhir", origin + testWindowMs, true},
		{"satu ms sebelum batas awal", origin - testWindowMs - 1, false},
		{"satu ms setelah batas akhir", origin + testWindowMs + 1, false},
		{"satu ms di dalam batas awal", origin - testWindowMs + 1, true},
		{"satu ms di dalam batas akhir", origin + testWindowMs - 1, true},
		{"identik", origin, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := matches(e, tc.onsetTS, e.Contributors["NODE-00000001"].Lat,
				e.Contributors["NODE-00000001"].Lon, testWindowMs, testRadiusKm)
			if got != tc.want {
				t.Errorf("matches(d=%dms) = %v, mau %v", tc.onsetTS-origin, got, tc.want)
			}
		})
	}
}

// Jendela itu SIMETRIS: onset yang lebih AWAL dari origin harus cocok sama
// jauhnya dengan yang lebih akhir. Ini yang membuat observasi tak berurut tidak
// membelah satu gempa (R-C1).
func TestMatchWindowIsSymmetric(t *testing.T) {
	const origin = int64(1_700_000_000_000)
	e := eventAt(origin, -6.9, 107.6)
	for d := int64(1); d <= testWindowMs; d += 997 {
		early := matches(e, origin-d, -6.9, 107.6, testWindowMs, testRadiusKm)
		late := matches(e, origin+d, -6.9, 107.6, testWindowMs, testRadiusKm)
		if early != late {
			t.Fatalf("d=%d asimetris: lebih awal=%v lebih akhir=%v", d, early, late)
		}
		if !early {
			t.Fatalf("d=%d (<= jendela) seharusnya cocok", d)
		}
	}
}

// destinasi great-circle: titik pada jarak km dari (lat,lon) dengan bearing
// derajat. Dipakai untuk menaruh titik uji TEPAT di radius.
func destinationKm(lat, lon, km, bearingDeg float64) (float64, float64) {
	const R = 6371.0
	φ1 := lat * math.Pi / 180
	λ1 := lon * math.Pi / 180
	θ := bearingDeg * math.Pi / 180
	δ := km / R
	φ2 := math.Asin(math.Sin(φ1)*math.Cos(δ) + math.Cos(φ1)*math.Sin(δ)*math.Cos(θ))
	λ2 := λ1 + math.Atan2(math.Sin(θ)*math.Sin(δ)*math.Cos(φ1), math.Cos(δ)-math.Sin(φ1)*math.Sin(φ2))
	return φ2 * 180 / math.Pi, λ2 * 180 / math.Pi
}

// Batas radius juga TERTUTUP. Diuji dengan toleransi eksplisit: haversine dan
// destinationKm memakai R yang sama, jadi selisihnya hanya pembulatan float —
// karena itu titik "tepat di radius" diuji sedikit di dalam dan sedikit di luar,
// dan jarak sebenarnya dilaporkan bila gagal.
func TestMatchRadiusBoundsAreClosed(t *testing.T) {
	const origin = int64(1_700_000_000_000)
	e := eventAt(origin, -6.9, 107.6)
	c := e.centroid()

	for _, bearing := range []float64{0, 45, 90, 135, 180, 225, 270, 315} {
		for _, tc := range []struct {
			name string
			km   float64
			want bool
		}{
			{"di pusat", 0, true},
			{"jauh di dalam", testRadiusKm / 2, true},
			{"tepat di dalam batas", testRadiusKm - 0.001, true},
			{"tepat di luar batas", testRadiusKm + 0.001, false},
			{"jauh di luar", testRadiusKm * 2, false},
		} {
			lat, lon := destinationKm(c.Lat, c.Lon, tc.km, bearing)
			got := matches(e, origin, lat, lon, testWindowMs, testRadiusKm)
			if got != tc.want {
				d := consensus.HaversineKm(lat, lon, c.Lat, c.Lon)
				t.Errorf("bearing %.0f° %s (%.3f km, terukur %.6f km): matches = %v, mau %v",
					bearing, tc.name, tc.km, d, got, tc.want)
			}
		}
	}
}

// Kedua syarat itu KONJUNGSI: gagal salah satu berarti tidak cocok.
func TestMatchRequiresBothWindowAndRadius(t *testing.T) {
	const origin = int64(1_700_000_000_000)
	e := eventAt(origin, -6.9, 107.6)
	c := e.centroid()
	near, nearLon := destinationKm(c.Lat, c.Lon, 10, 90)
	far, farLon := destinationKm(c.Lat, c.Lon, 500, 90)

	if !matches(e, origin, near, nearLon, testWindowMs, testRadiusKm) {
		t.Fatal("dekat dan dalam jendela seharusnya cocok")
	}
	if matches(e, origin, far, farLon, testWindowMs, testRadiusKm) {
		t.Error("dalam jendela tetapi jauh: tidak boleh cocok")
	}
	if matches(e, origin+testWindowMs+1, near, nearLon, testWindowMs, testRadiusKm) {
		t.Error("dekat tetapi di luar jendela: tidak boleh cocok")
	}
	if matches(e, origin+testWindowMs+1, far, farLon, testWindowMs, testRadiusKm) {
		t.Error("jauh dan di luar jendela: tidak boleh cocok")
	}
}

// Event terminal TETAP cocok. Itu bukan kelalaian: tombstone (§6.8) bergantung
// padanya untuk menyerap bukti yang terlambat, dan classify-lah yang tidak
// dijalankan — bukan predikat ini yang menolak.
func TestMatchIgnoresTerminalState(t *testing.T) {
	const origin = int64(1_700_000_000_000)
	for _, st := range []State{StateDetected, StateUnconfirmed, StateConfirmed, StateResolved, StateCancelled} {
		e := eventAt(origin, -6.9, 107.6)
		e.State = st
		if !matches(e, origin+1000, -6.9, 107.6, testWindowMs, testRadiusKm) {
			t.Errorf("state %s: predikat kecocokan tidak boleh bergantung pada state", st)
		}
	}
}

// Centroid bergerak seiring kontributor bertambah, dan predikat mengukur
// terhadap centroid SAAT INI — bukan terhadap kontributor pertama.
func TestMatchMeasuresAgainstCurrentCentroid(t *testing.T) {
	const origin = int64(1_700_000_000_000)
	e := eventAt(origin, -6.9, 107.6)

	// Titik 80 km ke timur: di luar radius terhadap satu kontributor.
	lat, lon := destinationKm(-6.9, 107.6, 80, 90)
	if matches(e, origin, lat, lon, testWindowMs, testRadiusKm) {
		t.Fatal("80 km dari satu-satunya kontributor seharusnya di luar radius 50 km")
	}

	// Tambahkan kontributor ber-PGA sama 90 km ke timur; centroid bergeser ~45 km
	// ke timur sehingga titik yang sama kini berada ~35 km dari centroid — masuk,
	// dan tidak di batas, supaya uji ini menguji pergeseran dan bukan pembulatan.
	mLat, mLon := destinationKm(-6.9, 107.6, 90, 90)
	e.Contributors["NODE-00000002"] = &Contributor{
		NodeID: "NODE-00000002", Lat: mLat, Lon: mLon, PeakPGA: 100, OnsetTS: origin,
	}
	if !matches(e, origin, lat, lon, testWindowMs, testRadiusKm) {
		c := e.centroid()
		t.Errorf("setelah centroid bergeser ke (%.4f,%.4f), titik %.1f km dari centroid seharusnya masuk",
			c.Lat, c.Lon, consensus.HaversineKm(lat, lon, c.Lat, c.Lon))
	}
}
