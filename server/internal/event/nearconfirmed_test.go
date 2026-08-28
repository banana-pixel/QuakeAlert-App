package event

import (
	"context"
	"testing"
	"time"
)

// TestNearConfirmedEmptyOnFreshTracker — log kosong sebelum ada trigger.
func TestNearConfirmedEmptyOnFreshTracker(t *testing.T) {
	h := newHarness(t)
	if got := h.trk.NearConfirmedLog(); len(got) != 0 {
		t.Errorf("NearConfirmedLog = %d entri, mau 0", len(got))
	}
}

// TestNearConfirmedSingleContributorNeverRecorded — satu kontributor TIDAK masuk
// log, bahkan setelah UNCONFIRMED. Ambang independensi minimum adalah 2.
func TestNearConfirmedSingleContributorNeverRecorded(t *testing.T) {
	h := newHarness(t)
	h.node("N1", baseLat, baseLon)

	h.ingest(v2("N1", MinPGAGal+10, onsetBase, PhasePrelim, 1))

	e := h.only()
	if e.State != StateUnconfirmed {
		t.Fatalf("state = %s, mau UNCONFIRMED", e.State)
	}
	// Satu kontributor = satu sel independen = di bawah ambang 2.
	if got := h.trk.NearConfirmedLog(); len(got) != 0 {
		t.Errorf("NearConfirmedLog = %d entri, mau 0: satu kontributor tidak cukup", len(got))
	}
}

// TestNearConfirmedTwoIndependentRecorded — dua kontributor independen masuk log.
func TestNearConfirmedTwoIndependentRecorded(t *testing.T) {
	h := newHarness(t)
	h.node("N1", baseLat, baseLon)
	h.nodeAt("N2", baseLat, baseLon, 8, 90) // 8 km > IndependenceCellKm=5

	h.ingest(v2("N1", MinPGAGal+10, onsetBase, PhasePrelim, 1))
	h.ingest(v2("N2", MinPGAGal+10, onsetBase+500, PhasePrelim, 1))

	log := h.trk.NearConfirmedLog()
	if len(log) != 1 {
		t.Fatalf("NearConfirmedLog = %d entri, mau 1", len(log))
	}
	entry := log[0]
	if entry.FirstTwoIndependentAt == 0 {
		t.Error("FirstTwoIndependentAt = 0, mau > 0")
	}
	if entry.IndependentCountAtPeak < 2 {
		t.Errorf("IndependentCountAtPeak = %d, mau >= 2", entry.IndependentCountAtPeak)
	}
	if entry.NodeCountAtPeak < 2 {
		t.Errorf("NodeCountAtPeak = %d, mau >= 2", entry.NodeCountAtPeak)
	}
}

// TestNearConfirmedConfirmedEventHasConfirmedAt — event yang CONFIRMED mendapat
// ConfirmedAt > 0 dan tidak dihitung "stalled".
func TestNearConfirmedConfirmedEventHasConfirmedAt(t *testing.T) {
	h := newHarness(t)
	h.threeNodeCluster()

	h.ingest(v2("N1", MinPGAGal+10, onsetBase, PhasePrelim, 1))
	h.ingest(v2("N2", MinPGAGal+10, onsetBase+500, PhasePrelim, 1))
	h.ingest(v2("N3", MinPGAGal+10, onsetBase+1000, PhasePrelim, 1))

	e := h.only()
	if e.State != StateConfirmed {
		t.Fatalf("state = %s, mau CONFIRMED", e.State)
	}

	log := h.trk.NearConfirmedLog()
	if len(log) != 1 {
		t.Fatalf("NearConfirmedLog = %d entri, mau 1", len(log))
	}
	entry := log[0]
	if entry.ConfirmedAt == 0 {
		t.Error("ConfirmedAt = 0, mau > 0: event sudah CONFIRMED")
	}
	if entry.EventID != e.ID {
		t.Errorf("EventID = %q, mau %q", entry.EventID, e.ID)
	}
	// Masih terbuka: TerminalAt belum terisi.
	if entry.TerminalAt != 0 {
		t.Errorf("TerminalAt = %d, mau 0: event masih terbuka", entry.TerminalAt)
	}
}

