package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// --- Integrasi Postgres untuk observation ledger (migrasi 000006) ---
//
// Butuh Postgres NYATA dengan PostGIS: yang diuji di sini adalah perilaku
// KOLOMNYA — round-trip GEOGRAPHY dan presisi NUMERIC(8,4) — dan tidak satu pun
// dari keduanya dapat diuji dengan fake. Aktif dengan:
//
//	TEST_DATABASE_URL=postgres://... go test ./internal/store -run TestObservation
//
// Tanpa env itu, seluruh test di file ini skip.

// §20.4 — round-trip.
//
// NUMERIC(8,4) diuji dengan nilai yang PERSIS seperti yang ditandatangani
// firmware (4 desimal). Regresi yang dicegah: pergeseran float antara string
// yang ditandatangani dan angka yang tersimpan, yang akan membuat baris ledger
// tidak lagi dapat memverifikasi ulang tanda tangannya sendiri.
func TestObservationRoundTrip(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	lat, lon := -6.9034443, 107.6431173
	upper := int64(1_699_999_997_000)
	o := &Observation{
		NodeID:               "NODE-0A1B2C3D",
		SourceClass:          "FIXED_ESP32",
		Phase:                "FINAL",
		PGAGal:               413.1300,
		DurMs:                8000,
		PublishTS:            1_700_000_005_000,
		ReceivedTS:           1_700_000_005_012,
		OnsetTSUpperBound:    &upper,
		OnsetTSSource:        "PUBLISH_BOUND",
		Lat:                  &lat,
		Lon:                  &lon,
		Signature:            "cb2cc5d59a7ce4922e8325d9f4cb8de816a84da968c5429d0d9fcab6d9f69e7b",
		VerifyResult:         "OK",
		SuppressedRejections: 0,
	}
	if err := st.InsertObservation(ctx, o); err != nil {
		t.Fatalf("InsertObservation: %v", err)
	}
	t.Cleanup(func() {
		_, _ = st.pool.Exec(ctx, `DELETE FROM sensor_observations WHERE node_id = $1`, o.NodeID)
	})

	var (
		gotPGA          string
		gotLat, gotLon  float64
		gotUpper        *int64
		gotProto        *int16
		gotObsSeq       *int64
		gotOnset        *int64
		gotSuppressed   int
		gotObservationI int64
	)
	err := st.pool.QueryRow(ctx, `
		SELECT observation_id,
		       pga_gal::text,
		       ST_Y(node_location::geometry),
		       ST_X(node_location::geometry),
		       onset_ts_upper_bound, proto_ver, obs_seq, onset_ts,
		       suppressed_rejections
		FROM sensor_observations
		WHERE node_id = $1
		ORDER BY observation_id DESC
		LIMIT 1`, o.NodeID).Scan(
		&gotObservationI, &gotPGA, &gotLat, &gotLon,
		&gotUpper, &gotProto, &gotObsSeq, &gotOnset, &gotSuppressed)
	if err != nil {
		t.Fatalf("baca kembali observasi: %v", err)
	}

	// NUMERIC(8,4): teks tersimpan harus membawa empat desimal yang sama dengan
	// string kanonik yang ditandatangani.
	if gotPGA != "413.1300" {
		t.Errorf("pga_gal = %q, want \"413.1300\" (presisi NUMERIC(8,4) hilang)", gotPGA)
	}
	// GEOGRAPHY round-trip: ST_MakePoint menerima (lon, lat) — urutan yang salah
	// akan memindahkan sensor Bandung ke Samudra Hindia tanpa error.
	if !closeEnough(gotLat, lat) || !closeEnough(gotLon, lon) {
		t.Errorf("node_location = (%f, %f), want (%f, %f)", gotLat, gotLon, lat, lon)
	}
	if gotUpper == nil || *gotUpper != upper {
		t.Errorf("onset_ts_upper_bound = %v, want %d", gotUpper, upper)
	}
	if gotProto != nil || gotObsSeq != nil || gotOnset != nil {
		t.Error("kolom Fase 2 harus NULL pada baris v1")
	}
	if gotSuppressed != 0 {
		t.Errorf("suppressed_rejections = %d, want 0", gotSuppressed)
	}
	// observation_id BIGSERIAL ADALAH urutan kedatangan; tidak ada counter lain.
	if gotObservationI <= 0 {
		t.Errorf("observation_id = %d, want > 0", gotObservationI)
	}
}

