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

// --- Tracker observability ---

// TrackerStatsSource adalah antarmuka sempit yang dibutuhkan handler stats:
// kembalikan potret counter tanpa mengimpor paket event secara langsung.
// Implementasi: *event.Tracker.
type TrackerStatsSource interface {
	Stats() TrackerStatsJSON
	NearConfirmedReport() NearConfirmedReportJSON
}

// TrackerStatsJSON dan NearConfirmedEntryJSON adalah tipe mirror yang
// menjembatani paket api dengan paket event tanpa impor langsung.
// Mereka identik secara struktural dengan event.TrackerStats dan
// event.NearConfirmedEntry; main.go mengisinya lewat adapter tipis.
type TrackerStatsJSON struct {
	Created            int64 `json:"event_created_total"`
	ForcedResolutions  int64 `json:"event_forced_resolutions_total"`
	ReonsetSplits      int64 `json:"event_reonset_splits_total"`
	DiameterRejections int64 `json:"event_diameter_rejections_total"`
	StaleAbsorbed      int64 `json:"event_stale_evidence_absorbed_total"`
	TombstoneEvictions int64 `json:"event_tombstone_evictions_total"`
	Reconciled         int64 `json:"event_reconciled_total"`
	PersistDropped     int64 `json:"event_persist_dropped_total"`
	UpsertFailures     int64 `json:"event_upsert_failures_total"`
	StateLogFailures   int64 `json:"event_state_log_failures_total"`
	StateLogSkipped    int64 `json:"event_state_log_skipped_total"`

	// Akuntansi catatan near-confirmation durable (P4-M2′). Dilaporkan, tidak
	// pernah diklaim nol (D-011 batasan 1).
	NearConfirmedDropped        int64 `json:"event_near_confirmed_persist_dropped_total"`
	NearConfirmedUpsertFailures int64 `json:"event_near_confirmed_upsert_failures_total"`

	TransitionToUnconfirmed int64 `json:"event_transitions_to_unconfirmed_total"`
	TransitionToConfirmed   int64 `json:"event_transitions_to_confirmed_total"`
	TransitionToResolved    int64 `json:"event_transitions_to_resolved_total"`
	TransitionToCancelled   int64 `json:"event_transitions_to_cancelled_total"`
	OpenGauge               int   `json:"event_open_gauge"`
	TombstoneGauge          int   `json:"event_tombstone_gauge"`

	// Latensi tahap server (P4-M3′). Onset->decided dipisah menurut provenance
	// onset: seri publish-bound adalah BATAS ATAS, bukan pengukuran.
	OnsetToDecidedSensor  LatencyStatsJSON `json:"event_latency_onset_to_decided_sensor_ms"`
	OnsetToDecidedPublish LatencyStatsJSON `json:"event_latency_onset_to_decided_publish_bound_ms"`
	DecidedToEmit         LatencyStatsJSON `json:"event_latency_decided_to_emit_ms"`
}

// LatencyStatsJSON adalah ringkasan satu seri latensi. observed adalah jumlah
// sampel kumulatif; p50/p95 hanya berbicara tentang jendela sampel terakhir yang
// disimpan Tracker, jadi keduanya dibawa bersama supaya sebuah persentil tidak
// dapat dibaca seolah mewakili seluruh riwayat.
type LatencyStatsJSON struct {
	Observed int64 `json:"observed"`
	P50Ms    int64 `json:"p50_ms"`
	P95Ms    int64 `json:"p95_ms"`
}

// NearConfirmedEntryJSON adalah potret satu event yang pernah mencapai
// >= 2 kontributor independen.
//
// MinIndependentCells dan AlgoVer adalah parameter yang BERLAKU saat persilangan
// itu, dibawa apa adanya: sebuah hitungan independensi hanya dapat ditafsirkan
// bersama ambang dan jarak pemisahan yang menghasilkannya. Source menyatakan
// apakah proses ini menyaksikan persilangannya atau membacanya kembali dari
// tabel durable saat boot (P4-M2′).
type NearConfirmedEntryJSON struct {
	EventID                string `json:"event_id"`
	FirstTwoIndependentAt  int64  `json:"first_two_independent_at_ms"`
	IndependentCountAtPeak int    `json:"independent_count_at_peak"`
	NodeCountAtPeak        int    `json:"node_count_at_peak"`
	ConfirmedAt            int64  `json:"confirmed_at_ms,omitempty"`
	TerminalState          string `json:"terminal_state,omitempty"`
	TerminalAt             int64  `json:"terminal_at_ms,omitempty"`
	MinIndependentCells    int    `json:"min_independent_cells"`
	AlgoVer                string `json:"algo_ver"`
	Source                 string `json:"source"`
	UpdatedInProcess       bool   `json:"updated_in_process,omitempty"`
}

