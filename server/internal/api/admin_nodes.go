package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

// --- Verifikasi node operator (migrasi 000005) ---

// Node provisioning terbuka bagi setiap pemegang JWT anonim (dengan batas laju),
// jadi kepercayaan tidak boleh didapat di endpoint yang sama: node baru lahir
// verified = false, tampak di /sensors sebagai pending, dan trigger-nya ditolak
// di internal/ingest sampai operator mengonfirmasi lewat rute di file ini.
// Dua tingkat (pending/verified) sengaja: peran per-node menambah kompleksitas
// tanpa perilaku yang memakainya selama operatornya satu orang.

type pendingNodeDTO struct {
	StationID    string  `json:"station_id"`
	SensorModel  string  `json:"sensor_model"`
	LocationName string  `json:"location_name"`
	Latitude     float64 `json:"latitude"`
	Longitude    float64 `json:"longitude"`
	CreatedAt    string  `json:"created_at"` // RFC3339 UTC
}

type listPendingNodesResponse struct {
	Nodes []pendingNodeDTO `json:"nodes"`
}

// HandleListPendingNodes mendaftar node yang menunggu konfirmasi operator,
// terbaru dulu. Kosong berarti tidak ada yang menunggu — bukan keadaan galat.
func (s *Server) HandleListPendingNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := s.repo.ListUnverifiedNodes(r.Context())
	if err != nil {
		s.log.Error("gagal list node pending", "err", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL", "gagal memuat node pending")
		return
	}

	out := make([]pendingNodeDTO, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, pendingNodeDTO{
			StationID:    n.StationID,
			SensorModel:  n.SensorModel,
			LocationName: n.LocationName,
			Latitude:     n.Lat,
			Longitude:    n.Lon,
			CreatedAt:    n.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, listPendingNodesResponse{Nodes: out})
}

type verifyNodeRequest struct {
	// Verified opsional: absen berarti setuju (kasus umum skrip verifikasi).
	// Kirim false untuk menarik kembali kepercayaan pada node yang sudah
	// dikonfirmasi — trigger-nya kembali ditolak tanpa menyentuh is_active.
	Verified *bool `json:"verified"`
}

type verifyNodeResponse struct {
	StationID string `json:"station_id"`
	Verified  bool   `json:"verified"`
}

// HandleVerifyNode mengubah status verifikasi satu node.
//
// station_id divalidasi terhadap pola yang sama dengan provisioning sebelum
// disentuh: ID yang salah bentuk adalah kesalahan panggilan (404), bukan sesuatu
// yang perlu sampai ke basis data.
func (s *Server) HandleVerifyNode(w http.ResponseWriter, r *http.Request) {
	stationID := chi.URLParam(r, "stationID")
	if !stationIDPattern.MatchString(stationID) {
		writeError(w, http.StatusNotFound, "NODE_NOT_FOUND",
			"station_id harus berpola NODE-XXXXXXXX (hex kapital)")
		return
	}

	var req verifyNodeRequest
	if r.ContentLength > 0 && !s.decodeBody(w, r, &req) {
		return
	}
	verified := true
	if req.Verified != nil {
		verified = *req.Verified
	}

	updated, err := s.repo.SetNodeVerified(r.Context(), stationID, verified)
	if err != nil {
		s.log.Error("gagal set node verified", "station_id", stationID, "err", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL", "gagal menyimpan verifikasi")
		return
	}
	if !updated {
		// Kontrak store: false = station_id tidak dikenal.
		writeError(w, http.StatusNotFound, "NODE_NOT_FOUND", "station_id tidak ditemukan")
		return
	}

	s.log.Info("verifikasi node diubah", "station_id", stationID, "verified", verified)
	writeJSON(w, http.StatusOK, verifyNodeResponse{StationID: stationID, Verified: verified})
}
