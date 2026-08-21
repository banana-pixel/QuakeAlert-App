package api

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/banana-pixel/quakealert/server/internal/store"
)

// rerollWindow adalah cooldown reroll pseudonym (kontrak: 1x/60s per user).
const rerollWindow = 60 * time.Second

// authWindow adalah batas pembuatan profil anonim per-IP (anti spam:
// POST /auth/anonymous menciptakan baris user_profiles baru tanpa identitas).
const authWindow = 30 * time.Second

// provisionSecretBytes adalah panjang entropy provisioning_secret (32 byte).
const provisionSecretBytes = 32

// maxRequestBodyBytes membatasi ukuran body JSON pada seluruh endpoint yang
// mem-decode request. Mencegah memory-exhaustion dari body tak terbatas.
const maxRequestBodyBytes = 1 << 20 // 1 MB

// defaultTokenTTL adalah masa hidup token anonim bila tidak dikonfigurasi.
// Panjang (30 hari) karena identitas anonim tidak punya alur refresh: klien
// memanggil /auth/anonymous sekali lalu menyimpan token secara lokal.
const defaultTokenTTL = 30 * 24 * time.Hour

// Batas paginasi & radius GET /api/v1/events (cermin kontrak OpenAPI).
const (
	defaultEventsLimit = store.DefaultEventsLimit
	maxEventsLimit     = store.MaxEventsLimit
	maxEventsRangeKm   = 2000
	// maxEventsOffset membatasi kedalaman paginasi agar offset liar tidak
	// memicu scan O(offset) berulang pada indeks started_at DESC.
	maxEventsOffset = 50_000
)

// maxFCMTokenLen mengikuti user_profiles.fcm_token VARCHAR(255): ditolak di
// handler agar klien mendapat 400 yang jelas, bukan 500 dari Postgres.
const maxFCMTokenLen = 255

// maxLocationNameLen mengikuti VARCHAR(150) pada iot_nodes.location_name dan
// user_profiles.location_name (migrasi 000002).
const maxLocationNameLen = 150

// Rentang coverage_radius_km yang diterima PUT /users/location. Batas atas
// mencerminkan slider Settings Android (AppSettingsRepository.RADIUS_RANGE =
// 50..300); batas bawah dibuat longgar agar versi klien yang menawarkan pilihan
// lebih sempit tidak ditolak. Nilai ini menentukan apakah dispatch membangunkan
// perangkat untuk sebuah gempa, jadi di luar rentang lebih baik 400 yang jelas
// daripada radius liar yang diam-diam tersimpan.
const (
	minCoverageRadiusKm = 1
	maxCoverageRadiusKm = 300
)

// stationIDPattern mencerminkan ProvisionRequest.station_id pada
// contracts/openapi/openapi.yaml (^NODE-[0-9A-F]{8}$).
var stationIDPattern = regexp.MustCompile(`^NODE-[0-9A-F]{8}$`)

// Repo adalah subset method store yang dibutuhkan API. Interface memudahkan
// pengujian dengan fake store (tanpa Postgres nyata).
type Repo interface {
	CreateNode(ctx context.Context, n *store.NewNode) error
	ListSensorsWithin(ctx context.Context, lat, lon float64, rangeKm int) ([]store.SensorStatus, error)
	GetUserLocation(ctx context.Context, userID string) (*store.UserLocation, error)
	UpdatePseudonym(ctx context.Context, userID, pseudonym string) (time.Time, error)
	CreateUserProfile(ctx context.Context, userID, pseudonym string) (time.Time, error)
	UpdateUserLocation(ctx context.Context, userID string, lat, lon float64, locationName string, coverageRadiusKm *int) (time.Time, int, error)
	UpdateUserFCMToken(ctx context.Context, userID, token string) (time.Time, error)
	ListEvents(ctx context.Context, limit, offset int, filter *store.EventFilter) ([]store.Event, error)
}

// SecretEncryptor mengenkripsi provisioning secret menjadi (ciphertext, nonce)
// AES-256-GCM sebelum disimpan (implementasi: crypto.Cipher).
type SecretEncryptor interface {
	Encrypt(plaintext []byte) (ciphertext, nonce []byte, err error)
}

// MQTTPublic adalah konfigurasi broker publik yang dikembalikan saat provisioning.
type MQTTPublic struct {
	Broker string
	Port   int
	TLS    bool
}

