package consensus

import "testing"

// Skenario: 3 node berdekatan (< 50 km) -> CONFIRMED.
func TestEvaluateConfirmed(t *testing.T) {
	rs := []Reading{
		{NodeID: "n1", Lat: -6.20, Lon: 106.80, PGA: 200, TS: 1000},
		{NodeID: "n2", Lat: -6.25, Lon: 106.85, PGA: 150, TS: 1000},
		{NodeID: "n3", Lat: -6.30, Lon: 106.90, PGA: 180, TS: 1000},
	}
	ev := Evaluate(rs, 2000)
	if ev == nil {
		t.Fatal("event nil, want CONFIRMED")
	}
	if ev.Status != StatusConfirmed {
		t.Fatalf("status = %s, want CONFIRMED", ev.Status)
	}
	if ev.NodeCount != 3 {
		t.Fatalf("node count = %d, want 3", ev.NodeCount)
	}
	if ev.MaxPGA != 200 {
		t.Fatalf("max pga = %v, want 200", ev.MaxPGA)
	}
}

// Skenario: 2 node berdekatan -> ADVISORY.
func TestEvaluateAdvisory(t *testing.T) {
	rs := []Reading{
		{NodeID: "n1", Lat: -6.20, Lon: 106.80, PGA: 50, TS: 1000},
		{NodeID: "n2", Lat: -6.25, Lon: 106.85, PGA: 60, TS: 1000},
	}
	ev := Evaluate(rs, 2000)
	if ev == nil || ev.Status != StatusAdvisory {
		t.Fatalf("status = %v, want ADVISORY", ev)
	}
}

// Skenario: 3 node tapi 1 node berjauhan (> 50 km) -> kluster terbesar hanya
// 2 node -> ADVISORY (node jauh tidak menaikkan konsensus jadi CONFIRMED).
func TestEvaluateSpatialSeparation(t *testing.T) {
	rs := []Reading{
		{NodeID: "n1", Lat: -6.20, Lon: 106.80, PGA: 100, TS: 1000},
		{NodeID: "n2", Lat: -6.25, Lon: 106.85, PGA: 100, TS: 1000},
		// Surabaya ~660 km dari Jakarta: klaster terpisah.
		{NodeID: "far", Lat: -7.25, Lon: 112.75, PGA: 500, TS: 1000},
	}
	ev := Evaluate(rs, 2000)
	if ev == nil {
		t.Fatal("event nil")
	}
	if ev.Status != StatusAdvisory {
		t.Fatalf("status = %s, want ADVISORY (klaster terbesar 2 node)", ev.Status)
	}
	if ev.NodeCount != 2 {
		t.Fatalf("node count = %d, want 2", ev.NodeCount)
	}
}

func TestEvaluateEmpty(t *testing.T) {
	if ev := Evaluate(nil, 0); ev != nil {
		t.Fatalf("Evaluate(nil) = %v, want nil", ev)
	}
}
