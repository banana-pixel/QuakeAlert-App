// Package config memuat konfigurasi runtime dari environment variable.
// Tanpa dependency eksternal (12-factor): semua via os.Getenv dengan default aman.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config adalah konfigurasi tervalidasi untuk server QuakeAlert.
type Config struct {
	// Postgres
	DatabaseURL string

	// MQTT
	MQTTBroker   string // mis. "tls://broker.quakealert.id:8883" atau "tcp://localhost:1883" (dev)
	MQTTUser     string
	MQTTPassword string
	MQTTClientID string

	// Crypto: kunci master AES-256-GCM (32 byte) untuk decrypt secret_key_enc node.
	// Disediakan sebagai hex 64-char via env; TIDAK pernah di-log.
	MasterKey [32]byte

	// Konsensus
	ConsensusWindow time.Duration

	// CooldownDuration: jeda minimum antar-emisi event gempa (dedup event_id)
	// sekaligus waktu menuju EVENT_RESOLVED (state machine SYSTEM_SPEC:
	// CONFIRMED -> COOLDOWN_RUNNING(90s) -> RESOLVED).
	CooldownDuration time.Duration

	// Timeout IO (Aturan Server #3: <= 2s)
	IOTimeout time.Duration

	// HTTP
	HTTPAddr string

	// WebSocket: daftar origin yang diizinkan (comma-separated di env).
	// Kosong = tolak semua lintas-origin (aman secara default). "*" = izinkan semua.
	WSAllowedOrigins []string

	// FCM (opsional): project ID + path file service account JSON.
	// Bila keduanya kosong, delivery background dinonaktifkan (hanya WebSocket).
	FCMProjectID       string
	FCMCredentialsFile string

	// REST API: secret JWT (HS256) untuk auth anonymous. Wajib di-set.
	JWTSecret []byte

	// JWTTokenTTL adalah masa hidup token anonim yang diterbitkan
	// POST /api/v1/auth/anonymous. Panjang secara default karena identitas
	// anonim tidak punya alur refresh.
	JWTTokenTTL time.Duration

	// Redis: dipakai rate-limiter reroll pseudonym (SET NX EX 60s).
	RedisURL string

	// AdminAPIKey adalah secret untuk endpoint operator (POST
	// /api/v1/admin/broadcasts). KOSONG = endpoint admin tidak didaftarkan sama
	// sekali, bukan didaftarkan tanpa proteksi: sebuah instalasi yang lupa
	// mengisinya harus kehilangan fiturnya, bukan kehilangan pagarnya.
	//
	// Bukan JWT dan bukan user_profiles.is_admin: pengirimnya adalah skrip shell
	// di host terpercaya, jadi secret tidak pernah ikut dalam APK dan pencabutan
	// hanya berarti mengganti env lalu restart.
	AdminAPIKey string

	// MQTT public config yang dikembalikan saat provisioning node (bukan koneksi
	// server → broker, melainkan yang dipakai firmware untuk konek).
	MQTTPublicBroker string
	MQTTPublicPort   int
	MQTTPublicTLS    bool

	// ObservationLedgerEnabled menyalakan pencatatan sensor_observations dan
	// alert_emissions. Default AKTIF: tanpa ledger, satu-satunya jejak masukan
	// sensor adalah log yang akan dirotasi. Mematikannya menghilangkan
	// pencatatan, dan HANYA pencatatan — tidak ada satu pun keputusan
	// peringatan yang berubah karenanya.
	ObservationLedgerEnabled bool

	// ObservationLedgerQueueSize adalah kapasitas antrean tulis ledger. Saat
	// penuh, baris TERTUA dibuang dan counter drop naik; antrean TIDAK pernah
	// memblokir produsennya, karena produsennya adalah jalur peringatan.
	ObservationLedgerQueueSize int

	// --- Fase 3: pelacakan siklus hidup event (§11.5) ---

	// EventTrackerEnabled memilih jalur deteksi di main.go: false =
	// consensus.Engine (perilaku Fase 2, byte-identik), true = event.Tracker.
	// Default FALSE untuk deploy pertama; jalur lama tetap dapat dieksekusi
	// selama tepat satu rilis (§11.3) sebelum Engine dihapus.
	EventTrackerEnabled bool

	// CorrelationWindow adalah W pada §6.3: dua observasi boleh menempel pada satu
	// event bila |onset - origin_ts| <= W. Menggantikan CONSENSUS_WINDOW_MS, yang
	// tetap dibaca sebagai fallback selama satu rilis — sebuah .env basi yang masih
	// menyebut nama lama harus MEWARISI nilainya secara terlihat, bukan diam-diam
	// memakai default baru yang lebih besar.
	CorrelationWindow time.Duration

	// AttachRadiusKm adalah jarak maksimum node ke sentroid event agar observasinya
	// menempel (dahulu ClusterRadiusKm).
	//
	// Batas atasnya DIVALIDASI (lihat validateEventTracker): bound bujur yang
	// membuat pencarian kandidat lengkap secara global hanya berlaku selama radius
	// tidak melebihi seperempat lingkar bumi. Batasnya matematis, bukan geografis:
	// tidak ada pita lintang yang disyaratkan.
	AttachRadiusKm float64

	// IndependenceCellKm adalah sisi sel independensi geografis (§7.3). Ikut masuk
	// ke algo_ver setiap baris keputusan, karena mengubahnya mengubah arti
	// "independent_cell_count" pada baris-baris lampau.
	IndependenceCellKm float64

	// MinIndependentCells adalah jumlah sel independen minimum untuk CONFIRMED
	// (§7.3): tiga sensor di satu meja bukan tiga bukti.
	MinIndependentCells int

	// MaxEventDiameterKm membatasi rentang geografis satu event (§6.4).
	MaxEventDiameterKm float64

	// EventResolveAfter adalah lama tanpa bukti baru sebelum event menjadi RESOLVED
	// (dahulu COOLDOWN_MS, nilai sama, nama yang jujur).
	EventResolveAfter time.Duration

	// EventSweepInterval adalah periode tick sweeper (§5.4).
	EventSweepInterval time.Duration

	// EventTrackerMaxOpen membatasi peta event terbuka (§15.4). Di batas, event
	// TERTUA dipaksa resolve — tidak pernah dibuang diam-diam.
	EventTrackerMaxOpen int

	// TerminalRetention adalah masa hidup tombstone (§6.8): selama ini, bukti yang
	// datang terlambat untuk event yang sudah terminal DISERAP, bukan menjadi event
	// kedua untuk gempa yang sama. Cocok dengan RECENT_WINDOW_MS di Android.
	TerminalRetention time.Duration

	// EventTrackerMaxTombstones membatasi jumlah tombstone, terpisah dari batas
	// event terbuka (§15.4).
	EventTrackerMaxTombstones int

	// Warnings adalah pesan yang WAJIB dilog pemanggil setelah logger siap.
	// Ada karena config tidak punya logger sendiri, sementara fallback nama env
	// yang usang harus terlihat oleh operator — fallback senyap adalah cara
	// termudah menjalankan produksi dengan jendela korelasi yang salah.
	Warnings []string

	// SingleNodeGeoTopicGuard melarang kluster satu-node memilih topik FCM
	// nasional. Default AKTIF. Matikan hanya untuk instalasi uji yang memang
	// hanya punya satu sensor dan menerima konsekuensinya.
	//
	// ALGO_VER SENGAJA TIDAK ADA DI SINI: versi algoritma harus mengikuti biner
	// yang membuat keputusan, jadi ia konstanta compile-time (ledger.AlgoVer).
	// Sebagai env var, operator dapat memberi label salah pada keputusan lampau.
	SingleNodeGeoTopicGuard bool
}