// AuthConfig memusatkan parameter token anonim: secret HS256 yang dipakai BAIK
// untuk menerbitkan (HandleAnonymousAuth) MAUPUN memverifikasi (middleware),
// plus masa hidup token. Disimpan di Server agar tidak ada dua sumber kebenaran
// untuk secret yang sama.
type AuthConfig struct {
	JWTSecret []byte
	TokenTTL  time.Duration
}

// Server memegang dependency handler REST.
type Server struct {
	repo    Repo
	cipher  SecretEncryptor
	limiter RateLimiter
	mqtt    MQTTPublic
	auth    AuthConfig
	log     *slog.Logger
}

// NewServer membuat Server API. TokenTTL yang kosong diisi defaultTokenTTL.
func NewServer(repo Repo, cipher SecretEncryptor, limiter RateLimiter, mqtt MQTTPublic, auth AuthConfig, log *slog.Logger) *Server {
	if auth.TokenTTL <= 0 {
		auth.TokenTTL = defaultTokenTTL
	}
	return &Server{repo: repo, cipher: cipher, limiter: limiter, mqtt: mqtt, auth: auth, log: log}
}

// --- Helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, apiError{Code: code, Message: msg})
}

// clientIP mengambil alamat IP klien dari r.RemoteAddr. Dengan chi
// middleware.RealIP (dipakai di Router), nilai ini sudah berasal dari
// X-Forwarded-For saat berada di belakang reverse proxy.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// maxBody membatasi body request menjadi maxRequestBodyBytes. Panggil sebelum
// json.Decoder; body yang melebihi limit memicu error decode yang dipetakan ke
// 413 RequestEntityTooLarge.
func maxBody(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
}

// decodeBody mem-decode body JSON berukuran max 1MB ke v dan memetakan error ke
// respons 400/413 yang sesuai. Mengembalikan false bila respons sudah ditulis.
func (s *Server) decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	maxBody(w, r)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "INVALID_ARGUMENT", "body melebihi batas 1MB")
			return false
		}
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "body JSON tidak valid")
		return false
	}
	// Tolak trailing data setelah SATU nilai JSON (mis. "{}JUNK", dua objek
	// bertumpuk, atau koma-menyimpang). dec.Token() hanya mengembalikan io.EOF
	// bila memang tidak ada lagi token valid.
	if _, err := dec.Token(); err != io.EOF {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "body JSON tidak valid")
		return false
	}
	return true
}

// --- Provision handler ---

type provisionRequest struct {
	// StationID opsional: bila node sudah punya ID di NVS, ID itu dikirim agar
	// firmware & DB memakai identitas yang sama (topik MQTT sensor/<id>/...).
	StationID    string  `json:"station_id"`
	SensorModel  string  `json:"sensor_model"`
	LocationName string  `json:"location_name"`
	Latitude     float64 `json:"latitude"`
	Longitude    float64 `json:"longitude"`
}

type provisionResponse struct {
	StationID          string `json:"station_id"`
	ProvisioningSecret string `json:"provisioning_secret"`
	MQTTBroker         string `json:"mqtt_broker"`
	MQTTPort           int    `json:"mqtt_port"`
	MQTTTLS            bool   `json:"mqtt_tls"`
}