// Observasi tanpa lokasi (A16) harus tersimpan dengan node_location NULL, bukan
// gagal: ST_MakePoint akan menolak NULL, jadi jalur CASE-nya diuji di sini.
func TestObservationWithoutLocationRoundTrip(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	o := &Observation{
		NodeID:        "NODE-DEADBEEF",
		SourceClass:   "FIXED_ESP32",
		Phase:         "FINAL",
		PGAGal:        16.6,
		DurMs:         0,
		PublishTS:     1_700_000_005_000,
		ReceivedTS:    1_700_000_005_100,
		OnsetTSSource: "PUBLISH_BOUND",
		VerifyResult:  "OK",
	}
	if err := st.InsertObservation(ctx, o); err != nil {
		t.Fatalf("InsertObservation tanpa lokasi: %v", err)
	}
	t.Cleanup(func() {
		_, _ = st.pool.Exec(ctx, `DELETE FROM sensor_observations WHERE node_id = $1`, o.NodeID)
	})

	var isNull bool
	var sig *string
	if err := st.pool.QueryRow(ctx, `
		SELECT node_location IS NULL, signature
		FROM sensor_observations WHERE node_id = $1
		ORDER BY observation_id DESC LIMIT 1`, o.NodeID).Scan(&isNull, &sig); err != nil {
		t.Fatalf("baca kembali: %v", err)
	}
	if !isNull {
		t.Error("node_location harus NULL bila koordinat tidak diketahui")
	}
	if sig != nil {
		t.Errorf("signature kosong harus tersimpan NULL, got %q", *sig)
	}
}

// Baris emisi: keputusan tanpa event_id (ADVISORY) dan dengan centroid.
func TestAlertEmissionRoundTrip(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	lat, lon := -6.9, 107.6
	mmi := "VI"
	pga := 120.5
	e := &AlertEmission{
		AlertType:   "EARTHQUAKE_ADVISORY",
		Status:      "ADVISORY",
		MMI:         &mmi,
		PGAGal:      &pga,
		NodeCount:   1,
		CentroidLat: &lat,
		CentroidLon: &lon,
		IsSevere:    false,
		Audience:    "NONE",
		DecidedAt:   1_700_000_005_000,
		AlgoVer:     "phase1-1.0",
	}
	if err := st.InsertAlertEmission(ctx, e); err != nil {
		t.Fatalf("InsertAlertEmission: %v", err)
	}
	t.Cleanup(func() {
		_, _ = st.pool.Exec(ctx, `DELETE FROM alert_emissions WHERE decided_at = $1`, e.DecidedAt)
	})

	var (
		eventIsNull    bool
		gotPGA         string
		gotLat, gotLon float64
	)
	if err := st.pool.QueryRow(ctx, `
		SELECT event_id IS NULL, pga_gal::text,
		       ST_Y(centroid::geometry), ST_X(centroid::geometry)
		FROM alert_emissions WHERE decided_at = $1
		ORDER BY emission_id DESC LIMIT 1`, e.DecidedAt).Scan(
		&eventIsNull, &gotPGA, &gotLat, &gotLon); err != nil {
		t.Fatalf("baca kembali emisi: %v", err)
	}
	if !eventIsNull {
		t.Error("ADVISORY tidak punya identitas event; event_id harus NULL")
	}
	if gotPGA != "120.5000" {
		t.Errorf("pga_gal = %q, want \"120.5000\"", gotPGA)
	}
	if !closeEnough(gotLat, lat) || !closeEnough(gotLon, lon) {
		t.Errorf("centroid = (%f, %f), want (%f, %f)", gotLat, gotLon, lat, lon)
	}
}

