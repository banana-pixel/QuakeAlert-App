package api

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Router membangun http.Handler chi untuk REST API v1. wsHandler adalah handler
// WebSocket (mis. Hub.ServeWS) yang juga diproteksi AuthMiddleware.
//
// Semua rute /api/v1/* dan /ws memerlukan Bearer JWT (HS256). /healthz publik.
func (s *Server) Router(jwtSecret []byte, wsHandler http.HandlerFunc, log *slog.Logger) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)

	// Health check publik (untuk load balancer / k8s probe).
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	auth := AuthMiddleware(jwtSecret, log)

	r.Group(func(r chi.Router) {
		r.Use(auth)
		r.Route("/api/v1", func(r chi.Router) {
			r.Post("/nodes/provision", s.HandleProvision)
			r.Get("/sensors", s.HandleListSensors)
			r.Post("/users/pseudonym/reroll", s.HandleRerollPseudonym)
		})
		// WebSocket realtime (WSS via reverse proxy TLS di produksi).
		r.Get("/ws", wsHandler)
	})

	return r
}