// HandleProvision membuat node baru + provisioning secret sekali-tampil.
// Secret disimpan terenkripsi AES-256-GCM (bukan hash) karena verifikasi HMAC
// butuh key mentah (ADR-0003).
func (s *Server) HandleProvision(w http.ResponseWriter, r *http.Request) {
	var req provisionRequest
	if !s.decodeBody(w, r, &req) {
		return
	}
	if req.SensorModel == "" || req.LocationName == "" {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "sensor_model & location_name wajib")
		return
	}
	if len(req.LocationName) > maxLocationNameLen {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "location_name maksimal 150 karakter")
		return
	}
	if req.Latitude < -90 || req.Latitude > 90 {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "latitude di luar rentang -90..90")
		return
	}
	if req.Longitude < -180 || req.Longitude > 180 {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "longitude di luar rentang -180..180")
		return
	}

	// station_id: pakai milik node bila valid, jika absen/kosong generate baru.
	stationID := req.StationID
	if stationID == "" {
		var gerr error
		stationID, gerr = randomStationID()
		if gerr != nil {
			s.log.Error("gagal generate station_id", "err", gerr)
			writeError(w, http.StatusInternalServerError, "INTERNAL", "gagal membuat station_id")
			return
		}
	} else if !stationIDPattern.MatchString(stationID) {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "station_id harus berpola NODE-XXXXXXXX (hex kapital)")
		return
	}

	secret, err := randomSecret()
	if err != nil {
		s.log.Error("gagal generate secret", "err", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL", "gagal membuat secret")
		return
	}

	enc, nonce, err := s.cipher.Encrypt([]byte(secret))
	if err != nil {
		s.log.Error("gagal enkripsi secret", "err", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL", "gagal menyimpan secret")
		return
	}

	if err := s.repo.CreateNode(r.Context(), &store.NewNode{
		StationID:    stationID,
		SensorModel:  req.SensorModel,
		LocationName: req.LocationName,
		Lat:          req.Latitude,
		Lon:          req.Longitude,
		SecretEnc:    enc,
		SecretNonce:  nonce,
	}); err != nil {
		// station_id sudah dipakai adalah konflik klien, bukan kegagalan server:
		// node yang pernah di-provision harus memakai secret di NVS-nya. 409
		// memberi firmware sinyal yang dapat ditindaklanjuti (jangan retry),
		// sementara 500 akan memicu retry sia-sia pada loop provisioning.
		if errors.Is(err, store.ErrNodeAlreadyExists) {
			s.log.Warn("provisioning ditolak: station_id sudah ada", "station_id", stationID)
			writeError(w, http.StatusConflict, "STATION_ALREADY_EXISTS", "station_id sudah terdaftar")
			return
		}
		s.log.Error("gagal create node", "err", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL", "gagal menyimpan node")
		return
	}

	writeJSON(w, http.StatusCreated, provisionResponse{
		StationID:          stationID,
		ProvisioningSecret: secret,
		MQTTBroker:         s.mqtt.Broker,
		MQTTPort:           s.mqtt.Port,
		MQTTTLS:            s.mqtt.TLS,
	})
}

// --- Sensors handler ---

type stationDTO struct {
	StationID    string  `json:"station_id"`
	SensorModel  string  `json:"sensor_model"`
	LocationName string  `json:"location_name"`
	Latitude     float64 `json:"latitude"`
	Longitude    float64 `json:"longitude"`
	Status       string  `json:"status"`
	LastPing     string  `json:"last_ping"`
	RSSIdBm      int     `json:"rssi_dbm"`
	LatencyMs    int     `json:"latency_ms"`
}

type sensorsResponse struct {
	RangeKm            int          `json:"range_km"`
	ActiveSensorsCount int          `json:"active_sensors_count"`
	Stations           []stationDTO `json:"stations"`
}

// onlineThresholdSec: node dianggap Offline bila >90s tanpa heartbeat.
const onlineThresholdSec = 90

// HandleListSensors mengembalikan sensor dalam radius range_km dari lokasi user.
func (s *Server) HandleListSensors(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "user tidak terautentikasi")
		return
	}

	rangeKm := 50
	if v := r.URL.Query().Get("range_km"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 500 {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "range_km harus 1..500")
			return
		}
		rangeKm = n
	}

	loc, err := s.repo.GetUserLocation(r.Context(), userID)
	if errors.Is(err, store.ErrUserNotFound) {
		writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "profil user tidak ditemukan")
		return
	}
	if err != nil {
		s.log.Error("gagal ambil lokasi user", "err", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL", "gagal memuat lokasi user")
		return
	}
	if !loc.HasLocation {
		// Belum ada lokasi → kembalikan daftar kosong (bukan error).
		writeJSON(w, http.StatusOK, sensorsResponse{RangeKm: rangeKm, Stations: []stationDTO{}})
		return
	}

	sensors, err := s.repo.ListSensorsWithin(r.Context(), loc.Lat, loc.Lon, rangeKm)
	if err != nil {
		s.log.Error("gagal list sensors", "err", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL", "gagal memuat sensor")
		return
	}

	stations := make([]stationDTO, 0, len(sensors))
	active := 0
	for _, sc := range sensors {
		status := "Offline"
		if sc.IsActive && sc.SecondsSincePing <= onlineThresholdSec {
			status = "Online"
			active++
		}
		stations = append(stations, stationDTO{
			StationID:    sc.StationID,
			SensorModel:  sc.SensorModel,
			LocationName: sc.LocationName,
			Latitude:     sc.Lat,
			Longitude:    sc.Lon,
			Status:       status,
			LastPing:     humanizeAgo(sc.SecondsSincePing),
			RSSIdBm:      sc.LastRSSI,
			LatencyMs:    sc.LastLatencyMs,
		})
	}

	writeJSON(w, http.StatusOK, sensorsResponse{
		RangeKm:            rangeKm,
		ActiveSensorsCount: active,
		Stations:           stations,
	})
}

// --- Reroll pseudonym handler ---

type rerollResponse struct {
	Pseudonym string `json:"pseudonym"`
	UpdatedAt string `json:"updated_at"` // RFC3339 UTC
}

// HandleRerollPseudonym mengganti pseudonym user (rate-limited 1x/60s).
func (s *Server) HandleRerollPseudonym(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "user tidak terautentikasi")
		return
	}

	allowed, err := s.limiter.Allow(r.Context(), "reroll:"+userID, rerollWindow)
	if err != nil {
		s.log.Error("rate limiter error", "err", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL", "gagal memeriksa rate limit")
		return
	}
	if !allowed {
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "reroll hanya 1x per 60 detik")
		return
	}

	pseudonym, err := randomPseudonym()
	if err != nil {
		s.log.Error("gagal generate pseudonym", "err", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL", "gagal membuat pseudonym")
		return
	}

	updatedAt, err := s.repo.UpdatePseudonym(r.Context(), userID, pseudonym)
	if errors.Is(err, store.ErrUserNotFound) {
		writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "profil user tidak ditemukan")
		return
	}
	if err != nil {
		s.log.Error("gagal update pseudonym", "err", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL", "gagal menyimpan pseudonym")
		return
	}

	writeJSON(w, http.StatusOK, rerollResponse{
		Pseudonym: pseudonym,
		UpdatedAt: updatedAt.UTC().Format(time.RFC3339),
	})
}

