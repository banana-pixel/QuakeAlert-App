package consensus

import (
	"math"
	"testing"
)

func TestHaversineKm(t *testing.T) {
	// Jarak Jakarta (-6.2088,106.8456) ke Bandung (-6.9175,107.6191)
	// kira-kira ~116 km (great-circle). Toleransi 2 km.
	d := haversineKm(-6.2088, 106.8456, -6.9175, 107.6191)
	if math.Abs(d-116.2) > 2 {
		t.Fatalf("haversine Jakarta-Bandung = %.2f km, want ~116 km", d)
	}

}

func TestHaversineZero(t *testing.T) {
	if d := haversineKm(-6.2, 106.8, -6.2, 106.8); d != 0 {
		t.Fatalf("jarak titik identik = %v, want 0", d)
	}
}
