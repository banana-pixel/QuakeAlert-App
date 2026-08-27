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
//
// PASANGAN LINTAS REPOSITORI (§14.4): nilai ini HARUS >= firmware
// TRIGGER_MAX_AGE_MS (firmware/src/config.h, 300000 ms). Firmware menyerah
// mencoba kirim tepat pada batas itu; server yang lebih ketat akan menolak
// percobaan terakhir yang masih sah, dan tidak ada satu pun test yang akan
// gagal karenanya — kegagalannya hanya terlihat sebagai laporan yang hilang di
// lapangan. Asimetrinya terhadap MaxClockSkew (+30s) disengaja: laporan tidak
// dapat datang dari masa depan secara sah, tetapi ia dapat datang terlambat.
const MaxTriggerAge = 5 * time.Minute

// MaxOnsetSkew adalah toleransi pemeriksaan koherensi onset v2 (§14.4).
//
// Dua ms tidak akan pernah cukup dan sepuluh detik tidak akan pernah menangkap
// apa pun; angka ini dipilih dari yang diketahui firmware: PRELIM dipublish
// dalam satu iterasi loop setelah onset dikonfirmasi, dan FINAL dalam satu
// iterasi setelah detrigger, jadi percobaan PERTAMA selalu rapat terhadap
// dur_ms. Percobaan berikutnya tidak: seluruh maksud attempt_no adalah bahwa
// keterlambatannya tidak terbatas, sehingga batas ATAS hanya berlaku pada
// attempt_no == 1.
const MaxOnsetSkew = 2 * time.Second

// Kesalahan verifikasi (life-safety: setiap penolakan di-log untuk audit).
var (
	ErrNodeInactive   = errors.New("node non-aktif")
	ErrNodeUnverified = errors.New("node belum diverifikasi operator")
	ErrClockSkew      = errors.New("ts di luar jendela freshness (trigger: -5m/+30s)")
	ErrReplay         = errors.New("ts <= last_seen_ts (replay/stale)")
	ErrBadSignature   = errors.New("HMAC tidak valid")

	// ErrOnsetIncoherent adalah pemeriksaan waktu KEDUA dan independen (§14.4),
	// hanya untuk v2: onset yang ditandatangani harus konsisten dengan dur_ms dan
	// ts yang ditandatangani bersamanya. Jam node yang melompat di tengah event
	// gagal di sini, dan hari ini tidak ada apa pun yang menyadarinya.
	ErrOnsetIncoherent = errors.New("onset_ts tidak koheren dengan dur_ms/ts")

	// ErrDuplicateObservation menolak observasi v2 yang obs_seq+phase-nya sudah
	// pernah diterima — termasuk PRELIM yang diulang SETELAH FINAL, satu-satunya
	// urutan yang gerbang last_seen_ts monotonik tidak dapat menangkap: percobaan
	// ulang men-stempel ULANG ts, sehingga ts-nya justru lebih baru.
	ErrDuplicateObservation = errors.New("observasi duplikat (node_id, obs_seq, phase)")
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

	// seq mendeduplikasi observasi v2 pada (node_id, obs_seq, phase). Selalu ada
	// — bukan opsional — karena sebuah gerbang keamanan yang dapat dimatikan
	// dengan lupa memanggil setter bukan gerbang.
	seq *seqCache
}