// minAdminKeyLen adalah panjang minimum ADMIN_API_KEY. 32 byte acak (mis.
// `openssl rand -hex 32`) berada di luar jangkauan brute force lewat HTTP.
const minAdminKeyLen = 32

// Load membaca & memvalidasi konfigurasi dari environment.
func Load() (*Config, error) {
	cfg := &Config{
		DatabaseURL:      getEnv("DATABASE_URL", "postgres://quakealert:devpassword@localhost:5432/quakealert?sslmode=disable"),
		MQTTBroker:       getEnv("MQTT_BROKER", "tcp://localhost:1883"),
		MQTTUser:         getEnv("MQTT_USER", ""),
		MQTTPassword:     getEnv("MQTT_PASSWORD", ""),
		MQTTClientID:     getEnv("MQTT_CLIENT_ID", "quakealert-server"),
		HTTPAddr:         getEnv("HTTP_ADDR", ":8080"),
		ConsensusWindow:  time.Duration(getEnvInt("CONSENSUS_WINDOW_MS", 8000)) * time.Millisecond,
		CooldownDuration: time.Duration(getEnvInt("COOLDOWN_MS", 90000)) * time.Millisecond,
		IOTimeout:        time.Duration(getEnvInt("IO_TIMEOUT_MS", 2000)) * time.Millisecond,

		FCMProjectID:       getEnv("FCM_PROJECT_ID", ""),
		FCMCredentialsFile: getEnv("FCM_CREDENTIALS_FILE", ""),

		RedisURL:         getEnv("REDIS_URL", "redis://localhost:6379/0"),
		MQTTPublicBroker: getEnv("MQTT_PUBLIC_BROKER", "broker.quakealert.id"),
		MQTTPublicPort:   getEnvInt("MQTT_PUBLIC_PORT", 8883),
		MQTTPublicTLS:    getEnvBool("MQTT_PUBLIC_TLS", true),

		AdminAPIKey: getEnv("ADMIN_API_KEY", ""),

		ObservationLedgerEnabled:   getEnvBool("OBSERVATION_LEDGER_ENABLED", true),
		ObservationLedgerQueueSize: getEnvInt("OBSERVATION_LEDGER_QUEUE_SIZE", 1024),
		SingleNodeGeoTopicGuard:    getEnvBool("SINGLE_NODE_GEO_TOPIC_GUARD", true),

		JWTTokenTTL: time.Duration(getEnvInt("JWT_TTL_HOURS", 720)) * time.Hour,

		EventTrackerEnabled:       getEnvBool("EVENT_TRACKER_ENABLED", false),
		AttachRadiusKm:            getEnvFloat("ATTACH_RADIUS_KM", 50),
		IndependenceCellKm:        getEnvFloat("INDEPENDENCE_CELL_KM", 5),
		MinIndependentCells:       getEnvInt("MIN_INDEPENDENT_CELLS", 2),
		MaxEventDiameterKm:        getEnvFloat("MAX_EVENT_DIAMETER_KM", 120),
		EventResolveAfter:         time.Duration(getEnvInt("EVENT_RESOLVE_AFTER_MS", 90000)) * time.Millisecond,
		EventSweepInterval:        time.Duration(getEnvInt("EVENT_SWEEP_INTERVAL_MS", 5000)) * time.Millisecond,
		EventTrackerMaxOpen:       getEnvInt("EVENT_TRACKER_MAX_OPEN", 256),
		TerminalRetention:         time.Duration(getEnvInt("TERMINAL_RETENTION_MS", 900000)) * time.Millisecond,
		EventTrackerMaxTombstones: getEnvInt("EVENT_TRACKER_MAX_TOMBSTONES", 512),
	}

	cfg.CorrelationWindow, cfg.Warnings = loadCorrelationWindow()

	if origins := os.Getenv("WS_ALLOWED_ORIGINS"); origins != "" {
		for _, o := range strings.Split(origins, ",") {
			if trimmed := strings.TrimSpace(o); trimmed != "" {
				cfg.WSAllowedOrigins = append(cfg.WSAllowedOrigins, trimmed)
			}
		}
	}

	// Master key wajib untuk verifikasi HMAC (decrypt secret node).
	keyHex := os.Getenv("MASTER_KEY_HEX")
	if keyHex == "" {
		return nil, fmt.Errorf("MASTER_KEY_HEX wajib di-set (hex 64-char untuk AES-256 key)")
	}
	key, err := decodeKey(keyHex)
	if err != nil {
		return nil, fmt.Errorf("MASTER_KEY_HEX tidak valid: %w", err)
	}
	cfg.MasterKey = key

	// JWT secret wajib untuk auth REST/WS (HS256). Minimal 32 byte agar aman.
	jwtSecret := os.Getenv("JWT_SECRET")
	if len(jwtSecret) < 32 {
		return nil, fmt.Errorf("JWT_SECRET wajib di-set (minimal 32 byte untuk HS256), dapat %d byte", len(jwtSecret))
	}
	cfg.JWTSecret = []byte(jwtSecret)

	// Kunci admin opsional, tetapi bila diisi harus benar-benar sebuah secret:
	// kunci pendek yang lolos di sini akan menjadi satu-satunya hal yang
	// memisahkan penyerang dari mengirim notifikasi ke seluruh pengguna.
	if cfg.AdminAPIKey != "" && len(cfg.AdminAPIKey) < minAdminKeyLen {
		return nil, fmt.Errorf("ADMIN_API_KEY minimal %d byte, dapat %d", minAdminKeyLen, len(cfg.AdminAPIKey))
	}

	if cfg.JWTTokenTTL <= 0 {
		return nil, fmt.Errorf("JWT_TTL_HOURS harus > 0, dapat %s", cfg.JWTTokenTTL)
	}

	if cfg.IOTimeout > 2*time.Second {
		return nil, fmt.Errorf("IO_TIMEOUT_MS harus <= 2000 (Aturan Server #3), dapat %s", cfg.IOTimeout)
	}

	if cfg.CooldownDuration <= 0 {
		return nil, fmt.Errorf("COOLDOWN_MS harus > 0, dapat %s", cfg.CooldownDuration)
	}

	if err := cfg.validateEventTracker(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// defaultCorrelationWindowMs adalah W baku Fase 3 (§6.3). Lebih besar dari
// CONSENSUS_WINDOW_MS Fase 2 (8000) karena ia mengukur jarak ke origin_ts, bukan
// lebar satu jendela publish.
const defaultCorrelationWindowMs = 20000

// loadCorrelationWindow menerapkan aturan fallback satu-rilis: CORRELATION_WINDOW_MS
// menang; bila ia tidak ada tetapi CONSENSUS_WINDOW_MS ada, nilai lama DIPAKAI dan
// sebuah peringatan dikembalikan. Yang dihindari di sini adalah kegagalan senyap:
// sebuah .env yang belum diperbarui akan menyempitkan jendela korelasi menjadi
// kurang dari separuhnya, dan tidak ada satu pun log yang menyebutkannya.
func loadCorrelationWindow() (time.Duration, []string) {
	if v := os.Getenv("CORRELATION_WINDOW_MS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return time.Duration(n) * time.Millisecond, nil
		}
	}
	if v := os.Getenv("CONSENSUS_WINDOW_MS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return time.Duration(n) * time.Millisecond, []string{fmt.Sprintf(
				"CONSENSUS_WINDOW_MS=%d dipakai sebagai CORRELATION_WINDOW_MS (nama lama, "+
					"didukung satu rilis); setel CORRELATION_WINDOW_MS secara eksplisit", n)}
		}
	}
	return defaultCorrelationWindowMs * time.Millisecond, nil
}

