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
//   - kunci admin   : POST /api/v1/admin/* (X-Admin-Key, BUKAN JWT)
//
// Path didaftarkan penuh (tanpa chi Route/Mount) karena ketiga grup berbagi
// prefix /api/v1 dengan middleware berbeda — Mount pada prefix yang sama akan
// bertabrakan.
func (s *Server) Router(wsHandler http.HandlerFunc, log *slog.Logger) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)

	// Health check publik (untuk load balancer / k8s probe). Status code adalah
	// kontrak pertama (200/503); body JSON membedakan "mati" dari "terbatas"
	// bagi klien yang membacanya — lihat HandleHealthz.
	r.Get("/healthz", s.HandleHealthz)

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
		// Pembatalan registrasi: capability = provisioning secret mentah
		// (ADR-0003), hanya node verified=FALSE, idempoten. Lihat HandleRevokeNode.
		r.Post("/api/v1/nodes/revoke", s.HandleRevokeNode)
		r.Get("/api/v1/sensors", s.HandleListSensors)
		r.Post("/api/v1/users/pseudonym/reroll", s.HandleRerollPseudonym)
		r.Put("/api/v1/users/location", s.HandleUpdateLocation)
		r.Put("/api/v1/users/fcm-token", s.HandleUpdateFCMToken)
		// Chat: kanal mana yang boleh diakses dijawab server, bukan ditebak
		// klien. Kirim tetap lewat REST agar durable dan bisa diulang; socket
		// hanya memfanout apa yang sudah tersimpan.
		r.Get("/api/v1/chat/channels", s.HandleListChatChannels)
		r.Get("/api/v1/chat/messages", s.HandleListChatMessages)
		r.Post("/api/v1/chat/messages", s.HandleCreateChatMessage)
		// Pengumuman operator: dibaca oleh pengguna biasa, ditulis hanya oleh
		// pemegang kunci admin di grup terpisah di bawah.
		r.Get("/api/v1/broadcasts", s.HandleListBroadcasts)
		// WebSocket realtime (WSS via reverse proxy TLS di produksi).
		r.Get("/ws", wsHandler)
	})

	// Rute operator. Didaftarkan HANYA bila ADMIN_API_KEY terisi: instalasi yang
	// lupa mengisinya kehilangan fiturnya, bukan pagarnya — sebuah endpoint yang
	// dapat menyalakan notifikasi di setiap perangkat tidak boleh punya keadaan
	// "terbuka karena belum dikonfigurasi".
	if len(s.adminKey) > 0 {
		r.Group(func(r chi.Router) {
			r.Use(AdminKeyMiddleware(s.adminKey, log))
			r.Post("/api/v1/admin/broadcasts", s.HandleCreateBroadcast)
			// Drill: satu kunci yang sama, tetapi jalur fanout yang sepenuhnya
			// lain (topic test_alerts saja, tanpa konsensus dan tanpa baris
			// earthquake_events). Lihat testalert.go.
			r.Post("/api/v1/admin/test-alert", s.HandleCreateTestAlert)
			// Verifikasi node: sisi lain gerbang konsensus (migrasi 000005).
			// Daftar pending dulu, konfirmasi satu per satu; body {"verified":
			// false} menarik kembali kepercayaan pada node yang sudah sah.
			r.Get("/api/v1/admin/nodes/pending", s.HandleListPendingNodes)
			r.Post("/api/v1/admin/nodes/{stationID}/verify", s.HandleVerifyNode)
		})
	} else {
		log.Warn("ADMIN_API_KEY tidak di-set — endpoint siaran admin tidak didaftarkan")
	}

	return r
}
