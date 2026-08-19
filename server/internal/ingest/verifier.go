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

// Verify mem-parse payload mentah lalu menjalankan seluruh pipa verifikasi.
// Mengembalikan Trigger tervalidasi bila lolos; error spesifik bila ditolak.
//
// Pemanggil yang butuh node_id SEBELUM verifikasi kripto (mis. Subscriber, untuk
// mencocokkan node_id dengan segmen topik MQTT) memakai ParseTrigger lalu
// VerifyTrigger agar payload tidak di-parse dua kali di hot path.
func (v *Verifier) Verify(ctx context.Context, raw []byte) (*Trigger, error) {
	t, err := ParseTrigger(raw)
	if err != nil {
		return nil, err
	}
	if err := v.VerifyTrigger(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

// VerifyTrigger menjalankan pipa verifikasi atas trigger yang struktur payloadnya
// SUDAH divalidasi ParseTrigger: node aktif -> clock skew -> HMAC -> anti-replay.
// Urutan sengaja: tolak murah dulu, kripto terakhir.
func (v *Verifier) VerifyTrigger(ctx context.Context, t *Trigger) error {
	// 1. Clock skew — murah, tolak sebelum sentuh DB/kripto.
	nowMs := v.now().UnixMilli()
	if diff := nowMs - t.TS; diff > int64(MaxClockSkew/time.Millisecond) || diff < -int64(MaxClockSkew/time.Millisecond) {
		v.log.Warn("trigger ditolak: clock skew", "node_id", t.NodeID, "ts", t.TS, "server_ms", nowMs)
		return ErrClockSkew
	}

	// 2. Ambil secret node.
	node, err := v.store.GetNodeSecret(ctx, t.NodeID)
	if err != nil {
		return err
	}
	if !node.IsActive {
		v.log.Warn("trigger ditolak: node inactive", "node_id", t.NodeID)
		return ErrNodeInactive
	}

	// 3. Anti-replay awal (cek in-memory nilai last_seen; penegakan final via DB atomik).
	if t.TS <= node.LastSeenTS {
		v.log.Warn("trigger ditolak: replay", "node_id", t.NodeID, "ts", t.TS, "last_seen", node.LastSeenTS)
		return ErrReplay
	}

	// 4. Decrypt secret & verifikasi HMAC.
	secret, err := v.cipher.Decrypt(node.SecretEnc, node.SecretNonce)
	if err != nil {
		v.log.Error("gagal decrypt secret node", "node_id", t.NodeID, "err", err)
		return err
	}
	canonical := CanonicalString(t.NodeID, t.PGA, t.DurMs, t.TS)
	if !VerifyHMAC(secret, canonical, t.Signature) {
		v.log.Warn("trigger ditolak: HMAC invalid", "node_id", t.NodeID)
		return ErrBadSignature
	}

	// 5. Penegakan anti-replay final & atomik di DB (menang atas race antar-goroutine).
	ok, err := v.store.UpdateLastSeen(ctx, t.NodeID, t.TS)
	if err != nil {
		return err
	}
	if !ok {
		v.log.Warn("trigger ditolak: replay (race DB)", "node_id", t.NodeID, "ts", t.TS)
		return ErrReplay
	}

	return nil
}
