package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/banana-pixel/quakealert/server/internal/store"
)

// --- Helper server untuk revoke: butuh cipher yang bisa Decrypt ---

func newRevokeTestServer(repo Repo, cipher SecretEncryptor, limiter RateLimiter) http.Handler {
	srv := NewServer(repo, cipher, limiter,
		MQTTPublic{Broker: "b", Port: 8883, TLS: true},
		AuthConfig{JWTSecret: []byte(testSecret), TokenTTL: testTokenTTL},
		testLogger())
	return srv.Router(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusSwitchingProtocols)
	}, testLogger())
}

const (
	revokeStation = "NODE-163A149F"
	revokeSecret  = "secret-yang-ditampilkan-sekali"
)

// postRevoke mengirim POST /nodes/revoke terautentikasi dengan body JSON,
// mengembalikan (status, body).
func postRevoke(h http.Handler, body string) (int, string) {
	req := authedRequest(http.MethodPost, "/api/v1/nodes/revoke", body, testSecret, "user-1")
	rec := do(h, req)
	return rec.Code, rec.Body.String()
}

func pendingNodeRepo() *fakeRepo {
	return &fakeRepo{
		nodeSecret: &store.NodeSecret{
			StationID:   revokeStation,
			SecretEnc:   append([]byte("enc:"), revokeSecret...),
			SecretNonce: []byte("nonce--12byte"),
			Verified:    false,
		},
		deleteAffected: true,
	}
}

// --- Tests ---

