package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// doHealthz menjalankan GET /healthz (publik: tanpa JWT, tanpa kunci admin).
func doHealthz(h http.Handler) *httptest.ResponseRecorder {
	return do(h, httptest.NewRequest(http.MethodGet, "/healthz", nil))
}

// Kontrak pertama tetap status code: probe load balancer tidak membaca body.
func TestHealthz_AllOkIs200(t *testing.T) {
	h := newTestServer(&fakeRepo{}, NewMemoryRateLimiter())

	rec := doHealthz(h)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200", rec.Code)
	}
	var resp healthzResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("body bukan JSON: %v — %s", err, rec.Body.String())
	}
	if resp.Status != "ok" || resp.Database != "ok" || resp.MQTT != "unknown" {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestHealthz_DatabaseDownIs503(t *testing.T) {
	h := newTestServer(&fakeRepo{pingErr: errBoom}, NewMemoryRateLimiter())

	rec := doHealthz(h)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, mau 503 (database kritikal)", rec.Code)
	}
	var resp healthzResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Database != "down" || resp.Status != "degraded" {
		t.Fatalf("resp = %+v", resp)
	}
}

// MQTT down menurunkan field-nya, bukan status code: ingest berhenti, tetapi
// alert yang sudah terdispatch tetap sampai — keadaan 'terbatas', bukan 'mati'.
func TestHealthz_MQTTDownStays200(t *testing.T) {
	repo := &fakeRepo{}
	srv := NewServer(repo, fakeCipher{}, NewMemoryRateLimiter(),
		MQTTPublic{Broker: "b", Port: 8883, TLS: true},
		AuthConfig{JWTSecret: []byte(testSecret), TokenTTL: testTokenTTL},
		testLogger())
	srv.SetMQTTHealthCheck(func() bool { return false })
	h := srv.Router(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusSwitchingProtocols)
	}, testLogger())

	rec := doHealthz(h)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200 (mqtt bukan dependensi status code)", rec.Code)
	}
	var resp healthzResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.MQTT != "down" || resp.Status != "degraded" {
		t.Fatalf("resp = %+v", resp)
	}
}

// Endpoint ini publik dan harus tetap begitu: badge yang jujur tidak boleh
// bergantung pada sesi token yang valid.
func TestHealthz_IsPublic(t *testing.T) {
	h := newTestServer(&fakeRepo{}, NewMemoryRateLimiter())
	rec := do(h, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz tanpa auth = %d, mau 200", rec.Code)
	}
}
