package store

// --- Integrasi Postgres untuk pembacaan penelusuran P4-M1′ ---
//
// Butuh Postgres NYATA, dan alasannya sama seperti ledger_test.go: yang diuji di
// sini adalah perilaku KUERI-nya, bukan perilaku Go. Dua hal yang tidak dapat
// diuji dengan fake:
//
//	ListLastNObservations  — dua ORDER BY di dalam satu kueri (batas dari ujung
//	                         TERBARU, hasil dalam urutan KANONIK naik). Sebuah
//	                         subquery yang salah arah tetap mengembalikan N baris
//	                         dan tetap terlihat benar dari Go.
//	ListEmissionsForTrace  — SELECT PERTAMA terhadap alert_emissions di seluruh
//	                         kode. Cast event_id::text dan kolom nullable
//	                         migrasi 000007/000008 hanya nyata di Postgres.
//
//	TEST_DATABASE_URL=postgres://... go test ./internal/store -run TestTrace
//
// Tanpa env itu seluruh test di berkas ini skip.

import (
	"context"
	"testing"
)

// seedTraceObs menulis satu observasi lewat penulis PRODUKSI (InsertObservation)
// dan mendaftarkan pembersihannya. Bukan INSERT tangan: kueri yang diuji harus
// membaca baris berbentuk sama dengan yang benar-benar ditulis produksi.
func seedTraceObs(t *testing.T, st *Store, nodeID string, pga float64, receivedTS int64, withLoc bool) {
	t.Helper()
	ctx := context.Background()
	lat, lon := -6.9034443, 107.6431173
	upper := receivedTS - 300
	o := &Observation{
		NodeID: nodeID, SourceClass: "FIXED_ESP32", Phase: "FINAL",
		PGAGal: pga, DurMs: 300,
		PublishTS: receivedTS - 20, ReceivedTS: receivedTS,
		OnsetTSUpperBound: &upper, OnsetTSSource: "PUBLISH_BOUND",
		VerifyResult: "OK",
	}
	if withLoc {
		o.Lat, o.Lon = &lat, &lon
	}
	if err := st.InsertObservation(ctx, o); err != nil {
		t.Fatalf("InsertObservation: %v", err)
	}
	t.Cleanup(func() {
		_, _ = st.pool.Exec(ctx, `DELETE FROM sensor_observations WHERE node_id = $1`, nodeID)
	})
}

// Batas diambil dari ujung TERBARU, hasilnya naik. Regresi yang dicegah: sebuah
// kueri yang membatasi dari ujung TERTUA tetap mengembalikan N baris terurut naik
// dan tetap lulus setiap pemeriksaan yang hanya melihat panjang dan urutan.
func TestTraceListLastNTakesNewestReturnsAscending(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	const node = "NODE-7RACE001"
	base := int64(1_700_000_000_000)

	for i := 0; i < 5; i++ {
		seedTraceObs(t, st, node, 20+float64(i), base+int64(i)*1000, true)
	}

	got, err := st.ListLastNObservations(ctx, 3)
	if err != nil {
		t.Fatalf("ListLastNObservations: %v", err)
	}
	// Basis data bisa memuat baris lain dari test paralel; saring ke node ini.
	var mine []ReplayObservation
	for _, o := range got {
		if o.NodeID == node {
			mine = append(mine, o)
		}
	}
	if len(mine) == 0 {
		t.Fatal("tidak satu pun baris node uji terbaca")
	}
	// Yang terbaca harus dari ujung TERBARU: baris tertua (base+0) tidak boleh ada
	// bila ketiga slot terisi baris node ini.
	if len(mine) == 3 && mine[0].ReceivedTS != base+2000 {
		t.Errorf("baris pertama received_ts = %d; mau %d (batas dari ujung TERBARU)",
			mine[0].ReceivedTS, base+2000)
	}
	for i := 1; i < len(mine); i++ {
		if mine[i-1].ReceivedTS > mine[i].ReceivedTS {
			t.Fatalf("urutan TIDAK naik pada indeks %d: %d lalu %d",
				i, mine[i-1].ReceivedTS, mine[i].ReceivedTS)
		}
	}
}

