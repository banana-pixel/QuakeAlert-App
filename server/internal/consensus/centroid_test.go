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
