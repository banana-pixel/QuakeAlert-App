package store

// --- Bukti sesi read-only untuk CLI forensik P4-M6′ (D-015) ---
//
// Berkas TERPISAH dan itu disengaja: pembaca M1′/M2′ yang sudah divalidasi
// pemilik (event_timeline.go, trace/replay readers di event_lifecycle.go)
// tidak tersentuh satu bita pun oleh delta ini. Satu-satunya permukaan baru
// adalah SessionReadOnly + pure helper di bawah, semuanya SELECT.
//
// Koreksi teknis terhadap redaksi persetujuan (dilaporkan di H, bukan
// disembunyikan): yang diset lewat DSN adalah default_transaction_read_only
// (options=-c default_transaction_read_only=on). Di PG16 yang terukur,
//   default_transaction_read_only → on|client  (bukti penegakan klien)
//   transaction_read_only         → on|override (turunan efektif, selalu
//     override by design — bahkan saat writable off|override).
// Menuntut transaction_read_only source=client tidak terpenuhi di Postgres
// nyata (clip ведет ke gagal-tertutup palsu). Karena itu SessionReadOnly
// memeriksa KEDUANYA: default on/client membuktikan DSN menegakkan, efektif
// on membuktikan sesi efektif read-only. Tetap gagal-tertutup.
//
// Kekuatan bukti, dinyatakan tepat (jangan dibaca lebih):
// SessionReadOnly bertanya kepada server, lewat pool pgx yang SAMA dengan
// yang dipakai keempat pembacaan forensik. Ini TIDAK membuktikan setiap
// koneksi fisik pool disampel satu per satu: pool (maxConns/minConns di
// store.go) dapat membuka beberapa backend dari DSN yang sama, dan kueri ini
// dijawab oleh salah satunya. Karena seluruh backend pool dibangun dari
// DATABASE_URL yang sama, hasilnya membuktikan konfigurasi pool, bukan
// sensus per-backend.

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ReadOnlyProof adalah hasil verifikasi sesi yang aman dicetak: hanya
// setting/source dari kedua GUC + application_name. Tidak ada DSN, hostname,
// password, atau secret dalam struct ini — dan FormatReadOnlyBanner menjaga itu.
type ReadOnlyProof struct {
	DefaultSetting   string
	DefaultSource    string
	EffectiveSetting string
	EffectiveSource  string
	ApplicationName  string
}

// validateDefaultRO gagal-tertutup untuk penegakan klien: hanya on/client.
// validateEffectiveRO gagal-tertutup untuk efektivitas: hanya on (source
// efektif selalu override by design, bahkan saat writable). Keduanya pure
// supaya dapat diuji tanpa database.
func validateDefaultRO(setting, source string) error {
	if setting != "on" {
		return fmt.Errorf("default_transaction_read_only setting=%q; mau \"on\"", setting)
	}
	if source != "client" {
		return fmt.Errorf("default_transaction_read_only source=%q; mau \"client\"", source)
	}
	return nil
}

func validateEffectiveRO(setting string) error {
	if setting != "on" {
		return fmt.Errorf("transaction_read_only setting=%q; mau \"on\"", setting)
	}
	return nil
}

// validateReadOnly mempertahankan nama lama untuk kompatibilitas uji: ia
// memvalidasi pasangan default (on/client). Efektif diperiksa terpisah.
func validateReadOnly(setting, source string) error {
	return validateDefaultRO(setting, source)
}

