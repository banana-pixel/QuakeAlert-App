package api

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/banana-pixel/quakealert/server/internal/store"
)

// rerollWindow adalah cooldown reroll pseudonym (kontrak: 1x/60s per user).
const rerollWindow = 60 * time.Second

// provisionSecretBytes adalah panjang entropy provisioning_secret (32 byte).
const provisionSecretBytes = 32

// Repo adalah subset method store yang dibutuhkan API. Interface memudahkan
// pengujian dengan fake store (tanpa Postgres nyata).
type Repo interface {
	CreateNode(ctx context.Context, n *store.NewNode) error
	ListSensorsWithin(ctx context.Context, lat, lon float64, rangeKm int) ([]store.SensorStatus, error)
	GetUserLocation(ctx context.Context, userID string) (*store.UserLocation, error)
	UpdatePseudonym(ctx context.Context, userID, pseudonym string) (time.Time, error)
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

// Server memegang dependency handler REST.
type Server struct {
	repo    Repo
	cipher  SecretEncryptor
	limiter RateLimiter
	mqtt    MQTTPublic
	log     *slog.Logger
}

// NewServer membuat Server API.
func NewServer(repo Repo, cipher SecretEncryptor, limiter RateLimiter, mqtt MQTTPublic, log *slog.Logger) *Server {
	return &Server{repo: repo, cipher: cipher, limiter: limiter, mqtt: mqtt, log: log}
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

// --- Provision handler ---

type provisionRequest struct {
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
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "body JSON tidak valid")
		return
	}
	if req.SensorModel == "" || req.LocationName == "" {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "sensor_model & location_name wajib")
		return
	}
	if len(req.LocationName) > 150 {
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

	stationID, err := randomStationID()
	if err != nil {
		s.log.Error("gagal generate station_id", "err", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL", "gagal membuat station_id")
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

// --- Generators ---

const hexAlphabet = "0123456789ABCDEF"

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

func randomPseudonym() (string, error) {
	b := make([]byte, 2)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("Quakezen-%02X%02X", b[0], b[1]), nil
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
