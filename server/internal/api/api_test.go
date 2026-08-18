package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/banana-pixel/quakealert/server/internal/store"
)

// --- Fakes ---

type fakeRepo struct {
	created   *store.NewNode
	loc       *store.UserLocation
	locErr    error
	sensors   []store.SensorStatus
	pseudonym string
	updatedAt time.Time
	updateErr error
}

func (f *fakeRepo) CreateNode(_ context.Context, n *store.NewNode) error {
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

const testSecret = "this-is-a-32-byte-minimum-secret!"

func newTestServer(repo Repo, limiter RateLimiter) http.Handler {
	srv := NewServer(repo, fakeCipher{}, limiter, MQTTPublic{Broker: "b", Port: 8883, TLS: true}, testLogger())
	return srv.Router([]byte(testSecret), func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusSwitchingProtocols)
	}, testLogger())
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
	if !strings.HasPrefix(resp.StationID, "NODE-") {
		t.Fatalf("station_id = %q", resp.StationID)
	}
	if resp.ProvisioningSecret == "" || !resp.MQTTTLS || resp.MQTTPort != 8883 {
		t.Fatalf("field mqtt/secret salah: %+v", resp)
	}
	if repo.created == nil || repo.created.Lat != -6.87 {
		t.Fatalf("node tidak tersimpan benar: %+v", repo.created)
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

	do := func() int {
		req := authedRequest(http.MethodPost, "/api/v1/users/pseudonym/reroll", "", testSecret, "user-x")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := do(); code != http.StatusOK {
		t.Fatalf("reroll pertama = %d, mau 200", code)
	}
	if code := do(); code != http.StatusTooManyRequests {
		t.Fatalf("reroll kedua = %d, mau 429", code)
	}
}
