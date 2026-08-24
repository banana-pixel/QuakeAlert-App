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
// Dipakai oleh heartbeat (gate + pengukuran latency) — JANGAN diubah tanpa
// mempertimbangkan akurasi latency /sensors.
const MaxClockSkew = 30 * time.Second

// MaxTriggerAge melonggarkan gerbang freshness KHUSUS trigger agar publish
// ulang pasca-reconnect MQTT (firmware TRIGGER_MAX_AGE_MS = 5 menit) tetap
// diterima server. Anti-replay TIDAK bergantung pada konstanta ini: last_seen_ts
// monotonik ketat (UPDATE ... WHERE last_seen_ts < $2) menolak replay kapan pun.
const MaxTriggerAge = 5 * time.Minute

// Kesalahan verifikasi (life-safety: setiap penolakan di-log untuk audit).
var (
	ErrNodeInactive   = errors.New("node non-aktif")
	ErrNodeUnverified = errors.New("node belum diverifikasi operator")
	ErrClockSkew      = errors.New("ts di luar jendela freshness (trigger: -5m/+30s)")
	ErrReplay         = errors.New("ts <= last_seen_ts (replay/stale)")
	ErrBadSignature   = errors.New("HMAC tidak valid")
)

// nodeSource adalah subset store yang dibutuhkan verifier. Interface, bukan
// *store.Store konkret, agar pipa verifikasi dapat diuji dengan fake store —
// pola yang sama dengan api.Repo.
type nodeSource interface {
	GetNodeSecret(ctx context.Context, stationID string) (*store.NodeSecret, error)
	UpdateLastSeen(ctx context.Context, stationID string, ts int64) (bool, error)
}

// secretDecryptor membuka secret_key_enc node (implementasi: crypto.Cipher).
type secretDecryptor interface {
	Decrypt(ciphertext, nonce []byte) ([]byte, error)
}

// Verifier memverifikasi trigger: struktur -> node aktif & terverifikasi ->
// clock skew -> HMAC -> anti-replay. Urutan sengaja: tolak murah dulu,
// kripto terakhir.
type Verifier struct {
	store  nodeSource
	cipher secretDecryptor
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
// SUDAH divalidasi ParseTrigger: node aktif & terverifikasi -> clock skew ->
// HMAC -> anti-replay. Urutan sengaja: tolak murah dulu, kripto terakhir.
//
// Gerbang verified (migrasi 000005) berdiri sebelum HMAC dengan alasan yang sama
// seperti is_active: sebuah node yang belum dikonfirmasi operator tidak boleh
// ikut voting menuju ambang 3-node CONFIRMED, seberapa pun sah tanda tangannya —
// siapa pun yang bisa provision juga bisa menandatangani. Heartbeat node yang
// sama TETAP diterima di jalur lain, jadi stasiunnya tetap tampak di /sensors
// sebagai pending; hanya suaranya dalam konsensus yang tidak ada.
func (v *Verifier) VerifyTrigger(ctx context.Context, t *Trigger) error {
	// 1. Clock skew — murah, tolak sebelum sentuh DB/kripto. Trigger memakai
	// MaxTriggerAge (lebih longgar) agar retry publish ulang tetap diterima;
	// anti-replay di bawah tetap menjamin satu kali penerimaan per ts.
	nowMs := v.now().UnixMilli()
	if diff := nowMs - t.TS; diff > int64(MaxTriggerAge/time.Millisecond) || diff < -int64(MaxClockSkew/time.Millisecond) {
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
	if !node.Verified {
		v.log.Warn("trigger ditolak: node belum terverifikasi", "node_id", t.NodeID)
		return ErrNodeUnverified
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
