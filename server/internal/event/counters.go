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

	// Kedua counter P4-M2′ dihitung TERPISAH dari persistDropped/upsertFailures,
	// dan pemisahannya adalah keseluruhan alasannya ada: satuan event yang hilang
	// berarti baris earthquake_events yang hilang, sedangkan catatan
	// near-confirmation yang hilang berarti JAWABAN forensik yang hilang pada
	// event yang barisnya mungkin tertulis sempurna. Satu counter untuk keduanya
	// akan membuat kedua kerugian itu tidak dapat dibedakan justru saat seseorang
	// perlu membedakannya.
	//
	// Keduanya DILAPORKAN, dan tidak ada target nol untuk keduanya (D-011 batasan
	// 1, S1): antreannya sengaja boleh membuang, jadi sebuah SLO nol-buangan hanya
	// dapat ditepati dengan memblokir jalur peringatan.
	nearConfirmedDropped        int64
	nearConfirmedUpsertFailures int64
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
	onsetSensor := t.latency.onsetToDecidedSensor.snapshot()
	onsetPublish := t.latency.onsetToDecidedPublish.snapshot()
	decidedEmit := t.latency.decidedToEmit.snapshot()
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
		"event_near_confirmed_persist_dropped_total", c.nearConfirmedDropped,
		"event_near_confirmed_upsert_failures_total", c.nearConfirmedUpsertFailures,
		"event_open_gauge", open,
		"event_tombstone_gauge", tombs,
		// Latensi tahap server (P4-M3′). Dicetak bersama counter lain dengan
		// alasan yang sama: sebuah angka yang tidak pernah dicetak sama dengan
		// angka yang tidak ada.
		"event_latency_onset_to_decided_sensor_ms", latencyAttr(onsetSensor),
		"event_latency_onset_to_decided_publish_bound_ms", latencyAttr(onsetPublish),
		"event_latency_decided_to_emit_ms", latencyAttr(decidedEmit),
	)
}

// latencyAttr membuat satu seri latensi terbaca dalam satu baris log, dengan
// jumlah sampel ikut serta: p95 atas tiga sampel dan p95 atas dua ratus sampel
// adalah dua klaim yang sangat berbeda, dan pembaca log harus dapat membedakannya
// tanpa meninggalkan barisnya.
func latencyAttr(s LatencyStats) slog.Value {
	return slog.GroupValue(
		slog.Int64("observed", s.Observed),
		slog.Int64("p50_ms", s.P50Ms),
		slog.Int64("p95_ms", s.P95Ms),
	)
}

// TrackerStats adalah potret seluruh counter §15.5 pada satu titik waktu.
// Nilai type, bukan pointer: aman dibawa keluar dari lock dan diserialisasi
// oleh handler HTTP tanpa menyentuh Tracker lagi.
type TrackerStats struct {
	// Counter kumulatif — monoton naik sejak proses dimulai.
	Created            int64 `json:"event_created_total"`
	ForcedResolutions  int64 `json:"event_forced_resolutions_total"`
	ReonsetSplits      int64 `json:"event_reonset_splits_total"`
	DiameterRejections int64 `json:"event_diameter_rejections_total"`
	StaleAbsorbed      int64 `json:"event_stale_evidence_absorbed_total"`
	TombstoneEvictions int64 `json:"event_tombstone_evictions_total"`
	Reconciled         int64 `json:"event_reconciled_total"`
	PersistDropped     int64 `json:"event_persist_dropped_total"`
	UpsertFailures     int64 `json:"event_upsert_failures_total"`
	StateLogFailures   int64 `json:"event_state_log_failures_total"`
	StateLogSkipped    int64 `json:"event_state_log_skipped_total"`

	// Akuntansi pembuangan/kegagalan catatan near-confirmation durable (P4-M2′,
	// D-012). DILAPORKAN, tidak pernah diklaim nol.
	NearConfirmedDropped        int64 `json:"event_near_confirmed_persist_dropped_total"`
	NearConfirmedUpsertFailures int64 `json:"event_near_confirmed_upsert_failures_total"`

	// Transisi per state tujuan.
	TransitionToUnconfirmed int64 `json:"event_transitions_to_unconfirmed_total"`
	TransitionToConfirmed   int64 `json:"event_transitions_to_confirmed_total"`
	TransitionToResolved    int64 `json:"event_transitions_to_resolved_total"`
	TransitionToCancelled   int64 `json:"event_transitions_to_cancelled_total"`

	// Gauge — snapshot ukuran map saat ini, bukan counter kumulatif.
	OpenGauge      int `json:"event_open_gauge"`
	TombstoneGauge int `json:"event_tombstone_gauge"`

	// Latensi tahap server (P4-M3′, D-011). Onset->decided DIPISAH menurut
	// provenance onset: seri PUBLISH_BOUND adalah batas atas, bukan pengukuran,
	// karena jangkarnya sendiri disimpulkan dari publish_ts - dur_ms. Menyatukan
	// keduanya akan menyebut sebuah batas sebagai pengukuran. Lihat latency.go.
	OnsetToDecidedSensor  LatencyStats `json:"event_latency_onset_to_decided_sensor_ms"`
	OnsetToDecidedPublish LatencyStats `json:"event_latency_onset_to_decided_publish_bound_ms"`
	DecidedToEmit         LatencyStats `json:"event_latency_decided_to_emit_ms"`
}

// Stats mengembalikan potret seluruh counter dan gauge dalam satu pengambilan
// kunci. Aman dipanggil dari goroutine mana pun, termasuk handler HTTP.
func (t *Tracker) Stats() TrackerStats {
	t.mu.Lock()
	c := t.counters
	open := len(t.openLocked())
	tombs := len(t.tombstonesLocked())
	// Diambil di bawah kunci yang sama: satu potret, bukan dua yang dapat
	// menggambarkan dua titik waktu berbeda.
	onsetSensor := t.latency.onsetToDecidedSensor.snapshot()
	onsetPublish := t.latency.onsetToDecidedPublish.snapshot()
	decidedEmit := t.latency.decidedToEmit.snapshot()
	t.mu.Unlock()

	return TrackerStats{
		Created:            c.created,
		ForcedResolutions:  c.forcedResolutions,
		ReonsetSplits:      c.reonsetSplits,
		DiameterRejections: c.diameterRejections,
		StaleAbsorbed:      c.staleAbsorbed,
		TombstoneEvictions: c.tombstoneEvictions,
		Reconciled:         c.reconciled,
		PersistDropped:     c.persistDropped,
		UpsertFailures:     c.upsertFailures,
		StateLogFailures:   c.stateLogFailures,
		StateLogSkipped:    c.stateLogSkipped,

		NearConfirmedDropped:        c.nearConfirmedDropped,
		NearConfirmedUpsertFailures: c.nearConfirmedUpsertFailures,

		TransitionToUnconfirmed: c.transitions[StateUnconfirmed],
		TransitionToConfirmed:   c.transitions[StateConfirmed],
		TransitionToResolved:    c.transitions[StateResolved],
		TransitionToCancelled:   c.transitions[StateCancelled],
		OpenGauge:               open,
		TombstoneGauge:          tombs,
		OnsetToDecidedSensor:    onsetSensor,
		OnsetToDecidedPublish:   onsetPublish,
		DecidedToEmit:           decidedEmit,
	}
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
func (t *Tracker) NearConfirmedDropped() int64 {
	return t.readCounter(func(c *counters) int64 { return c.nearConfirmedDropped })
}
func (t *Tracker) NearConfirmedUpsertFailures() int64 {
	return t.readCounter(func(c *counters) int64 { return c.nearConfirmedUpsertFailures })
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
