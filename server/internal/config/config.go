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
	}

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

	return cfg, nil
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

func getEnvBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}
