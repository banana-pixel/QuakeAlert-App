package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/banana-pixel/quakealert/server/internal/store"
)

// adminNodeRequest membuat request admin untuk rute verifikasi node.
func adminNodeRequest(method, target, body, key string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if key != "" {
		req.Header.Set(AdminKeyHeader, key)
	}
	return req
}

func TestListPendingNodes_ReturnsWhatTheStoreHolds(t *testing.T) {
	repo := &fakeRepo{pendingNodes: []store.PendingNode{{
		StationID:    "NODE-163A149F",
		SensorModel:  "MPU 6050",
		LocationName: "Cimahi, Jawa Barat",
		Lat:          -6.87,
		Lon:          107.54,
		CreatedAt:    time.Date(2026, 8, 24, 9, 0, 0, 0, time.UTC),
	}}}
	h := newAdminServer(repo, nil, adminTestKey)

	rec := do(h, adminNodeRequest(http.MethodGet, "/api/v1/admin/nodes/pending", "", adminTestKey))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200: %s", rec.Code, rec.Body.String())
	}

	var resp listPendingNodesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Nodes) != 1 || resp.Nodes[0].StationID != "NODE-163A149F" {
		t.Fatalf("nodes = %+v", resp.Nodes)
	}
	if resp.Nodes[0].CreatedAt != "2026-08-24T09:00:00Z" {
		t.Fatalf("created_at = %q, mau RFC3339 UTC", resp.Nodes[0].CreatedAt)
	}
}

