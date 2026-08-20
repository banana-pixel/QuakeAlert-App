package api

import (
	"context"
	"sync"
	"time"
)

// RateLimiter membatasi frekuensi aksi per-key (mis. reroll pseudonym 1x/60s).
// Allow mengembalikan true bila aksi diizinkan (dan menandai key ter-pakai
// selama window), false bila masih dalam cooldown.
//
// Kontrak sengaja minimal agar bisa didukung Redis (SET key NX EX 60) maupun
// implementasi in-memory untuk dev/test. Semua IO membawa context (Aturan #3).
type RateLimiter interface {
	Allow(ctx context.Context, key string, window time.Duration) (bool, error)
}

// MemoryRateLimiter adalah RateLimiter in-memory (dev/test / fallback bila Redis
// tak tersedia). Aman untuk konkurensi. TIDAK cocok untuk multi-instance
// (gunakan Redis di produksi multi-replica).
//
// Map key hanya dipangkas saat ukuran melewati maxMemoryLimiterKeys, membatasi
// pertumbuhan memori tak terkendali (mis. key per-IP dari /auth/anonymous).
type MemoryRateLimiter struct {
	mu      sync.Mutex
	expires map[string]time.Time
}

// maxMemoryLimiterKeys membatasi ukuran map sebelum pemangkasan entri basi.
const maxMemoryLimiterKeys = 10_000

// NewMemoryRateLimiter membuat limiter in-memory kosong.
func NewMemoryRateLimiter() *MemoryRateLimiter {
	return &MemoryRateLimiter{expires: make(map[string]time.Time)}
}

// Allow menandai key selama window bila belum ada entri aktif.
func (m *MemoryRateLimiter) Allow(_ context.Context, key string, window time.Duration) (bool, error) {
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()

	if exp, ok := m.expires[key]; ok && now.Before(exp) {
		return false, nil
	}
	if len(m.expires) >= maxMemoryLimiterKeys {
		m.pruneLocked(now)
	}
	m.expires[key] = now.Add(window)
	return true, nil
}

// pruneLocked menghapus entri yang sudah kedaluwarsa. Harus dipanggil dengan
// m.mu terkunci.
func (m *MemoryRateLimiter) pruneLocked(now time.Time) {
	for k, exp := range m.expires {
		if !now.Before(exp) {
			delete(m.expires, k)
		}
	}
}
