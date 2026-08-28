package event

import (
	"context"
	"testing"
	"time"
)

// TestStatsZeroOnFreshTracker — Stats() mengembalikan semua nol pada Tracker baru.
func TestStatsZeroOnFreshTracker(t *testing.T) {
	h := newHarness(t)
	s := h.trk.Stats()

	if s.Created != 0 {
		t.Errorf("Created = %d, mau 0", s.Created)
	}
	if s.OpenGauge != 0 {
		t.Errorf("OpenGauge = %d, mau 0", s.OpenGauge)
	}
	if s.TombstoneGauge != 0 {
		t.Errorf("TombstoneGauge = %d, mau 0", s.TombstoneGauge)
	}
	if s.TransitionToUnconfirmed != 0 {
		t.Errorf("TransitionToUnconfirmed = %d, mau 0", s.TransitionToUnconfirmed)
	}
	if s.TransitionToConfirmed != 0 {
		t.Errorf("TransitionToConfirmed = %d, mau 0", s.TransitionToConfirmed)
	}
	if s.TransitionToResolved != 0 {
		t.Errorf("TransitionToResolved = %d, mau 0", s.TransitionToResolved)
	}
	if s.TransitionToCancelled != 0 {
		t.Errorf("TransitionToCancelled = %d, mau 0", s.TransitionToCancelled)
	}
	if s.ReonsetSplits != 0 {
		t.Errorf("ReonsetSplits = %d, mau 0", s.ReonsetSplits)
	}
	if s.DiameterRejections != 0 {
		t.Errorf("DiameterRejections = %d, mau 0", s.DiameterRejections)
	}
	if s.StaleAbsorbed != 0 {
		t.Errorf("StaleAbsorbed = %d, mau 0", s.StaleAbsorbed)
	}
	if s.TombstoneEvictions != 0 {
		t.Errorf("TombstoneEvictions = %d, mau 0", s.TombstoneEvictions)
	}
	if s.ForcedResolutions != 0 {
		t.Errorf("ForcedResolutions = %d, mau 0", s.ForcedResolutions)
	}
	if s.Reconciled != 0 {
		t.Errorf("Reconciled = %d, mau 0", s.Reconciled)
	}
	if s.PersistDropped != 0 {
		t.Errorf("PersistDropped = %d, mau 0", s.PersistDropped)
	}
	if s.UpsertFailures != 0 {
		t.Errorf("UpsertFailures = %d, mau 0", s.UpsertFailures)
	}
	if s.StateLogFailures != 0 {
		t.Errorf("StateLogFailures = %d, mau 0", s.StateLogFailures)
	}
	if s.StateLogSkipped != 0 {
		t.Errorf("StateLogSkipped = %d, mau 0", s.StateLogSkipped)
	}
}

// TestStatsCreatedCounter — Stats().Created naik setiap event baru.
func TestStatsCreatedCounter(t *testing.T) {
	h := newHarness(t)
	h.node("N1", baseLat, baseLon)
	h.nodeAt("N2", baseLat, baseLon, 200, 90) // jauh — event terpisah

	h.ingest(v2("N1", MinPGAGal+1, onsetBase, PhasePrelim, 1))
	if got := h.trk.Stats().Created; got != 1 {
		t.Errorf("Created = %d, mau 1 setelah satu trigger", got)
	}

	// Onset jauh di luar jendela korelasi agar tidak menempel ke event pertama.
	h.ingest(v2("N2", MinPGAGal+1, onsetBase+int64(defaultOptions().CorrelationWindowMs)*2, PhasePrelim, 1))
	if got := h.trk.Stats().Created; got != 2 {
		t.Errorf("Created = %d, mau 2 setelah dua event terpisah", got)
	}
}