// NearConfirmedCoverageJSON adalah selubung cakupan jawaban near-confirmed (B1,
// P4-M2′). Ia ada karena `entries: []` punya dua arti yang sangat berbeda —
// "tidak ada satu pun persilangan yang pernah terjadi" dan "tidak ada yang dapat
// dijawab" — dan tanpa selubung ini keduanya terkirim sebagai byte yang identik.
// Pada fleet satu-node arti pertama adalah jawaban yang BENAR, jadi keduanya
// justru harus dapat dibedakan.
//
// Tidak ada field bernama complete, healthy, atau valid, dan ketiadaan itu
// disengaja: ini pengukuran cakupan, bukan penilaian.
type NearConfirmedCoverageJSON struct {
	ProcessStartedAtMs int64 `json:"process_started_at_ms"`
	AsOfMs             int64 `json:"as_of_ms"`

	DurableReadAttempted bool   `json:"durable_read_attempted"`
	DurableReadOK        bool   `json:"durable_read_ok"`
	DurableReadAtMs      int64  `json:"durable_read_at_ms,omitempty"`
	DurableRowsLoaded    int    `json:"durable_rows_loaded"`
	DurableReadError     string `json:"durable_read_error,omitempty"`

	EntriesRecordedInProcess int `json:"entries_recorded_in_process"`
	EntriesLoadedFromDurable int `json:"entries_loaded_from_durable"`

	AlgoVer             string `json:"algo_ver"`
	MinIndependentCells int    `json:"min_independent_cells"`
}

// NearConfirmedReportJSON adalah badan respons endpoint near-confirmed.
//
// `entries` TETAP array tingkat atas dengan nama yang sama: selubungnya ADITIF,
// bukan pembungkus, sehingga pembaca yang sudah ada (skrip simulasi) membacanya
// seperti sebelumnya.
type NearConfirmedReportJSON struct {
	Entries  []NearConfirmedEntryJSON  `json:"entries"`
	Coverage NearConfirmedCoverageJSON `json:"coverage"`
}

// SetTrackerStats memasang sumber statistik Tracker. Opsional: tanpa ini,
// kedua endpoint mengembalikan 503 (Tracker belum aktif — EVENT_TRACKER_ENABLED
// mungkin mati, kompatibilitas Fase 2 penuh terjaga).
func (s *Server) SetTrackerStats(src TrackerStatsSource) {
	s.trackerStats = src
}

// HandleTrackerStats melayani GET /api/v1/admin/tracker/stats.
// Mengembalikan potret seluruh counter §15.5 tanpa harus grep log.
func (s *Server) HandleTrackerStats(w http.ResponseWriter, r *http.Request) {
	if s.trackerStats == nil {
		writeError(w, http.StatusServiceUnavailable, "TRACKER_DISABLED",
			"event tracker tidak aktif (EVENT_TRACKER_ENABLED=false)")
		return
	}
	writeJSON(w, http.StatusOK, s.trackerStats.Stats())
}

// HandleTrackerNearConfirmed melayani GET /api/v1/admin/tracker/near-confirmed.
// Mengembalikan semua event yang pernah mencapai >= 2 kontributor independen,
// dengan outcome-nya: apakah CONFIRMED, kapan terminal, berapa lama macet.
//
// Sejak P4-M2′ jawabannya membawa selubung cakupan, dan itu bukan hiasan: pada
// fleet satu-node daftar yang BENAR adalah kosong (S2 — kuorum tidak terjangkau),
// jadi tanpa selubung itu jawaban yang benar tidak dapat dibedakan dari tidak
// adanya jawaban sama sekali.
func (s *Server) HandleTrackerNearConfirmed(w http.ResponseWriter, r *http.Request) {
	if s.trackerStats == nil {
		writeError(w, http.StatusServiceUnavailable, "TRACKER_DISABLED",
			"event tracker tidak aktif (EVENT_TRACKER_ENABLED=false)")
		return
	}
	rep := s.trackerStats.NearConfirmedReport()
	if rep.Entries == nil {
		// `entries` harus selalu berupa array, bukan null: pembaca yang menghitung
		// panjangnya tidak boleh melihat dua bentuk berbeda untuk "kosong".
		rep.Entries = []NearConfirmedEntryJSON{}
	}
	writeJSON(w, http.StatusOK, rep)
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
