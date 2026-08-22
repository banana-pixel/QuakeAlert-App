package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/banana-pixel/quakealert/server/internal/store"
)

// --- Fakes ---

type fakeRepo struct {
	created   *store.NewNode
	createErr error
	loc       *store.UserLocation
	locErr    error
	sensors   []store.SensorStatus
	pseudonym string
	updatedAt time.Time
	updateErr error

	// Profil anonim
	profileID        string
	profilePseudonym string
	profileCreatedAt time.Time
	profileErr       error

	// Lokasi user
	locUserID  string
	locLat     float64
	locLon     float64
	locName    string
	locSetErr  error
	locUpdated time.Time

	// FCM
	fcmToken  string
	fcmErr    error
	fcmUpdate time.Time

	// Events
	events      []store.Event
	eventsErr   error
	eventsLimit int
	eventsOff   int
	eventsFilt  *store.EventFilter

	// Chat
	regionCode    string
	regionSets    int
	regionSetErr  error
	ensuredChan   string
	ensuredKind   string
	ensuredName   string
	ensureCalls   int
	chatIdentity  *store.UserChatIdentity
	chatIdentErr  error
	chatChannels  []store.ChatChannel
	chatChanErr   error
	chatMessages  []store.ChatMessage
	chatMsgErr    error
	chatMsgLimit  int
	chatMsgBefore *time.Time
	chatMsgChan   string
	inserted      *store.ChatMessage
	insertErr     error
	insertedBody  string
	insertedChan  string
	insertedClID  string
}

func (f *fakeRepo) CreateNode(_ context.Context, n *store.NewNode) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.created = n
	return nil
}
func (f *fakeRepo) ListSensorsWithin(_ context.Context, _, _ float64, _ int) ([]store.SensorStatus, error) {
	return f.sensors, nil
}
func (f *fakeRepo) GetUserLocation(_ context.Context, _ string) (*store.UserLocation, error) {
	return f.loc, f.locErr
}
func (f *fakeRepo) UpdatePseudonym(_ context.Context, _, p string) (time.Time, error) {
	f.pseudonym = p
	return f.updatedAt, f.updateErr
}
func (f *fakeRepo) CreateUserProfile(_ context.Context, userID, pseudonym string) (time.Time, error) {
	f.profileID, f.profilePseudonym = userID, pseudonym
	return f.profileCreatedAt, f.profileErr
}
func (f *fakeRepo) UpdateUserLocation(_ context.Context, userID string, lat, lon float64, name string) (time.Time, error) {
	f.locUserID, f.locLat, f.locLon, f.locName = userID, lat, lon, name
	return f.locUpdated, f.locSetErr
}
func (f *fakeRepo) UpdateUserFCMToken(_ context.Context, _, token string) (time.Time, error) {
	f.fcmToken = token
	return f.fcmUpdate, f.fcmErr
}
func (f *fakeRepo) ListEvents(_ context.Context, limit, offset int, filter *store.EventFilter) ([]store.Event, error) {
	f.eventsLimit, f.eventsOff, f.eventsFilt = limit, offset, filter
	return f.events, f.eventsErr
}

func (f *fakeRepo) SetUserRegion(_ context.Context, _, regionCode string) error {
	if f.regionSetErr != nil {
		return f.regionSetErr
	}
	f.regionCode = regionCode
	f.regionSets++
	return nil
}
func (f *fakeRepo) GetUserChatIdentity(_ context.Context, _ string) (*store.UserChatIdentity, error) {
	if f.chatIdentErr != nil {
		return nil, f.chatIdentErr
	}
	if f.chatIdentity == nil {
		return &store.UserChatIdentity{Pseudonym: "AnonimTenang"}, nil
	}
	return f.chatIdentity, nil
}
func (f *fakeRepo) ListChatChannels(_ context.Context, _ string) ([]store.ChatChannel, error) {
	return f.chatChannels, f.chatChanErr
}
func (f *fakeRepo) EnsureChatChannel(
	_ context.Context, channelID, kind, displayName string,
) (string, error) {
	f.ensureCalls++
	f.ensuredChan, f.ensuredKind, f.ensuredName = channelID, kind, displayName
	return displayName, nil
}
func (f *fakeRepo) ListChatMessages(
	_ context.Context, channelID string, limit int, before *time.Time,
) ([]store.ChatMessage, error) {
	f.chatMsgChan, f.chatMsgLimit, f.chatMsgBefore = channelID, limit, before
	return f.chatMessages, f.chatMsgErr
}
func (f *fakeRepo) InsertChatMessage(
	_ context.Context, channelID, senderID, pseudonym, locationTag, body, clientMessageID string,
) (*store.ChatMessage, error) {
	if f.insertErr != nil {
		return nil, f.insertErr
	}
	f.insertedChan, f.insertedBody, f.insertedClID = channelID, body, clientMessageID
	if f.inserted != nil {
		return f.inserted, nil
	}
	return &store.ChatMessage{
		MessageID:       "msg-1",
		ChannelID:       channelID,
		SenderID:        senderID,
		SenderPseudonym: pseudonym,
		LocationTag:     locationTag,
		Body:            body,
		CreatedAt:       time.Unix(1_781_913_558, 0).UTC(),
	}, nil
}

type fakeCipher struct{}

func (fakeCipher) Encrypt(pt []byte) (ct, nonce []byte, err error) {
	return append([]byte("enc:"), pt...), []byte("nonce--12byte"), nil
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// authedRequest membuat request dengan Bearer JWT valid untuk userID.
func authedRequest(method, target, body, secret, userID string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	tok := mintHS256(jwtClaims{Sub: userID, Exp: time.Now().Add(time.Hour).Unix()}, []byte(secret))
	req.Header.Set("Authorization", "Bearer "+tok)
	return req
}

// expiredRequest membuat request dengan Bearer JWT yang sudah kedaluwarsa.
func expiredRequest(method, target, body, secret, userID string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	tok := mintHS256(jwtClaims{Sub: userID, Exp: time.Now().Add(-time.Minute).Unix()}, []byte(secret))
	req.Header.Set("Authorization", "Bearer "+tok)
	return req
}

const testSecret = "this-is-a-32-byte-minimum-secret!"

// testTokenTTL cukup panjang agar expires_at pada respons auth jelas di masa depan.
const testTokenTTL = 24 * time.Hour

func newTestServer(repo Repo, limiter RateLimiter) http.Handler {
	srv := NewServer(repo, fakeCipher{}, limiter,
		MQTTPublic{Broker: "b", Port: 8883, TLS: true},
		AuthConfig{JWTSecret: []byte(testSecret), TokenTTL: testTokenTTL},
		testLogger())
	return srv.Router(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusSwitchingProtocols)
	}, testLogger())
}