// TestStatsTransitionCounters — Stats().TransitionTo* mencerminkan transisi nyata.
func TestStatsTransitionCounters(t *testing.T) {
	h := newHarness(t)
	h.threeNodeCluster()

	// Ingest N1 saja: DETECTED -> UNCONFIRMED.
	h.ingest(v2("N1", MinPGAGal+1, onsetBase, PhasePrelim, 1))
	s := h.trk.Stats()
	if s.TransitionToUnconfirmed != 1 {
		t.Errorf("TransitionToUnconfirmed = %d, mau 1", s.TransitionToUnconfirmed)
	}
	if s.TransitionToConfirmed != 0 {
		t.Errorf("TransitionToConfirmed = %d, mau 0 belum CONFIRMED", s.TransitionToConfirmed)
	}

	// Ingest N2 dan N3: UNCONFIRMED -> CONFIRMED.
	h.ingest(v2("N2", MinPGAGal+1, onsetBase+500, PhasePrelim, 1))
	h.ingest(v2("N3", MinPGAGal+1, onsetBase+1000, PhasePrelim, 1))
	s = h.trk.Stats()
	if s.TransitionToConfirmed != 1 {
		t.Errorf("TransitionToConfirmed = %d, mau 1", s.TransitionToConfirmed)
	}
}

// TestStatsOpenGaugeTracksLiveEvents — OpenGauge naik saat event terbuka.
func TestStatsOpenGaugeTracksLiveEvents(t *testing.T) {
	h := newHarness(t)
	h.node("N1", baseLat, baseLon)

	if got := h.trk.Stats().OpenGauge; got != 0 {
		t.Errorf("OpenGauge = %d, mau 0 sebelum trigger", got)
	}

	h.ingest(v2("N1", MinPGAGal+1, onsetBase, PhasePrelim, 1))
	if got := h.trk.Stats().OpenGauge; got != 1 {
		t.Errorf("OpenGauge = %d, mau 1 setelah satu trigger", got)
	}
}

// TestStatsTombstoneGaugeAfterResolve — TombstoneGauge naik saat event terminal,
// OpenGauge turun.
func TestStatsTombstoneGaugeAfterResolve(t *testing.T) {
	h := newHarness(t)
	h.confirmThreeNodes()

	// Sebelum sweep: satu event terbuka.
	s := h.trk.Stats()
	if s.OpenGauge != 1 {
		t.Errorf("OpenGauge = %d, mau 1 sebelum sweep", s.OpenGauge)
	}
	if s.TombstoneGauge != 0 {
		t.Errorf("TombstoneGauge = %d, mau 0 sebelum sweep", s.TombstoneGauge)
	}

	// Sweep menutup event.
	h.clock.advance(time.Duration(defaultOptions().ResolveAfterMs+1) * time.Millisecond)
	h.trk.sweep(context.Background())

	s = h.trk.Stats()
	if s.OpenGauge != 0 {
		t.Errorf("OpenGauge = %d, mau 0 setelah sweep", s.OpenGauge)
	}
	if s.TombstoneGauge != 1 {
		t.Errorf("TombstoneGauge = %d, mau 1 setelah sweep", s.TombstoneGauge)
	}
}

// TestStatsStaleAbsorbedCounter — StaleAbsorbed naik saat bukti terlambat
// diserap tombstone.
func TestStatsStaleAbsorbedCounter(t *testing.T) {
	h := newHarness(t)
	h.confirmThreeNodes()
	h.nodeAt("N4", baseLat, baseLon, 12, 90)

	h.clock.advance(time.Duration(defaultOptions().ResolveAfterMs+1) * time.Millisecond)
	h.trk.sweep(context.Background())

	// Bukti terlambat dari N4.
	h.ingest(v2("N4", MinPGAGal+10, onsetBase+500, PhaseFinal, 1))

	if got := h.trk.Stats().StaleAbsorbed; got != 1 {
		t.Errorf("StaleAbsorbed = %d, mau 1", got)
	}
}

