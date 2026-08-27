package event

import (
	"context"
	"time"
)

// Run adalah SATU sweeper yang menggantikan satu timer per event di dispatcher.
// Dimaksudkan dijalankan sebagai `go tracker.Run(ctx)`, di samping
// `go ledgerWriter.Run(ledgerCtx)`.
//
// Satu goroutine dan satu timer per event tidak berbatas pada jumlah event, tak
// terlihat oleh uji apa pun yang tidak tidur, dan tidak dapat direkonsiliasi
// setelah restart. Satu sweeper di atas satu map berbatas, dapat disuntik jamnya,
// dan deterministik.
func (t *Tracker) Run(ctx context.Context) {
	interval := time.Duration(t.opt.SweepIntervalMs) * time.Millisecond
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	counterTicker := time.NewTicker(counterLogInterval)
	defer counterTicker.Stop()

	for {
		select {
		case <-ticker.C:
			t.sweep(ctx)
		case <-counterTicker.C:
			t.logCounters("periodik")
		case <-ctx.Done():
			t.logCounters("shutdown")
			return
		}
	}
}

// sweep menutup event yang bukti barunya berhenti datang dan mengevakuasi
// tombstone yang sudah melewati masa retensinya. Kedua pekerjaan itu satu tick
// karena keduanya adalah "waktu telah berlalu" dan retensi terminal karenanya tidak
// butuh timer kedua maupun pemilik penghapusan kedua (§5.4, §6.8).
func (t *Tracker) sweep(ctx context.Context) {
	t.mu.Lock()
	transitions := t.sweepLocked()
	t.mu.Unlock()

	t.publish(ctx, transitions)
}

func (t *Tracker) sweepLocked() []Snapshot {
	now := t.now().UnixMilli()
	var out []Snapshot

	for _, e := range t.openLocked() {
		if now-e.LastEvidenceTS <= t.opt.ResolveAfterMs {
			continue
		}
		// DETECTED KEDALUWARSA, tidak "diselesaikan": ia tidak pernah publik, jadi
		// tidak ada all-clear yang berutang kepada siapa pun, dan D->RESOLVED
		// ilegal justru karena itu (§5.2). Ia juga tidak meninggalkan tombstone:
		// tombstone ada untuk mencegah alert kedua yang dapat dilihat pengguna, dan
		// event yang tak pernah terlihat tidak dapat menghasilkan satu pun.
		if e.State == StateDetected {
			t.dropLocked(e)
			continue
		}
		if s := t.forceTransitionLocked(e, StateResolved, now, ReasonNoNewEvidence); s != nil {
			out = append(out, *s)
		}
	}

	for _, e := range t.tombstonesLocked() {
		if now-e.TerminalAt > t.opt.TerminalRetentionMs {
			t.dropLocked(e)
		}
	}
	return out
}

// InvalidateContributor mencabut satu node dari setiap event terbuka, menjalankan
// classify ulang, dan membatalkan apa pun yang jatuh di bawah lantainya.
//
// Ini SATU-SATUNYA jalan masuk ke CANCELLED, dan Fase 3 tidak menambahkan satu pun
// pemanggil otomatis: sebuah invalidator otomatis adalah mekanisme yang dapat
// menarik kembali peringatan gempa publik tanpa manusia, digerakkan heuristik yang
// belum ada yang memvalidasi. Urutan yang benar adalah menyimpan buktinya lebih
// dulu, mengukur perilaku node yang sesungguhnya, baru menulis aturannya.
//
// reason boleh ReasonOperatorRetracted bila pencabutannya adalah keputusan operator
// atas event itu sendiri, bukan atas mutu buktinya; kosong berarti
// EVIDENCE_INVALIDATED.
func (t *Tracker) InvalidateContributor(ctx context.Context, nodeID, reason string) {
	if nodeID == "" {
		return
	}
	if reason == "" {
		reason = ReasonEvidenceInvalid
	}

	t.mu.Lock()
	now := t.now().UnixMilli()
	var out []Snapshot
	for _, e := range t.openLocked() {
		if _, ok := e.Contributors[nodeID]; !ok {
			continue
		}
		delete(e.Contributors, nodeID)

		// Tanpa satu pun kontributor tersisa, seluruh bukti event ini telah ditarik.
		if len(e.Contributors) == 0 {
			e.Invalidated = true
		}

		// classify tetap murni: ia melaporkan DETECTED untuk apa pun yang kini di
		// bawah lantai. Yang menerjemahkan "jatuh di bawah lantai setelah
		// pencabutan" menjadi CANCELLED adalah aturan §7.5, dan ia hidup di sini —
		// bukan di dalam classify, yang tidak tahu apa-apa soal pencabutan.
		next := classify(e)
		if next == StateDetected {
			next = StateCancelled
		}
		if s := t.forceTransitionLocked(e, next, now, reason); s != nil {
			out = append(out, *s)
		}
	}
	t.mu.Unlock()

	t.publish(ctx, out)
}