// TestNearConfirmedResolvedWithoutConfirmation — event yang RESOLVED tanpa pernah
// CONFIRMED: ConfirmedAt == 0 dan TerminalState == "RESOLVED".
func TestNearConfirmedResolvedWithoutConfirmation(t *testing.T) {
	h := newHarness(t)
	h.node("N1", baseLat, baseLon)
	h.nodeAt("N2", baseLat, baseLon, 8, 90)

	// Dua node independen, tapi hanya dua — kuorum butuh 3 (MinNodesConfirmed).
	h.ingest(v2("N1", MinPGAGal+10, onsetBase, PhasePrelim, 1))
	h.ingest(v2("N2", MinPGAGal+10, onsetBase+500, PhasePrelim, 1))

	e := h.only()
	if e.State != StateUnconfirmed {
		t.Fatalf("state = %s, mau UNCONFIRMED: dua node tidak cukup untuk CONFIRMED", e.State)
	}

	// Sweep menutup event.
	h.clock.advance(time.Duration(defaultOptions().ResolveAfterMs+1) * time.Millisecond)
	h.trk.sweep(context.Background())

	if e.State != StateResolved {
		t.Fatalf("state = %s, mau RESOLVED setelah sweep", e.State)
	}

	log := h.trk.NearConfirmedLog()
	if len(log) != 1 {
		t.Fatalf("NearConfirmedLog = %d entri, mau 1", len(log))
	}
	entry := log[0]

	// Tidak pernah CONFIRMED.
	if entry.ConfirmedAt != 0 {
		t.Errorf("ConfirmedAt = %d, mau 0: event tidak pernah CONFIRMED", entry.ConfirmedAt)
	}
	// Terminal sebagai RESOLVED.
	if entry.TerminalState != string(StateResolved) {
		t.Errorf("TerminalState = %q, mau %q", entry.TerminalState, StateResolved)
	}
	if entry.TerminalAt == 0 {
		t.Error("TerminalAt = 0, mau > 0: event sudah terminal")
	}
	// Durasi stall = TerminalAt - FirstTwoIndependentAt.
	stallMs := entry.TerminalAt - entry.FirstTwoIndependentAt
	if stallMs <= 0 {
		t.Errorf("stallMs = %d, mau > 0", stallMs)
	}
}

// TestNearConfirmedConfirmedThenResolvedHasBothTimestamps — event CONFIRMED lalu
// RESOLVED memiliki ConfirmedAt DAN TerminalAt.
func TestNearConfirmedConfirmedThenResolvedHasBothTimestamps(t *testing.T) {
	h := newHarness(t)
	e := h.confirmThreeNodes()

	// Sweep menutup event.
	h.clock.advance(time.Duration(defaultOptions().ResolveAfterMs+1) * time.Millisecond)
	h.trk.sweep(context.Background())

	if e.State != StateResolved {
		t.Fatalf("state = %s, mau RESOLVED", e.State)
	}

	log := h.trk.NearConfirmedLog()
	if len(log) != 1 {
		t.Fatalf("NearConfirmedLog = %d entri, mau 1", len(log))
	}
	entry := log[0]

	if entry.ConfirmedAt == 0 {
		t.Error("ConfirmedAt = 0, mau > 0")
	}
	if entry.TerminalState != string(StateResolved) {
		t.Errorf("TerminalState = %q, mau RESOLVED", entry.TerminalState)
	}
	if entry.TerminalAt == 0 {
		t.Error("TerminalAt = 0, mau > 0")
	}
	if entry.ConfirmedAt > entry.TerminalAt {
		t.Errorf("ConfirmedAt %d > TerminalAt %d: urutan tidak masuk akal", entry.ConfirmedAt, entry.TerminalAt)
	}
}

// TestNearConfirmedCancelledWithoutConfirmation — event yang CANCELLED tanpa
// pernah CONFIRMED karena invalidasi kontributor.
func TestNearConfirmedCancelledWithoutConfirmation(t *testing.T) {
	h := newHarness(t)
	h.node("N1", baseLat, baseLon)
	h.nodeAt("N2", baseLat, baseLon, 8, 90)

	h.ingest(v2("N1", MinPGAGal+10, onsetBase, PhasePrelim, 1))
	h.ingest(v2("N2", MinPGAGal+10, onsetBase+500, PhasePrelim, 1))

	e := h.only()
	if e.State != StateUnconfirmed {
		t.Fatalf("state = %s, mau UNCONFIRMED", e.State)
	}

	// Kedua kontributor diinvalidasi.
	h.trk.InvalidateContributor(context.Background(), "N1", "")
	h.trk.InvalidateContributor(context.Background(), "N2", "")

	if e.State != StateCancelled {
		t.Fatalf("state = %s, mau CANCELLED", e.State)
	}

	log := h.trk.NearConfirmedLog()
	if len(log) != 1 {
		t.Fatalf("NearConfirmedLog = %d entri, mau 1", len(log))
	}
	entry := log[0]

	if entry.ConfirmedAt != 0 {
		t.Errorf("ConfirmedAt = %d, mau 0", entry.ConfirmedAt)
	}
	if entry.TerminalState != string(StateCancelled) {
		t.Errorf("TerminalState = %q, mau CANCELLED", entry.TerminalState)
	}
}