// limit <= 0 adalah GALAT, bukan "semua": pembacaan tanpa batas atas pada tabel
// ledger tumbuh diam-diam sampai menjadi masalah produksi.
func TestTraceListLastNRejectsNonPositiveLimit(t *testing.T) {
	st := newTestStore(t)
	for _, n := range []int{0, -1} {
		if _, err := st.ListLastNObservations(context.Background(), n); err == nil {
			t.Errorf("limit=%d diterima; mau galat", n)
		}
	}
}

// Kueri TIDAK menyaring: baris tanpa lokasi dan baris di bawah lantai PGA harus
// tetap terbaca, karena jumlah yang tersaring harus dapat DILAPORKAN pemanggil.
func TestTraceListLastNDoesNotFilter(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	const node = "NODE-7RACE002"
	base := int64(1_700_000_100_000)

	seedTraceObs(t, st, node, 2.0, base, true)        // di bawah lantai
	seedTraceObs(t, st, node, 40.0, base+1000, false) // tanpa node_location

	got, err := st.ListLastNObservations(ctx, 50)
	if err != nil {
		t.Fatalf("ListLastNObservations: %v", err)
	}
	var below, noLoc bool
	for _, o := range got {
		if o.NodeID != node {
			continue
		}
		if o.PGAGal == 2.0 {
			below = true
		}
		if o.PGAGal == 40.0 && (o.Lat == nil || o.Lon == nil) {
			noLoc = true
		}
	}
	if !below {
		t.Error("baris di bawah lantai PGA disaring oleh kueri")
	}
	if !noLoc {
		t.Error("baris tanpa node_location disaring oleh kueri")
	}
}

// SELECT pertama terhadap alert_emissions. Ditulis lewat InsertAlertEmission —
// penulis PRODUKSI — lalu dibaca kembali, sehingga yang diperiksa adalah kedua
// arah pada kolom yang sama.
func TestTraceListEmissionsRoundTripAdvisoryRow(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	const id = "bbbbbbbb-0000-4000-8000-00000000e001"
	decided := int64(1_700_000_200_000)

	seedEvent(t, st, newPhase3Event(id, EventStateUnconfirmed, 1))

	eid, state := id, EventStateUnconfirmed
	rev, ws := 1, 3
	pga := 73.0537
	if err := st.InsertAlertEmission(ctx, &AlertEmission{
		EventID: &eid, AlertType: "EARTHQUAKE_ADVISORY", Status: "ADVISORY",
		PGAGal: &pga, NodeCount: 1, IsSevere: false, Audience: "NONE",
		DecidedAt: decided, AlgoVer: "phase3-1.1/ic=5",
		WSClientCount: &ws, EventState: &state, EventRevision: &rev,
	}); err != nil {
		t.Fatalf("InsertAlertEmission: %v", err)
	}
	t.Cleanup(func() {
		_, _ = st.pool.Exec(ctx, `DELETE FROM alert_emissions WHERE event_id = $1`, id)
	})

	got, err := st.ListEmissionsForTrace(ctx, decided-1000, decided+1000)
	if err != nil {
		t.Fatalf("ListEmissionsForTrace: %v", err)
	}
	var row *TraceEmission
	for i := range got {
		if got[i].EventID != nil && *got[i].EventID == id {
			row = &got[i]
		}
	}
	if row == nil {
		t.Fatalf("baris emisi tidak terbaca kembali (terbaca %d baris)", len(got))
	}
	if row.EmissionID <= 0 {
		t.Errorf("emission_id = %d; mau > 0", row.EmissionID)
	}
	if row.AlertType != "EARTHQUAKE_ADVISORY" || row.Status != "ADVISORY" {
		t.Errorf("alert_type/status = %q/%q", row.AlertType, row.Status)
	}
	if row.EventState == nil || *row.EventState != EventStateUnconfirmed {
		t.Errorf("event_state = %v; mau UNCONFIRMED", row.EventState)
	}
	if row.EventRevision == nil || *row.EventRevision != 1 {
		t.Errorf("event_revision = %v; mau 1", row.EventRevision)
	}
	if row.WSClientCount == nil || *row.WSClientCount != 3 {
		t.Errorf("ws_client_count = %v; mau 3", row.WSClientCount)
	}
	if row.DecidedAt != decided {
		t.Errorf("decided_at = %d; mau %d", row.DecidedAt, decided)
	}
}

