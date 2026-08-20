package consensus

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/banana-pixel/quakealert/server/internal/store"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

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

// Skenario: kluster 3 node dengan PGA di bawah ambang MinPGAGal (16.6 gal)
// dianggap noise -> nil (bukan alert).
func TestEvaluatePGABelowThreshold(t *testing.T) {
	rs := []Reading{
		{NodeID: "n1", Lat: -6.20, Lon: 106.80, PGA: 5, TS: 1000},
		{NodeID: "n2", Lat: -6.25, Lon: 106.85, PGA: 8, TS: 1000},
		{NodeID: "n3", Lat: -6.30, Lon: 106.90, PGA: 12, TS: 1000},
	}
	if ev := Evaluate(rs, 2000); ev != nil {
		t.Fatalf("Evaluate(PGA rendah) = %v, want nil (noise)", ev)
	}
}

// Skenario: location_name event berasal dari node TERDEKAT dengan centroid
// (centroid tepat di posisi n2 -> label "B" yang dipakai).
func TestEvaluateLocationName(t *testing.T) {
	rs := []Reading{
		{NodeID: "n1", Lat: 0, Lon: 0, PGA: 100, TS: 1000, LocationName: "A"},
		{NodeID: "n2", Lat: 0.1, Lon: 0, PGA: 100, TS: 1000, LocationName: "B"},
		{NodeID: "n3", Lat: 0.2, Lon: 0, PGA: 100, TS: 1000, LocationName: "C"},
	}
	ev := Evaluate(rs, 2000)
	if ev == nil {
		t.Fatal("event nil")
	}
	// Centroid bobot merata = (0.1, 0) = posisi n2.
	if ev.LocationName != "B" {
		t.Fatalf("location_name = %q, want B", ev.LocationName)
	}
}

// --- fakeLocator: tanpa Postgres, Ingest dapat dipakai langsung di test ---

type fakeLocator struct {
	locs map[string]*store.NodeLocation
}

func (f *fakeLocator) GetNodeLocation(_ context.Context, id string) (*store.NodeLocation, error) {
	if l, ok := f.locs[id]; ok {
		return l, nil
	}
	return nil, store.ErrNodeNotFound
}

func threeNodeLocator() *fakeLocator {
	return &fakeLocator{locs: map[string]*store.NodeLocation{
		"n1": {StationID: "n1", Lat: -6.20, Lon: 106.80, LocationName: "Jakarta"},
		"n2": {StationID: "n2", Lat: -6.25, Lon: 106.85, LocationName: "Bandung"},
		"n3": {StationID: "n3", Lat: -6.30, Lon: 106.90, LocationName: "Bogor"},
	}}
}

func ingestThree(e *Engine, ctx context.Context, now time.Time) {
	e.Ingest(ctx, "n1", 200, now.UnixMilli())
	e.Ingest(ctx, "n2", 150, now.UnixMilli())
	e.Ingest(ctx, "n3", 180, now.UnixMilli())
}

// Skenario: emisi pertama ADVISORY (node 1) lalu eskalasi CONFIRMED (node 3);
// re-emisi dalam cooldown ditekan; setelah cooldown lewat, gelombang gempa baru
// boleh emisi lagi. Ini yang membuat event_id stabil (dispatcher hanya dipanggil
// sekali per gempa, tanpa spam re-emisi).
func TestEngineCooldownSuppressesReEmission(t *testing.T) {
	calls := 0
	e := NewEngine(8*time.Second, 90*time.Second, threeNodeLocator(),
		func(_ context.Context, _ *Event) { calls++ }, testLogger())

	now := time.UnixMilli(1_700_000_000_000)
	e.now = func() time.Time { return now }
	ctx := context.Background()

	// Gelombang pertama: node 1 -> ADVISORY, node 3 -> eskalasi CONFIRMED.
	ingestThree(e, ctx, now)
	if calls != 2 {
		t.Fatalf("emisi gelombang pertama = %d, want 2 (ADVISORY + CONFIRMED)", calls)
	}

	// Re-trigger dalam cooldown (30s kemudian, window 8s sudah ter-prune):
	// tidak ada emisi baru sama sekali.
	now = now.Add(30 * time.Second)
	ingestThree(e, ctx, now)
	if calls != 2 {
		t.Fatalf("re-emisi dalam cooldown = %d, want tetap 2", calls)
	}

	// Setelah cooldown (90s) lewat: gelombang baru kembali emisi.
	now = now.Add(90 * time.Second)
	ingestThree(e, ctx, now)
	if calls != 4 {
		t.Fatalf("emisi setelah cooldown = %d, want 4", calls)
	}
}

