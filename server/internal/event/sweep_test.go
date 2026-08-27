package event

import (
	"context"
	"testing"
	"time"
)

func msDur(ms int64) time.Duration { return time.Duration(ms) * time.Millisecond }

// §18.2 sweeper: lewati ResolveAfter -> tepat SATU transisi RESOLVED, satu frame;
// maju lagi -> tidak ada apa pun lagi.
func TestSweeperResolvesOnceAndThenNothing(t *testing.T) {
	h := newHarness(t)
	e := h.confirmThreeNodes()
	rev := e.Revision

	// Belum melewati tenggat: tidak boleh terjadi apa pun.
	h.clock.advance(msDur(defaultOptions().ResolveAfterMs))
	h.trk.sweep(context.Background())
	if e.State != StateConfirmed {
		t.Fatalf("state = %s pada tenggat TEPAT; batasnya harus terlampaui dulu", e.State)
	}

	h.clock.advance(time.Millisecond)
	h.trk.sweep(context.Background())
	if e.State != StateResolved {
		t.Fatalf("state = %s, mau RESOLVED", e.State)
	}
	if e.Revision != rev+1 {
		t.Errorf("revision = %d, mau %d", e.Revision, rev+1)
	}
	if got := h.emit.countFor(StateResolved); got != 1 {
		t.Errorf("frame RESOLVED = %d, mau 1", got)
	}
	if got := h.trk.Transitions(StateResolved); got != 1 {
		t.Errorf("event_transitions_total{RESOLVED} = %d, mau 1", got)
	}

	// Tick berulang tidak boleh menghasilkan apa pun: RESOLVED -> apa pun ilegal.
	before := len(h.emit.frames)
	for i := 0; i < 5; i++ {
		h.clock.advance(msDur(defaultOptions().SweepIntervalMs))
		h.trk.sweep(context.Background())
	}
	if len(h.emit.frames) != before {
		t.Errorf("frame tambahan = %v, mau tidak ada", h.emit.states()[before:])
	}
	if got := h.trk.Transitions(StateResolved); got != 1 {
		t.Errorf("event_transitions_total{RESOLVED} = %d setelah lima tick, mau tetap 1", got)
	}
	// Alasan resolusi adalah kosakata tertutup §5.3, bukan teks bebas.
	last := h.emit.frames[len(h.emit.frames)-1]
	if last.Reason != ReasonNoNewEvidence {
		t.Errorf("reason = %q, mau NO_NEW_EVIDENCE", last.Reason)
	}
	if last.From != StateConfirmed {
		t.Errorf("from_state = %s, mau CONFIRMED", last.From)
	}
}

// Bukti baru menunda tenggat, karena tenggat diturunkan dari WAKTU BUKTI yang
// tersimpan dan bukan dari timer hidup (§5.4).
func TestNewEvidencePostponesResolution(t *testing.T) {
	h := newHarness(t)
	e := h.confirmThreeNodes()
	opt := defaultOptions()

	h.clock.advance(msDur(opt.ResolveAfterMs - 1000))
	h.ingest(v2("N1", MinPGAGal+40, onsetBase, PhaseFinal, 1))
	h.clock.advance(msDur(2000))
	h.trk.sweep(context.Background())

	if e.State != StateResolved {
		// benar: 2 s setelah bukti terakhir masih jauh di dalam tenggat
	} else {
		t.Fatal("resolusi terjadi walau ada bukti 2 s lalu")
	}

	h.clock.advance(msDur(opt.ResolveAfterMs))
	h.trk.sweep(context.Background())
	if e.State != StateResolved {
		t.Errorf("state = %s, mau RESOLVED setelah tenggat penuh tanpa bukti", e.State)
	}
}

// UNCONFIRMED juga diselesaikan oleh timeout, dengan reason yang sama, dan TIDAK
// dibatalkan: bukti PRELIM yang memenuhi lantai adalah guncangan nyata sejauh yang
// sistem tahu (§6.5 kasus 8).
func TestSweeperResolvesUnconfirmedNotCancelled(t *testing.T) {
	h := newHarness(t)
	h.node("N1", baseLat, baseLon)
	h.ingest(v2("N1", MinPGAGal+1, onsetBase, PhasePrelim, 1))

	h.clock.advance(msDur(defaultOptions().ResolveAfterMs + 1))
	h.trk.sweep(context.Background())

	e := h.only()
	if e.State != StateResolved {
		t.Fatalf("state = %s, mau RESOLVED", e.State)
	}
	if e.EverConfirmed {
		t.Error("EverConfirmed harus false: FCM all-clear tidak berutang kepada siapa pun di sini")
	}
}

