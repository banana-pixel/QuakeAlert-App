package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeTrackerStats adalah TrackerStatsSource yang mengembalikan potret tetap.
// Cukup untuk menguji BENTUK JSON yang dijanjikan kontrak; keakuratan angkanya
// diuji di paket event, tempat pengukurannya sebenarnya terjadi.
type fakeTrackerStats struct {
	stats  TrackerStatsJSON
	report NearConfirmedReportJSON
}

func (f *fakeTrackerStats) Stats() TrackerStatsJSON { return f.stats }

func (f *fakeTrackerStats) NearConfirmedReport() NearConfirmedReportJSON { return f.report }

// newTrackerStatsServer membangun handler dengan kunci operator dan sumber
// statistik terpasang.
func newTrackerStatsServer(src TrackerStatsSource) http.Handler {
	srv := NewServer(&fakeRepo{}, fakeDecryptCipher{}, NewMemoryRateLimiter(),
		MQTTPublic{Broker: "b", Port: 8883, TLS: true},
		AuthConfig{JWTSecret: []byte(testSecret), TokenTTL: testTokenTTL},
		testLogger())
	srv.SetAdminAPIKey(adminTestKey)
	if src != nil {
		srv.SetTrackerStats(src)
	}
	return srv.Router(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusSwitchingProtocols)
	}, testLogger())
}

func trackerStatsRequest(key string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/tracker/stats", nil)
	if key != "" {
		req.Header.Set(AdminKeyHeader, key)
	}
	return req
}

// TestTrackerStats_LatencyFieldNames — ketiga seri latensi P4-M3′ muncul dengan
// NAMA yang tertulis di contracts/openapi/openapi.yaml, masing-masing sebagai
// objek {observed, p50_ms, p95_ms}.
//
// Nama-nama itu adalah kontraknya: alat forensik apa pun membacanya dari luar,
// jadi mengganti sebuah tag json diam-diam merusak pembaca tanpa satu pun
// kegagalan kompilasi. Uji ini yang membuat perubahan seperti itu berbunyi.
func TestTrackerStats_LatencyFieldNames(t *testing.T) {
	src := &fakeTrackerStats{stats: TrackerStatsJSON{
		OnsetToDecidedSensor:  LatencyStatsJSON{Observed: 7, P50Ms: 620, P95Ms: 1480},
		OnsetToDecidedPublish: LatencyStatsJSON{Observed: 3, P50Ms: 5200, P95Ms: 6100},
		DecidedToEmit:         LatencyStatsJSON{Observed: 11, P50Ms: 1, P95Ms: 4},
	}}
	rec := do(newTrackerStatsServer(src), trackerStatsRequest(adminTestKey))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200: %s", rec.Code, rec.Body.String())
	}

	var got map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("respons bukan JSON objek: %v", err)
	}

	want := map[string]LatencyStatsJSON{
		"event_latency_onset_to_decided_sensor_ms":        src.stats.OnsetToDecidedSensor,
		"event_latency_onset_to_decided_publish_bound_ms": src.stats.OnsetToDecidedPublish,
		"event_latency_decided_to_emit_ms":                src.stats.DecidedToEmit,
	}
	for field, exp := range want {
		raw, ok := got[field]
		if !ok {
			t.Errorf("bidang %q tidak ada di respons", field)
			continue
		}
		var v LatencyStatsJSON
		if err := json.Unmarshal(raw, &v); err != nil {
			t.Errorf("bidang %q bukan objek LatencyStats: %v", field, err)
			continue
		}
		if v != exp {
			t.Errorf("bidang %q = %+v, mau %+v", field, v, exp)
		}
	}
}

// TestTrackerStats_LatencySubfieldNames — sub-bidang objek latensi bernama
// observed / p50_ms / p95_ms, dan observed IKUT SERTA bahkan ketika nol.
//
// observed adalah yang membedakan "belum ada yang diukur" dari "latensinya nol";
// tanpa dia, p50 = 0 adalah klaim yang tidak dapat dibaca. Karena itu ia tidak
// boleh omitempty.
func TestTrackerStats_LatencySubfieldNames(t *testing.T) {
	src := &fakeTrackerStats{} // seluruh seri nol, seperti proses yang baru mulai
	rec := do(newTrackerStatsServer(src), trackerStatsRequest(adminTestKey))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200: %s", rec.Code, rec.Body.String())
	}

	var got struct {
		Sensor map[string]json.RawMessage `json:"event_latency_onset_to_decided_sensor_ms"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("respons bukan JSON objek: %v", err)
	}

	for _, key := range []string{"observed", "p50_ms", "p95_ms"} {
		if _, ok := got.Sensor[key]; !ok {
			t.Errorf("sub-bidang %q tidak ada — objek latensi harus melaporkan "+
				"ketiganya, termasuk saat nol", key)
		}
	}
}

// TestTrackerStats_DisabledTrackerUnchanged — tanpa sumber statistik, endpoint
// tetap 503 seperti sebelum P4-M3′. Penambahan bidang bersifat aditif dan tidak
// boleh mengubah jalur EVENT_TRACKER_ENABLED=false.
func TestTrackerStats_DisabledTrackerUnchanged(t *testing.T) {
	rec := do(newTrackerStatsServer(nil), trackerStatsRequest(adminTestKey))

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, mau 503 saat Tracker tidak aktif", rec.Code)
	}
}