// Skenario: cooldown bersifat PER-SEL spasial. Gempa di wilayah berbeda (sel
// grid berbeda) tidak saling menekan — dua gempa CONFIRMED di dua wilayah
// dalam jendela yang sama sama-sama diemisi (mencegah false-negative
// multi-region), sementara re-emisi di sel yang sama tetap di-dedup.
func TestEngineCooldownPerCell(t *testing.T) {
	calls := 0
	loc := &fakeLocator{locs: map[string]*store.NodeLocation{
		// Wilayah A: sekitar (-6, 107).
		"a1": {StationID: "a1", Lat: -6.20, Lon: 106.80, LocationName: "Jakarta"},
		"a2": {StationID: "a2", Lat: -6.25, Lon: 106.85, LocationName: "Bandung"},
		"a3": {StationID: "a3", Lat: -6.30, Lon: 106.90, LocationName: "Bogor"},
		// Wilayah B: sekitar (1, 120) — sel grid berbeda.
		"b1": {StationID: "b1", Lat: 1.20, Lon: 120.10, LocationName: "Manado"},
		"b2": {StationID: "b2", Lat: 1.25, Lon: 120.15, LocationName: "Gorontalo"},
		"b3": {StationID: "b3", Lat: 1.30, Lon: 120.20, LocationName: "Palu"},
	}}
	e := NewEngine(8*time.Second, 90*time.Second, loc,
		func(_ context.Context, _ *Event) { calls++ }, testLogger())

	now := time.UnixMilli(1_700_000_000_000)
	e.now = func() time.Time { return now }
	ctx := context.Background()

	ingest := func(ids ...string) {
		for _, id := range ids {
			e.Ingest(ctx, id, 200, now.UnixMilli())
		}
	}

	// Gempa wilayah A -> emisi (ADVISORY + eskalasi CONFIRMED).
	ingest("a1", "a2", "a3")
	if calls != 2 {
		t.Fatalf("wilayah A = %d emisi, want 2", calls)
	}

	// Gempa wilayah B, 10s kemudian (masih dalam cooldown global 90s):
	// sel berbeda -> tetap emisi penuh.
	now = now.Add(10 * time.Second)
	ingest("b1", "b2", "b3")
	if calls != 4 {
		t.Fatalf("wilayah B dalam cooldown global = %d emisi, want 4 (per-sel)", calls)
	}

	// Re-emisi wilayah A dalam cooldown sel-nya -> ditekan.
	now = now.Add(10 * time.Second)
	ingest("a1", "a2", "a3")
	if calls != 4 {
		t.Fatalf("re-emisi wilayah A = %d, want tetap 4", calls)
	}
}

// Skenario: ADVISORY (2 node) boleh naik eskalasi menjadi CONFIRMED saat node
// ke-3 masuk dalam cooldown; re-emisi CONFIRMED berikutnya tetap ditekan.
func TestEngineCooldownAllowsEscalation(t *testing.T) {
	var lastStatus Status
	calls := 0
	e := NewEngine(8*time.Second, 90*time.Second, threeNodeLocator(),
		func(_ context.Context, ev *Event) { calls++; lastStatus = ev.Status }, testLogger())

	now := time.UnixMilli(1_700_000_000_000)
	e.now = func() time.Time { return now }
	ctx := context.Background()

	// 2 node -> ADVISORY (emisi 1).
	e.Ingest(ctx, "n1", 100, now.UnixMilli())
	e.Ingest(ctx, "n2", 120, now.UnixMilli())
	if calls != 1 || lastStatus != StatusAdvisory {
		t.Fatalf("advisory = calls %d status %s, want 1/ADVISORY", calls, lastStatus)
	}

	// Node ke-3 masuk 5s kemudian -> eskalasi CONFIRMED diizinkan (emisi 2).
	now = now.Add(5 * time.Second)
	e.Ingest(ctx, "n3", 180, now.UnixMilli())
	if calls != 2 || lastStatus != StatusConfirmed {
		t.Fatalf("eskalasi = calls %d status %s, want 2/CONFIRMED", calls, lastStatus)
	}

	// Re-emisi CONFIRMED dalam cooldown -> ditekan.
	now = now.Add(10 * time.Second)
	ingestThree(e, ctx, now)
	if calls != 2 {
		t.Fatalf("re-emisi CONFIRMED = %d, want tetap 2", calls)
	}
}
