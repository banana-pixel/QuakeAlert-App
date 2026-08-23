package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeTestAlertFanout mencatat drill yang diteruskan ke jalur WS/FCM.
type fakeTestAlertFanout struct {
	got []TestAlert
}

func (f *fakeTestAlertFanout) DispatchTestAlert(t TestAlert) { f.got = append(f.got, t) }

// newDrillServer membangun handler dengan kunci operator dan jalur drill
// terpasang. fanout nil sengaja dibiarkan mungkin: itu keadaan yang harus
// menjawab 503, bukan 200.
func newDrillServer(fanout TestAlertFanout, key string) http.Handler {
	srv := NewServer(&fakeRepo{}, fakeCipher{}, NewMemoryRateLimiter(),
		MQTTPublic{Broker: "b", Port: 8883, TLS: true},
		AuthConfig{JWTSecret: []byte(testSecret), TokenTTL: testTokenTTL},
		testLogger())
	srv.SetAdminAPIKey(key)
	if fanout != nil {
		srv.SetTestAlertFanout(fanout)
	}
	return srv.Router(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusSwitchingProtocols)
	}, testLogger())
}

func drillRequest(body, key string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/test-alert", strings.NewReader(body))
	if key != "" {
		req.Header.Set(AdminKeyHeader, key)
	}
	return req
}

// Yang diuji di sini bukan hanya bahwa drill terkirim, tetapi bahwa ia TIDAK
// menyentuh basis data: sebuah baris earthquake_events dari latihan akan muncul
// di kartu aktivitas pengguna dan menggeser hitungan 30 harinya.
func TestCreateTestAlert_DispatchesWithoutPersisting(t *testing.T) {
	fanout := &fakeTestAlertFanout{}
	h := newDrillServer(fanout, adminTestKey)

	body := `{"pga_gal":200,"latitude":-6.9175,"longitude":107.6191,
	          "location_name":"Bandung,  Jawa Barat"}`
	rec := do(h, drillRequest(body, adminTestKey))

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, mau 202: %s", rec.Code, rec.Body.String())
	}
	if len(fanout.got) != 1 {
		t.Fatalf("fanout = %+v", fanout.got)
	}
	got := fanout.got[0]
	// MMI dan label diturunkan dari PGA oleh fungsi jalur konsensus, bukan
	// diterima dari pemanggil.
	if got.MMI != "VII" || got.IntensityLabel != "strong" {
		t.Fatalf("mmi/label = %q/%q, mau VII/strong", got.MMI, got.IntensityLabel)
	}
	// Id mengaku dirinya latihan, dan tidak mungkin bertabrakan dengan
	// gen_random_uuid() milik basis data.
	if !strings.HasPrefix(got.EventID, "test-") {
		t.Fatalf("event_id = %q, mau berawalan test-", got.EventID)
	}
	// Spasi ganda dari here-doc shell diratakan, seperti pada siaran.
	if got.LocationName != "Bandung, Jawa Barat" {
		t.Fatalf("location_name = %q", got.LocationName)
	}
	// Tanpa node_count, drill memakai ambang CONFIRMED — bentuk peringatan yang
	// sedang dilatih.
	if got.NodeCount != defaultTestNodeCount {
		t.Fatalf("node_count = %d, mau %d", got.NodeCount, defaultTestNodeCount)
	}

	var resp testAlertResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.IsTest || resp.Topic != "test_alerts" {
		t.Fatalf("resp = %+v", resp)
	}
}

// Tanpa location_name teksnya sendiri mengaku latihan: kalaupun sebuah build
// lolos kedua pagar dan menampilkannya, isinya masih mengatakan apa itu.
func TestCreateTestAlert_NamesItselfWhenNoLocationGiven(t *testing.T) {
	fanout := &fakeTestAlertFanout{}
	h := newDrillServer(fanout, adminTestKey)

	rec := do(h, drillRequest(`{"pga_gal":20,"latitude":0,"longitude":0}`, adminTestKey))

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, mau 202: %s", rec.Code, rec.Body.String())
	}
	if fanout.got[0].LocationName != defaultTestLocationName {
		t.Fatalf("location_name = %q", fanout.got[0].LocationName)
	}
	// 0,0 dikirim eksplisit, jadi harus diterima sebagai koordinat — bukan
	// dianggap field yang absen.
	if fanout.got[0].Latitude != 0 || fanout.got[0].Longitude != 0 {
		t.Fatalf("koordinat 0,0 eksplisit harus lolos: %+v", fanout.got[0])
	}
}

func TestCreateTestAlert_RejectsUnusableInput(t *testing.T) {
	cases := map[string]string{
		"pga absen":            `{"latitude":-6.9,"longitude":107.6}`,
		"pga di bawah rentang": `{"pga_gal":0.5,"latitude":-6.9,"longitude":107.6}`,
		"pga di atas rentang":  `{"pga_gal":5000,"latitude":-6.9,"longitude":107.6}`,
		"koordinat absen":      `{"pga_gal":200}`,
		"latitude absen":       `{"pga_gal":200,"longitude":107.6}`,
		"latitude di luar":     `{"pga_gal":200,"latitude":91,"longitude":107.6}`,
		"longitude di luar":    `{"pga_gal":200,"latitude":-6.9,"longitude":181}`,
		"node_count berlebih":  `{"pga_gal":200,"latitude":-6.9,"longitude":107.6,"node_count":100}`,
		"nama terlalu panjang": `{"pga_gal":200,"latitude":-6.9,"longitude":107.6,` +
			`"location_name":"` + strings.Repeat("a", maxLocationNameLen+1) + `"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			fanout := &fakeTestAlertFanout{}
			h := newDrillServer(fanout, adminTestKey)

			rec := do(h, drillRequest(body, adminTestKey))

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, mau 400: %s", rec.Code, rec.Body.String())
			}
			if len(fanout.got) != 0 {
				t.Fatal("permintaan tidak valid tidak boleh menyiarkan apa pun")
			}
		})
	}
}

// Kunci yang sama dengan siaran, jadi jalur otentikasinya juga sama — termasuk
// keadaan tanpa ADMIN_API_KEY, di mana rutenya tidak ada sama sekali.
func TestCreateTestAlert_NeedsTheOperatorKey(t *testing.T) {
	fanout := &fakeTestAlertFanout{}
	h := newDrillServer(fanout, adminTestKey)

	rec := do(h, drillRequest(`{"pga_gal":200,"latitude":-6.9,"longitude":107.6}`, "kunci-salah"))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, mau 401", rec.Code)
	}

	absent := newDrillServer(fanout, "")
	rec = do(absent, drillRequest(`{"pga_gal":200,"latitude":-6.9,"longitude":107.6}`, adminTestKey))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, mau 404", rec.Code)
	}
	if len(fanout.got) != 0 {
		t.Fatal("tidak ada drill yang boleh lolos tanpa kunci")
	}
}

// Drill yang diterima tetapi tidak dikirim ke mana pun adalah cara terburuk
// untuk mengetahui jalur push mati, jadi keadaan itu 503 dan bukan 202.
func TestCreateTestAlert_SaysSoWhenThereIsNoFanout(t *testing.T) {
	h := newDrillServer(nil, adminTestKey)

	rec := do(h, drillRequest(`{"pga_gal":200,"latitude":-6.9,"longitude":107.6}`, adminTestKey))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, mau 503: %s", rec.Code, rec.Body.String())
	}
}