// §7.5 — invalidasi pada node event CONFIRMED sehingga ia jatuh di bawah lantainya:
// CANCELLED dengan reason EVIDENCE_INVALIDATED.
func TestInvalidateContributorCancelsWhenEvidenceFallsBelowFloor(t *testing.T) {
	h := newHarness(t)
	h.threeNodeCluster()
	// Hanya N1 yang membawa PGA di atas lantai; dua lainnya menyumbang suara.
	h.ingest(v2("N1", MinPGAGal+100, onsetBase, PhasePrelim, 1))
	h.ingest(v2("N2", MinPGAGal-5, onsetBase+1000, PhasePrelim, 1))
	h.ingest(v2("N3", MinPGAGal-5, onsetBase+2000, PhasePrelim, 1))
	e := h.only()
	if e.State != StateConfirmed {
		t.Fatalf("persiapan: state = %s, mau CONFIRMED", e.State)
	}

	h.trk.InvalidateContributor(context.Background(), "N1", "")

	if e.State != StateCancelled {
		t.Fatalf("state = %s, mau CANCELLED", e.State)
	}
	if _, ok := e.Contributors["N1"]; ok {
		t.Error("kontributor yang dicabut harus hilang dari event")
	}
	last := h.emit.frames[len(h.emit.frames)-1]
	if last.To != StateCancelled || last.Reason != ReasonEvidenceInvalid {
		t.Errorf("frame terakhir = %s/%q, mau CANCELLED/EVIDENCE_INVALIDATED", last.To, last.Reason)
	}
	if !e.EverConfirmed {
		t.Error("EverConfirmed harus tetap true: pencabutan lebih mendesak daripada all-clear, bukan kurang")
	}
	if e.TerminalAt == 0 {
		t.Error("TerminalAt harus disetel supaya tombstone dapat dievakuasi oleh usia")
	}
}

// Invalidasi pada event yang SUDAH RESOLVED tidak menghasilkan transisi apa pun.
func TestInvalidateContributorOnResolvedEventDoesNothing(t *testing.T) {
	h := newHarness(t)
	e := h.confirmThreeNodes()
	h.clock.advance(msDur(defaultOptions().ResolveAfterMs + 1))
	h.trk.sweep(context.Background())
	before := len(h.emit.frames)
	rev := e.Revision

	h.trk.InvalidateContributor(context.Background(), "N1", "")

	if e.State != StateResolved {
		t.Errorf("state = %s, mau tetap RESOLVED", e.State)
	}
	if e.Revision != rev {
		t.Errorf("revision = %d, mau tetap %d", e.Revision, rev)
	}
	if _, ok := e.Contributors["N1"]; !ok {
		t.Error("kontributor tombstone tidak boleh dicabut: hanya event TERBUKA yang disentuh")
	}
	if len(h.emit.frames) != before {
		t.Errorf("frame tambahan = %v, mau tidak ada", h.emit.states()[before:])
	}
}

// Invalidasi yang menyisakan bukti yang masih di atas lantai TIDAK membatalkan apa
// pun; C -> U ilegal, jadi tidak ada penurunan derajat yang dapat terjadi.
func TestInvalidateContributorKeepsConfirmedWhenEvidenceSurvives(t *testing.T) {
	h := newHarness(t)
	h.threeNodeCluster()
	h.nodeAt("N4", baseLat, baseLon, 24, 90)
	for i, n := range []string{"N1", "N2", "N3", "N4"} {
		h.ingest(v2(n, MinPGAGal+50, onsetBase+int64(i)*1000, PhasePrelim, 1))
	}
	e := h.only()
	rev := e.Revision

	h.trk.InvalidateContributor(context.Background(), "N4", "")

	if e.State != StateConfirmed {
		t.Errorf("state = %s, mau tetap CONFIRMED", e.State)
	}
	if e.Revision != rev {
		t.Errorf("revision = %d, mau tetap %d: tidak ada transisi", e.Revision, rev)
	}
	if len(e.Contributors) != 3 {
		t.Errorf("kontributor = %d, mau 3", len(e.Contributors))
	}
}