// do menjalankan request terhadap handler dan mengembalikan recorder.
func do(h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// --- Tests ---

func TestProvision_Created(t *testing.T) {
	repo := &fakeRepo{}
	h := newTestServer(repo, NewMemoryRateLimiter())

	body := `{"sensor_model":"MPU 6050","location_name":"Cimahi","latitude":-6.87,"longitude":107.54}`
	req := authedRequest(http.MethodPost, "/api/v1/nodes/provision", body, testSecret, "user-1")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, mau 201. body=%s", rec.Code, rec.Body.String())
	}
	var resp provisionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// station_id absen di request → server generate sesuai pola kontrak.
	if !stationIDPattern.MatchString(resp.StationID) {
		t.Fatalf("station_id = %q, mau pola NODE-XXXXXXXX", resp.StationID)
	}
	if resp.ProvisioningSecret == "" || !resp.MQTTTLS || resp.MQTTPort != 8883 {
		t.Fatalf("field mqtt/secret salah: %+v", resp)
	}
	if repo.created == nil || repo.created.Lat != -6.87 {
		t.Fatalf("node tidak tersimpan benar: %+v", repo.created)
	}
	if repo.created.StationID != resp.StationID {
		t.Fatalf("station_id tersimpan %q != respons %q", repo.created.StationID, resp.StationID)
	}
}

// Gap 1: node yang sudah punya station_id di NVS mengirimkannya saat provisioning
// agar ID di firmware dan DB identik (topik MQTT sensor/<station_id>/...).
func TestProvision_StationIDDariNode(t *testing.T) {
	repo := &fakeRepo{}
	h := newTestServer(repo, NewMemoryRateLimiter())

	const want = "NODE-163A149F"
	body := `{"station_id":"` + want + `","sensor_model":"MPU 6050","location_name":"Cimahi","latitude":-6.87,"longitude":107.54}`
	req := authedRequest(http.MethodPost, "/api/v1/nodes/provision", body, testSecret, "user-1")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, mau 201. body=%s", rec.Code, rec.Body.String())
	}
	var resp provisionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.StationID != want {
		t.Fatalf("station_id respons = %q, mau %q", resp.StationID, want)
	}
	if repo.created == nil || repo.created.StationID != want {
		t.Fatalf("station_id tersimpan salah: %+v", repo.created)
	}
}

// station_id string kosong diperlakukan sama dengan absen → fallback generate.
func TestProvision_StationIDKosongFallback(t *testing.T) {
	repo := &fakeRepo{}
	h := newTestServer(repo, NewMemoryRateLimiter())

	body := `{"station_id":"","sensor_model":"MPU 6050","location_name":"Cimahi","latitude":-6.87,"longitude":107.54}`
	req := authedRequest(http.MethodPost, "/api/v1/nodes/provision", body, testSecret, "user-1")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, mau 201. body=%s", rec.Code, rec.Body.String())
	}
	var resp provisionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !stationIDPattern.MatchString(resp.StationID) {
		t.Fatalf("station_id = %q, mau hasil generate berpola NODE-XXXXXXXX", resp.StationID)
	}
}

func TestProvision_StationIDInvalid(t *testing.T) {
	cases := map[string]string{
		"tanpa prefix":     "163A149F",
		"hex kecil":        "NODE-163a149f",
		"terlalu pendek":   "NODE-163A149",
		"terlalu panjang":  "NODE-163A149FF",
		"karakter non-hex": "NODE-163A149G",
		"spasi di tengah":  "NODE-163A 49F",
	}
	for name, id := range cases {
		t.Run(name, func(t *testing.T) {
			repo := &fakeRepo{}
			h := newTestServer(repo, NewMemoryRateLimiter())
			body := `{"station_id":"` + id + `","sensor_model":"m","location_name":"x","latitude":-6.8,"longitude":107.5}`
			req := authedRequest(http.MethodPost, "/api/v1/nodes/provision", body, testSecret, "u")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, mau 400. body=%s", rec.Code, rec.Body.String())
			}
			if repo.created != nil {
				t.Fatalf("node tidak boleh dibuat untuk station_id invalid: %+v", repo.created)
			}
		})
	}
}

func TestProvision_InvalidLatitude(t *testing.T) {
	h := newTestServer(&fakeRepo{}, NewMemoryRateLimiter())
	body := `{"sensor_model":"m","location_name":"x","latitude":100,"longitude":10}`
	req := authedRequest(http.MethodPost, "/api/v1/nodes/provision", body, testSecret, "u")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, mau 400", rec.Code)
	}
}

func TestProvision_NoAuth(t *testing.T) {
	h := newTestServer(&fakeRepo{}, NewMemoryRateLimiter())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/provision", strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, mau 401", rec.Code)
	}
}

func TestListSensors_OnlineOffline(t *testing.T) {
	repo := &fakeRepo{
		loc: &store.UserLocation{HasLocation: true, Lat: -6.9, Lon: 107.6},
		sensors: []store.SensorStatus{
			{StationID: "NODE-A", IsActive: true, SecondsSincePing: 10},
			{StationID: "NODE-B", IsActive: true, SecondsSincePing: 200}, // stale → Offline
			{StationID: "NODE-C", IsActive: false, SecondsSincePing: 5},  // inactive → Offline
		},
	}
	h := newTestServer(repo, NewMemoryRateLimiter())
	req := authedRequest(http.MethodGet, "/api/v1/sensors?range_km=50", "", testSecret, "u")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200", rec.Code)
	}
	var resp sensorsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ActiveSensorsCount != 1 {
		t.Fatalf("active = %d, mau 1", resp.ActiveSensorsCount)
	}
	if len(resp.Stations) != 3 {
		t.Fatalf("stations = %d, mau 3", len(resp.Stations))
	}
	if resp.Stations[0].Status != "Online" {
		t.Fatalf("NODE-A harus Online, dapat %q", resp.Stations[0].Status)
	}
}

