package store

// --- Integrasi Postgres untuk lifecycle event (migrasi 000008) ---
//
// Butuh Postgres NYATA dengan PostGIS, dan alasannya sama seperti ledger_test.go:
// yang diuji di sini adalah perilaku SKEMA — penjaga revisi di ON CONFLICT, FK
// event_state_log -> earthquake_events, idempotensi UNIQUE (event_id, revision),
// dan jumlah indeks pada tabel log. Tidak satu pun dapat diuji dengan fake, dan
// justru di situlah letak jaminannya (§18.3).
//
//	TEST_DATABASE_URL=postgres://... go test ./internal/store -run TestEvent
//
// Tanpa env itu seluruh test di file ini skip.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// seedEvent menulis satu baris earthquake_events lewat UpsertEvent dan
// mendaftarkan pembersihannya (baris log lebih dulu — FK).
func seedEvent(t *testing.T, st *Store, e *EarthquakeEvent) {
	t.Helper()
	ctx := context.Background()
	if err := st.UpsertEvent(ctx, e); err != nil {
		t.Fatalf("UpsertEvent: %v", err)
	}
	t.Cleanup(func() {
		_, _ = st.pool.Exec(ctx, `DELETE FROM event_state_log WHERE event_id = $1`, e.EventID)
		_, _ = st.pool.Exec(ctx, `DELETE FROM earthquake_events WHERE event_id = $1`, e.EventID)
	})
}

func newPhase3Event(id string, state string, revision int) *EarthquakeEvent {
	status := "HAPPENING"
	if state == EventStateResolved || state == EventStateCancelled {
		status = "RESOLVED"
	}
	return &EarthquakeEvent{
		EventID:              id,
		Status:               status,
		CentroidLat:          -6.9034443,
		CentroidLon:          107.6431173,
		LocationName:         "Bandung",
		MMIScale:             "V",
		IntensityLabel:       "Sedang",
		MaxPGA:               42.5000,
		TriggeredNodes:       3,
		StartedAtMs:          1_700_000_000_000,
		EventState:           state,
		Revision:             revision,
		OriginTS:             1_699_999_998_000,
		OriginTSSource:       "SENSOR",
		IndependentCellCount: 2,
		AlgoVer:              "phase3-1.0/ic=5",
	}
}

// Penjaga revisi adalah satu-satunya hal yang membuat antrean drop-oldest aman
// untuk baris event: dua satuan persistensi untuk event yang sama harus tidak
// peduli urutan kedatangan. Yang dicegah regresinya: sebuah penulisan tertinggal
// memundurkan baris ke state yang lebih tua.
func TestEventUpsertRevisionGuard(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	const id = "aaaaaaaa-0000-4000-8000-000000000001"

	seedEvent(t, st, newPhase3Event(id, EventStateUnconfirmed, 1))

	readState := func() (string, int) {
		t.Helper()
		var s string
		var r int
		if err := st.pool.QueryRow(ctx,
			`SELECT event_state, revision FROM earthquake_events WHERE event_id = $1`, id).
			Scan(&s, &r); err != nil {
			t.Fatalf("baca baris: %v", err)
		}
		return s, r
	}

	if s, r := readState(); s != EventStateUnconfirmed || r != 1 {
		t.Fatalf("setelah insert: %s/%d, mau UNCONFIRMED/1", s, r)
	}

	// Revisi lebih tinggi diterima.
	if err := st.UpsertEvent(ctx, newPhase3Event(id, EventStateConfirmed, 2)); err != nil {
		t.Fatalf("upsert revisi 2: %v", err)
	}
	if s, r := readState(); s != EventStateConfirmed || r != 2 {
		t.Fatalf("setelah revisi 2: %s/%d, mau CONFIRMED/2", s, r)
	}

	// Revisi lebih rendah TIDAK boleh mengubah apa pun, dan tidak boleh error:
	// pemanggilnya adalah drain antrean, yang tidak punya tindakan benar apa pun
	// untuk merespons "penulisan ini sudah usang".
	if err := st.UpsertEvent(ctx, newPhase3Event(id, EventStateUnconfirmed, 1)); err != nil {
		t.Fatalf("upsert revisi usang harus no-op tanpa error, dapat: %v", err)
	}
	if s, r := readState(); s != EventStateConfirmed || r != 2 {
		t.Fatalf("revisi usang memundurkan baris: %s/%d", s, r)
	}

	// Revisi sama juga no-op.
	if err := st.UpsertEvent(ctx, newPhase3Event(id, EventStateResolved, 2)); err != nil {
		t.Fatalf("upsert revisi sama: %v", err)
	}
	if s, r := readState(); s != EventStateConfirmed || r != 2 {
		t.Fatalf("revisi sama mengubah baris: %s/%d", s, r)
	}
}