// §20.6 — idempotensi migrasi.
//
// 000006 dijalankan DUA KALI harus menjadi no-op. Dijalankan terhadap database
// yang skemanya sudah dimigrasi: bila salah satu pernyataan kehilangan IF NOT
// EXISTS, penerapan kedua akan gagal di sini alih-alih di produksi.
func TestMigration000006IsIdempotent(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	up, err := os.ReadFile(filepath.Join("..", "..", "..",
		"contracts", "db", "migrations", "000006_add_observation_ledger.up.sql"))
	if err != nil {
		t.Fatalf("baca migrasi: %v", err)
	}

	for i := 1; i <= 2; i++ {
		if _, err := st.pool.Exec(ctx, string(up)); err != nil {
			t.Fatalf("penerapan migrasi ke-%d gagal (idempotensi rusak): %v", i, err)
		}
	}

	// Kedua tabel tetap ada dan tetap satu.
	for _, table := range []string{"sensor_observations", "alert_emissions"} {
		var n int
		if err := st.pool.QueryRow(ctx,
			`SELECT count(*) FROM information_schema.tables
			 WHERE table_schema = 'public' AND table_name = $1`, table).Scan(&n); err != nil {
			t.Fatalf("periksa tabel %s: %v", table, err)
		}
		if n != 1 {
			t.Errorf("tabel %s ditemukan %d kali, want 1", table, n)
		}
	}
}

// closeEnough membandingkan koordinat dengan toleransi presisi ganda PostGIS.
func closeEnough(a, b float64) bool {
	d := a - b
	return d < 1e-7 && d > -1e-7
}

// §20.6 — rollback lengkap.
//
// down setelah up harus meninggalkan skema seperti sebelumnya. Rollback ini
// lengkap justru karena 000006 tidak menambahkan kolom ke tabel mana pun yang
// sudah ada: menghapus dua tabel baru sudah mengembalikan seluruh perubahan.
// Test menerapkan up kembali di akhir agar database uji tetap termigrasi bagi
// test lain.
func TestMigration000006DownRestoresSchema(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	dir := filepath.Join("..", "..", "..", "contracts", "db", "migrations")
	down, err := os.ReadFile(filepath.Join(dir, "000006_add_observation_ledger.down.sql"))
	if err != nil {
		t.Fatalf("baca migrasi down: %v", err)
	}
	up, err := os.ReadFile(filepath.Join(dir, "000006_add_observation_ledger.up.sql"))
	if err != nil {
		t.Fatalf("baca migrasi up: %v", err)
	}

	// Apa pun hasilnya, database uji harus keluar dalam keadaan termigrasi.
	t.Cleanup(func() {
		if _, err := st.pool.Exec(ctx, string(up)); err != nil {
			t.Fatalf("gagal menerapkan ulang migrasi setelah rollback: %v", err)
		}
	})

	if _, err := st.pool.Exec(ctx, string(down)); err != nil {
		t.Fatalf("rollback gagal: %v", err)
	}

	for _, table := range []string{"sensor_observations", "alert_emissions"} {
		var n int
		if err := st.pool.QueryRow(ctx,
			`SELECT count(*) FROM information_schema.tables
			 WHERE table_schema = 'public' AND table_name = $1`, table).Scan(&n); err != nil {
			t.Fatalf("periksa tabel %s: %v", table, err)
		}
		if n != 0 {
			t.Errorf("tabel %s masih ada setelah rollback", table)
		}
	}

	// Tabel yang sudah ada sebelum 000006 tidak boleh ikut terhapus.
	for _, table := range []string{"iot_nodes", "earthquake_events"} {
		var n int
		if err := st.pool.QueryRow(ctx,
			`SELECT count(*) FROM information_schema.tables
			 WHERE table_schema = 'public' AND table_name = $1`, table).Scan(&n); err != nil {
			t.Fatalf("periksa tabel %s: %v", table, err)
		}
		if n != 1 {
			t.Errorf("rollback menghapus tabel %s yang bukan miliknya", table)
		}
	}

	// down dijalankan dua kali juga harus aman (DROP ... IF EXISTS).
	if _, err := st.pool.Exec(ctx, string(down)); err != nil {
		t.Fatalf("rollback kedua gagal (bukan idempoten): %v", err)
	}
}