func TestListSensors_NoLocation(t *testing.T) {
	repo := &fakeRepo{loc: &store.UserLocation{HasLocation: false}}
	h := newTestServer(repo, NewMemoryRateLimiter())
	req := authedRequest(http.MethodGet, "/api/v1/sensors", "", testSecret, "u")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200", rec.Code)
	}
	var resp sensorsResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Stations) != 0 {
		t.Fatalf("stations harus kosong, dapat %d", len(resp.Stations))
	}
}

func TestReroll_RateLimited(t *testing.T) {
	repo := &fakeRepo{updatedAt: time.Unix(1_700_000_000, 0)}
	h := newTestServer(repo, NewMemoryRateLimiter())

	call := func() int {
		req := authedRequest(http.MethodPost, "/api/v1/users/pseudonym/reroll", "", testSecret, "user-x")
		return do(h, req).Code
	}

	if code := call(); code != http.StatusOK {
		t.Fatalf("reroll pertama = %d, mau 200", code)
	}
	if code := call(); code != http.StatusTooManyRequests {
		t.Fatalf("reroll kedua = %d, mau 429", code)
	}
}

// --- MEDIUM-2: konflik station_id → 409, bukan 500 ---

func TestProvision_Conflict(t *testing.T) {
	repo := &fakeRepo{createErr: store.ErrNodeAlreadyExists}
	h := newTestServer(repo, NewMemoryRateLimiter())

	body := `{"station_id":"NODE-163A149F","sensor_model":"MPU 6050","location_name":"Cimahi","latitude":-6.87,"longitude":107.54}`
	rec := do(h, authedRequest(http.MethodPost, "/api/v1/nodes/provision", body, testSecret, "u"))

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, mau 409. body=%s", rec.Code, rec.Body.String())
	}
	var e apiError
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if e.Code != "STATION_ALREADY_EXISTS" {
		t.Fatalf("code = %q, mau STATION_ALREADY_EXISTS", e.Code)
	}
}

// Error store lain tetap 500 (409 tidak boleh menelan kegagalan sesungguhnya).
func TestProvision_ErrorLainTetap500(t *testing.T) {
	repo := &fakeRepo{createErr: errors.New("koneksi db putus")}
	h := newTestServer(repo, NewMemoryRateLimiter())

	body := `{"sensor_model":"MPU 6050","location_name":"Cimahi","latitude":-6.87,"longitude":107.54}`
	rec := do(h, authedRequest(http.MethodPost, "/api/v1/nodes/provision", body, testSecret, "u"))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, mau 500", rec.Code)
	}
}

// --- POST /api/v1/auth/anonymous ---