// maxAcceptedTriggerAge MENCERMINKAN ingest.MaxTriggerAge. Disalin alih-alih
// diimpor supaya paket config tetap tidak bergantung pada apa pun di dalam
// server (dan lewat ingest, pada klien MQTT); config_test.go menjaga agar
// keduanya tidak menyimpang.
const maxAcceptedTriggerAge = 5 * time.Minute

// maxAttachRadiusKm MENCERMINKAN event.MaxAttachRadiusKm, disalin dengan alasan
// yang sama seperti maxAcceptedTriggerAge: paket config tidak boleh bergantung
// pada apa pun di dalam server. event/cell_test.go menjaga agar keduanya tidak
// menyimpang.
//
// Nilainya seperempat lingkar bumi, pi*R/2 dengan R = 6371 km. Ia adalah titik
// tempat bound rentang bujur asin(sin(r/R)/cos φ) berhenti menjadi batas ATAS:
// di atasnya sin(r/R) mengecil lagi, sehingga rumus yang sama diam-diam
// MEREMEHKAN rentangnya — dan rentang yang diremehkan adalah kandidat yang
// terlewat, yakni dua event untuk satu gempa.
const maxAttachRadiusKm = 10007.543398010286

// validateEventTracker menolak konfigurasi tracker yang tidak masuk akal SAAT BOOT.
// Divalidasi tanpa melihat EventTrackerEnabled: sebuah nilai salah yang menunggu
// flag dinyalakan adalah nilai salah yang akan ditemukan pada saat terburuk.
func (c *Config) validateEventTracker() error {
	switch {
	case c.CorrelationWindow <= 0:
		return fmt.Errorf("CORRELATION_WINDOW_MS harus > 0, dapat %s", c.CorrelationWindow)
	case c.AttachRadiusKm <= 0:
		return fmt.Errorf("ATTACH_RADIUS_KM harus > 0, dapat %g", c.AttachRadiusKm)
	case c.AttachRadiusKm > maxAttachRadiusKm:
		// INVARIAN CAKUPAN (I-COV): pencarian kandidat wajib memuat setiap event
		// yang sentroidnya dalam ATTACH_RADIUS_KM. Lebar pencariannya diturunkan dari
		// radius ini lewat asin(sin(r/R)/cos φ), yang hanya sebuah batas atas selama
		// r <= pi*R/2. Radius yang lebih besar dari itu tidak sekadar aneh secara
		// fisik — ia membuat indeks melewatkan kandidat tanpa satu pun galat.
		return fmt.Errorf("ATTACH_RADIUS_KM harus <= %.0f km (seperempat lingkar bumi, "+
			"batas keberlakuan bound pencarian kandidat), dapat %g", maxAttachRadiusKm, c.AttachRadiusKm)
	case c.MaxEventDiameterKm > 0 && c.AttachRadiusKm > c.MaxEventDiameterKm:
		// Radius menempel yang lebih besar dari diameter maksimum event adalah
		// konfigurasi yang saling membatalkan: setiap observasi yang menempel lewat
		// radius akan ditolak oleh penjaga diameter, sehingga ATTACH_RADIUS_KM
		// berhenti berarti apa pun dan bukti berpencar menjadi event terpisah.
		return fmt.Errorf("ATTACH_RADIUS_KM (%g) harus <= MAX_EVENT_DIAMETER_KM (%g)",
			c.AttachRadiusKm, c.MaxEventDiameterKm)
	case c.IndependenceCellKm <= 0:
		return fmt.Errorf("INDEPENDENCE_CELL_KM harus > 0, dapat %g", c.IndependenceCellKm)
	case c.MinIndependentCells < 1:
		return fmt.Errorf("MIN_INDEPENDENT_CELLS harus >= 1, dapat %d", c.MinIndependentCells)
	case c.MaxEventDiameterKm <= 0:
		return fmt.Errorf("MAX_EVENT_DIAMETER_KM harus > 0, dapat %g", c.MaxEventDiameterKm)
	case c.EventResolveAfter <= 0:
		return fmt.Errorf("EVENT_RESOLVE_AFTER_MS harus > 0, dapat %s", c.EventResolveAfter)
	case c.EventSweepInterval <= 0:
		return fmt.Errorf("EVENT_SWEEP_INTERVAL_MS harus > 0, dapat %s", c.EventSweepInterval)
	case c.EventSweepInterval > c.EventResolveAfter:
		// Sweeper yang lebih lambat dari tenggat resolusi menunda RESOLVED tanpa
		// batas atas yang berarti: tenggatnya menjadi periode sweep, bukan nilai
		// yang disetel operator.
		return fmt.Errorf("EVENT_SWEEP_INTERVAL_MS (%s) harus <= EVENT_RESOLVE_AFTER_MS (%s)",
			c.EventSweepInterval, c.EventResolveAfter)
	case c.EventTrackerMaxOpen < 1:
		return fmt.Errorf("EVENT_TRACKER_MAX_OPEN harus >= 1, dapat %d", c.EventTrackerMaxOpen)
	case c.EventTrackerMaxTombstones < 1:
		return fmt.Errorf("EVENT_TRACKER_MAX_TOMBSTONES harus >= 1, dapat %d", c.EventTrackerMaxTombstones)
	case c.TerminalRetention < maxAcceptedTriggerAge:
		// Inti D28: tombstone harus hidup setidaknya selama usia trigger yang masih
		// diterima verifier. Retensi yang lebih pendek berarti sebuah trigger yang
		// sah namun terlambat dapat lolos verifikasi SETELAH tombstone-nya hilang,
		// lalu membuat event kedua — peringatan publik kedua untuk satu gempa, yang
		// justru dicegah oleh tombstone.
		return fmt.Errorf("TERMINAL_RETENTION_MS (%s) harus >= usia trigger maksimum yang diterima (%s)",
			c.TerminalRetention, maxAcceptedTriggerAge)
	}
	return nil
}

func decodeKey(hexStr string) ([32]byte, error) {
	var out [32]byte
	if len(hexStr) != 64 {
		return out, fmt.Errorf("panjang harus 64 hex char (32 byte), dapat %d", len(hexStr))
	}
	for i := 0; i < 32; i++ {
		b, err := strconv.ParseUint(hexStr[i*2:i*2+2], 16, 8)
		if err != nil {
			return out, err
		}
		out[i] = byte(b)
	}
	return out, nil
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getEnvFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

func getEnvBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}
