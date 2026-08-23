package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/banana-pixel/quakealert/server/internal/store"
)

// adminTestKey panjangnya >= minAdminKeyLen agar sama dengan kunci yang benar-benar
// dipakai di produksi.
// errBoom mewakili kegagalan basis data apa pun.
var errBoom = errors.New("koneksi db putus")

const adminTestKey = "admin-key-yang-cukup-panjang-32b!"

// fakeBroadcastFanout mencatat siaran yang diteruskan ke jalur WS/FCM.
type fakeBroadcastFanout struct {
	got []AdminBroadcast
}

func (f *fakeBroadcastFanout) BroadcastAdmin(b AdminBroadcast) { f.got = append(f.got, b) }

// newAdminServer membangun handler dengan kunci operator terpasang.
func newAdminServer(repo Repo, fanout BroadcastFanout, key string) http.Handler {
	srv := NewServer(repo, fakeCipher{}, NewMemoryRateLimiter(),
		MQTTPublic{Broker: "b", Port: 8883, TLS: true},
		AuthConfig{JWTSecret: []byte(testSecret), TokenTTL: testTokenTTL},
		testLogger())
	srv.SetAdminAPIKey(key)
	if fanout != nil {
		srv.SetBroadcastFanout(fanout)
	}
	return srv.Router(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusSwitchingProtocols)
	}, testLogger())
}

// adminRequest membuat request bersenjata header kunci operator.
func adminRequest(body, key string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/broadcasts", strings.NewReader(body))
	if key != "" {
		req.Header.Set(AdminKeyHeader, key)
	}
	return req
}

func TestCreateBroadcast_StoresThenFansOut(t *testing.T) {
	repo := &fakeRepo{}
	fanout := &fakeBroadcastFanout{}
	h := newAdminServer(repo, fanout, adminTestKey)

	body := `{"title":"Uji  sistem","body":"Pesan\nuji"}`
	rec := do(h, adminRequest(body, adminTestKey))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, mau 201: %s", rec.Code, rec.Body.String())
	}
	// Spasi dan newline dari here-doc shell diratakan sebelum disimpan.
	if repo.insertedBTitle != "Uji sistem" || repo.insertedBBody != "Pesan uji" {
		t.Fatalf("tersimpan title=%q body=%q", repo.insertedBTitle, repo.insertedBBody)
	}
	if repo.insertedBRegn != "" {
		t.Fatalf("region_code absen harus nasional, dapat %q", repo.insertedBRegn)
	}
	// Fanout terjadi SETELAH penyimpanan dan memakai id yang dikembalikan store.
	if len(fanout.got) != 1 || fanout.got[0].ID != "bcast-1" {
		t.Fatalf("fanout = %+v", fanout.got)
	}

	var dto broadcastDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &dto); err != nil {
		t.Fatal(err)
	}
	if dto.BroadcastID != "bcast-1" || dto.RegionCode != nil {
		t.Fatalf("dto = %+v", dto)
	}
}

// Kunci yang salah tidak boleh menyentuh basis data sama sekali: 401 harus
// terjadi di middleware, sebelum handler.
func TestCreateBroadcast_WrongKeyIsRejectedBeforeTheHandler(t *testing.T) {
	for name, key := range map[string]string{
		"kunci salah":        "kunci-salah-yang-sama-panjangnya!",
		"kunci lebih pendek": "pendek",
		"header absen":       "",
	} {
		t.Run(name, func(t *testing.T) {
			repo := &fakeRepo{}
			fanout := &fakeBroadcastFanout{}
			h := newAdminServer(repo, fanout, adminTestKey)

			rec := do(h, adminRequest(`{"title":"a","body":"b"}`, key))

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, mau 401", rec.Code)
			}
			if repo.insertedBTitle != "" || len(fanout.got) != 0 {
				t.Fatal("permintaan tanpa kunci sah tidak boleh menulis atau menyiarkan")
			}
		})
	}
}

// Tanpa ADMIN_API_KEY rute admin TIDAK didaftarkan: bukan 401, tetapi 404 —
// instalasi yang lupa mengisinya tidak punya endpoint untuk ditebak kuncinya.
func TestCreateBroadcast_RouteIsAbsentWithoutAKey(t *testing.T) {
	h := newAdminServer(&fakeRepo{}, nil, "")

	rec := do(h, adminRequest(`{"title":"a","body":"b"}`, adminTestKey))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, mau 404", rec.Code)
	}
}