// --- Anonymous auth handler ---

type anonymousAuthResponse struct {
	Token     string `json:"token"`
	TokenType string `json:"token_type"`
	ExpiresAt string `json:"expires_at"` // RFC3339 UTC
	UserID    string `json:"user_id"`
	Pseudonym string `json:"pseudonym"`
	CreatedAt string `json:"created_at"` // RFC3339 UTC
}

// HandleAnonymousAuth membuat profil anonim baru (user_id UUID v4 + pseudonym)
// lalu menerbitkan JWT HS256 dengan klaim sub/iat/exp. Ini SATU-SATUNYA endpoint
// tanpa Bearer token — klien memanggilnya sekali saat first launch.
//
// user_id di-generate di sisi server (bukan mengandalkan DEFAULT uuid_generate_v4
// milik kolom) agar nilainya sudah tersedia sebelum INSERT dan dapat langsung
// dipakai sebagai klaim `sub` tanpa round-trip kedua.
func (s *Server) HandleAnonymousAuth(w http.ResponseWriter, r *http.Request) {
	// Anti-spam: batasi pembuatan profil anonim per-IP (endpoint publik tanpa
	// identitas, jadi IP adalah satu-satunya kunci yang tersedia). Setelah
	// authWindow, IP dipersilakan membuat profil baru lagi.
	allowed, err := s.limiter.Allow(r.Context(), "auth:"+clientIP(r), authWindow)
	if err != nil {
		s.log.Error("rate limiter error (auth)", "err", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL", "gagal memeriksa rate limit")
		return
	}
	if !allowed {
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "terlalu banyak pendaftaran, coba lagi nanti")
		return
	}

	userID, err := randomUserID()
	if err != nil {
		s.log.Error("gagal generate user_id", "err", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL", "gagal membuat user_id")
		return
	}
	pseudonym, err := randomPseudonym()
	if err != nil {
		s.log.Error("gagal generate pseudonym", "err", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL", "gagal membuat pseudonym")
		return
	}

	// Terbitkan token SEBELUM insert profil: bila mint gagal (salah-konfigurasi
	// secret/TTL), tidak ada profil yatim yang tertinggal; retry klien tidak
	// menumpuk user_profiles tanpa token.
	token, err := MintHS256(userID, s.auth.JWTSecret, s.auth.TokenTTL)
	if err != nil {
		s.log.Error("gagal menerbitkan token", "err", err, "user_id", userID)
		writeError(w, http.StatusInternalServerError, "INTERNAL", "gagal menerbitkan token")
		return
	}

	createdAt, err := s.repo.CreateUserProfile(r.Context(), userID, pseudonym)
	if err != nil {
		s.log.Error("gagal membuat profil anonim", "err", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL", "gagal membuat profil")
		return
	}

	writeJSON(w, http.StatusCreated, anonymousAuthResponse{
		Token:     token,
		TokenType: "Bearer",
		ExpiresAt: time.Now().Add(s.auth.TokenTTL).UTC().Format(time.RFC3339),
		UserID:    userID,
		Pseudonym: pseudonym,
		CreatedAt: createdAt.UTC().Format(time.RFC3339),
	})
}

// --- Update location handler ---

// updateLocationRequest memakai pointer untuk latitude/longitude agar field yang
// ABSEN dapat dibedakan dari nilai 0 — (0, 0) adalah koordinat yang sah (Null
// Island), jadi zero-value JSON tidak boleh diperlakukan sebagai "tidak dikirim".
type updateLocationRequest struct {
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
	// CoverageRadiusKm juga pointer, tetapi karena alasan yang berbeda dari
	// koordinat: ABSEN berarti "jangan ubah radius" (klien lama tidak mengenal
	// field ini), sedangkan 0 adalah nilai tak sah yang ditolak.
	CoverageRadiusKm *int   `json:"coverage_radius_km"`
	LocationName     string `json:"location_name"`
}

type updateLocationResponse struct {
	UserID       string  `json:"user_id"`
	Latitude     float64 `json:"latitude"`
	Longitude    float64 `json:"longitude"`
	LocationName *string `json:"location_name"`
	// CoverageRadiusKm adalah radius yang BERLAKU setelah update (nilai yang
	// baru dikirim, atau yang sudah tersimpan bila field diabaikan) sehingga
	// klien dapat memastikan preferensinya benar-benar sampai.
	CoverageRadiusKm int    `json:"coverage_radius_km"`
	UpdatedAt        string `json:"updated_at"` // RFC3339 UTC
}

// HandleUpdateLocation menyimpan koordinat user ke last_location (PostGIS) yang
// menjadi basis filter radius /sensors dan penargetan alert, sekaligus
// menyinkronkan coverage_radius_km pilihan user bila dikirim — kolom itulah yang
// dipakai dispatch untuk memutuskan perangkat mana yang perlu dibangunkan
// (store.FCMTokensWithin), jadi tanpanya penargetan hanya bisa memakai satu
// radius seragam untuk semua orang.
func (s *Server) HandleUpdateLocation(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "user tidak terautentikasi")
		return
	}

	var req updateLocationRequest
	if !s.decodeBody(w, r, &req) {
		return
	}
	if req.Latitude == nil || req.Longitude == nil {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "latitude & longitude wajib")
		return
	}
	if *req.Latitude < -90 || *req.Latitude > 90 {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "latitude di luar rentang -90..90")
		return
	}
	if *req.Longitude < -180 || *req.Longitude > 180 {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "longitude di luar rentang -180..180")
		return
	}
	if len(req.LocationName) > maxLocationNameLen {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "location_name maksimal 150 karakter")
		return
	}
	if req.CoverageRadiusKm != nil &&
		(*req.CoverageRadiusKm < minCoverageRadiusKm || *req.CoverageRadiusKm > maxCoverageRadiusKm) {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT",
			fmt.Sprintf("coverage_radius_km harus %d..%d", minCoverageRadiusKm, maxCoverageRadiusKm))
		return
	}

	updatedAt, radiusKm, err := s.repo.UpdateUserLocation(
		r.Context(), userID, *req.Latitude, *req.Longitude, req.LocationName, req.CoverageRadiusKm)
	if errors.Is(err, store.ErrUserNotFound) {
		writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "profil user tidak ditemukan")
		return
	}
	if err != nil {
		s.log.Error("gagal update lokasi user", "err", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL", "gagal menyimpan lokasi")
		return
	}

	resp := updateLocationResponse{
		UserID:           userID,
		Latitude:         *req.Latitude,
		Longitude:        *req.Longitude,
		CoverageRadiusKm: radiusKm,
		UpdatedAt:        updatedAt.UTC().Format(time.RFC3339),
	}
	// Kosong dipersistensikan sebagai NULL (semantik PUT), jadi respons harus
	// mencerminkan null—bukan "" — agar klien melihat state yang benar-benar tersimpan.
	if req.LocationName != "" {
		name := req.LocationName
		resp.LocationName = &name
	}
	writeJSON(w, http.StatusOK, resp)
}

