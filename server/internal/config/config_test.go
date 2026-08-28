package config

import (
	"testing"
	"time"

	"github.com/banana-pixel/quakealert/server/internal/ingest"
)

// maxAcceptedTriggerAge adalah salinan ingest.MaxTriggerAge (lihat komentarnya di
// config.go). Uji ini adalah satu-satunya hal yang menahan keduanya tetap sama;
// bila ingest melonggarkan gerbang freshness-nya, validasi TERMINAL_RETENTION_MS
// harus ikut longgar, atau invarian D28 dilanggar tanpa ada yang memberi tahu.
func TestMaxAcceptedTriggerAgeMirrorsIngest(t *testing.T) {
	if maxAcceptedTriggerAge != ingest.MaxTriggerAge {
		t.Fatalf("salinan menyimpang: config=%s ingest=%s — perbarui keduanya",
			maxAcceptedTriggerAge, ingest.MaxTriggerAge)
	}
}

// setMinimalEnv mengisi env yang WAJIB agar Load() sampai ke validasi Fase 3.
func setMinimalEnv(t *testing.T) {
	t.Helper()
	t.Setenv("MASTER_KEY_HEX", "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff")
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
}

func TestLoadEventTrackerDefaults(t *testing.T) {
	setMinimalEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.EventTrackerEnabled {
		t.Fatal("EVENT_TRACKER_ENABLED harus default false: rilis pertama memakai jalur Fase 2")
	}
	if len(cfg.Warnings) != 0 {
		t.Fatalf("tanpa env lama tidak boleh ada peringatan: %v", cfg.Warnings)
	}

	checks := []struct {
		name string
		got  any
		want any
	}{
		{"CorrelationWindow", cfg.CorrelationWindow, 20000 * time.Millisecond},
		{"AttachRadiusKm", cfg.AttachRadiusKm, 50.0},
		{"IndependenceCellKm", cfg.IndependenceCellKm, 5.0},
		{"MinIndependentCells", cfg.MinIndependentCells, 2},
		{"MaxEventDiameterKm", cfg.MaxEventDiameterKm, 120.0},
		{"EventResolveAfter", cfg.EventResolveAfter, 90000 * time.Millisecond},
		{"EventSweepInterval", cfg.EventSweepInterval, 5000 * time.Millisecond},
		{"EventTrackerMaxOpen", cfg.EventTrackerMaxOpen, 256},
		{"TerminalRetention", cfg.TerminalRetention, 900000 * time.Millisecond},
		{"EventTrackerMaxTombstones", cfg.EventTrackerMaxTombstones, 512},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, mau %v", c.name, c.got, c.want)
		}
	}
}

// CORRELATION_WINDOW_MS menang atas nama lama, dan kemenangan itu TIDAK
// memunculkan peringatan: .env yang sudah diperbarui harus senyap.
func TestLoadCorrelationWindowPrefersNewName(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("CORRELATION_WINDOW_MS", "30000")
	t.Setenv("CONSENSUS_WINDOW_MS", "8000")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.CorrelationWindow != 30*time.Second {
		t.Fatalf("CorrelationWindow = %s, mau 30s", cfg.CorrelationWindow)
	}
	if len(cfg.Warnings) != 0 {
		t.Fatalf("tidak boleh ada peringatan saat nama baru dipakai: %v", cfg.Warnings)
	}
	// Nama lama tetap mengisi ConsensusWindow: jalur Fase 2 masih dapat dieksekusi.
	if cfg.ConsensusWindow != 8*time.Second {
		t.Fatalf("ConsensusWindow = %s, mau 8s", cfg.ConsensusWindow)
	}
}

// .env basi: nama lama DIWARISI, bukan diabaikan, dan kepindahan itu terlihat.
func TestLoadCorrelationWindowFallsBackWithWarning(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("CONSENSUS_WINDOW_MS", "8000")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.CorrelationWindow != 8*time.Second {
		t.Fatalf("CorrelationWindow = %s, mau 8s (warisan nama lama)", cfg.CorrelationWindow)
	}
	if len(cfg.Warnings) != 1 {
		t.Fatalf("fallback harus memberi tepat satu peringatan, dapat %v", cfg.Warnings)
	}
}