func TestCreateBroadcast_RejectsEmptyAndOverlongText(t *testing.T) {
	cases := map[string]string{
		"judul kosong":          `{"title":"   ","body":"ada isinya"}`,
		"isi kosong":            `{"title":"ada judul","body":""}`,
		"judul terlalu panjang": `{"title":"` + strings.Repeat("a", maxBroadcastTitleLen+1) + `","body":"b"}`,
		"isi terlalu panjang":   `{"title":"a","body":"` + strings.Repeat("b", maxBroadcastBodyLen+1) + `"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			repo := &fakeRepo{}
			h := newAdminServer(repo, nil, adminTestKey)

			rec := do(h, adminRequest(body, adminTestKey))

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, mau 400: %s", rec.Code, rec.Body.String())
			}
			if repo.insertedBTitle != "" {
				t.Fatal("siaran tidak valid tidak boleh tersimpan")
			}
		})
	}
}

// Kunci wilayah dinormalisasi lewat fungsi yang sama dengan sinkronisasi lokasi:
// apa pun bentuk yang diketik operator harus mendarat di kunci yang ditinggali
// pengguna, bukan di ruang kosong yang mirip namanya.
func TestCreateBroadcast_NormalizesTheRegionKey(t *testing.T) {
	for _, typed := range []string{"ID-Jawa Barat", "id-jawa-barat", " ID-jawa-barat "} {
		t.Run(typed, func(t *testing.T) {
			repo := &fakeRepo{}
			h := newAdminServer(repo, nil, adminTestKey)

			body, err := json.Marshal(createBroadcastRequest{
				Title: "Uji", Body: "Isi", RegionCode: typed,
			})
			if err != nil {
				t.Fatal(err)
			}
			rec := do(h, adminRequest(string(body), adminTestKey))

			if rec.Code != http.StatusCreated {
				t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
			}
			if repo.insertedBRegn != "ID-jawa-barat" {
				t.Fatalf("region = %q, mau ID-jawa-barat", repo.insertedBRegn)
			}
		})
	}
}

// Bentuk yang tidak bisa dinormalisasi ditolak, bukan diperlakukan sebagai
// nasional: siaran satu provinsi yang tanpa sengaja membangunkan seluruh negeri
// adalah kegagalan yang lebih mahal daripada 400.
func TestCreateBroadcast_RejectsAnUnparseableRegion(t *testing.T) {
	repo := &fakeRepo{}
	h := newAdminServer(repo, nil, adminTestKey)

	rec := do(h, adminRequest(`{"title":"a","body":"b","region_code":"jawabarat"}`, adminTestKey))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, mau 400", rec.Code)
	}
	if repo.insertedBTitle != "" {
		t.Fatal("region tak terbaca tidak boleh jatuh ke siaran nasional")
	}
}

// Kegagalan store berarti TIDAK ada fanout: pengumuman yang terkirim tetapi tidak
// tersimpan tidak dapat dibuka ulang setelah disapu dari shade.
func TestCreateBroadcast_StoreFailureSuppressesTheFanout(t *testing.T) {
	repo := &fakeRepo{broadcastErr: errBoom}
	fanout := &fakeBroadcastFanout{}
	h := newAdminServer(repo, fanout, adminTestKey)

	rec := do(h, adminRequest(`{"title":"a","body":"b"}`, adminTestKey))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, mau 500", rec.Code)
	}
	if len(fanout.got) != 0 {
		t.Fatal("siaran yang gagal disimpan tidak boleh disiarkan")
	}
}

func TestListBroadcasts_ReturnsWhatTheStoreFiltered(t *testing.T) {
	created := time.Unix(1_781_913_558, 0).UTC()
	repo := &fakeRepo{broadcasts: []store.Broadcast{
		{ID: "b-2", Title: "Regional", Body: "isi", RegionCode: "ID-jawa-barat", CreatedAt: created},
		{ID: "b-1", Title: "Nasional", Body: "isi", CreatedAt: created},
	}}
	h := newTestServer(repo, NewMemoryRateLimiter())

	rec := do(h, authedRequest(http.MethodGet, "/api/v1/broadcasts", "", testSecret, "user-1"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var resp listBroadcastsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Broadcasts) != 2 {
		t.Fatalf("siaran = %+v", resp.Broadcasts)
	}
	// Wilayah diambil dari profil pemanggil, bukan dari query string.
	if repo.broadcastUser != "user-1" {
		t.Fatalf("user = %q", repo.broadcastUser)
	}
	if repo.broadcastLimit != store.DefaultBroadcastLimit {
		t.Fatalf("limit default = %d", repo.broadcastLimit)
	}
	if resp.Broadcasts[0].RegionCode == nil || *resp.Broadcasts[0].RegionCode != "ID-jawa-barat" {
		t.Fatalf("region regional hilang: %+v", resp.Broadcasts[0])
	}
	if resp.Broadcasts[1].RegionCode != nil {
		t.Fatal("siaran nasional harus punya region_code null")
	}
}

func TestListBroadcasts_LimitIsBounded(t *testing.T) {
	for _, raw := range []string{"0", "-1", "abc", "51"} {
		t.Run(raw, func(t *testing.T) {
			h := newTestServer(&fakeRepo{}, NewMemoryRateLimiter())
			rec := do(h, authedRequest(
				http.MethodGet, "/api/v1/broadcasts?limit="+raw, "", testSecret, "user-1"))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, mau 400", rec.Code)
			}
		})
	}
}

// Daftar Pembaruan butuh auth: wilayah pembacanya diambil dari profilnya sendiri,
// jadi tanpa identitas tidak ada pertanyaan yang bisa dijawab.
func TestListBroadcasts_RequiresAuth(t *testing.T) {
	h := newTestServer(&fakeRepo{}, NewMemoryRateLimiter())

	rec := do(h, httptest.NewRequest(http.MethodGet, "/api/v1/broadcasts", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, mau 401", rec.Code)
	}
}