// TestNearConfirmedDetectedOnlyNeverRecorded — event yang tidak pernah melewati
// lantai PGA (tetap DETECTED) tidak masuk log meski punya banyak kontributor
// independen secara spasial.
func TestNearConfirmedDetectedOnlyNeverRecorded(t *testing.T) {
	h := newHarness(t)
	h.node("N1", baseLat, baseLon)
	h.nodeAt("N2", baseLat, baseLon, 8, 90)
	h.nodeAt("N3", baseLat, baseLon, 16, 90)

	// PGA di bawah MinPGAGal: tetap DETECTED.
	h.ingest(v2("N1", MinPGAGal-0.1, onsetBase, PhasePrelim, 1))
	h.ingest(v2("N2", MinPGAGal-0.1, onsetBase+500, PhasePrelim, 1))
	h.ingest(v2("N3", MinPGAGal-0.1, onsetBase+1000, PhasePrelim, 1))

	e := h.only()
	if e.State != StateDetected {
		t.Fatalf("state = %s, mau DETECTED", e.State)
	}

	// DETECTED tidak pernah masuk near-confirmed log karena classify()
	// mengembalikan DETECTED sebelum transisi forceTransition dipanggil,
	// sehingga recordNearConfirmedLocked tidak terpanggil.
	if got := h.trk.NearConfirmedLog(); len(got) != 0 {
		t.Errorf("NearConfirmedLog = %d entri, mau 0: DETECTED tidak publik", len(got))
	}
}

// TestNearConfirmedLogSortedByFirstTwoIndependentAt — entri diurutkan ascending.
func TestNearConfirmedLogSortedByFirstTwoIndependentAt(t *testing.T) {
	// Dua event terpisah; onset kedua jauh di luar jendela korelasi pertama.
	h := newHarness(t)
	h.node("N1", baseLat, baseLon)
	h.nodeAt("N2", baseLat, baseLon, 8, 90)
	// Event B — onset sangat jauh lebih awal dalam waktu simulasi.
	h.nodeAt("N3", baseLat+5, baseLon, 0, 0)  // cluster B node 1
	h.nodeAt("N4", baseLat+5, baseLon, 8, 90) // cluster B node 2

	onsetA := onsetBase
	onsetB := onsetBase + int64(defaultOptions().CorrelationWindowMs)*3

	h.ingest(v2("N1", MinPGAGal+10, onsetA, PhasePrelim, 1))
	h.ingest(v2("N2", MinPGAGal+10, onsetA+500, PhasePrelim, 1))

	h.ingest(v2("N3", MinPGAGal+10, onsetB, PhasePrelim, 1))
	h.ingest(v2("N4", MinPGAGal+10, onsetB+500, PhasePrelim, 1))

	log := h.trk.NearConfirmedLog()
	if len(log) != 2 {
		t.Fatalf("NearConfirmedLog = %d entri, mau 2", len(log))
	}
	if log[0].FirstTwoIndependentAt > log[1].FirstTwoIndependentAt {
		t.Errorf("urutan salah: log[0].FirstTwoIndependentAt %d > log[1] %d",
			log[0].FirstTwoIndependentAt, log[1].FirstTwoIndependentAt)
	}
}

// TestNearConfirmedSurvivedTombstoneEviction — entri di log bertahan setelah
// tombstone dievakuasi dari Tracker map. Near-confirmed log bersifat append-only
// dan tidak terikat pada umur tombstone.
func TestNearConfirmedSurvivedTombstoneEviction(t *testing.T) {
	h := newHarness(t)
	e := h.confirmThreeNodes()

	// Sweep menutup event.
	h.clock.advance(time.Duration(defaultOptions().ResolveAfterMs+1) * time.Millisecond)
	h.trk.sweep(context.Background())

	if e.State != StateResolved {
		t.Fatalf("state = %s, mau RESOLVED", e.State)
	}

	// Lewati retensi tombstone: tombstone dievakuasi dari map.
	h.clock.advance(time.Duration(defaultOptions().TerminalRetentionMs+1) * time.Millisecond)
	h.trk.sweep(context.Background())

	// Map events sekarang kosong.
	if n := len(h.events()); n != 0 {
		t.Errorf("event terlacak = %d, mau 0 setelah tombstone dievakuasi", n)
	}

	// Near-confirmed log TETAP berisi entri.
	log := h.trk.NearConfirmedLog()
	if len(log) != 1 {
		t.Fatalf("NearConfirmedLog = %d entri, mau 1 setelah evakuasi tombstone", len(log))
	}
	if log[0].TerminalState != string(StateResolved) {
		t.Errorf("TerminalState = %q, mau RESOLVED", log[0].TerminalState)
	}
}

