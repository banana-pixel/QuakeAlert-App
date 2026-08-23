package api

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/banana-pixel/quakealert/server/internal/store"
)

// AdminKeyHeader adalah header yang membawa ADMIN_API_KEY. Header terpisah, bukan
// Authorization: Bearer, agar tidak ada jalur di mana sebuah JWT pengguna biasa
// bisa keliru diterima sebagai kunci operator (dan sebaliknya).
const AdminKeyHeader = "X-Admin-Key"

// Batas siaran, cermin kolom broadcasts (migrasi 000004).
const (
	maxBroadcastTitleLen = store.MaxBroadcastTitleLen
	maxBroadcastBodyLen  = store.MaxBroadcastBodyLen
)

// AdminKeyMiddleware memproteksi endpoint operator dengan satu shared secret.
//
// Perbandingannya subtle.ConstantTimeCompare, bukan ==: pembandingan string di
// Go keluar pada byte pertama yang berbeda, dan endpoint yang dapat diukur
// waktunya memberi penyerang cara menebak kunci satu byte sekaligus alih-alih
// seluruhnya. Panjang disamakan lebih dulu karena ConstantTimeCompare sendiri
// mengembalikan 0 untuk panjang berbeda tanpa membaca isinya.
//
// Penolakan dicatat bersama IP: satu-satunya tanda bahwa seseorang sedang
// mencoba menebak kunci ini adalah deretan 401 di log.
func AdminKeyMiddleware(key []byte, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sent := []byte(strings.TrimSpace(r.Header.Get(AdminKeyHeader)))
			if len(sent) != len(key) || subtle.ConstantTimeCompare(sent, key) != 1 {
				log.Warn("kunci admin ditolak", "ip", clientIP(r), "path", r.URL.Path)
				writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED",
					"header "+AdminKeyHeader+" tidak valid")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// AdminBroadcast adalah siaran yang SUDAH tersimpan dan siap difanout.
//
// Didefinisikan di paket api dengan alasan yang sama seperti ChatEvent: api dan
// dispatch tidak saling impor, dan cmd/quakealert yang menjembatani keduanya.
type AdminBroadcast struct {
	ID         string
	Title      string
	Body       string
	RegionCode string
	CreatedAt  time.Time
}

// BroadcastFanout menyiarkan pengumuman yang sudah tersimpan (WebSocket + FCM).
//
// Opsional, seperti ChatFanout: tanpa itu siaran tetap tersimpan dan tetap
// terbaca di daftar Pembaruan, hanya tidak sampai sebagai push.
type BroadcastFanout interface {
	BroadcastAdmin(AdminBroadcast)
}

// SetBroadcastFanout memasang jalur siaran. Setter, bukan parameter NewServer,
// karena hub WebSocket dibangun setelah Server di cmd/quakealert.
func (s *Server) SetBroadcastFanout(f BroadcastFanout) {
	s.broadcasts = f
}

// SetAdminAPIKey memasang kunci operator. Kunci kosong berarti Router TIDAK
// mendaftarkan rute admin sama sekali — instalasi yang lupa mengisinya
// kehilangan fiturnya, bukan pagarnya.
func (s *Server) SetAdminAPIKey(key string) {
	s.adminKey = []byte(key)
}

// --- POST /api/v1/admin/broadcasts ---

type createBroadcastRequest struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	// RegionCode opsional: kosong/absen = nasional. Nilainya adalah kunci yang
	// sama dengan kanal chat regional, mis. "ID-jawa-barat".
	RegionCode string `json:"region_code"`
}

type broadcastDTO struct {
	BroadcastID string  `json:"broadcast_id"`
	Title       string  `json:"title"`
	Body        string  `json:"body"`
	RegionCode  *string `json:"region_code"`
	CreatedAt   string  `json:"created_at"`
}

type listBroadcastsResponse struct {
	Broadcasts []broadcastDTO `json:"broadcasts"`
}