func TestAuthAnonymous_Created(t *testing.T) {
	created := time.Unix(1_700_000_000, 0).UTC()
	repo := &fakeRepo{profileCreatedAt: created}
	h := newTestServer(repo, NewMemoryRateLimiter())

	// Sengaja TANPA header Authorization: endpoint ini harus publik.
	rec := do(h, httptest.NewRequest(http.MethodPost, "/api/v1/auth/anonymous", nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, mau 201. body=%s", rec.Code, rec.Body.String())
	}

	var resp anonymousAuthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.TokenType != "Bearer" {
		t.Fatalf("token_type = %q, mau Bearer", resp.TokenType)
	}
	if resp.CreatedAt != created.Format(time.RFC3339) {
		t.Fatalf("created_at = %q, mau %q", resp.CreatedAt, created.Format(time.RFC3339))
	}
	if !strings.HasPrefix(resp.Pseudonym, "Quakezen-") {
		t.Fatalf("pseudonym = %q, mau berawalan Quakezen-", resp.Pseudonym)
	}
	if resp.UserID != repo.profileID || repo.profilePseudonym != resp.Pseudonym {
		t.Fatalf("profil tersimpan (%q,%q) != respons (%q,%q)",
			repo.profileID, repo.profilePseudonym, resp.UserID, resp.Pseudonym)
	}
	if !uuidV4Pattern.MatchString(resp.UserID) {
		t.Fatalf("user_id = %q, mau UUID v4", resp.UserID)
	}

	// Token terbitan sendiri wajib lolos verifier dengan sub = user_id.
	claims, err := verifyHS256(resp.Token, []byte(testSecret), time.Now())
	if err != nil {
		t.Fatalf("token hasil terbit tidak lolos verifikasi: %v", err)
	}
	if claims.Sub != resp.UserID {
		t.Fatalf("sub = %q, mau %q", claims.Sub, resp.UserID)
	}
	if claims.Iat == 0 || claims.Exp <= claims.Iat {
		t.Fatalf("klaim iat/exp tidak wajar: iat=%d exp=%d", claims.Iat, claims.Exp)
	}
	if got, want := claims.Exp-claims.Iat, int64(testTokenTTL/time.Second); got != want {
		t.Fatalf("exp-iat = %d detik, mau %d", got, want)
	}
}

// Token yang baru diterbitkan langsung dapat dipakai pada endpoint terproteksi.
func TestAuthAnonymous_TokenBisaDipakai(t *testing.T) {
	repo := &fakeRepo{
		profileCreatedAt: time.Unix(1_700_000_000, 0).UTC(),
		locUpdated:       time.Unix(1_700_000_100, 0).UTC(),
	}
	h := newTestServer(repo, NewMemoryRateLimiter())

	rec := do(h, httptest.NewRequest(http.MethodPost, "/api/v1/auth/anonymous", nil))
	var auth anonymousAuthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &auth); err != nil {
		t.Fatalf("decode: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/v1/users/location",
		strings.NewReader(`{"latitude":-6.9,"longitude":107.6}`))
	req.Header.Set("Authorization", "Bearer "+auth.Token)
	if code := do(h, req).Code; code != http.StatusOK {
		t.Fatalf("PUT lokasi dengan token baru = %d, mau 200", code)
	}
	if repo.locUserID != auth.UserID {
		t.Fatalf("user_id pada store = %q, mau %q", repo.locUserID, auth.UserID)
	}
}

func TestAuthAnonymous_StoreGagal(t *testing.T) {
	repo := &fakeRepo{profileErr: errors.New("insert gagal")}
	h := newTestServer(repo, NewMemoryRateLimiter())

	rec := do(h, httptest.NewRequest(http.MethodPost, "/api/v1/auth/anonymous", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, mau 500", rec.Code)
	}
}

// --- PUT /api/v1/users/location ---

func TestUpdateLocation_OK(t *testing.T) {
	updated := time.Unix(1_700_000_500, 0).UTC()
	repo := &fakeRepo{locUpdated: updated}
	h := newTestServer(repo, NewMemoryRateLimiter())

	body := `{"latitude":-6.8721,"longitude":107.5422,"location_name":"Cimahi, West Java, ID"}`
	rec := do(h, authedRequest(http.MethodPut, "/api/v1/users/location", body, testSecret, "user-1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200. body=%s", rec.Code, rec.Body.String())
	}

	var resp updateLocationResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.UserID != "user-1" || resp.Latitude != -6.8721 || resp.Longitude != 107.5422 {
		t.Fatalf("respons salah: %+v", resp)
	}
	if resp.LocationName == nil || *resp.LocationName != "Cimahi, West Java, ID" {
		t.Fatalf("location_name = %v", resp.LocationName)
	}
	if resp.UpdatedAt != updated.Format(time.RFC3339) {
		t.Fatalf("updated_at = %q, mau %q", resp.UpdatedAt, updated.Format(time.RFC3339))
	}
	// Store menerima lat/lon dalam urutan yang benar (bukan tertukar).
	if repo.locLat != -6.8721 || repo.locLon != 107.5422 {
		t.Fatalf("store menerima lat=%v lon=%v (tertukar?)", repo.locLat, repo.locLon)
	}
}

// location_name absen → disimpan kosong (NULL di DB) dan dilaporkan null.
func TestUpdateLocation_TanpaLocationName(t *testing.T) {
	repo := &fakeRepo{locUpdated: time.Unix(1_700_000_500, 0).UTC()}
	h := newTestServer(repo, NewMemoryRateLimiter())

	rec := do(h, authedRequest(http.MethodPut, "/api/v1/users/location",
		`{"latitude":-6.9,"longitude":107.6}`, testSecret, "u"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200. body=%s", rec.Code, rec.Body.String())
	}
	if repo.locName != "" {
		t.Fatalf("location_name yang dikirim ke store = %q, mau kosong", repo.locName)
	}

	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if v, ok := raw["location_name"]; !ok || v != nil {
		t.Fatalf("location_name = %#v, mau null eksplisit", v)
	}
}

// (0, 0) adalah koordinat sah: zero-value JSON tidak boleh dianggap "absen".
func TestUpdateLocation_NolNolValid(t *testing.T) {
	repo := &fakeRepo{locUpdated: time.Unix(1_700_000_500, 0).UTC()}
	h := newTestServer(repo, NewMemoryRateLimiter())

	rec := do(h, authedRequest(http.MethodPut, "/api/v1/users/location",
		`{"latitude":0,"longitude":0}`, testSecret, "u"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200. body=%s", rec.Code, rec.Body.String())
	}
	if repo.locUserID != "u" {
		t.Fatalf("store tidak dipanggil untuk (0,0)")
	}
}

// Radius peringatan bukan lagi input klien: ia tetap 200 km
// (dispatch.AlertRadiusKm) dengan override intensitas untuk gempa parah. Klien
// yang masih mengirim coverage_radius_km harus mendapat 400 yang jelas, bukan
// diam-diam diterima lalu diabaikan — kalau tidak, aplikasi lama akan terus
// menampilkan radius pilihan yang tidak berpengaruh apa pun pada siapa yang
// dibangunkan.
func TestUpdateLocation_CoverageRadiusDitolak(t *testing.T) {
	repo := &fakeRepo{locUpdated: time.Unix(1_700_000_500, 0).UTC()}
	h := newTestServer(repo, NewMemoryRateLimiter())

	rec := do(h, authedRequest(http.MethodPut, "/api/v1/users/location",
		`{"latitude":-6.9,"longitude":107.6,"coverage_radius_km":120}`, testSecret, "u"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, mau 400. body=%s", rec.Code, rec.Body.String())
	}
	if repo.locUserID != "" {
		t.Fatalf("store tidak boleh dipanggil untuk field yang sudah dihapus dari kontrak")
	}
}

// Respons tidak boleh lagi membawa coverage_radius_km: field yang tetap ada di
// JSON akan membuat klien percaya masih ada preferensi radius yang berlaku.
func TestUpdateLocation_ResponsTanpaCoverageRadius(t *testing.T) {
	repo := &fakeRepo{locUpdated: time.Unix(1_700_000_500, 0).UTC()}
	h := newTestServer(repo, NewMemoryRateLimiter())

	rec := do(h, authedRequest(http.MethodPut, "/api/v1/users/location",
		`{"latitude":-6.9,"longitude":107.6}`, testSecret, "u"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200. body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "coverage_radius_km") {
		t.Fatalf("respons masih membawa coverage_radius_km: %s", rec.Body.String())
	}
}

func TestUpdateLocation_PayloadKurang(t *testing.T) {
	cases := map[string]string{
		"body kosong":       ``,
		"objek kosong":      `{}`,
		"tanpa longitude":   `{"latitude":-6.9}`,
		"tanpa latitude":    `{"longitude":107.6}`,
		"json rusak":        `{"latitude":`,
		"field tak dikenal": `{"latitude":-6.9,"longitude":107.6,"altitude":100}`,
		"trailing garbage":  `{"latitude":-6.9,"longitude":107.6}JUNK`,
		"dua objek":         `{"latitude":-6.9,"longitude":107.6}{"a":1}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			repo := &fakeRepo{locUpdated: time.Unix(1_700_000_500, 0).UTC()}
			h := newTestServer(repo, NewMemoryRateLimiter())
			rec := do(h, authedRequest(http.MethodPut, "/api/v1/users/location", body, testSecret, "u"))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, mau 400. body=%s", rec.Code, rec.Body.String())
			}
			if repo.locUserID != "" {
				t.Fatalf("store tidak boleh dipanggil untuk payload invalid")
			}
		})
	}
}

func TestUpdateLocation_KoordinatInvalid(t *testing.T) {
	cases := map[string]string{
		"lat > 90":    `{"latitude":90.1,"longitude":107.6}`,
		"lat < -90":   `{"latitude":-90.1,"longitude":107.6}`,
		"lon > 180":   `{"latitude":-6.9,"longitude":180.1}`,
		"lon < -180":  `{"latitude":-6.9,"longitude":-180.1}`,
		"nama > 150c": `{"latitude":-6.9,"longitude":107.6,"location_name":"` + strings.Repeat("x", 151) + `"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			repo := &fakeRepo{}
			h := newTestServer(repo, NewMemoryRateLimiter())
			rec := do(h, authedRequest(http.MethodPut, "/api/v1/users/location", body, testSecret, "u"))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, mau 400. body=%s", rec.Code, rec.Body.String())
			}
			if repo.locUserID != "" {
				t.Fatalf("store tidak boleh dipanggil untuk koordinat invalid")
			}
		})
	}
}

func TestUpdateLocation_TanpaJWT(t *testing.T) {
	h := newTestServer(&fakeRepo{}, NewMemoryRateLimiter())
	req := httptest.NewRequest(http.MethodPut, "/api/v1/users/location",
		strings.NewReader(`{"latitude":-6.9,"longitude":107.6}`))
	if code := do(h, req).Code; code != http.StatusUnauthorized {
		t.Fatalf("status = %d, mau 401", code)
	}
}

func TestUpdateLocation_JWTKedaluwarsa(t *testing.T) {
	h := newTestServer(&fakeRepo{}, NewMemoryRateLimiter())
	req := expiredRequest(http.MethodPut, "/api/v1/users/location",
		`{"latitude":-6.9,"longitude":107.6}`, testSecret, "u")
	if code := do(h, req).Code; code != http.StatusUnauthorized {
		t.Fatalf("status = %d, mau 401", code)
	}
}

// Token valid tetapi profilnya sudah hilang dari DB → 401 (bukan 500).
func TestUpdateLocation_ProfilTidakAda(t *testing.T) {
	repo := &fakeRepo{locSetErr: store.ErrUserNotFound}
	h := newTestServer(repo, NewMemoryRateLimiter())
	rec := do(h, authedRequest(http.MethodPut, "/api/v1/users/location",
		`{"latitude":-6.9,"longitude":107.6}`, testSecret, "hilang"))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, mau 401. body=%s", rec.Code, rec.Body.String())
	}
}

// --- PUT /api/v1/users/fcm-token ---

func TestUpdateFCMToken_OK(t *testing.T) {
	updated := time.Unix(1_700_000_700, 0).UTC()
	repo := &fakeRepo{fcmUpdate: updated}
	h := newTestServer(repo, NewMemoryRateLimiter())

	const token = "fMEP0vJq:APA91bH-Xy9"
	rec := do(h, authedRequest(http.MethodPut, "/api/v1/users/fcm-token",
		`{"fcm_token":"`+token+`"}`, testSecret, "u"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200. body=%s", rec.Code, rec.Body.String())
	}
	if repo.fcmToken != token {
		t.Fatalf("token tersimpan = %q, mau %q", repo.fcmToken, token)
	}
	var resp updateFCMTokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.UpdatedAt != updated.Format(time.RFC3339) {
		t.Fatalf("updated_at = %q", resp.UpdatedAt)
	}
}

func TestUpdateFCMToken_PayloadInvalid(t *testing.T) {
	cases := map[string]string{
		"body kosong":       ``,
		"objek kosong":      `{}`,
		"token kosong":      `{"fcm_token":""}`,
		"token > 255":       `{"fcm_token":"` + strings.Repeat("t", 256) + `"}`,
		"field tak dikenal": `{"fcm_token":"abc","device":"pixel"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			repo := &fakeRepo{}
			h := newTestServer(repo, NewMemoryRateLimiter())
			rec := do(h, authedRequest(http.MethodPut, "/api/v1/users/fcm-token", body, testSecret, "u"))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, mau 400. body=%s", rec.Code, rec.Body.String())
			}
			if repo.fcmToken != "" {
				t.Fatalf("store tidak boleh dipanggil untuk payload invalid")
			}
		})
	}
}

// Batas 255 karakter bersifat inklusif (cermin VARCHAR(255)).
func TestUpdateFCMToken_BatasPanjangInklusif(t *testing.T) {
	repo := &fakeRepo{fcmUpdate: time.Unix(1_700_000_700, 0).UTC()}
	h := newTestServer(repo, NewMemoryRateLimiter())
	body := `{"fcm_token":"` + strings.Repeat("t", 255) + `"}`
	if code := do(h, authedRequest(http.MethodPut, "/api/v1/users/fcm-token", body, testSecret, "u")).Code; code != http.StatusOK {
		t.Fatalf("status = %d, mau 200 untuk token 255 karakter", code)
	}
}

func TestUpdateFCMToken_TanpaJWT(t *testing.T) {
	h := newTestServer(&fakeRepo{}, NewMemoryRateLimiter())
	req := httptest.NewRequest(http.MethodPut, "/api/v1/users/fcm-token",
		strings.NewReader(`{"fcm_token":"abc"}`))
	if code := do(h, req).Code; code != http.StatusUnauthorized {
		t.Fatalf("status = %d, mau 401", code)
	}
}

func TestUpdateFCMToken_JWTKedaluwarsa(t *testing.T) {
	h := newTestServer(&fakeRepo{}, NewMemoryRateLimiter())
	req := expiredRequest(http.MethodPut, "/api/v1/users/fcm-token", `{"fcm_token":"abc"}`, testSecret, "u")
	if code := do(h, req).Code; code != http.StatusUnauthorized {
		t.Fatalf("status = %d, mau 401", code)
	}
}

// --- GET /api/v1/events ---

// sampleEvents adalah dua event terurut terbaru-dulu (seperti keluaran store).
func sampleEvents() []store.Event {
	resolved := time.Unix(1_700_000_300, 0).UTC()
	return []store.Event{
		{
			EventID: "evt-2", Status: "HAPPENING",
			Lat: -6.8721, Lon: 107.5422, LocationName: "Cimahi, West Java, ID",
			MMIScale: "V", IntensityLabel: "Strong", MaxPGA: 413.13,
			TriggeredNodes: 4, StartedAt: time.Unix(1_700_000_200, 0).UTC(),
		},
		{
			EventID: "evt-1", Status: "RESOLVED",
			Lat: -7.1, Lon: 108.0, LocationName: "Tasikmalaya, West Java, ID",
			MMIScale: "IV", IntensityLabel: "Light", MaxPGA: 33.5,
			TriggeredNodes: 3, StartedAt: time.Unix(1_700_000_100, 0).UTC(),
			ResolvedAt: &resolved,
		},
	}
}

// Tanpa token pun endpoint melayani: data gempa bersifat publik.
func TestListEvents_PublikTanpaToken(t *testing.T) {
	repo := &fakeRepo{events: sampleEvents()}
	h := newTestServer(repo, NewMemoryRateLimiter())

	rec := do(h, httptest.NewRequest(http.MethodGet, "/api/v1/events", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200. body=%s", rec.Code, rec.Body.String())
	}

	var resp eventsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Limit != defaultEventsLimit || resp.Offset != 0 || resp.Count != 2 {
		t.Fatalf("paginasi salah: %+v", resp)
	}
	if resp.RangeKm != nil {
		t.Fatalf("range_km = %v, mau null tanpa filter", *resp.RangeKm)
	}
	if repo.eventsFilt != nil {
		t.Fatalf("filter spasial harus nil: %+v", repo.eventsFilt)
	}
	if repo.eventsLimit != defaultEventsLimit {
		t.Fatalf("limit ke store = %d, mau %d", repo.eventsLimit, defaultEventsLimit)
	}

	// Urutan dari store dipertahankan (terbaru dulu) & pemetaan field benar.
	if resp.Events[0].EventID != "evt-2" || resp.Events[1].EventID != "evt-1" {
		t.Fatalf("urutan event berubah: %+v", resp.Events)
	}
	got := resp.Events[0]
	if got.PGA != 413.13 || got.MMI != "V" || got.Latitude != -6.8721 || got.Longitude != 107.5422 {
		t.Fatalf("pemetaan field salah: %+v", got)
	}
	if got.CreatedAt != time.Unix(1_700_000_200, 0).UTC().Format(time.RFC3339) {
		t.Fatalf("created_at = %q (harus dari started_at)", got.CreatedAt)
	}
	if got.ResolvedAt != nil {
		t.Fatalf("resolved_at harus absen untuk HAPPENING, dapat %q", *got.ResolvedAt)
	}
	if resp.Events[1].ResolvedAt == nil {
		t.Fatal("resolved_at wajib ada untuk RESOLVED")
	}
}

// depth_km hadir sebagai null eksplisit (kontrak: jaringan MEMS tidak
// mengestimasi kedalaman, tetapi field dipertahankan agar model klien stabil).
func TestListEvents_DepthKmSelaluNull(t *testing.T) {
	repo := &fakeRepo{events: sampleEvents()}
	h := newTestServer(repo, NewMemoryRateLimiter())

	rec := do(h, httptest.NewRequest(http.MethodGet, "/api/v1/events", nil))
	var raw struct {
		Events []map[string]any `json:"events"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for i, e := range raw.Events {
		v, ok := e["depth_km"]
		if !ok {
			t.Fatalf("event[%d]: field depth_km harus ada di payload", i)
		}
		if v != nil {
			t.Fatalf("event[%d]: depth_km = %#v, mau null", i, v)
		}
	}
}

func TestListEvents_Paginasi(t *testing.T) {
	repo := &fakeRepo{events: sampleEvents()[:1]}
	h := newTestServer(repo, NewMemoryRateLimiter())

	rec := do(h, authedRequest(http.MethodGet, "/api/v1/events?limit=1&offset=5", "", testSecret, "u"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200. body=%s", rec.Code, rec.Body.String())
	}
	if repo.eventsLimit != 1 || repo.eventsOff != 5 {
		t.Fatalf("store menerima limit=%d offset=%d, mau 1/5", repo.eventsLimit, repo.eventsOff)
	}
}

func TestListEvents_ParamInvalid(t *testing.T) {
	cases := []struct {
		name, query string
	}{
		{"limit nol", "?limit=0"},
		{"limit di atas maksimum", "?limit=101"},
		{"limit bukan angka", "?limit=abc"},
		{"limit negatif", "?limit=-1"},
		{"offset negatif", "?offset=-1"},
		{"offset bukan angka", "?offset=xyz"},
		{"range_km nol", "?range_km=0&latitude=-6.9&longitude=107.6"},
		{"range_km di atas maksimum", "?range_km=2001&latitude=-6.9&longitude=107.6"},
		{"range_km bukan angka", "?range_km=abc&latitude=-6.9&longitude=107.6"},
		{"longitude absen", "?range_km=50&latitude=-6.9"},
		{"latitude absen", "?range_km=50&longitude=107.6"},
		{"latitude di luar rentang", "?range_km=50&latitude=91&longitude=107.6"},
		{"longitude di luar rentang", "?range_km=50&latitude=-6.9&longitude=181"},
		{"koordinat tanpa range_km", "?latitude=-6.9&longitude=107.6"},
		{"offset melebihi batas", "?offset=50001"},
		{"min_pga negatif", "?min_pga=-1"},
		{"min_pga di atas maksimum", "?min_pga=2001"},
		{"min_pga bukan angka", "?min_pga=kuat"},
		{"since bukan RFC3339", "?since=2026-08-01"},
		{"until bukan RFC3339", "?until=kemarin"},
		{"rentang waktu terbalik", "?since=2026-08-10T00:00:00Z&until=2026-08-01T00:00:00Z"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepo{events: sampleEvents()}
			h := newTestServer(repo, NewMemoryRateLimiter())
			rec := do(h, httptest.NewRequest(http.MethodGet, "/api/v1/events"+tc.query, nil))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, mau 400. body=%s", rec.Code, rec.Body.String())
			}
			if repo.eventsLimit != 0 {
				t.Fatalf("store tidak boleh dipanggil untuk parameter invalid")
			}
		})
	}
}

func TestListEvents_FilterKoordinatEksplisit(t *testing.T) {
	repo := &fakeRepo{events: sampleEvents()}
	h := newTestServer(repo, NewMemoryRateLimiter())

	rec := do(h, httptest.NewRequest(http.MethodGet,
		"/api/v1/events?range_km=300&latitude=-6.9&longitude=107.6", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200. body=%s", rec.Code, rec.Body.String())
	}
	if repo.eventsFilt == nil {
		t.Fatal("filter spasial harus terbentuk")
	}
	if repo.eventsFilt.RangeKm != 300 || repo.eventsFilt.Lat != -6.9 || repo.eventsFilt.Lon != 107.6 {
		t.Fatalf("filter = %+v", repo.eventsFilt)
	}

	var resp eventsResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.RangeKm == nil || *resp.RangeKm != 300 {
		t.Fatalf("range_km pada respons = %v, mau 300", resp.RangeKm)
	}
}

// range_km tanpa koordinat eksplisit memakai lokasi tersimpan user (butuh token).
func TestListEvents_FilterDariLokasiUser(t *testing.T) {
	repo := &fakeRepo{
		events: sampleEvents(),
		loc:    &store.UserLocation{HasLocation: true, Lat: -6.95, Lon: 107.65},
	}
	h := newTestServer(repo, NewMemoryRateLimiter())

	rec := do(h, authedRequest(http.MethodGet, "/api/v1/events?range_km=100", "", testSecret, "u"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200. body=%s", rec.Code, rec.Body.String())
	}
	if repo.eventsFilt == nil || repo.eventsFilt.Lat != -6.95 || repo.eventsFilt.Lon != 107.65 {
		t.Fatalf("filter = %+v, mau memakai lokasi user", repo.eventsFilt)
	}
}

// Anonim + range_km tanpa koordinat: ditolak 400, BUKAN diam-diam mengembalikan
// event seluruh negeri (kegagalan senyap pada endpoint life-safety).
func TestListEvents_RangeTanpaAcuanDitolak(t *testing.T) {
	repo := &fakeRepo{events: sampleEvents()}
	h := newTestServer(repo, NewMemoryRateLimiter())

	rec := do(h, httptest.NewRequest(http.MethodGet, "/api/v1/events?range_km=100", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, mau 400. body=%s", rec.Code, rec.Body.String())
	}
	if repo.eventsLimit != 0 {
		t.Fatal("store tidak boleh dipanggil tanpa acuan koordinat")
	}
}

// Terautentikasi tetapi belum pernah mengirim lokasi → 400 dengan pesan jelas.
func TestListEvents_RangeTanpaLokasiTersimpan(t *testing.T) {
	repo := &fakeRepo{events: sampleEvents(), loc: &store.UserLocation{HasLocation: false}}
	h := newTestServer(repo, NewMemoryRateLimiter())

	rec := do(h, authedRequest(http.MethodGet, "/api/v1/events?range_km=100", "", testSecret, "u"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, mau 400. body=%s", rec.Code, rec.Body.String())
	}
}

// Token yang dikirim tetapi kedaluwarsa ditolak 401 — tidak di-downgrade menjadi
// akses anonim, agar bug klien tidak tersembunyi di balik 200.
func TestListEvents_JWTKedaluwarsaDitolak(t *testing.T) {
	repo := &fakeRepo{events: sampleEvents()}
	h := newTestServer(repo, NewMemoryRateLimiter())

	rec := do(h, expiredRequest(http.MethodGet, "/api/v1/events", "", testSecret, "u"))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, mau 401. body=%s", rec.Code, rec.Body.String())
	}
	if repo.eventsLimit != 0 {
		t.Fatal("store tidak boleh dipanggil untuk token kedaluwarsa")
	}
}

func TestListEvents_SkemaAuthSalahDitolak(t *testing.T) {
	h := newTestServer(&fakeRepo{events: sampleEvents()}, NewMemoryRateLimiter())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	if code := do(h, req).Code; code != http.StatusUnauthorized {
		t.Fatalf("status = %d, mau 401", code)
	}
}

func TestListEvents_KosongJadiArrayBukanNull(t *testing.T) {
	h := newTestServer(&fakeRepo{events: nil}, NewMemoryRateLimiter())
	rec := do(h, httptest.NewRequest(http.MethodGet, "/api/v1/events", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"events":[]`) {
		t.Fatalf("events harus [] bukan null: %s", rec.Body.String())
	}
}

func TestListEvents_StoreGagal(t *testing.T) {
	repo := &fakeRepo{eventsErr: errors.New("query gagal")}
	h := newTestServer(repo, NewMemoryRateLimiter())
	if code := do(h, httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)).Code; code != http.StatusInternalServerError {
		t.Fatalf("status = %d, mau 500", code)
	}
}

// --- Router: endpoint publik tidak boleh ikut terproteksi, dan sebaliknya ---

func TestRouter_TingkatAkses(t *testing.T) {
	repo := &fakeRepo{
		events:           sampleEvents(),
		profileCreatedAt: time.Unix(1_700_000_000, 0).UTC(),
	}
	h := newTestServer(repo, NewMemoryRateLimiter())

	cases := []struct {
		name, method, path string
		wantNoAuth         int
	}{
		{"healthz publik", http.MethodGet, "/healthz", http.StatusOK},
		{"auth anonymous publik", http.MethodPost, "/api/v1/auth/anonymous", http.StatusCreated},
		{"events publik", http.MethodGet, "/api/v1/events", http.StatusOK},
		{"sensors wajib auth", http.MethodGet, "/api/v1/sensors", http.StatusUnauthorized},
		{"provision wajib auth", http.MethodPost, "/api/v1/nodes/provision", http.StatusUnauthorized},
		{"reroll wajib auth", http.MethodPost, "/api/v1/users/pseudonym/reroll", http.StatusUnauthorized},
		{"lokasi wajib auth", http.MethodPut, "/api/v1/users/location", http.StatusUnauthorized},
		{"fcm wajib auth", http.MethodPut, "/api/v1/users/fcm-token", http.StatusUnauthorized},
		{"ws wajib auth", http.MethodGet, "/ws", http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(h, httptest.NewRequest(tc.method, tc.path, nil))
			if rec.Code != tc.wantNoAuth {
				t.Fatalf("%s %s tanpa auth = %d, mau %d. body=%s",
					tc.method, tc.path, rec.Code, tc.wantNoAuth, rec.Body.String())
			}
		})
	}
}

// uuidV4Pattern memvalidasi bentuk UUID v4 yang di-generate randomUserID.
var uuidV4Pattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestRandomUserID_UUIDv4(t *testing.T) {
	seen := make(map[string]struct{}, 64)
	for i := 0; i < 64; i++ {
		id, err := randomUserID()
		if err != nil {
			t.Fatalf("randomUserID: %v", err)
		}
		if !uuidV4Pattern.MatchString(id) {
			t.Fatalf("id = %q bukan UUID v4", id)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("id duplikat: %q", id)
		}
		seen[id] = struct{}{}
	}
}

func TestMintHS256_ArgumenInvalid(t *testing.T) {
	cases := []struct {
		name   string
		userID string
		secret []byte
		ttl    time.Duration
	}{
		{"sub kosong", "", []byte(testSecret), time.Hour},
		{"secret kosong", "u", nil, time.Hour},
		{"ttl nol", "u", []byte(testSecret), 0},
		{"ttl negatif", "u", []byte(testSecret), -time.Hour},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := MintHS256(tc.userID, tc.secret, tc.ttl); err == nil {
				t.Fatal("mau error, dapat nil")
			}
		})
	}
}

// Ketiga filter baru (min_pga, since, until) tiba di store apa adanya dan tidak
// memerlukan koordinat: pertanyaan "gempa kuat sepekan terakhir" sah tanpa radius.
func TestListEvents_FilterIntensitasDanWaktu(t *testing.T) {
	repo := &fakeRepo{events: sampleEvents()}
	h := newTestServer(repo, NewMemoryRateLimiter())

	rec := do(h, httptest.NewRequest(http.MethodGet,
		"/api/v1/events?min_pga=137.2&since=2026-08-01T00:00:00Z&until=2026-08-10T12:00:00%2B07:00", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200. body=%s", rec.Code, rec.Body.String())
	}
	f := repo.eventsFilt
	if f == nil {
		t.Fatal("filter harus terbentuk dari min_pga/since/until")
	}
	if f.MinPGA == nil || *f.MinPGA != 137.2 {
		t.Fatalf("MinPGA = %v, mau 137.2", f.MinPGA)
	}
	if f.RangeKm != 0 {
		t.Fatalf("RangeKm = %d, mau 0 (tanpa filter spasial)", f.RangeKm)
	}
	if f.Since == nil || !f.Since.Equal(time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("Since = %v", f.Since)
	}
	// Offset +07:00 dinormalkan ke UTC sebelum sampai ke store.
	if f.Until == nil || !f.Until.Equal(time.Date(2026, 8, 10, 5, 0, 0, 0, time.UTC)) {
		t.Fatalf("Until = %v, mau 05:00Z", f.Until)
	}
	if f.Until.Location() != time.UTC {
		t.Fatalf("Until zona = %v, mau UTC", f.Until.Location())
	}

	// range_km hanya dicerminkan bila filter spasial aktif; 0 km akan terbaca
	// klien sebagai "tidak ada event di sekitarmu".
	var resp eventsResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.RangeKm != nil {
		t.Fatalf("range_km pada respons = %v, mau null", *resp.RangeKm)
	}
}

// Paginasi tidak boleh berubah karena filter: halaman tetap penuh sebesar limit,
// karena penyaringan terjadi di SQL, bukan setelah limit dipotong.
func TestListEvents_FilterTidakMemotongHalaman(t *testing.T) {
	page := make([]store.Event, 0, defaultEventsLimit)
	for i := 0; i < defaultEventsLimit; i++ {
		e := sampleEvents()[0]
		e.EventID = fmt.Sprintf("evt-%02d", i)
		page = append(page, e)
	}
	repo := &fakeRepo{events: page}
	h := newTestServer(repo, NewMemoryRateLimiter())

	rec := do(h, httptest.NewRequest(http.MethodGet, "/api/v1/events?min_pga=16.6", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200. body=%s", rec.Code, rec.Body.String())
	}
	var resp eventsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Count != defaultEventsLimit || len(resp.Events) != defaultEventsLimit {
		t.Fatalf("count = %d, mau halaman penuh %d", resp.Count, defaultEventsLimit)
	}
	if repo.eventsLimit != defaultEventsLimit {
		t.Fatalf("limit ke store = %d, mau %d", repo.eventsLimit, defaultEventsLimit)
	}
}

// Filter spasial dan non-spasial bisa digabung dalam satu kueri.
func TestListEvents_FilterGabungan(t *testing.T) {
	repo := &fakeRepo{events: sampleEvents()}
	h := newTestServer(repo, NewMemoryRateLimiter())

	rec := do(h, httptest.NewRequest(http.MethodGet,
		"/api/v1/events?range_km=250&latitude=-6.9&longitude=107.6&min_pga=16.6", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200. body=%s", rec.Code, rec.Body.String())
	}
	f := repo.eventsFilt
	if f == nil || f.RangeKm != 250 || f.MinPGA == nil || *f.MinPGA != 16.6 {
		t.Fatalf("filter = %+v", f)
	}
	var resp eventsResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.RangeKm == nil || *resp.RangeKm != 250 {
		t.Fatalf("range_km pada respons = %v, mau 250", resp.RangeKm)
	}
}