// --- Update FCM token handler ---

type updateFCMTokenRequest struct {
	FCMToken string `json:"fcm_token"`
}

type updateFCMTokenResponse struct {
	UpdatedAt string `json:"updated_at"` // RFC3339 UTC
}

// HandleUpdateFCMToken menyimpan registration token FCM perangkat agar alert
// tetap sampai saat aplikasi di background (life-safety: kanal kedua di samping WS).
func (s *Server) HandleUpdateFCMToken(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "user tidak terautentikasi")
		return
	}

	var req updateFCMTokenRequest
	if !s.decodeBody(w, r, &req) {
		return
	}
	if req.FCMToken == "" {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "fcm_token wajib")
		return
	}
	if len(req.FCMToken) > maxFCMTokenLen {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "fcm_token maksimal 255 karakter")
		return
	}

	updatedAt, err := s.repo.UpdateUserFCMToken(r.Context(), userID, req.FCMToken)
	if errors.Is(err, store.ErrUserNotFound) {
		writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "profil user tidak ditemukan")
		return
	}
	if err != nil {
		s.log.Error("gagal update fcm token", "err", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL", "gagal menyimpan fcm_token")
		return
	}

	writeJSON(w, http.StatusOK, updateFCMTokenResponse{
		UpdatedAt: updatedAt.UTC().Format(time.RFC3339),
	})
}

