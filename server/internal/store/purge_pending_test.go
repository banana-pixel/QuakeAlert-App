package store

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// --- Integration tests untuk PurgeAbandonedPendingNodes ---
//
// Test ini butuh Postgres NYATA (predikat penghapusan adalah SQL, dan justru
// di situlah keselamatannya). Aktif dengan env:
//
//	TEST_DATABASE_URL=postgres://... go test ./internal/store -run TestPurgeAbandoned
//
// Tanpa env itu, seluruh test di file ini skip — pola yang sama dengan CI tanpa
// layanan database: unit test murni tetap berjalan.

// testDBURL membaca TEST_DATABASE_URL; kosong berarti lingkungan tidak menyediakan
// database integrasi.
func testDBURL(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL tidak diset — lewati integrasi Postgres")
	}
	return dsn
}

// newTestStore membuat Store terhubung ke database integrasi.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := New(context.Background(), testDBURL(t))
	if err != nil {
		t.Fatalf("koneksi database uji: %v", err)
	}
	t.Cleanup(st.Close)
	return st
}

// seedNode menyisipkan baris iot_nodes langsung, mengendalikan verified,
// created_at, dan last_heartbeat — ketiga kolom yang menentukan kelayakan reap.
// Secret dummy: kolomnya NOT NULL tetapi tidak dibaca oleh purge.
func (s *Store) seedNode(t *testing.T, stationID string, verified bool, age time.Duration, heartbeatAt *time.Time) {
	t.Helper()
	_, err := s.pool.Exec(context.Background(), `
		INSERT INTO iot_nodes (
			station_id, sensor_model, location_name, location,
			secret_key_enc, secret_key_nonce, created_at, last_heartbeat, verified
		) VALUES (
			$1, 'MPU 6050', 'Cimahi',
			ST_SetSRID(ST_MakePoint($2, $3), 4326)::geography,
			'\x00'::bytea, '\x00'::bytea,
			NOW() - make_interval(secs => $4::int),
			COALESCE($5, NOW() - make_interval(secs => $4::int)),
			$6
		)`,
		stationID, 107.54, -6.87, int(age.Seconds()), heartbeatAt, verified,
	)
	if err != nil {
		t.Fatalf("seed node %s: %v", stationID, err)
	}
	t.Cleanup(func() {
		_, _ = s.pool.Exec(context.Background(),
			`DELETE FROM iot_nodes WHERE station_id = $1`, stationID)
	})
}

func nodeExists(t *testing.T, st *Store, stationID string) bool {
	t.Helper()
	var exists bool
	err := st.pool.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM iot_nodes WHERE station_id = $1)`, stationID).
		Scan(&exists)
	if err != nil {
		t.Fatalf("cek keberadaan %s: %v", stationID, err)
	}
	return exists
}

const (
	reapOldPending   = "NODE-REAP0001" // usia 15 hari, tak pernah heartbeat → DIHAPUS
	keepFreshPending = "NODE-KEEP0001" // usia 1 hari, pending → DIPERTAHANKAN
	keepHeartbeated  = "NODE-KEEP0002" // usia 15 hari tapi pernah heartbeat → DIPERTAHANKAN
	keepVerifiedOld  = "NODE-KEEP0003" // usia 15 hari, terverifikasi → DIPERTAHANKAN
)

var noHeartbeat *time.Time = nil

// Skenario 1 & 2 & 3 & 4 dalam satu basis data, agar predikat dibuktikan
// memilah — bukan menghapus semua yang lama.
func TestPurgeAbandonedPendingNodes_ReapsOnlyEligible(t *testing.T) {
	st := newTestStore(t)

	// 1. Node sasaran: pending, tua, tidak pernah berdenyut.
	st.seedNode(t, reapOldPending, false, 15*24*time.Hour, noHeartbeat)
	// 2. Pending tapi masih muda.
	st.seedNode(t, keepFreshPending, false, 24*time.Hour, noHeartbeat)
	// 3. Tua, pending, TETAPI pernah heartbeat (instalasi sah menunggu verifikasi).
	hb := time.Now().Add(-2 * time.Minute)
	st.seedNode(t, keepHeartbeated, false, 15*24*time.Hour, &hb)
	// 4. Tua, tidak pernah heartbeat, tapi sudah TERVERIFIKASI operator.
	st.seedNode(t, keepVerifiedOld, true, 15*24*time.Hour, noHeartbeat)

	deleted, err := st.PurgeAbandonedPendingNodes(context.Background(), 14*24*time.Hour)
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("rows deleted = %d, mau tepat 1", deleted)
	}

	if nodeExists(t, st, reapOldPending) {
		t.Error("node pending tua tanpa heartbeat harus dihapus")
	}
	if !nodeExists(t, st, keepFreshPending) {
		t.Error("pending muda harus dipertahankan")
	}
	if !nodeExists(t, st, keepHeartbeated) {
		t.Error("pending tua yang pernah heartbeat harus dipertahankan")
	}
	if !nodeExists(t, st, keepVerifiedOld) {
		t.Error("node terverifikasi tidak boleh tersentuh berapa pun umurnya")
	}
}

// Skenario 6: banyak node terlantar sekaligus — sweep menghapus SEMUA yang
// memenuhi predikat dalam satu operasi set-at-a-time.
func TestPurgeAbandonedPendingNodes_MultipleEligible(t *testing.T) {
	st := newTestStore(t)
	const n = 5
	for i := 0; i < n; i++ {
		st.seedNode(t, fmt.Sprintf("NODE-MULTI%03X", i), false, 20*24*time.Hour, noHeartbeat)
	}

	deleted, err := st.PurgeAbandonedPendingNodes(context.Background(), 14*24*time.Hour)
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if deleted < n {
		t.Fatalf("rows deleted = %d, minimal %d (milik tes ini)", deleted, n)
	}
	for i := 0; i < n; i++ {
		if nodeExists(t, st, fmt.Sprintf("NODE-MULTI%03X", i)) {
			t.Errorf("NODE-MULTI%03X harus terhapus", i)
		}
	}
}

// Skenario 5: sweep berulang pada basis data yang sudah bersih menghapus 0 baris
// — idempoten, aman dijalankan setiap hari maupun dua kali berturut-turut.
func TestPurgeAbandonedPendingNodes_RepeatSweepIsIdempotent(t *testing.T) {
	st := newTestStore(t)
	st.seedNode(t, "NODE-IDEM001", false, 15*24*time.Hour, noHeartbeat)

	first, err := st.PurgeAbandonedPendingNodes(context.Background(), 14*24*time.Hour)
	if err != nil {
		t.Fatalf("sweep pertama: %v", err)
	}
	if first == 0 {
		t.Fatal("sweep pertama harus menghapus node uji")
	}

	second, err := st.PurgeAbandonedPendingNodes(context.Background(), 14*24*time.Hour)
	if err != nil {
		t.Fatalf("sweep kedua: %v", err)
	}
	if second != 0 {
		t.Fatalf("sweep kedua harus menghapus 0 baris, dapat %d", second)
	}
}

// Jaga-jaga tipe: pastikan pool pgxpool tersedia via Store (dipakai helper di atas).
var _ *pgxpool.Pool // kompilasi-only referensi agar import tidak menggantung tanpa guna