// Baris pra-000008/000007: event_id, event_state, event_revision, dan
// ws_client_count semuanya NULL. NULL berarti TIDAK PERNAH DILAPORKAN, bukan nol,
// dan kuerinya harus membawa perbedaan itu utuh sampai ke Go.
func TestTraceListEmissionsCarriesNullsAsNull(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	decided := int64(1_700_000_300_000)

	if err := st.InsertAlertEmission(ctx, &AlertEmission{
		AlertType: "EARTHQUAKE_ADVISORY", Status: "ADVISORY",
		NodeCount: 1, IsSevere: false, Audience: "NONE",
		DecidedAt: decided, AlgoVer: "phase1-1.0",
	}); err != nil {
		t.Fatalf("InsertAlertEmission: %v", err)
	}
	t.Cleanup(func() {
		_, _ = st.pool.Exec(ctx, `DELETE FROM alert_emissions WHERE decided_at = $1`, decided)
	})

	got, err := st.ListEmissionsForTrace(ctx, decided, decided)
	if err != nil {
		t.Fatalf("ListEmissionsForTrace: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("baris = %d; mau 1", len(got))
	}
	row := got[0]
	if row.EventID != nil {
		t.Errorf("event_id = %v; mau NULL", row.EventID)
	}
	if row.EventState != nil || row.EventRevision != nil {
		t.Errorf("event_state/event_revision = %v/%v; mau NULL", row.EventState, row.EventRevision)
	}
	if row.WSClientCount != nil {
		t.Errorf("ws_client_count = %v; mau NULL (tidak dilaporkan, BUKAN nol)", row.WSClientCount)
	}
}

// Jendela decided_at inklusif di kedua ujung: sebuah transisi tepat di tepi
// jendela tidak boleh hilang karena perbandingan yang salah satu ujungnya
// eksklusif.
func TestTraceListEmissionsWindowIsInclusive(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	lo, hi := int64(1_700_000_400_000), int64(1_700_000_400_500)

	for _, ts := range []int64{lo, hi} {
		if err := st.InsertAlertEmission(ctx, &AlertEmission{
			AlertType: "EARTHQUAKE_ADVISORY", Status: "ADVISORY",
			NodeCount: 1, Audience: "NONE",
			DecidedAt: ts, AlgoVer: "phase1-1.0",
		}); err != nil {
			t.Fatalf("InsertAlertEmission: %v", err)
		}
	}
	t.Cleanup(func() {
		_, _ = st.pool.Exec(ctx,
			`DELETE FROM alert_emissions WHERE decided_at BETWEEN $1 AND $2`, lo, hi)
	})

	got, err := st.ListEmissionsForTrace(ctx, lo, hi)
	if err != nil {
		t.Fatalf("ListEmissionsForTrace: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("baris = %d; mau 2 (kedua tepi inklusif)", len(got))
	}
	if got[0].DecidedAt != lo || got[1].DecidedAt != hi {
		t.Errorf("urutan = %d,%d; mau %d,%d (decided_at naik)",
			got[0].DecidedAt, got[1].DecidedAt, lo, hi)
	}
}