func TestLoadEventTrackerRejectsBadValues(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
	}{
		{"jendela nol", map[string]string{"CORRELATION_WINDOW_MS": "0"}},
		{"radius nol", map[string]string{"ATTACH_RADIUS_KM": "0"}},
		{"sel independensi negatif", map[string]string{"INDEPENDENCE_CELL_KM": "-1"}},
		{"sel minimum nol", map[string]string{"MIN_INDEPENDENT_CELLS": "0"}},
		{"diameter nol", map[string]string{"MAX_EVENT_DIAMETER_KM": "0"}},
		{"resolve nol", map[string]string{"EVENT_RESOLVE_AFTER_MS": "0"}},
		{"sweep nol", map[string]string{"EVENT_SWEEP_INTERVAL_MS": "0"}},
		{"sweep lebih lambat dari resolve", map[string]string{
			"EVENT_SWEEP_INTERVAL_MS": "90001", "EVENT_RESOLVE_AFTER_MS": "90000"}},
		{"max open nol", map[string]string{"EVENT_TRACKER_MAX_OPEN": "0"}},
		{"max tombstone nol", map[string]string{"EVENT_TRACKER_MAX_TOMBSTONES": "0"}},
		{"retensi lebih pendek dari usia trigger (D28)", map[string]string{
			"TERMINAL_RETENTION_MS": "299999"}},
		// I-COV: bound rentang bujur pencarian kandidat hanya berlaku sampai
		// seperempat lingkar bumi. Radius di atasnya membuat indeks melewatkan
		// kandidat tanpa satu pun galat, jadi ia ditolak SAAT BOOT.
		{"radius di luar keberlakuan bound pencarian", map[string]string{
			"ATTACH_RADIUS_KM": "10008", "MAX_EVENT_DIAMETER_KM": "40000"}},
		{"radius setengah globe", map[string]string{
			"ATTACH_RADIUS_KM": "20000", "MAX_EVENT_DIAMETER_KM": "40000"}},
		// Radius menempel yang melebihi diameter maksimum saling membatalkan:
		// setiap penempelan lewat radius akan ditolak penjaga diameter.
		{"radius melebihi diameter", map[string]string{
			"ATTACH_RADIUS_KM": "200", "MAX_EVENT_DIAMETER_KM": "120"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setMinimalEnv(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			if _, err := Load(); err == nil {
				t.Fatal("Load harus menolak konfigurasi ini")
			}
		})
	}
}

// Batas bawah D28 harus DITERIMA persis pada nilainya, bukan hanya di atasnya.
func TestLoadTerminalRetentionAcceptsExactTriggerAge(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("TERMINAL_RETENTION_MS", "300000")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.TerminalRetention != 5*time.Minute {
		t.Fatalf("TerminalRetention = %s, mau 5m", cfg.TerminalRetention)
	}
}

// Batas atas radius harus DITERIMA persis pada nilainya, bukan hanya di bawahnya:
// batasnya adalah titik keberlakuan rumus, dan tepat di titik itu rumusnya masih
// berlaku.
func TestLoadAttachRadiusAcceptsExactUpperBound(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("ATTACH_RADIUS_KM", "10007.543398010286")
	t.Setenv("MAX_EVENT_DIAMETER_KM", "40000")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AttachRadiusKm != maxAttachRadiusKm {
		t.Fatalf("AttachRadiusKm = %g, mau %g", cfg.AttachRadiusKm, maxAttachRadiusKm)
	}
}

// Setelan baku harus lulus validasi radius: sebuah invarian yang menolak
// defaultnya sendiri adalah invarian yang salah.
func TestLoadDefaultAttachRadiusIsAccepted(t *testing.T) {
	setMinimalEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AttachRadiusKm != 50 {
		t.Fatalf("AttachRadiusKm baku = %g, mau 50", cfg.AttachRadiusKm)
	}
	if cfg.AttachRadiusKm > cfg.MaxEventDiameterKm {
		t.Fatalf("radius baku %g melebihi diameter baku %g", cfg.AttachRadiusKm, cfg.MaxEventDiameterKm)
	}
}
