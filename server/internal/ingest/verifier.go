package ingest

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/banana-pixel/quakealert/server/internal/crypto"
	"github.com/banana-pixel/quakealert/server/internal/store"
)

// MaxClockSkew adalah toleransi drift ts vs waktu server (.clinerules/10 #7).
const MaxClockSkew = 30 * time.Second

// Kesalahan verifikasi (life-safety: setiap penolakan di-log untuk audit).
var (
	ErrNodeInactive = errors.New("node non-aktif")
	ErrClockSkew    = errors.New("ts menyimpang > 30s dari waktu server")
	ErrReplay       = errors.New("ts <= last_seen_ts (replay/stale)")
	ErrBadSignature = errors.New("HMAC tidak valid")
)

// Verifier memverifikasi trigger: struktur -> node aktif -> clock skew ->
// HMAC -> anti-replay. Urutan sengaja: tolak murah dulu, kripto terakhir.
type Verifier struct {
	store  *store.Store
	cipher *crypto.Cipher
	log    *slog.Logger
	now    func() time.Time // diinjeksi untuk test
}

// NewVerifier membuat verifier.
func NewVerifier(st *store.Store, c *crypto.Cipher, log *slog.Logger) *Verifier {
	return &Verifier{store: st, cipher: c, log: log, now: time.Now}
}

// Verify menjalankan seluruh pipa verifikasi terhadap payload mentah.
// Mengembalikan Trigger tervalidasi bila lolos; error spesifik bila ditolak.
func (v *Verifier) Verify(ctx context.Context, raw []byte) (*Trigger, error) {
	t, err := ParseTrigger(raw)
	if err != nil {
		return nil, err
	}

	// 1. Clock skew — murah, tolak sebelum sentuh DB/kripto.
	nowMs := v.now().UnixMilli()
	if diff := nowMs - t.TS; diff > int64(MaxClockSkew/time.Millisecond) || diff < -int64(MaxClockSkew/time.Millisecond) {
		v.log.Warn("trigger ditolak: clock skew", "node_id", t.NodeID, "ts", t.TS, "server_ms", nowMs)
		return nil, ErrClockSkew
	}

	// 2. Ambil secret node.
	node, err := v.store.GetNodeSecret(ctx, t.NodeID)
	if err != nil {
		return nil, err
	}
	if !node.IsActive {
		v.log.Warn("trigger ditolak: node inactive", "node_id", t.NodeID)
		return nil, ErrNodeInactive
	}

	// 3. Anti-replay awal (cek in-memory nilai last_seen; penegakan final via DB atomik).
	if t.TS <= node.LastSeenTS {
		v.log.Warn("trigger ditolak: replay", "node_id", t.NodeID, "ts", t.TS, "last_seen", node.LastSeenTS)
		return nil, ErrReplay
	}

	// 4. Decrypt secret & verifikasi HMAC.
	secret, err := v.cipher.Decrypt(node.SecretEnc, node.SecretNonce)
	if err != nil {
		v.log.Error("gagal decrypt secret node", "node_id", t.NodeID, "err", err)
		return nil, err
	}
	canonical := CanonicalString(t.NodeID, t.PGA, t.DurMs, t.TS)
	if !VerifyHMAC(secret, canonical, t.Signature) {
		v.log.Warn("trigger ditolak: HMAC invalid", "node_id", t.NodeID)
		return nil, ErrBadSignature
	}

	// 5. Penegakan anti-replay final & atomik di DB (menang atas race antar-goroutine).
	ok, err := v.store.UpdateLastSeen(ctx, t.NodeID, t.TS)
	if err != nil {
		return nil, err
	}
	if !ok {
		v.log.Warn("trigger ditolak: replay (race DB)", "node_id", t.NodeID, "ts", t.TS)
		return nil, ErrReplay
	}

	return t, nil
}
