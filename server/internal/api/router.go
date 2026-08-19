package api

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Router membangun http.Handler chi untuk REST API v1. wsHandler adalah handler
// WebSocket (mis. Hub.ServeWS) yang juga diproteksi AuthMiddleware. Secret JWT
// diambil dari Server.auth agar penerbitan (HandleAnonymousAuth) dan verifikasi
// (middleware) tidak bisa memakai kunci yang berbeda.
//
// Tiga tingkat akses:
//   - publik        : /healthz, POST /api/v1/auth/anonymous
//   - auth opsional : GET /api/v1/events (token boleh absen; bila ada wajib valid)
//   - auth wajib    : sisa /api/v1/* dan /ws
//
// Path didaftarkan penuh (tanpa chi Route/Mount) karena ketiga grup berbagi
// prefix /api/v1 dengan middleware berbeda — Mount pada prefix yang sama akan
// bertabrakan.
func (s *Server) Router(wsHandler http.HandlerFunc, log *slog.Logger) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)

	// Health check publik (untuk load balancer / k8s probe).
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// Bootstrap identitas: klien belum punya token saat memanggil ini.
	r.Post("/api/v1/auth/anonymous", s.HandleAnonymousAuth)

	// Riwayat gempa bersifat publik; token opsional hanya memperkaya (lokasi
	// tersimpan user dipakai sebagai acuan filter radius).
	r.Group(func(r chi.Router) {
		r.Use(OptionalAuthMiddleware(s.auth.JWTSecret, log))
		r.Get("/api/v1/events", s.HandleListEvents)
	})

	r.Group(func(r chi.Router) {
		r.Use(AuthMiddleware(s.auth.JWTSecret, log))
		r.Post("/api/v1/nodes/provision", s.HandleProvision)
		r.Get("/api/v1/sensors", s.HandleListSensors)
		r.Post("/api/v1/users/pseudonym/reroll", s.HandleRerollPseudonym)
		r.Put("/api/v1/users/location", s.HandleUpdateLocation)
		r.Put("/api/v1/users/fcm-token", s.HandleUpdateFCMToken)
		// WebSocket realtime (WSS via reverse proxy TLS di produksi).
		r.Get("/ws", wsHandler)
	})

	return r
}
