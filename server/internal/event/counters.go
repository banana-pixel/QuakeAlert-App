package event

import (
	"log/slog"
	"time"
)

// counters adalah seluruh angka §15.5. Hanya-log, seperti yang ditetapkan Fase 1:
// belum ada endpoint /metrics, dan sebuah counter yang tidak pernah dicetak sama
// dengan counter yang tidak ada.
//
// Semuanya dilindungi t.mu — setiap kenaikan terjadi di dalam bagian yang sudah
// terkunci, jadi tidak ada atomic di sini dan tidak ada kunci kedua.
type counters struct {
	created            int64
	transitions        map[State]int64
	forcedResolutions  int64
	reonsetSplits      int64
	diameterRejections int64
	staleAbsorbed      int64
	tombstoneEvictions int64
	reconciled         int64

	persistDropped   int64
	upsertFailures   int64
	stateLogFailures int64
	stateLogSkipped  int64
}

func newCounters() counters {
	return counters{transitions: make(map[State]int64, 4)}
}

// counterLogInterval mengikuti pola ledger.Writer.logCounters, dengan alasan yang
// sama dan nilai yang sama.
const counterLogInterval = 5 * time.Minute

// logCounters mencetak seluruh counter sekali. Dipanggil oleh sweeper pada
// counterLogInterval dan sekali lagi saat shutdown.
func (t *Tracker) logCounters(reason string) {
	t.mu.Lock()
	c := t.counters
	open := len(t.openLocked())
	tombs := len(t.tombstonesLocked())
	transitions := make(map[State]int64, len(c.transitions))
	for k, v := range c.transitions {
		transitions[k] = v
	}
	t.mu.Unlock()

	t.log.Info("event: counters",
		"reason", reason,
		"event_created_total", c.created,
		"event_transitions_total", transitionsAttr(transitions),
		"event_forced_resolutions_total", c.forcedResolutions,
		"event_reonset_splits_total", c.reonsetSplits,
		"event_diameter_rejections_total", c.diameterRejections,
		"event_stale_evidence_absorbed_total", c.staleAbsorbed,
		"event_tombstone_evictions_total", c.tombstoneEvictions,
		"event_reconciled_total", c.reconciled,
		"event_persist_dropped_total", c.persistDropped,
		"event_upsert_failures_total", c.upsertFailures,
		"event_state_log_failures_total", c.stateLogFailures,
		"event_state_log_skipped_total", c.stateLogSkipped,
		"event_open_gauge", open,
		"event_tombstone_gauge", tombs,
	)
}

// transitionsAttr membuat label {to} dapat dibaca dalam satu baris log.
func transitionsAttr(m map[State]int64) slog.Value {
	attrs := make([]slog.Attr, 0, len(m))
	for _, st := range []State{StateUnconfirmed, StateConfirmed, StateResolved, StateCancelled} {
		if v, ok := m[st]; ok {
			attrs = append(attrs, slog.Int64(string(st), v))
		}
	}
	return slog.GroupValue(attrs...)
}

// Counter yang dibaca uji dan §22. Diekspor satu per satu, bukan sebagai struct,
// supaya tidak ada pemanggil yang dapat menulisinya.
func (t *Tracker) Created() int64 { return t.readCounter(func(c *counters) int64 { return c.created }) }
func (t *Tracker) ForcedResolutions() int64 {
	return t.readCounter(func(c *counters) int64 { return c.forcedResolutions })
}
func (t *Tracker) ReonsetSplits() int64 {
	return t.readCounter(func(c *counters) int64 { return c.reonsetSplits })
}
func (t *Tracker) DiameterRejections() int64 {
	return t.readCounter(func(c *counters) int64 { return c.diameterRejections })
}
func (t *Tracker) StaleEvidenceAbsorbed() int64 {
	return t.readCounter(func(c *counters) int64 { return c.staleAbsorbed })
}
func (t *Tracker) TombstoneEvictions() int64 {
	return t.readCounter(func(c *counters) int64 { return c.tombstoneEvictions })
}
func (t *Tracker) Reconciled() int64 {
	return t.readCounter(func(c *counters) int64 { return c.reconciled })
}
func (t *Tracker) PersistDropped() int64 {
	return t.readCounter(func(c *counters) int64 { return c.persistDropped })
}
func (t *Tracker) UpsertFailures() int64 {
	return t.readCounter(func(c *counters) int64 { return c.upsertFailures })
}
func (t *Tracker) StateLogFailures() int64 {
	return t.readCounter(func(c *counters) int64 { return c.stateLogFailures })
}
func (t *Tracker) StateLogSkipped() int64 {
	return t.readCounter(func(c *counters) int64 { return c.stateLogSkipped })
}

// Transitions mengembalikan event_transitions_total untuk satu state tujuan.
func (t *Tracker) Transitions(to State) int64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.counters.transitions[to]
}

// OpenGauge dan TombstoneGauge adalah ukuran map, bukan counter kumulatif.
func (t *Tracker) OpenGauge() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.openLocked())
}

func (t *Tracker) TombstoneGauge() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.tombstonesLocked())
}

func (t *Tracker) readCounter(f func(*counters) int64) int64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return f(&t.counters)
}
