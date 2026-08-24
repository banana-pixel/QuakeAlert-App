package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// Healthz adalah probe kesehatan publik (di luar /api/v1, tanpa auth).
//
// Kontraknya berlapis, supaya setiap pemakai mendapat jawaban sekelasnya:
//   - Load balancer / k8s membaca STATUS CODE saja: 200 selama dependensi
//     kritikal hidup, 503 begitu tidak. Tidak perlu parser.
//   - Klien Android membaca BODY: {"status","database","mqtt"} membedakan
//     "server mati" (503 → Offline di badge) dari "server hidup tetapi satu
//     jalur di belakangnya sedang tidak sehat" (200 + field bukan "ok" →
//     Limited). Basis data kritikal: tanpa itu tidak ada riwayat, chat,
//     maupun sesi. MQTT diturunkan menjadi 200 + "down", bukan 503 — broker
//     yang mati menghentikan ingest bacaan baru, tetapi alert yang sudah
//     terdispatch tetap sampai, jadi keadaannya 'terbatas', bukan 'mati'.
//   - Dependensi yang belum diprobe dilaporkan apa adanya ("unknown") dan
//     tidak ikut menentukan status; field yang tidak diketahui tidak boleh
//     menggiring verdict.
type healthzResponse struct {
	Status   string `json:"status"`
	Database string `json:"database"`
	MQTT     string `json:"mqtt,omitempty"`
}

// healthzTimeout membatasi tiap pengecekan dependensi. Probe yang menggantung
// lebih buruk daripada probe yang gagal: ia menahan slot reverse proxy dan
// memberi jawaban terlambat yang sudah tidak benar lagi.
const healthzTimeout = 500 * time.Millisecond

func (s *Server) HandleHealthz(w http.ResponseWriter, r *http.Request) {
	resp := healthzResponse{Status: "ok", Database: "unknown", MQTT: "unknown"}

	if s.repo != nil {
		ctx, cancel := context.WithTimeout(r.Context(), healthzTimeout)
		defer cancel()
		resp.Database = "ok"
		if err := s.repo.Ping(ctx); err != nil {
			s.log.Warn("healthz: database down", "err", err)
			resp.Database = "down"
			resp.Status = "degraded"
		}
	}

	if s.mqttHealth != nil {
		resp.MQTT = "ok"
		if !s.mqttHealth() {
			resp.MQTT = "down"
			resp.Status = "degraded"
		}
	}

	code := http.StatusOK
	if resp.Status == "degraded" && resp.Database == "down" {
		// Hanya basis data yang menjatuhkan status code: dialah satu-satunya
		// dependensi yang membuat seluruh API berhenti bekerja.
		code = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(resp)
}