// Transisi ke RESOLVED lewat UpsertEvent harus mengisi resolved_at sekali dan
// tidak menggesernya lagi pada penulisan berikutnya.
func TestEventUpsertResolvedStampsResolvedAtOnce(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	const id = "aaaaaaaa-0000-4000-8000-000000000002"

	seedEvent(t, st, newPhase3Event(id, EventStateConfirmed, 1))

	if err := st.UpsertEvent(ctx, newPhase3Event(id, EventStateResolved, 2)); err != nil {
		t.Fatalf("upsert RESOLVED: %v", err)
	}
	var first *string
	if err := st.pool.QueryRow(ctx,
		`SELECT resolved_at::text FROM earthquake_events WHERE event_id = $1`, id).
		Scan(&first); err != nil {
		t.Fatalf("baca resolved_at: %v", err)
	}
	if first == nil {
		t.Fatal("resolved_at masih NULL setelah transisi ke RESOLVED")
	}

	if err := st.UpsertEvent(ctx, newPhase3Event(id, EventStateResolved, 3)); err != nil {
		t.Fatalf("upsert RESOLVED lagi: %v", err)
	}
	var second *string
	if err := st.pool.QueryRow(ctx,
		`SELECT resolved_at::text FROM earthquake_events WHERE event_id = $1`, id).
		Scan(&second); err != nil {
		t.Fatalf("baca resolved_at kedua: %v", err)
	}
	if second == nil || *second != *first {
		t.Fatalf("resolved_at bergeser: %v -> %v", first, second)
	}
}

