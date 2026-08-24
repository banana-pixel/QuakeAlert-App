package ingest

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"testing"
	"time"

	"github.com/banana-pixel/quakealert/server/internal/store"
)

// fakeNodeSource mencatat apakah anti-replay DB pernah disentuh — gerbang
// verified harus berdiri SEBELUM kripto & replay, jadi tolakannya terlihat
// dari tidak pernah ada panggilan lanjutan.
type fakeNodeSource struct {
	node        *store.NodeSecret
	getErr      error
	lastSeenHit bool
}

func (f *fakeNodeSource) GetNodeSecret(_ context.Context, _ string) (*store.NodeSecret, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.node, nil
}

func (f *fakeNodeSource) UpdateLastSeen(_ context.Context, _ string, _ int64) (bool, error) {
	f.lastSeenHit = true
	return true, nil
}

type fakeDecryptor struct{ secret []byte }

func (f *fakeDecryptor) Decrypt(_, _ []byte) ([]byte, error) { return f.secret, nil }

// newTestVerifier memasang verifier dengan waktu tetap agar payload uji bisa
// membawa ts yang selalu lolos cek clock skew.
func newTestVerifier(t *testing.T, src *fakeNodeSource) *Verifier {
	t.Helper()
	now := time.UnixMilli(1_700_000_005_000)
	v := NewVerifier(nil, nil, slog.Default())
	v.store = src
	v.cipher = &fakeDecryptor{secret: []byte("test-key")}
	v.now = func() time.Time { return now }
	return v
}

func validTrigger(t *testing.T, secret []byte) []byte {
	t.Helper()
	canonical := CanonicalString("NODE-0A1B2C3D", 413.13, 8000, 1_700_000_005_000)
	raw := `{"node_id":"NODE-0A1B2C3D","pga":413.13,"dur_ms":8000,"ts":1700000005000,"signature":"` +
		ComputeHMAC(secret, canonical) + `"}`
	return []byte(raw)
}

func TestVerifyTrigger_VerifiedNodePasses(t *testing.T) {
	src := &fakeNodeSource{node: &store.NodeSecret{
		StationID: "NODE-0A1B2C3D", IsActive: true, Verified: true,
	}}
	v := newTestVerifier(t, src)

	if _, err := v.Verify(context.Background(), validTrigger(t, []byte("test-key"))); err != nil {
		t.Fatalf("trigger dari node terverifikasi ditolak: %v", err)
	}
	if !src.lastSeenHit {
		t.Fatal("anti-replay DB seharusnya dijalankan untuk trigger yang sah")
	}
}

func TestVerifyTrigger_UnverifiedRejected(t *testing.T) {
	src := &fakeNodeSource{node: &store.NodeSecret{
		StationID: "NODE-0A1B2C3D", IsActive: true, Verified: false,
	}}
	v := newTestVerifier(t, src)

	_, err := v.Verify(context.Background(), validTrigger(t, []byte("test-key")))
	if !errors.Is(err, ErrNodeUnverified) {
		t.Fatalf("want ErrNodeUnverified, got %v", err)
	}
	// Gerbang konsensus berarti tidak ada jejak di jalur lanjutan:
	if src.lastSeenHit {
		t.Fatal("trigger node belum terverifikasi tidak boleh sampai ke anti-replay DB")
	}
}

func TestVerifyTrigger_UnverifiedBeatsBadSignature(t *testing.T) {
	// Urutan tolak-murah-dulu: HMAC tidak valid TIDAK boleh menyamarkan
	// alasan penolakan yang sesungguhnya (belum terverifikasi).
	src := &fakeNodeSource{node: &store.NodeSecret{
		StationID: "NODE-0A1B2C3D", IsActive: true, Verified: false,
	}}
	v := newTestVerifier(t, src)

	bad := []byte(`{"node_id":"NODE-0A1B2C3D","pga":10,"dur_ms":1000,"ts":1700000005000,"signature":"` +
		"0000000000000000000000000000000000000000000000000000000000000000" + `"}`)
	_, err := v.Verify(context.Background(), bad)
	if !errors.Is(err, ErrNodeUnverified) {
		t.Fatalf("gerbang verified harus mendahului HMAC, got %v", err)
	}
}

func TestVerifyTrigger_InactiveRejected(t *testing.T) {
	src := &fakeNodeSource{node: &store.NodeSecret{
		StationID: "NODE-0A1B2C3D", IsActive: false, Verified: true,
	}}
	v := newTestVerifier(t, src)

	_, err := v.Verify(context.Background(), validTrigger(t, []byte("test-key")))
	if !errors.Is(err, ErrNodeInactive) {
		t.Fatalf("want ErrNodeInactive, got %v", err)
	}
}

func TestVerifyTrigger_ReplayRejected(t *testing.T) {
	src := &fakeNodeSource{node: &store.NodeSecret{
		StationID: "NODE-0A1B2C3D", IsActive: true, Verified: true,
		LastSeenTS: 1_700_000_006_000, // lebih baru daripada ts trigger
	}}
	v := newTestVerifier(t, src)

	_, err := v.Verify(context.Background(), validTrigger(t, []byte("test-key")))
	if !errors.Is(err, ErrReplay) {
		t.Fatalf("want ErrReplay, got %v", err)
	}
}

// Jendela freshness trigger asimetris: usia ke belakang boleh sampai
// MaxTriggerAge (5 menit, untuk retry publish ulang), ke depan tetap ketat
// MaxClockSkew (30 detik). Batas heartbeat tidak tersentuh perubahan ini.
func TestVerifyTrigger_TriggerFreshnessWindow(t *testing.T) {
	base := time.UnixMilli(1_700_010_000_000) // cukup di atas batas bawah kontrak ts agar -5m tetap valid
	cases := []struct {
		name    string
		offset  time.Duration // ts relatif thd waktu server
		wantErr error
	}{
		{"tepat 5 menit (batas retry firmware)", -5 * time.Minute, nil},
		{"5 menit + 1 ms ditolak", -(5*time.Minute + time.Millisecond), ErrClockSkew},
		{"masa depan 30s diterima", 30 * time.Second, nil},
		{"masa depan 30s+1ms ditolak", 30*time.Second + time.Millisecond, ErrClockSkew},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts := base.Add(tc.offset).UnixMilli()
			src := &fakeNodeSource{node: &store.NodeSecret{
				StationID: "NODE-0A1B2C3D", IsActive: true, Verified: true,
			}}
			v := newTestVerifier(t, src)
			v.now = func() time.Time { return base }

			canonical := CanonicalString("NODE-0A1B2C3D", 413.13, 8000, ts)
			raw := []byte(`{"node_id":"NODE-0A1B2C3D","pga":413.13,"dur_ms":8000,"ts":` +
				fmtInt64(ts) + `,"signature":"` + ComputeHMAC([]byte("test-key"), canonical) + `"}`)

			_, err := v.Verify(context.Background(), raw)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("want %v, got %v", tc.wantErr, err)
			}
		})
	}
}

func fmtInt64(v int64) string {
	return strconv.FormatInt(v, 10)
}
