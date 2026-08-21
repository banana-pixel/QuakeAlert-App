package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
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
