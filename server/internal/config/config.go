// Package config memuat konfigurasi runtime dari environment variable.
// Tanpa dependency eksternal (12-factor): semua via os.Getenv dengan default aman.
package config

import (
	"fmt"
	"os"
	"strconv"
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

	// Timeout IO (Aturan Server #3: <= 2s)
	IOTimeout time.Duration

	// HTTP
	HTTPAddr string
}

// Load membaca & memvalidasi konfigurasi dari environment.
func Load() (*Config, error) {
	cfg := &Config{
		DatabaseURL:     getEnv("DATABASE_URL", "postgres://quakealert:devpassword@localhost:5432/quakealert?sslmode=disable"),
		MQTTBroker:      getEnv("MQTT_BROKER", "tcp://localhost:1883"),
		MQTTUser:        getEnv("MQTT_USER", ""),
		MQTTPassword:    getEnv("MQTT_PASSWORD", ""),
		MQTTClientID:    getEnv("MQTT_CLIENT_ID", "quakealert-server"),
		HTTPAddr:        getEnv("HTTP_ADDR", ":8080"),
		ConsensusWindow: time.Duration(getEnvInt("CONSENSUS_WINDOW_MS", 8000)) * time.Millisecond,
		IOTimeout:       time.Duration(getEnvInt("IO_TIMEOUT_MS", 2000)) * time.Millisecond,
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

	if cfg.IOTimeout > 2*time.Second {
		return nil, fmt.Errorf("IO_TIMEOUT_MS harus <= 2000 (Aturan Server #3), dapat %s", cfg.IOTimeout)
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