// Jalur bahagia: secret cocok + node belum terverifikasi → baris dihapus.
func TestRevoke_DeletesUnverifiedNode(t *testing.T) {
	repo := pendingNodeRepo()
	h := newRevokeTestServer(repo, fakeDecryptCipher{}, NewMemoryRateLimiter())

	body := `{"station_id":"` + revokeStation + `","provisioning_secret":"` + revokeSecret + `"}`
	code, respBody := postRevoke(h, body)

	if code != http.StatusOK {
		t.Fatalf("status = %d, mau 200. body=%s", code, respBody)
	}
	var resp revokeResponse
	if err := json.Unmarshal([]byte(respBody), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Deleted || resp.StationID != revokeStation {
		t.Fatalf("respons salah: %+v", resp)
	}
	if repo.deletedID != revokeStation {
		t.Fatalf("DeleteUnverifiedNode dipanggil dengan %q, mau %q", repo.deletedID, revokeStation)
	}
}

// IDEMPOTEN: node yang tidak ada (sudah dibatalkan / tak pernah provision)
// menjawab 200 {"deleted":false} — retry jaringan dan double-tap bukan error.
func TestRevoke_UnknownNodeIsIdempotentSuccess(t *testing.T) {
	repo := &fakeRepo{} // nodeSecret nil → GetNodeSecret = ErrNodeNotFound
	h := newRevokeTestServer(repo, fakeDecryptCipher{}, NewMemoryRateLimiter())

	body := `{"station_id":"` + revokeStation + `","provisioning_secret":"apa-saja"}`
	code, respBody := postRevoke(h, body)

	if code != http.StatusOK {
		t.Fatalf("status = %d, mau 200 idempoten. body=%s", code, respBody)
	}
	var resp revokeResponse
	if err := json.Unmarshal([]byte(respBody), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Deleted {
		t.Fatal("deleted harus false untuk node yang tidak ada")
	}
	if repo.deletedID != "" {
		t.Fatal("DELETE tidak boleh tersentuh untuk node yang tidak ada")
	}
}

// SECRET SALAH → 403. Node TIDAK dihapus; pesan tidak membedakan
// salah-id vs salah-secret (tidak membocorkan keberadaan).
func TestRevoke_WrongSecretForbidden(t *testing.T) {
	repo := pendingNodeRepo()
	h := newRevokeTestServer(repo, fakeDecryptCipher{}, NewMemoryRateLimiter())

	body := `{"station_id":"` + revokeStation + `","provisioning_secret":"kunci-salah"}`
	code, respBody := postRevoke(h, body)

	if code != http.StatusForbidden {
		t.Fatalf("status = %d, mau 403. body=%s", code, respBody)
	}
	if repo.deletedID != "" {
		t.Fatal("node tidak boleh terhapus dengan secret yang salah")
	}
}

// NODE TERVERIFIKASI → 409 meski secret benar. Invariant produksi: pembatalan
// pengguna mustahil menyentuh node yang sudah dipercaya operator.
func TestRevoke_VerifiedNodeConflict(t *testing.T) {
	repo := pendingNodeRepo()
	repo.nodeSecret.Verified = true
	h := newRevokeTestServer(repo, fakeDecryptCipher{}, NewMemoryRateLimiter())

	body := `{"station_id":"` + revokeStation + `","provisioning_secret":"` + revokeSecret + `"}`
	code, _ := postRevoke(h, body)

	if code != http.StatusConflict {
		t.Fatalf("status = %d, mau 409 untuk node terverifikasi", code)
	}
	if repo.deletedID != "" {
		t.Fatal("node terverifikasi TIDAK BOLEH dihapus lewat revoke")
	}
}

// BALAPAN verifikasi: secret benar saat dibaca, tetapi operator memverifikasi
// node sebelum DELETE berjalan → predikat verified=FALSE menolak (0 baris).
// Handler harus memetakan itu ke 409, bukan 200.
func TestRevoke_LosesRaceWithVerify(t *testing.T) {
	repo := pendingNodeRepo()
	repo.deleteAffected = false // UPDATE verify menang lock baris dulu
	h := newRevokeTestServer(repo, fakeDecryptCipher{}, NewMemoryRateLimiter())

	body := `{"station_id":"` + revokeStation + `","provisioning_secret":"` + revokeSecret + `"}`
	code, _ := postRevoke(h, body)

	if code != http.StatusConflict {
		t.Fatalf("status = %d, mau 409 saat kalah balapan verifikasi", code)
	}
}

// station_id salah bentuk ditolak 404 SEBELUM menyentuh basis data —
// kontrak yang sama dengan endpoint verifikasi admin.
func TestRevoke_MalformedStationID(t *testing.T) {
	for name, sid := range map[string]string{
		"bukan pola":   "hello",
		"hex kecil":    "node-163a149f",
		"id tidak ada": "",
	} {
		repo := pendingNodeRepo()
		h := newRevokeTestServer(repo, fakeDecryptCipher{}, NewMemoryRateLimiter())

		body := `{"station_id":"` + sid + `","provisioning_secret":"x"}`
		code, _ := postRevoke(h, body)

		if code != http.StatusNotFound {
			t.Fatalf("%s: status = %d, mau 404", name, code)
		}
		if repo.nodeSecret != nil && repo.deletedID != "" {
			t.Fatalf("%s: DB tidak boleh disentuh untuk ID salah bentuk", name)
		}
	}
}

// Tanpa JWT → 401 oleh middleware, seperti seluruh rute auth wajib lainnya.
func TestRevoke_Unauthenticated(t *testing.T) {
	repo := pendingNodeRepo()
	h := newRevokeTestServer(repo, fakeDecryptCipher{}, NewMemoryRateLimiter())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/revoke",
		strings.NewReader(`{"station_id":"`+revokeStation+`","provisioning_secret":"x"}`))
	code := do(h, req).Code

	if code == http.StatusOK {
		t.Fatal("revoke tanpa autentikasi tidak boleh berhasil")
	}
	if repo.deletedID != "" {
		t.Fatal("node tidak boleh tersentuh tanpa autentikasi")
	}
}

// Gagal dekripsi (ciphertext rusak / key_version mismatch) adalah kerusakan
// internal → 500, BUKAN jatuh ke perbandingan gagal-terbuka yang menjadi 403:
// sebuah baris yang rusak tidak boleh terlihat sebagai "secret salah".
func TestRevoke_DecryptFailureIsInternal(t *testing.T) {
	repo := pendingNodeRepo()
	repo.nodeSecret.SecretEnc = []byte("bukan-ciphertext")
	h := newRevokeTestServer(repo, fakeDecryptCipher{}, NewMemoryRateLimiter())

	body := `{"station_id":"` + revokeStation + `","provisioning_secret":"` + revokeSecret + `"}`
	code, _ := postRevoke(h, body)

	if code != http.StatusInternalServerError {
		t.Fatalf("status = %d, mau 500 saat dekripsi gagal", code)
	}
	if repo.deletedID != "" {
		t.Fatal("node tidak boleh terhapus saat dekripsi gagal")
	}
}

// Body rusak → 400 dari decodeBody, tanpa menyentuh repo.
func TestRevoke_MalformedBody(t *testing.T) {
	repo := pendingNodeRepo()
	h := newRevokeTestServer(repo, fakeDecryptCipher{}, NewMemoryRateLimiter())

	code, _ := postRevoke(h, `{station_id bukan json}`)

	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, mau 400", code)
	}
	if repo.deletedID != "" {
		t.Fatal("DB tidak boleh disentuh untuk body rusak")
	}
}
