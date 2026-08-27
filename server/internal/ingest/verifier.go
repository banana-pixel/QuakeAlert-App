package ingest

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/banana-pixel/quakealert/server/internal/crypto"
	"github.com/banana-pixel/quakealert/server/internal/ledger"
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

// observationWriter menerima satu baris ledger per trigger yang sampai —
// diterima maupun ditolak. Implementasi: *ledger.Writer, yang antreannya
// berbatas dan tidak pernah memblokir; interface di sini mengikuti pola
// nodeSource/secretDecryptor agar pipa verifikasi tetap dapat diuji tanpa
// basis data, dan agar verifier tidak dapat memanggil apa pun yang menulis
// secara sinkron.
//
// Nil = ledger dinonaktifkan; setiap jalur pencatatan menjadi no-op.
type observationWriter interface {
	RecordObservation(o *ledger.Observation)
}

// Verifier memverifikasi trigger: struktur -> node aktif & terverifikasi ->
// clock skew -> HMAC -> anti-replay. Urutan sengaja: tolak murah dulu,
// kripto terakhir.
type Verifier struct {
	store  nodeSource
	cipher secretDecryptor
	log    *slog.Logger
	now    func() time.Time // diinjeksi untuk test
	ledger observationWriter
}

// NewVerifier membuat verifier.
func NewVerifier(st *store.Store, c *crypto.Cipher, log *slog.Logger) *Verifier {
	return &Verifier{store: st, cipher: c, log: log, now: time.Now}
}

// WithLedger memasang observation ledger dan mengembalikan v (pola chaining yang
// sama dengan Subscriber.WithHeartbeat). Setter, bukan parameter konstruktor,
// karena ledger bersifat opsional: memasangnya di NewVerifier akan memaksa
// setiap pemanggil — termasuk seluruh test yang ada — untuk menyebutkannya.
func (v *Verifier) WithLedger(w observationWriter) *Verifier {
	v.ledger = w
	return v
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
	// Jam server dibaca SATU KALI di sini, lalu dipakai sebagai gerbang
	// freshness DAN sebagai received_ts baris ledger. Membacanya dua kali akan
	// membuat received_ts tidak sepadan dengan keputusan yang baru saja dibuat
	// atas dasar waktu yang sama.
	receivedTS := v.now().UnixMilli()
	err := v.verify(ctx, t, receivedTS)
	v.recordObservation(t, receivedTS, err)
	return err
}

// verify adalah pipa verifikasi itu sendiri. Dipisah dari VerifyTrigger agar ada
// TEPAT SATU titik pencatatan yang mencakup jalur sukses maupun kelima jalur
// penolakan — sebuah `return` baru di dalam pipa tidak dapat luput dari ledger.
func (v *Verifier) verify(ctx context.Context, t *Trigger, nowMs int64) error {
	// 1. Clock skew — murah, tolak sebelum sentuh DB/kripto. Trigger memakai
	// MaxTriggerAge (lebih longgar) agar retry publish ulang tetap diterima;
	// anti-replay di bawah tetap menjamin satu kali penerimaan per ts.
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

// ---------------------------------------------------------------------------
// Observation ledger (§5.1)
// ---------------------------------------------------------------------------

// TriggerObservation memetakan trigger yang sudah di-parse ke satu baris
// sensor_observations. Dipisahkan dan diekspor agar pemetaannya dapat diuji
// tanpa menjalankan seluruh pipa verifikasi.
//
// Yang TIDAK diisi di sini, dan mengapa:
//
//	ProtoVer  — payload v1 tidak membawa versi protokol; NULL adalah kebenaran,
//	            bukan data yang hilang.
//	ObsSeq    — belum ada di kabel sampai Fase 2.
//	OnsetTS   — onset yang sebenarnya tidak dapat diketahui dari payload v1.
//	Lat/Lon   — di-snapshot oleh writer di luar jalur peringatan; menambahkan
//	            satu query basis data di sini akan mengembalikan latensi DB ke
//	            depan konsensus, yaitu hal yang dihindari seluruh rancangan ini.
func TriggerObservation(t *Trigger, receivedTS int64, verifyResult string) *ledger.Observation {
	// onset_ts_upper_bound = publish_ts - dur_ms. BATAS ATAS, bukan estimasi
	// onset: ts distempel saat publish dan distempel ULANG pada setiap retry,
	// dan v1 tidak membawa nomor percobaan, sehingga selisihnya terhadap onset
	// sebenarnya (publish_delay >= 0) tidak terbatas dan tidak terobservasi.
	// Boleh untuk mengurutkan dan mengelompokkan; TIDAK boleh untuk
	// mengkalibrasi jendela korelasi berbasis onset.
	upperBound := t.TS - t.DurMs

	return &ledger.Observation{
		NodeID:            t.NodeID,
		SourceClass:       ledger.SourceClassFixedESP32,
		Phase:             ledger.PhaseFinal,
		PGAGal:            t.PGA,
		DurMs:             t.DurMs,
		PublishTS:         t.TS,
		ReceivedTS:        receivedTS,
		OnsetTSUpperBound: &upperBound,
		OnsetTSSource:     ledger.OnsetSourcePublish,
		Signature:         t.Signature,
		VerifyResult:      verifyResult,
	}
}

// recordObservation mencatat hasil verifikasi. verifyErr nil berarti diterima.
func (v *Verifier) recordObservation(t *Trigger, receivedTS int64, verifyErr error) {
	if v.ledger == nil {
		return
	}
	result, ok := verifyResultName(verifyErr)
	if !ok {
		// Kegagalan infrastruktur (basis data tidak terjangkau, dekripsi secret
		// gagal) SENGAJA tidak dicatat: itu bukan pernyataan tentang perilaku
		// sensor, dan menyimpannya sebagai verify_result akan menyalahkan node
		// atas kesalahan server. Kegagalan seperti itu sudah dicatat sebagai log
		// ERROR di pipa di atas — dan bila basis datanya yang sedang mati,
		// INSERT ledger pun akan gagal.
		return
	}
	v.ledger.RecordObservation(TriggerObservation(t, receivedTS, result))
}

// verifyResultName menerjemahkan hasil verifikasi menjadi nilai kolom
// verify_result. ok=false berarti error tersebut bukan keputusan verifikasi dan
// tidak boleh menjadi baris ledger.
//
// verify_result mencatat OTENTIKASI saja. Tidak ada nilai untuk "lokasi node
// tidak diketahui": kasus tersebut terekam sebagai verify_result = 'OK' dengan
// node_location NULL, karena observasinya memang sah — yang tidak ada hanyalah
// koordinat yang dibutuhkan konsensus.
func verifyResultName(err error) (string, bool) {
	switch {
	case err == nil:
		return ledger.VerifyResultOK, true
	case errors.Is(err, ErrClockSkew):
		return "ErrClockSkew", true
	case errors.Is(err, ErrNodeInactive):
		return "ErrNodeInactive", true
	case errors.Is(err, ErrNodeUnverified):
		return "ErrNodeUnverified", true
	case errors.Is(err, ErrReplay):
		return "ErrReplay", true
	case errors.Is(err, ErrBadSignature):
		return "ErrBadSignature", true
	default:
		return "", false
	}
}