// SessionReadOnly menanyakan server lewat pool yang sama dan gagal-tertutup
// bila sesi tidak terbukti read-only yang dipasang klien.
//
// Dua baris dibaca sekaligus (satu kueri, read-only):
//
//	default_transaction_read_only → harus on|client (penegakan DSN)
//	transaction_read_only         → harus on (efektif; source-nya override)
//
// plus current_setting('application_name') sebagai metadata korelasi (bukan
// bukti). Tidak ada INSERT/UPDATE/DELETE.
func (s *Store) SessionReadOnly(ctx context.Context) (ReadOnlyProof, error) {
	var proof ReadOnlyProof
	const q = `SELECT name, setting, source FROM pg_settings WHERE name IN ('default_transaction_read_only','transaction_read_only')`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return ReadOnlyProof{}, fmt.Errorf("verifikasi sesi read-only: %w", err)
	}
	defer rows.Close()
	found := map[string][2]string{}
	for rows.Next() {
		var name, setting, source string
		if err := rows.Scan(&name, &setting, &source); err != nil {
			return ReadOnlyProof{}, fmt.Errorf("verifikasi sesi read-only: %w", err)
		}
		found[name] = [2]string{setting, source}
	}
	if err := rows.Err(); err != nil {
		return ReadOnlyProof{}, fmt.Errorf("verifikasi sesi read-only: %w", err)
	}
	def, okDef := found["default_transaction_read_only"]
	eff, okEff := found["transaction_read_only"]
	if !okDef || !okEff {
		return ReadOnlyProof{}, fmt.Errorf("verifikasi sesi read-only: baris pg_settings hilang")
	}
	if err := validateDefaultRO(def[0], def[1]); err != nil {
		return ReadOnlyProof{}, err
	}
	if err := validateEffectiveRO(eff[0]); err != nil {
		return ReadOnlyProof{}, err
	}
	proof.DefaultSetting, proof.DefaultSource = def[0], def[1]
	proof.EffectiveSetting, proof.EffectiveSource = eff[0], eff[1]
	// application_name hanya korelasi (MUST NOT dibaca sebagai bukti).
	// current_setting(..., true) mengembalikan NULL (bukan galat) bila unset;
	// perlakukan NULL/kosong sebagai "".
	var appName *string
	if err := s.pool.QueryRow(ctx, `SELECT current_setting('application_name', true)`).Scan(&appName); err != nil {
		return ReadOnlyProof{}, fmt.Errorf("baca application_name: %w", err)
	}
	if appName != nil {
		proof.ApplicationName = *appName
	}
	return proof, nil
}

// isSafeAppName melaporkan apakah application_name aman disisipkan ke DSN
// tanpa encoding: hanya [A-Za-z0-9_.-], 1..64 byte. Karakter lain (&, =, ?,
// spasi, ;, /) akan memecah parser DSN dan ditolak (gagal-tertutup).
func isSafeAppName(s string) bool {
	if len(s) == 0 || len(s) > 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
			c == '_' || c == '-' || c == '.' {
			continue
		}
		return false
	}
	return true
}

// EnsureApplicationName mengembalikan DSN dengan application_name yang
// dijamin ada, tanpa mengubah kredensial/host/parameter lain. Bila DSN sudah
// membawa application_name (pemeriksaan case-insensitive), DSN dikembalikan
// apa adanya. Bila appName tidak aman, mengembalikan galat (pemanggil wajib
// die, bukan fallback diam-diam). Tidak pernah mencetak DSN.
func EnsureApplicationName(dsn, appName string) (string, error) {
	if !isSafeAppName(appName) {
		return "", fmt.Errorf("application_name tidak aman: %q", appName)
	}
	if strings.Contains(strings.ToLower(dsn), "application_name=") {
		return dsn, nil
	}
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + "application_name=" + appName, nil
}

// DefaultAppName membangun application_name korelasi yang aman DSN:
// m6-<8alnum>-<YYYYMMDDTHHMMSSZ>. Hanya [A-Za-z0-9.-]. Short dari karakter
// alfanumerik event_id (UUID: 8 hex pertama). Gagal-tertutup bila tidak ada
// 8 alfanumerik. Timestamp UTC presisi-detik. Dipakai CLI operator; diuji di
// sini supaya scripts/event_timeline.go tetap tipis.
func DefaultAppName(eventID string, now time.Time) (string, error) {
	var alnum []byte
	for i := 0; i < len(eventID) && len(alnum) < 8; i++ {
		c := eventID[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			alnum = append(alnum, c)
		}
	}
	if len(alnum) != 8 {
		return "", fmt.Errorf("event_id tidak memuat 8 alfanumerik untuk application_name")
	}
	ts := now.UTC().Format("20060102T150405Z")
	return "m6-" + string(alnum) + "-" + ts, nil
}

// FormatReadOnlyBanner memformat bukti sesi menjadi baris-baris aman stdout.
// Hanya kedua GUC + application_name. Secara konstruksi tidak dapat memuat
// DSN/password/host karena inputnya hanya struct di atas; pemanggil dilarang
// menggabungkan DSN ke string ini.
func FormatReadOnlyBanner(p ReadOnlyProof) string {
	app := p.ApplicationName
	if app == "" {
		app = "(kosong)"
	}
	return fmt.Sprintf("sesi read-only : default_transaction_read_only=%s/%s effective_transaction_read_only=%s/%s application_name=%s\n"+
		"                   (pg_settings lewat pool yang sama; bukan sensus per-backend pool)",
		p.DefaultSetting, p.DefaultSource, p.EffectiveSetting, p.EffectiveSource, app)
}