// NewVerifier membuat verifier.
func NewVerifier(st *store.Store, c *crypto.Cipher, log *slog.Logger) *Verifier {
	return &Verifier{
		store:  st,
		cipher: c,
		log:    log,
		now:    time.Now,
		seq:    newSeqCache(MaxTriggerAge),
	}
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

	// 2. Koherensi onset v2 (§14.4) — juga murah, dan juga sebelum DB/kripto.
	// Pemeriksaan ini INDEPENDEN dari gerbang skew di atas: yang pertama
	// membandingkan ts dengan jam SERVER, yang ini membandingkan field-field yang
	// ditandatangani satu sama lain menurut jam SENSOR. Node yang jamnya melompat
	// di tengah event lolos yang pertama dan gagal di yang kedua.
	if err := verifyOnsetCoherence(t); err != nil {
		v.log.Warn("trigger ditolak: onset tidak koheren",
			"node_id", t.NodeID, "phase", t.Phase, "onset_ts", t.OnsetTS,
			"dur_ms", t.DurMs, "ts", t.TS, "attempt_no", t.AttemptNo)
		return err
	}

	// 3. Ambil secret node.
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

	// 4. Anti-replay awal (cek in-memory nilai last_seen; penegakan final via DB
	// atomik). Gerbang monotonik ini TETAP berlaku untuk v2, tidak diganti oleh
	// dedup obs_seq di bawah: ia satu-satunya pertahanan yang dimiliki node v1,
	// dan pertahanan berlapis di sini berharga satu perbandingan.
	if t.TS <= node.LastSeenTS {
		v.log.Warn("trigger ditolak: replay", "node_id", t.NodeID, "ts", t.TS, "last_seen", node.LastSeenTS)
		return ErrReplay
	}

	// 5. Decrypt secret & verifikasi HMAC.
	secret, err := v.cipher.Decrypt(node.SecretEnc, node.SecretNonce)
	if err != nil {
		v.log.Error("gagal decrypt secret node", "node_id", t.NodeID, "err", err)
		return err
	}
	if !VerifyHMAC(secret, canonicalFor(t), t.Signature) {
		v.log.Warn("trigger ditolak: HMAC invalid", "node_id", t.NodeID, "v2", t.IsV2())
		return ErrBadSignature
	}

	// 6. Dedup (node_id, obs_seq, phase) untuk v2 — SESUDAH HMAC, dan urutan itu
	// bukan optimasi. Mengisi cache sebelum tanda tangan diperiksa akan membuat
	// siapa pun yang dapat memublikasikan ke broker mampu "memakai" obs_seq milik
	// node lain lebih dulu, sehingga observasi ASLI berikutnya ditolak sebagai
	// duplikat. Deduplikasi hanya boleh memercayai identitas yang sudah terbukti.
	if t.IsV2() {
		key := seqKey{nodeID: t.NodeID, obsSeq: *t.ObsSeq, phase: t.Phase}
		if !v.seq.admit(key, nowMs) {
			v.log.Warn("trigger ditolak: duplikat obs_seq",
				"node_id", t.NodeID, "obs_seq", *t.ObsSeq, "phase", t.Phase,
				"attempt_no", *t.AttemptNo)
			return ErrDuplicateObservation
		}
	}

	// 7. Penegakan anti-replay final & atomik di DB (menang atas race antar-goroutine).
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

// verifyOnsetCoherence adalah pemeriksaan waktu KEDUA dan independen (§14.4),
// berlaku hanya untuk v2 — v1 tidak membawa onset, jadi tidak ada apa pun untuk
// diperiksa dan payload v1 lolos apa adanya (§12.1: v1 tidak pernah dihentikan).
//
// Yang diperiksa, dan mengapa masing-masing:
//
//  1. onset_ts <= ts. Getaran tidak dapat dimulai setelah laporannya dipublish.
//  2. ts - onset_ts >= dur_ms - MaxOnsetSkew. dur_ms adalah waktu yang BERLALU
//     antara onset dan publish; ia tidak dapat melebihi selisih itu.
//  3. Hanya pada attempt_no == 1: ts - onset_ts <= dur_ms + MaxOnsetSkew.
//     Percobaan pertama dipublish satu iterasi loop setelah keadaannya berubah.
//     Percobaan berikutnya sengaja dikecualikan: keterlambatan retry tidak
//     terbatas, dan itulah justru yang attempt_no ada untuk membuat terlihat.
//  4. Pada FINAL: onset_ts <= detrigger_ts <= ts dan
//     |detrigger_ts - onset_ts - dur_ms| <= MaxOnsetSkew. Inilah pemeriksaan
//     terkuat dari keempatnya, karena ia sepenuhnya berada di dalam jam sensor
//     dan tidak bergantung sama sekali pada penundaan publish.
//
// Yang TIDAK dilakukan: dur_ms maupun detrigger_ts tidak pernah menjadi gerbang
// KEPUTUSAN (D21). Keduanya di sini hanya memeriksa konsistensi internal laporan
// yang ditandatangani, bukan apakah getarannya layak menjadi peringatan.
func verifyOnsetCoherence(t *Trigger) error {
	if !t.IsV2() {
		return nil
	}

	onset := *t.OnsetTS
	tol := int64(MaxOnsetSkew / time.Millisecond)

	if onset > t.TS {
		return ErrOnsetIncoherent
	}
	elapsed := t.TS - onset
	if elapsed < t.DurMs-tol {
		return ErrOnsetIncoherent
	}
	if *t.AttemptNo == 1 && elapsed > t.DurMs+tol {
		return ErrOnsetIncoherent
	}

	if t.Phase == PhaseFinal {
		detrigger := *t.DetriggerTS
		if detrigger < onset || detrigger > t.TS {
			return ErrOnsetIncoherent
		}
		if diff := detrigger - onset - t.DurMs; diff > tol || diff < -tol {
			return ErrOnsetIncoherent
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Observation ledger (§5.1)
// ---------------------------------------------------------------------------

// TriggerObservation memetakan trigger yang sudah di-parse ke satu baris
// sensor_observations, untuk kedua versi protokol. Dipisahkan dan diekspor agar
// pemetaannya dapat diuji tanpa menjalankan seluruh pipa verifikasi.
//
// Yang TIDAK diisi di sini, dan mengapa:
//
//	Lat/Lon   — di-snapshot oleh writer di luar jalur peringatan; menambahkan
//	            satu query basis data di sini akan mengembalikan latensi DB ke
//	            depan konsensus, yaitu hal yang dihindari seluruh rancangan ini.
//
// Untuk payload v1, ProtoVer/ObsSeq/AttemptNo/OnsetTS/DetriggerTS tetap NULL:
// v1 tidak membawanya dan tidak ada cara merekonstruksinya di server, jadi NULL
// adalah kebenaran dan bukan data yang hilang.
func TriggerObservation(t *Trigger, receivedTS int64, verifyResult string) *ledger.Observation {
	// onset_ts_upper_bound = publish_ts - dur_ms. BATAS ATAS, bukan estimasi
	// onset: ts distempel saat publish dan distempel ULANG pada setiap retry,
	// sehingga selisihnya terhadap onset sebenarnya (publish_delay >= 0) tidak
	// terbatas. Boleh untuk mengurutkan dan mengelompokkan; TIDAK boleh untuk
	// mengkalibrasi jendela korelasi berbasis onset.
	//
	// Tetap dihitung dan disimpan untuk v2, meski onset yang SEBENARNYA sudah
	// diketahui di sana (§5.1 aturan 2): batas yang dapat dibandingkan dengan
	// kebenarannya adalah satu-satunya cara mengukur publish_delay — dan
	// publish_delay itulah yang membuat batas tersebut tidak dapat dipakai
	// mengkalibrasi apa pun di Fase 3.
	upperBound := t.TS - t.DurMs

	o := &ledger.Observation{
		NodeID:            t.NodeID,
		SourceClass:       ledger.SourceClassFixedESP32,
		Phase:             t.EffectivePhase(),
		PGAGal:            t.PGA,
		DurMs:             t.DurMs,
		PublishTS:         t.TS,
		ReceivedTS:        receivedTS,
		OnsetTSUpperBound: &upperBound,
		OnsetTSSource:     ledger.OnsetSourcePublish,
		Signature:         t.Signature,
		VerifyResult:      verifyResult,
	}
	if !t.IsV2() {
		return o
	}

	protoVer := int16(*t.ProtoVer)
	attemptNo := int16(*t.AttemptNo)
	o.ProtoVer = &protoVer
	o.ObsSeq = t.ObsSeq
	o.AttemptNo = &attemptNo
	o.OnsetTS = t.OnsetTS
	o.DetriggerTS = t.DetriggerTS
	// SENSOR, bukan PUBLISH_BOUND: onset di sini adalah instan yang dilaporkan
	// jam sensor dan ikut ditandatangani. Diskriminator inilah yang membuat
	// fleet campuran dapat dikorelasikan dengan benar di Fase 3 (§12.3).
	o.OnsetTSSource = ledger.OnsetSourceSensor
	return o
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
	case errors.Is(err, ErrOnsetIncoherent):
		return "ErrOnsetIncoherent", true
	case errors.Is(err, ErrDuplicateObservation):
		return "ErrDuplicateObservation", true
	default:
		return "", false
	}
}
