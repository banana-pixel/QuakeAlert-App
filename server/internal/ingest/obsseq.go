package ingest

import (
	"sync"
	"time"
)

// seqKey adalah identitas observasi protokol v2: (node_id, obs_seq, phase).
// Inilah kunci deduplikasi yang benar (§14.3) — sensor yang menetapkannya,
// bukan server yang menebaknya.
type seqKey struct {
	nodeID string
	obsSeq int64
	phase  string
}

// seqCache menolak observasi v2 yang sudah pernah diterima.
//
// Mengapa di memori, bukan di basis data: gerbang ini berada di jalur peringatan,
// dan menaruh sebuah SELECT/INSERT di depan konsensus adalah hal yang seluruh
// rancangan ledger asinkron dihindari (I9/D17). Sebuah unique index pada
// sensor_observations tidak dapat menggantikannya: penulisan ledger asinkron,
// berbatas, dan boleh dibuang, sehingga konflik indeksnya tiba jauh setelah
// keputusan yang seharusnya ia cegah.
//
// Mengapa itu cukup: retensi cache SAMA dengan MaxTriggerAge, jendela freshness
// trigger. Duplikat yang lebih tua dari itu sudah ditolak ErrClockSkew sebelum
// mencapai cache, jadi tidak ada duplikat yang dapat lolos dengan cara menunggu.
// Konsekuensi restart proses pun terbatas oleh alasan yang sama: cache yang
// kosong hanya dapat melewatkan duplikat yang tiba dalam lima menit setelah
// restart, dan bagi PRELIM/FINAL dari satu obs_seq gerbang last_seen_ts
// monotonik tetap berdiri di belakangnya.
//
// Kunci menyimpan waktu penerimaan, bukan hanya keberadaan, karena tanpa itu
// prune tidak dapat membedakan entri yang boleh dilupakan dari yang masih
// menjaga sesuatu.
type seqCache struct {
	ttlMs int64

	mu        sync.Mutex
	seen      map[seqKey]int64
	lastPrune int64
}

func newSeqCache(ttl time.Duration) *seqCache {
	return &seqCache{
		ttlMs: int64(ttl / time.Millisecond),
		seen:  make(map[seqKey]int64),
	}
}

// admit mencatat k sebagai terlihat dan melaporkan apakah ia BARU. false berarti
// duplikat.
//
// Cek dan pencatatan terjadi di bawah satu lock: dua percobaan kirim dari node
// yang sama dapat diproses oleh dua goroutine callback MQTT sekaligus, dan
// memisahkan keduanya akan membuat keduanya lolos.
func (c *seqCache) admit(k seqKey, nowMs int64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.pruneLocked(nowMs)
	if _, dup := c.seen[k]; dup {
		return false
	}
	c.seen[k] = nowMs
	return true
}

// pruneLocked membuang entri yang lebih tua dari ttl, paling sering sekali per
// ttl. Dijadwalkan oleh kedatangan, bukan oleh timer: tanpa trigger tidak ada
// yang perlu dibersihkan, dan sebuah goroutine yang berjalan selamanya untuk map
// yang biasanya kosong adalah biaya tanpa imbalan.
func (c *seqCache) pruneLocked(nowMs int64) {
	if nowMs-c.lastPrune < c.ttlMs {
		return
	}
	c.lastPrune = nowMs
	for k, seenAt := range c.seen {
		if nowMs-seenAt > c.ttlMs {
			delete(c.seen, k)
		}
	}
}

// size melaporkan jumlah entri; dipakai test untuk memastikan prune bekerja.
func (c *seqCache) size() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.seen)
}