// --- List events handler ---

type eventDTO struct {
	EventID        string   `json:"event_id"`
	Status         string   `json:"status"`
	PGA            float64  `json:"pga"` // gal (satuan kanonik)
	MMI            string   `json:"mmi"`
	IntensityLabel string   `json:"intensity_label"`
	Latitude       float64  `json:"latitude"`  // centroid, BUKAN episenter
	Longitude      float64  `json:"longitude"` // centroid, BUKAN episenter
	DepthKm        *float64 `json:"depth_km"`  // selalu null (lihat kontrak OpenAPI)
	LocationName   string   `json:"location_name"`
	TriggeredNodes int      `json:"triggered_nodes_count"`
	CreatedAt      string   `json:"created_at"`            // RFC3339 UTC, dari started_at
	ResolvedAt     *string  `json:"resolved_at,omitempty"` // absen selama HAPPENING
}

type eventsResponse struct {
	Limit   int        `json:"limit"`
	Offset  int        `json:"offset"`
	Count   int        `json:"count"`
	RangeKm *int       `json:"range_km"` // null bila filter spasial tidak aktif
	Events  []eventDTO `json:"events"`
}

// HandleListEvents mengembalikan riwayat event terkonfirmasi (created_at DESC).
// Auth opsional (OptionalAuthMiddleware): tanpa token endpoint tetap melayani,
// dan bila token valid dikirim tanpa koordinat eksplisit, lokasi tersimpan user
// dipakai sebagai acuan filter radius.
func (s *Server) HandleListEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	limit := defaultEventsLimit
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > maxEventsLimit {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "limit harus 1..100")
			return
		}
		limit = n
	}

	offset := 0
	if v := q.Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > maxEventsOffset {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "offset harus 0..50000")
			return
		}
		offset = n
	}

	filter, err := s.eventFilterFrom(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
		return
	}

	events, err := s.repo.ListEvents(r.Context(), limit, offset, filter)
	if err != nil {
		s.log.Error("gagal list events", "err", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL", "gagal memuat event")
		return
	}

	out := make([]eventDTO, 0, len(events))
	for _, e := range events {
		dto := eventDTO{
			EventID:        e.EventID,
			Status:         e.Status,
			PGA:            e.MaxPGA,
			MMI:            e.MMIScale,
			IntensityLabel: e.IntensityLabel,
			Latitude:       e.Lat,
			Longitude:      e.Lon,
			LocationName:   e.LocationName,
			TriggeredNodes: e.TriggeredNodes,
			CreatedAt:      e.StartedAt.UTC().Format(time.RFC3339),
		}
		if e.ResolvedAt != nil {
			resolved := e.ResolvedAt.UTC().Format(time.RFC3339)
			dto.ResolvedAt = &resolved
		}
		out = append(out, dto)
	}

	resp := eventsResponse{Limit: limit, Offset: offset, Count: len(out), Events: out}
	if filter != nil {
		rangeKm := filter.RangeKm
		resp.RangeKm = &rangeKm
	}
	writeJSON(w, http.StatusOK, resp)
}

