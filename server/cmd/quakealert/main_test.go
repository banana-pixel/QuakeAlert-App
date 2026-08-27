package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/banana-pixel/quakealert/server/internal/config"
	"github.com/banana-pixel/quakealert/server/internal/event"
)

// Skenario: klien native (OkHttp pada aplikasi Android) tidak mengirim header
// Origin. Sebelum perbaikan ini gorilla memanggil CheckOrigin pada setiap
// upgrade dan konfigurasi default (WS_ALLOWED_ORIGINS kosong) menolaknya dengan
// 403 — jalur alert realtime tidak pernah tersambung dari perangkat.
func TestWSOriginChecker(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	cases := []struct {
		name    string
		allowed []string
		origin  string // "" = header tidak dikirim
		want    bool
	}{
		{"tanpa Origin & allowlist kosong (klien native)", nil, "", true},
		{"tanpa Origin & allowlist terisi", []string{"https://app.example"}, "", true},
		{"origin browser dalam allowlist", []string{"https://app.example"}, "https://app.example", true},
		{"origin browser di luar allowlist", []string{"https://app.example"}, "https://evil.example", false},
		{"origin browser & allowlist kosong", nil, "https://evil.example", false},
		{"wildcard mengizinkan origin apa pun", []string{"*"}, "https://any.example", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/ws", nil)
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			if got := wsOriginChecker(tc.allowed, log)(r); got != tc.want {
				t.Fatalf("checkOrigin = %v, ingin %v", got, tc.want)
			}
		})
	}
}

// eventOptions adalah satu-satunya tempat parameter korelasi berpindah dari
// config ke Tracker, dan salah-petakan di sini tidak dapat gagal build: setiap
// field bertipe angka atau durasi, jadi menukar dua di antaranya menghasilkan
// server yang berjalan dengan ambang yang salah tanpa satu pun keluhan. Uji ini
// memakai nilai yang SEMUANYA berbeda supaya penukaran apa pun terlihat.
func TestEventOptionsMapsEveryConfiguredThreshold(t *testing.T) {
	cfg := &config.Config{
		CorrelationWindow:         21 * time.Second,
		AttachRadiusKm:            51,
		IndependenceCellKm:        6,
		MinIndependentCells:       3,
		MaxEventDiameterKm:        121,
		EventResolveAfter:         91 * time.Second,
		EventSweepInterval:        6 * time.Second,
		EventTrackerMaxOpen:       257,
		TerminalRetention:         901 * time.Second,
		EventTrackerMaxTombstones: 513,
	}

	got := eventOptions(cfg)
	want := event.Options{
		CorrelationWindowMs: 21000,
		AttachRadiusKm:      51,
		IndependenceCellKm:  6,
		MinIndependentCells: 3,
		MaxEventDiameterKm:  121,
		ResolveAfterMs:      91000,
		SweepIntervalMs:     6000,
		MaxOpen:             257,
		TerminalRetentionMs: 901000,
		MaxTombstones:       513,
	}
	if got != want {
		t.Fatalf("eventOptions = %+v, mau %+v", got, want)
	}
}