func TestListPendingNodes_EmptyIsOkNotError(t *testing.T) {
	h := newAdminServer(&fakeRepo{}, nil, adminTestKey)

	rec := do(h, adminNodeRequest(http.MethodGet, "/api/v1/admin/nodes/pending", "", adminTestKey))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"nodes":[]`) {
		t.Fatalf("mau nodes kosong, dapat %s", rec.Body.String())
	}
}

func TestVerifyNode_DefaultsToVerified(t *testing.T) {
	repo := &fakeRepo{}
	h := newAdminServer(repo, nil, adminTestKey)

	rec := do(h, adminNodeRequest(http.MethodPost,
		"/api/v1/admin/nodes/NODE-163A149F/verify", "", adminTestKey))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200: %s", rec.Code, rec.Body.String())
	}
	if repo.verifiedID != "NODE-163A149F" || !repo.verifiedTo {
		t.Fatalf("store dipanggil id=%q verified=%v", repo.verifiedID, repo.verifiedTo)
	}
}

func TestVerifyNode_ExplicitRevoke(t *testing.T) {
	repo := &fakeRepo{}
	h := newAdminServer(repo, nil, adminTestKey)

	rec := do(h, adminNodeRequest(http.MethodPost,
		"/api/v1/admin/nodes/NODE-163A149F/verify", `{"verified":false}`, adminTestKey))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200: %s", rec.Code, rec.Body.String())
	}
	if repo.verifiedTo {
		t.Fatal("verified=false harus diteruskan ke store (penarikan kepercayaan)")
	}
}

func TestVerifyNode_RejectsMalformedAndUnknownIDs(t *testing.T) {
	for name, target := range map[string]string{
		"bukan pola":   "/api/v1/admin/nodes/hello/verify",
		"hex kecil":    "/api/v1/admin/nodes/node-163a149f/verify",
		"id tidak ada": "/api/v1/admin/nodes/NODE-00000000/verify",
	} {
		t.Run(name, func(t *testing.T) {
			repo := &fakeRepo{verifyMissing: target == "/api/v1/admin/nodes/NODE-00000000/verify"}
			h := newAdminServer(repo, nil, adminTestKey)

			rec := do(h, adminNodeRequest(http.MethodPost, target, "", adminTestKey))
			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, mau 404: %s", rec.Code, rec.Body.String())
			}
			if repo.verifiedID != "" && name != "id tidak ada" {
				t.Fatal("station_id salah bentuk tidak boleh menyentuh store")
			}
		})
	}
}

// Gerbang kunci operator berlaku sama seperti rute admin lain: kunci yang
// salah tidak boleh sampai mengubah kepercayaan node.
func TestVerifyNode_WrongKeyNeverTouchesStore(t *testing.T) {
	repo := &fakeRepo{}
	h := newAdminServer(repo, nil, adminTestKey)

	rec := do(h, adminNodeRequest(http.MethodPost,
		"/api/v1/admin/nodes/NODE-163A149F/verify", "", "kunci-salah-yang-sama-panjangnya!"))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, mau 401", rec.Code)
	}
	if repo.verifiedID != "" {
		t.Fatal("kunci salah tidak boleh mengubah status node")
	}
}

// --- §7.5: pencabutan verifikasi mencabut bukti dari event terbuka ---

// recInvalidator mencatat setiap pencabutan bukti yang diminta handler.
type recInvalidator struct {
	nodes   []string
	reasons []string
}

func (r *recInvalidator) InvalidateContributor(_ context.Context, nodeID, reason string) {
	r.nodes = append(r.nodes, nodeID)
	r.reasons = append(r.reasons, reason)
}

// newAdminServerWithInvalidator sama dengan newAdminServer, plus jalur pencabutan
// bukti terpasang. Helper terpisah supaya uji rute admin lain tetap membuktikan
// bahwa jalur itu OPSIONAL.
func newAdminServerWithInvalidator(repo Repo, inv EvidenceInvalidator) http.Handler {
	srv := NewServer(repo, fakeDecryptCipher{}, NewMemoryRateLimiter(),
		MQTTPublic{Broker: "b", Port: 8883, TLS: true},
		AuthConfig{JWTSecret: []byte(testSecret), TokenTTL: testTokenTTL},
		testLogger())
	srv.SetAdminAPIKey(adminTestKey)
	srv.SetEvidenceInvalidator(inv)
	return srv.Router(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusSwitchingProtocols)
	}, testLogger())
}

// Menarik verifikasi mencabut bukti node itu, dengan reason KOSONG: kosakata
// alasan transisi milik paket event, dan paket api tidak menyebutnya (kosong
// berarti EVIDENCE_INVALIDATED).
func TestVerifyNode_RevokeInvalidatesEvidence(t *testing.T) {
	repo := &fakeRepo{}
	inv := &recInvalidator{}
	h := newAdminServerWithInvalidator(repo, inv)

	rec := do(h, adminNodeRequest(http.MethodPost,
		"/api/v1/admin/nodes/NODE-163A149F/verify", `{"verified":false}`, adminTestKey))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200: %s", rec.Code, rec.Body.String())
	}
	if len(inv.nodes) != 1 || inv.nodes[0] != "NODE-163A149F" {
		t.Fatalf("pencabutan bukti = %v, mau tepat satu untuk NODE-163A149F", inv.nodes)
	}
	if inv.reasons[0] != "" {
		t.Errorf("reason = %q, mau kosong: api tidak memiliki kosakata alasan transisi", inv.reasons[0])
	}
}

// Memverifikasi node TIDAK menyentuh event mana pun: bukti tidak pernah masuk
// secara retroaktif.
func TestVerifyNode_ApprovalDoesNotInvalidateAnything(t *testing.T) {
	inv := &recInvalidator{}
	h := newAdminServerWithInvalidator(&fakeRepo{}, inv)

	rec := do(h, adminNodeRequest(http.MethodPost,
		"/api/v1/admin/nodes/NODE-163A149F/verify", "", adminTestKey))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200: %s", rec.Code, rec.Body.String())
	}
	if len(inv.nodes) != 0 {
		t.Fatalf("pencabutan bukti = %v, mau tidak ada", inv.nodes)
	}
}

// Bila penulisan basis data gagal atau station_id tidak dikenal, tidak ada bukti
// yang dicabut: yang ditarik adalah kepercayaan yang benar-benar tercatat.
func TestVerifyNode_FailedWriteDoesNotInvalidateEvidence(t *testing.T) {
	for name, repo := range map[string]*fakeRepo{
		"store galat":  {verifyErr: errors.New("boom")},
		"id tidak ada": {verifyMissing: true},
	} {
		t.Run(name, func(t *testing.T) {
			inv := &recInvalidator{}
			h := newAdminServerWithInvalidator(repo, inv)

			rec := do(h, adminNodeRequest(http.MethodPost,
				"/api/v1/admin/nodes/NODE-163A149F/verify", `{"verified":false}`, adminTestKey))
			if rec.Code == http.StatusOK {
				t.Fatalf("status = %d, mau bukan 200", rec.Code)
			}
			if len(inv.nodes) != 0 {
				t.Fatalf("pencabutan bukti = %v, mau tidak ada", inv.nodes)
			}
		})
	}
}