// Mencabut SELURUH bukti membuat event menjadi Invalidated dan CANCELLED, apa pun
// PGA yang pernah dibawanya.
func TestInvalidateLastContributorCancelsEvent(t *testing.T) {
	h := newHarness(t)
	h.node("N1", baseLat, baseLon)
	h.ingest(v2("N1", MinPGAGal+200, onsetBase, PhasePrelim, 1))
	e := h.only()

	h.trk.InvalidateContributor(context.Background(), "N1", ReasonOperatorRetracted)

	if e.State != StateCancelled {
		t.Fatalf("state = %s, mau CANCELLED", e.State)
	}
	if !e.Invalidated {
		t.Error("Invalidated harus true tanpa satu pun kontributor tersisa")
	}
	last := h.emit.frames[len(h.emit.frames)-1]
	if last.Reason != ReasonOperatorRetracted {
		t.Errorf("reason = %q, mau OPERATOR_RETRACTION bila operator yang memintanya", last.Reason)
	}
}

// §15.4 — langit-langit event terbuka: event baru memaksa resolusi event terbuka
// TERTUA, menghitungnya, dan tidak pernah membuangnya diam-diam.
func TestExceedingMaxOpenForcesResolutionOfOldest(t *testing.T) {
	h := newHarness(t, func(o *Options) { o.MaxOpen = 2 })
	// Tiga gempa yang terpisah jauh dalam waktu, jadi tak satu pun berkorelasi.
	for i := 0; i < 3; i++ {
		id := "M" + string(rune('1'+i))
		h.node(id, baseLat+float64(i)*3, baseLon)
		h.ingest(v2(id, MinPGAGal+10, onsetBase+int64(i)*100000, PhasePrelim, 1))
	}

	if got := h.trk.OpenGauge(); got > 2 {
		t.Errorf("event_open_gauge = %d, mau <= 2", got)
	}
	if got := h.trk.ForcedResolutions(); got != 1 {
		t.Errorf("event_forced_resolutions_total = %d, mau 1", got)
	}
	if got := h.emit.countFor(StateResolved); got != 1 {
		t.Errorf("frame RESOLVED = %d, mau 1: dipaksa bukan berarti diam-diam", got)
	}
	// Resolusi paksa memakai kosakata yang sama dengan resolusi karena tenggat:
	// kliennya tidak dapat membedakan keduanya dan tidak perlu.
	var forced *Snapshot
	for i := range h.emit.frames {
		if h.emit.frames[i].To == StateResolved {
			forced = &h.emit.frames[i]
		}
	}
	if forced == nil {
		t.Fatal("tidak ada frame RESOLVED sama sekali")
	}
	if forced.Reason != ReasonNoNewEvidence {
		t.Errorf("reason = %q, mau NO_NEW_EVIDENCE", forced.Reason)
	}
}

// §15.4 — langit-langit tombstone: tombstone TERTUA dievakuasi dan
// event_tombstone_evictions_total naik. Counter itu harus NOL di operasi normal;
// harness menegaskannya untuk setiap uji lain di paket ini.
func TestExceedingMaxTombstonesEvictsOldestAndCounts(t *testing.T) {
	h := newHarness(t, func(o *Options) { o.MaxTombstones = 1 })
	h.permitEvictions()
	opt := defaultOptions()

	// Dua event yang terpisah jauh, keduanya diselesaikan, lalu satu event ketiga
	// yang pembuatannya menekan langit-langit tombstone.
	for i := 0; i < 3; i++ {
		id := "K" + string(rune('1'+i))
		h.node(id, baseLat+float64(i)*3, baseLon)
		h.ingest(v2(id, MinPGAGal+10, onsetBase+int64(i)*100000, PhasePrelim, 1))
		h.clock.advance(msDur(opt.ResolveAfterMs + 1))
		h.trk.sweep(context.Background())
	}

	if got := h.trk.TombstoneGauge(); got > 1 {
		t.Errorf("event_tombstone_gauge = %d, mau <= 1", got)
	}
	if got := h.trk.TombstoneEvictions(); got == 0 {
		t.Error("event_tombstone_evictions_total = 0; evakuasi karena TEKANAN harus terlihat")
	}
}

// Run harus berhenti saat context dibatalkan, dan berhenti dengan mencetak counter:
// sebuah counter yang tidak pernah dicetak sama dengan counter yang tidak ada.
func TestRunStopsOnContextCancel(t *testing.T) {
	h := newHarness(t, func(o *Options) { o.SweepIntervalMs = 1 })
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() { h.trk.Run(ctx); close(done) }()

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run tidak berhenti setelah context dibatalkan")
	}
}