// Pemutaran ulang satu transisi harus menghasilkan satu baris, tanpa error yang
// sampai ke pemanggil: §15 menyandarkan idempotensi replay pada constraint ini,
// bukan pada penjagaan di Go.
func TestEventStateLogDuplicateRevisionIsNoOp(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	const id = "aaaaaaaa-0000-4000-8000-000000000003"

	seedEvent(t, st, newPhase3Event(id, EventStateConfirmed, 2))

	from := EventStateUnconfirmed
	peak := 42.5
	l := &EventStateLog{
		EventID:          id,
		Revision:         2,
		FromState:        &from,
		ToState:          EventStateConfirmed,
		Reason:           "QUORUM_MET",
		DecidedAt:        1_700_000_001_000,
		NodeCount:        3,
		IndependentCells: 2,
		PeakPGA:          &peak,
		EvidenceSummary:  []byte(`{"contributors":[{"node_id":"NODE-00000001"}]}`),
		AlgoVer:          "phase3-1.0/ic=5",
	}
	if err := st.AppendStateLog(ctx, l); err != nil {
		t.Fatalf("AppendStateLog: %v", err)
	}
	if err := st.AppendStateLog(ctx, l); err != nil {
		t.Fatalf("AppendStateLog kedua harus no-op tanpa error, dapat: %v", err)
	}

	var n int
	if err := st.pool.QueryRow(ctx,
		`SELECT count(*) FROM event_state_log WHERE event_id = $1`, id).Scan(&n); err != nil {
		t.Fatalf("hitung baris log: %v", err)
	}
	if n != 1 {
		t.Fatalf("baris log = %d, mau 1", n)
	}

	// evidence_summary harus tiba sebagai JSONB yang dapat dibaca kembali utuh.
	var raw []byte
	if err := st.pool.QueryRow(ctx,
		`SELECT evidence_summary FROM event_state_log WHERE event_id = $1`, id).Scan(&raw); err != nil {
		t.Fatalf("baca evidence_summary: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("evidence_summary bukan JSON valid: %v", err)
	}
	if _, ok := decoded["contributors"]; !ok {
		t.Fatalf("evidence_summary kehilangan contributors: %s", raw)
	}
}

// FK-nya harus NYATA. Aturan urutan §9.5 (upsert lebih dulu, log dilewati bila
// upsert gagal) hanya bernilai bila basis data memang menolak induk yang tidak
// ada — kalau tidak, aturan itu menjaga sesuatu yang tidak pernah diperiksa.
func TestEventStateLogRequiresParentRow(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	err := st.AppendStateLog(ctx, &EventStateLog{
		EventID:          "aaaaaaaa-0000-4000-8000-0000000000ff",
		Revision:         1,
		ToState:          EventStateUnconfirmed,
		Reason:           "FLOOR_MET",
		DecidedAt:        1_700_000_001_000,
		NodeCount:        1,
		IndependentCells: 1,
		EvidenceSummary:  []byte(`{}`),
		AlgoVer:          "phase3-1.0/ic=5",
	})
	if err == nil {
		t.Fatal("baris log tanpa induk harus ditolak FK")
	}
}

// R-M1: tepat SATU indeks pada event_state_log di luar primary key, yaitu yang
// menopang UNIQUE (event_id, revision). Diuji terhadap pg_indexes agar duplikat
// yang sudah dihapus tidak bisa menyelinap kembali lewat migrasi berikutnya.
func TestEventStateLogHasExactlyOneSecondaryIndex(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	rows, err := st.pool.Query(ctx,
		`SELECT indexname, indexdef FROM pg_indexes WHERE tablename = 'event_state_log' ORDER BY indexname`)
	if err != nil {
		t.Fatalf("query pg_indexes: %v", err)
	}
	defer rows.Close()

	var secondary []string
	for rows.Next() {
		var name, def string
		if err := rows.Scan(&name, &def); err != nil {
			t.Fatalf("scan indeks: %v", err)
		}
		if name == "event_state_log_pkey" {
			continue
		}
		secondary = append(secondary, name+" => "+def)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate indeks: %v", err)
	}
	if len(secondary) != 1 {
		t.Fatalf("indeks sekunder event_state_log = %d, mau 1:\n%v", len(secondary), secondary)
	}
	if !strings.Contains(secondary[0], "(event_id, revision)") {
		t.Fatalf("indeks sekunder bukan (event_id, revision): %s", secondary[0])
	}
}

// LoadOpenEvents adalah bahan Reconcile (§15.3): ia harus mengembalikan event
// yang masih HAPPENING beserta evidence_summary dari revisi TERTINGGI-nya, agar
// observasi yang datang setelah restart menempel pada event_id yang sama alih-alih
// membentuk event kedua untuk gempa yang sama.
func TestEventLoadOpenEventsCarriesLatestEvidence(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	const (
		openID   = "aaaaaaaa-0000-4000-8000-000000000004"
		closedID = "aaaaaaaa-0000-4000-8000-000000000005"
	)

	seedEvent(t, st, newPhase3Event(openID, EventStateConfirmed, 2))
	seedEvent(t, st, newPhase3Event(closedID, EventStateResolved, 3))

	from := EventStateDetected
	appendLog := func(rev int, to, evidence string) {
		t.Helper()
		if err := st.AppendStateLog(ctx, &EventStateLog{
			EventID: openID, Revision: rev, FromState: &from, ToState: to,
			Reason: "FLOOR_MET", DecidedAt: int64(1_700_000_000_000 + rev),
			NodeCount: rev, IndependentCells: 1,
			EvidenceSummary: []byte(evidence), AlgoVer: "phase3-1.0/ic=5",
		}); err != nil {
			t.Fatalf("AppendStateLog rev %d: %v", rev, err)
		}
	}
	// Ditulis dengan urutan revisi TERBALIK: yang dipilih harus revisi tertinggi,
	// bukan baris yang terakhir masuk.
	appendLog(2, EventStateConfirmed, `{"marker":"latest"}`)
	appendLog(1, EventStateUnconfirmed, `{"marker":"older"}`)

	got, err := st.LoadOpenEvents(ctx)
	if err != nil {
		t.Fatalf("LoadOpenEvents: %v", err)
	}

	var found *EarthquakeEvent
	for _, e := range got {
		if e.EventID == closedID {
			t.Fatal("event RESOLVED tidak boleh ikut termuat")
		}
		if e.EventID == openID {
			found = e
		}
	}
	if found == nil {
		t.Fatalf("event terbuka tidak termuat (dapat %d baris)", len(got))
	}

	if found.EventState != EventStateConfirmed || found.Revision != 2 {
		t.Fatalf("state/revisi = %s/%d, mau CONFIRMED/2", found.EventState, found.Revision)
	}
	if found.OriginTS != 1_699_999_998_000 || found.OriginTSSource != "SENSOR" {
		t.Fatalf("origin = %d/%s", found.OriginTS, found.OriginTSSource)
	}
	if found.IndependentCellCount != 2 || found.AlgoVer != "phase3-1.0/ic=5" {
		t.Fatalf("cells/algo = %d/%s", found.IndependentCellCount, found.AlgoVer)
	}
	if found.StartedAtMs != 1_700_000_000_000 {
		t.Fatalf("started_at_ms = %d, mau 1700000000000", found.StartedAtMs)
	}
	var ev map[string]string
	if err := json.Unmarshal(found.LatestEvidence, &ev); err != nil {
		t.Fatalf("LatestEvidence bukan JSON: %v (%s)", err, found.LatestEvidence)
	}
	if ev["marker"] != "latest" {
		t.Fatalf("LatestEvidence dari revisi salah: %s", found.LatestEvidence)
	}
	// decided_at berasal dari baris yang SAMA dengan evidence-nya: §15.3 memakainya
	// sebagai waktu bukti terakhir, dan mengambilnya dari revisi lain akan membuat
	// event terlihat lebih segar daripada keadaannya.
	if found.LatestDecidedAt != 1_700_000_000_002 {
		t.Fatalf("LatestDecidedAt = %d, mau decided_at revisi 2 (1700000000002)", found.LatestDecidedAt)
	}
}

// Event terbuka TANPA satu pun baris log — satuan persistensinya dibuang (D30),
// atau barisnya pra-Fase-3. LEFT JOIN harus tetap mengembalikannya, dengan kedua
// kolom turunan itu kosong: justru event seperti inilah yang paling perlu
// direkonsiliasi.
func TestEventLoadOpenEventsWithoutStateLogReturnsZeroDerivedColumns(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	const id = "aaaaaaaa-0000-4000-8000-00000000000b"
	seedEvent(t, st, newPhase3Event(id, EventStateConfirmed, 4))

	got, err := st.LoadOpenEvents(ctx)
	if err != nil {
		t.Fatalf("LoadOpenEvents: %v", err)
	}
	var found *EarthquakeEvent
	for _, e := range got {
		if e.EventID == id {
			found = e
		}
	}
	if found == nil {
		t.Fatal("event tanpa baris log tidak termuat")
	}
	if found.LatestEvidence != nil {
		t.Errorf("LatestEvidence = %s, mau nil", found.LatestEvidence)
	}
	if found.LatestDecidedAt != 0 {
		t.Errorf("LatestDecidedAt = %d, mau 0", found.LatestDecidedAt)
	}
}

// ListActiveNodeLocations adalah masukan pemeriksaan-diri fleet (§7.3): predikatnya
// SQL, jadi yang diuji di sini adalah predikat itu — node yang tidak aktif, belum
// belum terverifikasi tidak boleh ikut menghitung sel independensi, karena node yang
// ikut dihitung tetapi tidak dapat menyumbang bukti akan membuat peringatan startup
// berbohong ke arah yang aman-terlihat.
func TestListActiveNodeLocationsOnlyCountsUsableNodes(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	seed := func(id string, active, verified bool) {
		t.Helper()
		_, err := st.pool.Exec(ctx, `
			INSERT INTO iot_nodes (
				station_id, sensor_model, location_name, location,
				secret_key_enc, secret_key_nonce, is_active, verified
			) VALUES (
				$1, 'MPU 6050', 'Cimahi',
				ST_SetSRID(ST_MakePoint(107.54, -6.87), 4326)::geography,
				'\x00'::bytea, '\x00'::bytea, $2, $3
			)`, id, active, verified)
		if err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
		t.Cleanup(func() {
			_, _ = st.pool.Exec(ctx, `DELETE FROM iot_nodes WHERE station_id = $1`, id)
		})
	}

	const (
		usable     = "NODE-FLEET001"
		unverified = "NODE-FLEET002"
		inactive   = "NODE-FLEET003"
	)
	seed(usable, true, true)
	seed(unverified, true, false)
	seed(inactive, false, true)

	got, err := st.ListActiveNodeLocations(ctx)
	if err != nil {
		t.Fatalf("ListActiveNodeLocations: %v", err)
	}

	seen := make(map[string]NodeLocation, len(got))
	for _, n := range got {
		seen[n.StationID] = n
	}
	if _, ok := seen[usable]; !ok {
		t.Error("node aktif+terverifikasi harus ikut")
	}
	for _, id := range []string{unverified, inactive} {
		if _, ok := seen[id]; ok {
			t.Errorf("%s tidak boleh ikut", id)
		}
	}
	if n := seen[usable]; n.Lat == 0 || n.Lon == 0 || n.LocationName == "" {
		t.Errorf("koordinat/nama tidak terbaca: %+v", n)
	}
}

// Baris pra-Fase-3 harus tetap terbaca: event_state NULL tiba sebagai string
// kosong dan tidak membuat LoadOpenEvents gagal. Ini setengah durable dari
// kriteria migrasi §22.1 #1 — separuh lainnya (up/down/up) dijalankan terhadap
// kontainer sekali pakai, bukan dari dalam suite ini.
func TestEventLoadOpenEventsReadsPrePhase3Row(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	// Ditulis lewat SaveEvent-nya jalur Fase 2 supaya bentuknya benar-benar bentuk
	// lama: tanpa event_state, tanpa origin_ts, revision terisi default kolomnya.
	got, err := st.SaveEvent(ctx, &EarthquakeEvent{
		Status: "HAPPENING", CentroidLat: -6.9, CentroidLon: 107.6,
		LocationName: "Bandung", MMIScale: "IV", IntensityLabel: "Ringan",
		MaxPGA: 18.25, TriggeredNodes: 3, StartedAtMs: 1_700_000_000_000,
	})
	if err != nil {
		t.Fatalf("SaveEvent: %v", err)
	}
	t.Cleanup(func() {
		_, _ = st.pool.Exec(ctx, `DELETE FROM earthquake_events WHERE event_id = $1`, got)
	})

	events, err := st.LoadOpenEvents(ctx)
	if err != nil {
		t.Fatalf("LoadOpenEvents: %v", err)
	}
	for _, e := range events {
		if e.EventID != got {
			continue
		}
		if e.EventState != "" || e.Revision != 0 || e.OriginTS != 0 ||
			e.OriginTSSource != "" || e.AlgoVer != "" {
			t.Fatalf("baris lama tidak boleh punya nilai lifecycle: %+v", e)
		}
		if e.LatestEvidence != nil {
			t.Fatalf("baris lama tidak punya state-log: %s", e.LatestEvidence)
		}
		if e.Status != "HAPPENING" || e.MaxPGA != 18.25 {
			t.Fatalf("baris lama tidak terbaca utuh: %+v", e)
		}
		return
	}
	t.Fatalf("baris lama %s tidak termuat", got)
}

// ResolveEvent tetap idempoten (WHERE status='HAPPENING') dan kini juga menurunkan
// event_state — tetapi HANYA bila barisnya memang punya event_state. Baris lama
// tidak boleh mendadak memperoleh state yang tidak pernah dimilikinya.
func TestEventResolveEventSetsStateAndStaysIdempotent(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	const id = "aaaaaaaa-0000-4000-8000-000000000007"

	seedEvent(t, st, newPhase3Event(id, EventStateConfirmed, 1))

	if err := st.ResolveEvent(ctx, id); err != nil {
		t.Fatalf("ResolveEvent: %v", err)
	}
	var status string
	var state *string
	if err := st.pool.QueryRow(ctx,
		`SELECT status, event_state FROM earthquake_events WHERE event_id = $1`, id).
		Scan(&status, &state); err != nil {
		t.Fatalf("baca baris: %v", err)
	}
	if status != "RESOLVED" || state == nil || *state != EventStateResolved {
		t.Fatalf("status/state = %s/%v", status, state)
	}

	// Panggilan kedua tidak mengubah baris dan melaporkan ErrEventNotFound —
	// perilaku Fase 1/2 yang tidak boleh bergeser.
	if err := st.ResolveEvent(ctx, id); err == nil {
		t.Fatal("ResolveEvent kedua harus melaporkan ErrEventNotFound")
	} else if !errors.Is(err, ErrEventNotFound) {
		t.Fatalf("error = %v, mau ErrEventNotFound", err)
	}

	// Baris lama: event_state NULL harus TETAP NULL setelah diresolusi.
	legacy, err := st.SaveEvent(ctx, &EarthquakeEvent{
		Status: "HAPPENING", CentroidLat: -6.9, CentroidLon: 107.6,
		LocationName: "Bandung", MMIScale: "IV", IntensityLabel: "Ringan",
		MaxPGA: 18.25, TriggeredNodes: 3, StartedAtMs: 1_700_000_000_000,
	})
	if err != nil {
		t.Fatalf("SaveEvent: %v", err)
	}
	t.Cleanup(func() {
		_, _ = st.pool.Exec(ctx, `DELETE FROM earthquake_events WHERE event_id = $1`, legacy)
	})
	if err := st.ResolveEvent(ctx, legacy); err != nil {
		t.Fatalf("ResolveEvent baris lama: %v", err)
	}
	var legacyState *string
	if err := st.pool.QueryRow(ctx,
		`SELECT event_state FROM earthquake_events WHERE event_id = $1`, legacy).
		Scan(&legacyState); err != nil {
		t.Fatalf("baca baris lama: %v", err)
	}
	if legacyState != nil {
		t.Fatalf("baris lama memperoleh event_state %q dari resolusi", *legacyState)
	}
}

// §10.2 — proyeksi ListEvents kini membawa lima kolom siklus hidup. Yang diuji
// bukan pemetaannya melainkan KESELARASAN antara daftar kolom dan rows.Scan:
// menambah satu kolom tanpa menambah target scan tidak dapat gagal build, dan
// menukar dua kolom bertipe sama tidak dapat gagal di mana pun kecuali di sini.
//
// Dua bentuk baris sekaligus, karena keduanya harus melewati kolom yang sama:
// baris Fase 3 dengan nilai lengkap, dan baris pra-Fase-3 yang kolomnya NULL dan
// harus tiba sebagai nol lewat COALESCE — bukan sebagai galat scan.
func TestListEventsProjectsLifecycleColumns(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	const id = "aaaaaaaa-0000-4000-8000-00000000000c"

	seedEvent(t, st, newPhase3Event(id, EventStateConfirmed, 4))

	legacy, err := st.SaveEvent(ctx, &EarthquakeEvent{
		Status: "RESOLVED", CentroidLat: -6.95, CentroidLon: 107.7,
		LocationName: "Cimahi", MMIScale: "IV", IntensityLabel: "Ringan",
		MaxPGA: 19.5, TriggeredNodes: 3, StartedAtMs: 1_699_000_000_000,
	})
	if err != nil {
		t.Fatalf("SaveEvent: %v", err)
	}
	t.Cleanup(func() {
		_, _ = st.pool.Exec(ctx, `DELETE FROM earthquake_events WHERE event_id = $1`, legacy)
	})

	events, err := st.ListEvents(ctx, 200, 0, nil)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}

	var seenPhase3, seenLegacy bool
	for _, e := range events {
		switch e.EventID {
		case id:
			seenPhase3 = true
			if e.EventState != EventStateConfirmed || e.Revision != 4 ||
				e.OriginTS != 1_699_999_998_000 || e.OriginTSSource != "SENSOR" ||
				e.IndependentCellCount != 2 {
				t.Errorf("baris Fase 3 salah proyeksi: %+v", e)
			}
			if e.MaxPGA != 42.5 || e.TriggeredNodes != 3 || e.Status != "HAPPENING" {
				t.Errorf("kolom lama tergeser oleh kolom baru: %+v", e)
			}
		case legacy:
			seenLegacy = true
			if e.EventState != "" || e.OriginTS != 0 || e.OriginTSSource != "" ||
				e.IndependentCellCount != 0 {
				t.Errorf("baris pra-Fase-3 tidak boleh punya nilai lifecycle: %+v", e)
			}
			if e.MaxPGA != 19.5 || e.LocationName != "Cimahi" {
				t.Errorf("baris pra-Fase-3 tidak terbaca utuh: %+v", e)
			}
		}
	}
	if !seenPhase3 {
		t.Error("baris Fase 3 CONFIRMED tidak muncul di umpan publik")
	}
	if !seenLegacy {
		t.Error("baris pra-Fase-3 tidak muncul di umpan publik")
	}
}
