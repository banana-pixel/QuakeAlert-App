package event

import (
	"context"
	"testing"
	"time"
)

// confirmThreeNodes membawa satu event sampai CONFIRMED dan mengembalikannya.
func (h *harness) confirmThreeNodes() *Event {
	h.t.Helper()
	h.threeNodeCluster()
	for i, n := range []string{"N1", "N2", "N3"} {
		h.ingest(v2(n, MinPGAGal+10, onsetBase+int64(i)*1000, PhasePrelim, 1))
	}
	e := h.only()
	if e.State != StateConfirmed {
		h.t.Fatalf("persiapan: state = %s, mau CONFIRMED", e.State)
	}
	return e
}

// R-H2/D28 — uji yang diwajibkan §18.2.
//
// Sebelum tombstone, bukti yang terlambat tidak mencocoki apa pun dan §6.2 langkah 6
// membuat event BARU dengan event_id baru: pengguna melihat EVENT_RESOLVED untuk
// event A lalu, beberapa detik kemudian, EARTHQUAKE_ALERT segar untuk event B yang
// menggambarkan guncangan yang sama. AlertDedup berkunci pada TYPE:event_id, jadi id
// baru secara konstruksi adalah alert baru.
func TestLateFinalAfterResolveDoesNotRealarm(t *testing.T) {
	h := newHarness(t)
	e := h.confirmThreeNodes()
	h.nodeAt("N4", baseLat, baseLon, 12, 90)

	// Jam melewati ResolveAfter; sweeper menutup event.
	h.clock.advance(time.Duration(defaultOptions().ResolveAfterMs+1) * time.Millisecond)
	h.trk.sweep(context.Background())

	if e.State != StateResolved {
		t.Fatalf("state = %s, mau RESOLVED", e.State)
	}
	framesAfterResolve := len(h.emit.frames)
	revAfterResolve := e.Revision
	if framesAfterResolve != 3 {
		t.Fatalf("frame = %v, mau UNCONFIRMED, CONFIRMED, RESOLVED", h.emit.states())
	}

	// FINAL yang terlambat dari node keempat, onset masih di dalam jendela.
	h.ingest(v2("N4", MinPGAGal+80, onsetBase+3000, PhaseFinal, 1))

	if n := len(h.events()); n != 1 {
		t.Fatalf("event = %d, mau tetap 1: id kedua untuk guncangan yang sama akan menjadi alert kedua (%s)",
			n, describe(h.events()))
	}
	if _, ok := e.Contributors["N4"]; !ok {
		t.Error("observasi terlambat harus DISERAP ke himpunan kontributor tombstone")
	}
	if e.State != StateResolved {
		t.Errorf("state = %s, mau tetap RESOLVED: terminal berarti terminal", e.State)
	}
	if e.Revision != revAfterResolve {
		t.Errorf("revision = %d, mau tetap %d: tidak ada transisi, jadi tidak ada revisi",
			e.Revision, revAfterResolve)
	}
	if len(h.emit.frames) != framesAfterResolve {
		t.Errorf("frame tambahan = %v; harus NOL di setiap kanal", h.emit.states()[framesAfterResolve:])
	}
	if got := h.trk.StaleEvidenceAbsorbed(); got != 1 {
		t.Errorf("event_stale_evidence_absorbed_total = %d, mau 1", got)
	}
	// PGA puncak tombstone tetap naik: ia terus menyerap bukti untuk event
	// fisiknya sendiri, hanya saja tidak ada yang mendengar.
	if e.peakPGA() != MinPGAGal+80 {
		t.Errorf("peak_pga = %.1f, mau %.1f", e.peakPGA(), MinPGAGal+80)
	}
}

// Perilaku pasca-retensi (R11), ditegaskan supaya ia menjadi keputusan dan bukan
// kecelakaan: setelah TerminalRetentionMs, observasi yang cocok membuat event BARU.
// Di luar jendela itu alert pertama sudah menua keluar dari UI Android
// (RECENT_WINDOW_MS), jadi event baru adalah deskripsi yang jujur atas bukti baru.
func TestAfterTerminalRetentionAMatchingObservationCreatesANewEvent(t *testing.T) {
	h := newHarness(t)
	e := h.confirmThreeNodes()
	oldID := e.ID
	h.nodeAt("N4", baseLat, baseLon, 12, 90)

	opt := defaultOptions()
	h.clock.advance(time.Duration(opt.ResolveAfterMs+1) * time.Millisecond)
	h.trk.sweep(context.Background())

	h.clock.advance(time.Duration(opt.TerminalRetentionMs+1) * time.Millisecond)
	h.trk.sweep(context.Background())

	if _, ok := h.events()[oldID]; ok {
		t.Fatal("tombstone harus dievakuasi oleh USIA setelah TerminalRetentionMs")
	}

	h.ingest(v2("N4", MinPGAGal+80, onsetBase+3000, PhaseFinal, 1))
	es := h.events()
	if len(es) != 1 {
		t.Fatalf("event = %d, mau 1 yang baru", len(es))
	}
	for id := range es {
		if id == oldID {
			t.Error("event lama dibangkitkan kembali; RESOLVED tidak pernah dibuka ulang")
		}
	}
	if got := h.trk.Created(); got != 2 {
		t.Errorf("event_created_total = %d, mau 2", got)
	}
	if got := h.trk.StaleEvidenceAbsorbed(); got != 0 {
		t.Errorf("event_stale_evidence_absorbed_total = %d, mau 0: tidak ada tombstone lagi", got)
	}
}

// TerminalRetentionMs (15 menit) jauh lebih longgar daripada MaxTriggerAge (5 menit),
// gerbang kesegaran yang membatasi seberapa terlambat sebuah observasi dapat sampai.
// Jadi dalam praktiknya SETIAP observasi yang diterima verifier untuk event yang baru
// selesai mendarat di tombstone, dan cabang pasca-retensi hanya terjangkau bila
// durasi event itu sendiri mendorongnya melewati 15 menit. Uji ini mengunci
// pertidaksamaan itu, bukan sekadar mengomentarinya.
func TestTerminalRetentionOutlastsMaxTriggerAge(t *testing.T) {
	const maxTriggerAgeMs = int64(5 * 60 * 1000) // ingest.MaxTriggerAge
	if got := defaultOptions().TerminalRetentionMs; got <= maxTriggerAgeMs {
		t.Fatalf("TerminalRetentionMs = %d harus > MaxTriggerAge = %d", got, maxTriggerAgeMs)
	}
}

// Sebuah event DETECTED tidak meninggalkan tombstone: ia tidak pernah terlihat, jadi
// ia tidak dapat menghasilkan alert kedua yang dapat dilihat pengguna.
func TestExpiredDetectedEventLeavesNoTombstone(t *testing.T) {
	h := newHarness(t)
	h.node("N1", baseLat, baseLon)
	h.ingest(v2("N1", MinPGAGal-0.1, onsetBase, PhasePrelim, 1))

	h.clock.advance(time.Duration(defaultOptions().ResolveAfterMs+1) * time.Millisecond)
	h.trk.sweep(context.Background())

	if n := len(h.events()); n != 0 {
		t.Errorf("event terlacak = %d, mau 0: DETECTED KEDALUWARSA, tidak diselesaikan", n)
	}
	if got := h.trk.TombstoneGauge(); got != 0 {
		t.Errorf("event_tombstone_gauge = %d, mau 0", got)
	}
	if len(h.emit.frames) != 0 {
		t.Errorf("frame = %v, mau kosong", h.emit.states())
	}
}
