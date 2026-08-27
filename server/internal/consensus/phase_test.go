package consensus

import (
	"context"
	"testing"
	"time"
)

// TestPrelimAndFinalCountAsOneNode adalah D7: satu node yang memublikasikan
// PRELIM lalu FINAL untuk event yang SAMA harus tetap dihitung satu node oleh
// konsensus — bukan dua. Bila tidak, ambang 3-node CONFIRMED dapat dicapai oleh
// dua perangkat saja, dan protokol v2 sendiri menjadi cara memalsukan konsensus.
//
// Yang menjamin sifat itu adalah bentuk masukannya, bukan sebuah cabang if:
// readings dikunci per node_id, dan Ingest tidak menerima phase sama sekali.
// Test ini mengunci keduanya pada perilaku yang teramati.
func TestPrelimAndFinalCountAsOneNode(t *testing.T) {
	var events []*Event
	e := NewEngine(8*time.Second, 90*time.Second, threeNodeLocator(),
		func(_ context.Context, ev *Event) { events = append(events, ev) }, testLogger())

	now := time.UnixMilli(1_700_000_000_000)
	e.now = func() time.Time { return now }
	ctx := context.Background()

	// n1 melaporkan PRELIM (puncak sejauh ini), lalu FINAL (puncak sebenarnya).
	e.Ingest(ctx, "n1", 120, now.UnixMilli())
	e.Ingest(ctx, "n1", 200, now.Add(2500*time.Millisecond).UnixMilli())
	// n2 melaporkan sekali. Dua PERANGKAT, tiga laporan.
	e.Ingest(ctx, "n2", 150, now.UnixMilli())

	// Eskalasi ke CONFIRMED diizinkan bahkan di dalam cooldown, jadi bila
	// PRELIM+FINAL terhitung dua node, ambang 3-node sudah tercapai di sini dan
	// sebuah CONFIRMED akan muncul dari dua perangkat saja.
	for _, ev := range events {
		if ev.Status == StatusConfirmed {
			t.Fatalf("CONFIRMED diemisi oleh dua perangkat (node_count=%d)", ev.NodeCount)
		}
	}

	// Perangkat ketiga menutup ambang; sekarang barulah CONFIRMED sah.
	e.Ingest(ctx, "n3", 180, now.UnixMilli())
	last := events[len(events)-1]
	if last.Status != StatusConfirmed || last.NodeCount != 3 {
		t.Fatalf("status = %v node_count = %d, want CONFIRMED 3", last.Status, last.NodeCount)
	}
}

// TestPhaseEscalationKeepsPeakPGA: dari kedua laporan satu node, yang bertahan
// di window adalah PGA TERTINGGI. PRELIM membawa puncak SEJAUH INI dan FINAL
// membawa puncak sebenarnya, jadi urutan kedatangan tidak boleh menurunkan nilai
// yang sudah diketahui — sebuah PRELIM yang diulang setelah FINAL tidak dapat
// mengecilkan kembali guncangan yang sudah terukur.
func TestPhaseEscalationKeepsPeakPGA(t *testing.T) {
	var last *Event
	e := NewEngine(8*time.Second, 90*time.Second, threeNodeLocator(),
		func(_ context.Context, ev *Event) { last = ev }, testLogger())

	now := time.UnixMilli(1_700_000_000_000)
	e.now = func() time.Time { return now }
	ctx := context.Background()

	e.Ingest(ctx, "n1", 200, now.UnixMilli())   // FINAL: puncak
	e.Ingest(ctx, "n1", 120, now.UnixMilli()+1) // PRELIM diulang: lebih kecil
	if last == nil {
		t.Fatal("tidak ada event yang diemisi")
	}
	if last.MaxPGA != 200 {
		t.Fatalf("max_pga = %v, want 200 (puncak dipertahankan)", last.MaxPGA)
	}
}