// eventFilterFrom membangun filter spasial dari query string. Mengembalikan
// (nil, nil) bila range_km tidak dikirim (tanpa filter).
//
// Acuan koordinat: latitude+longitude eksplisit bila ada; jika tidak, lokasi
// tersimpan user yang terautentikasi. Tanpa keduanya, range_km tidak dapat
// diartikan dan request ditolak 400 alih-alih diam-diam mengabaikan filter —
// mengembalikan event seluruh negeri kepada klien yang meminta radius 50 km
// adalah kegagalan senyap yang berbahaya.
func (s *Server) eventFilterFrom(r *http.Request) (*store.EventFilter, error) {
	q := r.URL.Query()
	rawRange := q.Get("range_km")
	latStr, lonStr := q.Get("latitude"), q.Get("longitude")

	if rawRange == "" {
		if latStr != "" || lonStr != "" {
			return nil, errors.New("latitude/longitude hanya berlaku bersama range_km")
		}
		return nil, nil
	}

	rangeKm, err := strconv.Atoi(rawRange)
	if err != nil || rangeKm < 1 || rangeKm > maxEventsRangeKm {
		return nil, errors.New("range_km harus 1..2000")
	}

	if (latStr == "") != (lonStr == "") {
		return nil, errors.New("latitude & longitude harus dikirim bersamaan")
	}
	if latStr != "" {
		lat, lerr := strconv.ParseFloat(latStr, 64)
		if lerr != nil || lat < -90 || lat > 90 {
			return nil, errors.New("latitude di luar rentang -90..90")
		}
		lon, lerr := strconv.ParseFloat(lonStr, 64)
		if lerr != nil || lon < -180 || lon > 180 {
			return nil, errors.New("longitude di luar rentang -180..180")
		}
		return &store.EventFilter{Lat: lat, Lon: lon, RangeKm: rangeKm}, nil
	}

	// Fallback: lokasi tersimpan user (hanya tersedia bila request terautentikasi).
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		return nil, errors.New("range_km butuh latitude & longitude (atau token dengan lokasi tersimpan)")
	}
	loc, err := s.repo.GetUserLocation(r.Context(), userID)
	if err != nil || loc == nil || !loc.HasLocation {
		return nil, errors.New("range_km butuh latitude & longitude: lokasi user belum tersimpan")
	}
	return &store.EventFilter{Lat: loc.Lat, Lon: loc.Lon, RangeKm: rangeKm}, nil
}

// --- Generators ---

const hexAlphabet = "0123456789ABCDEF"

// randomUserID menghasilkan UUID v4 (RFC 4122) dari crypto/rand tanpa dependency
// eksternal. Formatnya cocok dengan kolom user_profiles.user_id bertipe UUID.
func randomUserID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40 // versi 4
	b[8] = (b[8] & 0x3f) | 0x80 // varian RFC 4122
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

func randomStationID() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("NODE-%02X%02X%02X%02X", b[0], b[1], b[2], b[3]), nil
}

func randomSecret() (string, error) {
	b := make([]byte, provisionSecretBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	out := make([]byte, len(b)*2)
	for i, v := range b {
		out[i*2] = hexAlphabet[v>>4]
		out[i*2+1] = hexAlphabet[v&0x0f]
	}
	return "sec_" + string(out), nil
}

// randomPseudonym menghasilkan pseudonim 8-hex (4 byte = 32 bit entropy).
// Ruang 2^32 menekan tabrakan nama antar-profil anonim jauh di bawah ambang
// (birthday collision praktis tidak terjadi hingga ~65k profil).
func randomPseudonym() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("Quakezen-%02X%02X%02X%02X", b[0], b[1], b[2], b[3]), nil
}

// humanizeAgo mengubah detik menjadi label relatif ("33s ago", "5m ago").
func humanizeAgo(secs int64) string {
	switch {
	case secs < 0:
		return "just now"
	case secs < 60:
		return strconv.FormatInt(secs, 10) + "s ago"
	case secs < 3600:
		return strconv.FormatInt(secs/60, 10) + "m ago"
	default:
		return strconv.FormatInt(secs/3600, 10) + "h ago"
	}
}