// HandleCreateBroadcast menyimpan pengumuman operator lalu menyiarkannya.
//
// Urutannya penting dan tidak boleh dibalik: simpan dulu, fanout kemudian.
// Siaran yang terkirim tetapi tidak tersimpan tidak dapat dibaca lagi begitu
// notifikasinya disapu dari shade, dan sebuah pengumuman yang tidak bisa dibuka
// ulang praktis tidak pernah dikirim. Kegagalan fanout, sebaliknya, hanya
// menunda: barisnya sudah ada dan akan muncul saat aplikasi membuka daftar
// Pembaruan.
func (s *Server) HandleCreateBroadcast(w http.ResponseWriter, r *http.Request) {
	var req createBroadcastRequest
	if !s.decodeBody(w, r, &req) {
		return
	}

	title, err := broadcastText(req.Title, maxBroadcastTitleLen, "title")
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
		return
	}
	body, err := broadcastText(req.Body, maxBroadcastBodyLen, "body")
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
		return
	}

	// Wilayah dinormalisasi lewat fungsi yang SAMA dengan sinkronisasi lokasi.
	// Operator yang mengetik "Jawa Barat" dan ponsel yang mengirim "Jawa Barat"
	// harus menghasilkan satu kunci; kalau tidak, siaran akan menyasar ruang
	// yang tidak ditinggali siapa pun.
	region := strings.TrimSpace(req.RegionCode)
	if region != "" {
		region = normalizeRegionCode(region)
		if region == "" {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT",
				"region_code tidak valid; pakai bentuk <ISO2>-<slug-admin1>, mis. ID-jawa-barat")
			return
		}
	}

	ctx := r.Context()
	saved, err := s.repo.InsertBroadcast(ctx, title, body, region)
	if err != nil {
		s.log.Error("gagal simpan siaran", "err", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL", "gagal menyimpan siaran")
		return
	}

	s.log.Info("siaran admin dibuat", "broadcast_id", saved.ID, "region", saved.RegionCode)

	if s.broadcasts != nil {
		s.broadcasts.BroadcastAdmin(AdminBroadcast{
			ID:         saved.ID,
			Title:      saved.Title,
			Body:       saved.Body,
			RegionCode: saved.RegionCode,
			CreatedAt:  saved.CreatedAt,
		})
	}

	writeJSON(w, http.StatusCreated, toBroadcastDTO(*saved))
}

// --- GET /api/v1/broadcasts ---

// HandleListBroadcasts menjawab siaran yang berlaku bagi pemanggil: yang
// nasional ditambah yang menyasar wilayahnya sendiri.
//
// Wilayah diambil dari region_code TERSIMPAN, bukan dari query string: kalau
// tidak, siapa pun dapat membaca pengumuman wilayah lain hanya dengan mengubah
// URL — dan yang lebih sering terjadi, tidak akan pernah melihat pengumuman
// wilayahnya sendiri begitu klien salah menyusun kuncinya.
func (s *Server) HandleListBroadcasts(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "user tidak terautentikasi")
		return
	}

	limit := store.DefaultBroadcastLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > store.MaxBroadcastLimit {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT",
				"limit harus bilangan 1.."+strconv.Itoa(store.MaxBroadcastLimit))
			return
		}
		limit = n
	}

	rows, err := s.repo.ListBroadcastsForUser(r.Context(), userID, limit)
	if err != nil {
		s.log.Error("gagal membaca siaran", "err", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL", "gagal membaca siaran")
		return
	}

	out := make([]broadcastDTO, 0, len(rows))
	for _, b := range rows {
		out = append(out, toBroadcastDTO(b))
	}
	writeJSON(w, http.StatusOK, listBroadcastsResponse{Broadcasts: out})
}

func toBroadcastDTO(b store.Broadcast) broadcastDTO {
	dto := broadcastDTO{
		BroadcastID: b.ID,
		Title:       b.Title,
		Body:        b.Body,
		CreatedAt:   b.CreatedAt.UTC().Format(time.RFC3339),
	}
	if b.RegionCode != "" {
		region := b.RegionCode
		dto.RegionCode = &region
	}
	return dto
}

// broadcastText menormalkan dan memvalidasi satu bidang teks siaran.
//
// Meratakan spasi dengan strings.Fields seperti nodeLocationName: judul yang
// dikirim dari skrip shell sering membawa newline atau spasi ganda dari
// here-doc, dan sebuah pengumuman yang tampak rusak merusak kepercayaan pada
// pengumuman berikutnya. Panjang diukur setelah normalisasi agar batasnya
// mengenai teks yang benar-benar tersimpan.
func broadcastText(raw string, maxLen int, field string) (string, error) {
	text := strings.Join(strings.Fields(raw), " ")
	if text == "" {
		return "", errors.New(field + " wajib")
	}
	if len(text) > maxLen {
		return "", fmt.Errorf("%s maksimal %d karakter", field, maxLen)
	}
	return text, nil
}