// TestStatsIndividualGettersMatchStats — setiap getter individual harus
// menghasilkan nilai yang sama dengan Stats().
func TestStatsIndividualGettersMatchStats(t *testing.T) {
	h := newHarness(t)
	h.confirmThreeNodes()

	s := h.trk.Stats()

	if got := h.trk.Created(); got != s.Created {
		t.Errorf("Created() = %d, Stats().Created = %d: harus sama", got, s.Created)
	}
	if got := h.trk.ForcedResolutions(); got != s.ForcedResolutions {
		t.Errorf("ForcedResolutions() = %d, Stats().ForcedResolutions = %d", got, s.ForcedResolutions)
	}
	if got := h.trk.ReonsetSplits(); got != s.ReonsetSplits {
		t.Errorf("ReonsetSplits() = %d, Stats().ReonsetSplits = %d", got, s.ReonsetSplits)
	}
	if got := h.trk.DiameterRejections(); got != s.DiameterRejections {
		t.Errorf("DiameterRejections() = %d, Stats().DiameterRejections = %d", got, s.DiameterRejections)
	}
	if got := h.trk.StaleEvidenceAbsorbed(); got != s.StaleAbsorbed {
		t.Errorf("StaleEvidenceAbsorbed() = %d, Stats().StaleAbsorbed = %d", got, s.StaleAbsorbed)
	}
	if got := h.trk.TombstoneEvictions(); got != s.TombstoneEvictions {
		t.Errorf("TombstoneEvictions() = %d, Stats().TombstoneEvictions = %d", got, s.TombstoneEvictions)
	}
	if got := h.trk.PersistDropped(); got != s.PersistDropped {
		t.Errorf("PersistDropped() = %d, Stats().PersistDropped = %d", got, s.PersistDropped)
	}
	if got := h.trk.UpsertFailures(); got != s.UpsertFailures {
		t.Errorf("UpsertFailures() = %d, Stats().UpsertFailures = %d", got, s.UpsertFailures)
	}
	if got := h.trk.StateLogFailures(); got != s.StateLogFailures {
		t.Errorf("StateLogFailures() = %d, Stats().StateLogFailures = %d", got, s.StateLogFailures)
	}
	if got := h.trk.StateLogSkipped(); got != s.StateLogSkipped {
		t.Errorf("StateLogSkipped() = %d, Stats().StateLogSkipped = %d", got, s.StateLogSkipped)
	}
	if got := h.trk.Transitions(StateUnconfirmed); got != s.TransitionToUnconfirmed {
		t.Errorf("Transitions(UNCONFIRMED) = %d, Stats().TransitionToUnconfirmed = %d", got, s.TransitionToUnconfirmed)
	}
	if got := h.trk.Transitions(StateConfirmed); got != s.TransitionToConfirmed {
		t.Errorf("Transitions(CONFIRMED) = %d, Stats().TransitionToConfirmed = %d", got, s.TransitionToConfirmed)
	}
	if got := h.trk.OpenGauge(); got != s.OpenGauge {
		t.Errorf("OpenGauge() = %d, Stats().OpenGauge = %d", got, s.OpenGauge)
	}
	if got := h.trk.TombstoneGauge(); got != s.TombstoneGauge {
		t.Errorf("TombstoneGauge() = %d, Stats().TombstoneGauge = %d", got, s.TombstoneGauge)
	}
}

// TestStatsPersistFailureCounters — UpsertFailures dan StateLogSkipped terisi
// saat persistensi gagal, dan Stats() mencerminkannya.
func TestStatsPersistFailureCounters(t *testing.T) {
	h := newHarness(t)
	h.withPersister(func(p *recPersister) { p.failUpsert = true })
	h.confirmThreeNodes()

	s := h.trk.Stats()
	if s.UpsertFailures < 1 {
		t.Errorf("UpsertFailures = %d, mau > 0 saat toko selalu gagal", s.UpsertFailures)
	}
	if s.StateLogSkipped < 1 {
		t.Errorf("StateLogSkipped = %d, mau > 0 saat upsert gagal", s.StateLogSkipped)
	}
}