// TestNearConfirmedOperatorQuestions — verifikasi bahwa keempat pertanyaan
// operator dapat dijawab dari NearConfirmedLog.
//
// Skenario: tiga event.
//   - Event A: dua independen, tidak pernah CONFIRMED, RESOLVED.
//   - Event B: tiga independen, CONFIRMED, RESOLVED.
//   - Event C: dua independen, masih terbuka.
func TestNearConfirmedOperatorQuestions(t *testing.T) {
	// Event A — dua node, tidak CONFIRMED.
	hA := newHarness(t)
	hA.node("N1", baseLat, baseLon)
	hA.nodeAt("N2", baseLat, baseLon, 8, 90)
	hA.ingest(v2("N1", MinPGAGal+10, onsetBase, PhasePrelim, 1))
	hA.ingest(v2("N2", MinPGAGal+10, onsetBase+500, PhasePrelim, 1))
	hA.clock.advance(time.Duration(defaultOptions().ResolveAfterMs+1) * time.Millisecond)
	hA.trk.sweep(context.Background())

	logA := hA.trk.NearConfirmedLog()
	if len(logA) != 1 {
		t.Fatalf("A: log = %d entri, mau 1", len(logA))
	}
	eA := logA[0]

	// Event B — tiga node, CONFIRMED.
	hB := newHarness(t)
	hB.confirmThreeNodes()
	hB.clock.advance(time.Duration(defaultOptions().ResolveAfterMs+1) * time.Millisecond)
	hB.trk.sweep(context.Background())

	logB := hB.trk.NearConfirmedLog()
	if len(logB) != 1 {
		t.Fatalf("B: log = %d entri, mau 1", len(logB))
	}
	eB := logB[0]

	// Event C — dua node, masih terbuka.
	hC := newHarness(t)
	hC.node("N1", baseLat, baseLon)
	hC.nodeAt("N2", baseLat, baseLon, 8, 90)
	hC.ingest(v2("N1", MinPGAGal+10, onsetBase, PhasePrelim, 1))
	hC.ingest(v2("N2", MinPGAGal+10, onsetBase+500, PhasePrelim, 1))

	logC := hC.trk.NearConfirmedLog()
	if len(logC) != 1 {
		t.Fatalf("C: log = %d entri, mau 1", len(logC))
	}
	eC := logC[0]

	// Q1: berapa event stalled (tidak pernah CONFIRMED)?
	// A dan C tidak pernah CONFIRMED; B ya.
	if eA.ConfirmedAt != 0 {
		t.Errorf("A: ConfirmedAt = %d, mau 0 (stalled)", eA.ConfirmedAt)
	}
	if eB.ConfirmedAt == 0 {
		t.Errorf("B: ConfirmedAt = 0, mau > 0 (confirmed)")
	}
	if eC.ConfirmedAt != 0 {
		t.Errorf("C: ConfirmedAt = %d, mau 0 (stalled, masih terbuka)", eC.ConfirmedAt)
	}

	// Q2: berapa lama mereka stall?
	// A: stallMs = TerminalAt - FirstTwoIndependentAt.
	stallA := eA.TerminalAt - eA.FirstTwoIndependentAt
	if stallA <= 0 {
		t.Errorf("A stallMs = %d, mau > 0", stallA)
	}
	// C masih terbuka: TerminalAt == 0.
	if eC.TerminalAt != 0 {
		t.Errorf("C: TerminalAt = %d, mau 0 (masih terbuka)", eC.TerminalAt)
	}

	// Q3: berapa yang akhirnya CONFIRMED?
	// Hanya B.
	if eB.ConfirmedAt == 0 {
		t.Error("B: tidak CONFIRMED")
	}

	// Q4: berapa yang mati tanpa konfirmasi?
	// A: ConfirmedAt == 0 && TerminalAt > 0.
	if !(eA.ConfirmedAt == 0 && eA.TerminalAt > 0) {
		t.Errorf("A: tidak memenuhi 'mati tanpa konfirmasi': confirmed=%d terminal=%d",
			eA.ConfirmedAt, eA.TerminalAt)
	}
}
