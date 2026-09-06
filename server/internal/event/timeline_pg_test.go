package event

// --- Integrasi Postgres NYATA untuk garis-waktu event (P4-M6′, D-015) ---
//
// Berkas ini menguji rakitan LENGKAP M6′ terhadap skema sungguhan: baris ditulis
// lewat penulis PRODUKSI, dibaca lewat ketiga pembaca read-only M6′, lalu
// dirakit oleh BuildTimeline. Yang tidak dapat diuji oleh fake, dan justru itu
// inti keempat keluaran wajib:
//
//	evidence_summary bolak-balik — EvidenceSummary.JSON() ditulis ke kolom JSONB,
//	                  dan JSONB MENORMALKAN: spasi hilang, urutan kunci tidak
//	                  dijamin, angka dinormalkan. historicEvidence harus tetap
//	                  membacanya. Bila tidak, satu-satunya jalan pulang dari
//	                  transisi ke node (D12) hilang dan setiap transisi tampak
//	                  TANPA KONTRIBUTOR — kegagalan yang berbunyi seperti temuan.
//	jendela nyata     — TimelineWindowBounds menghitung batas, Postgres yang
//	                  menyaringnya. Sebuah ketidakcocokan antara aritmetika Go dan
//	                  predikat SQL hanya terlihat bila keduanya dijalankan.
//	obs_seq & NULL    — pointer yang melewati batas basis data: obs_seq NULL tetap
//	                  NULL, dan anotasi tautan obs_seq karena itu tetap
//	                  UNAVAILABLE alih-alih membandingkan nol.
//
//	TEST_DATABASE_URL=postgres://... go test ./internal/event -run TestPGTimeline
//
// Tanpa env itu seluruh uji di berkas ini skip, pola yang sama dengan
// nearconfirmed_pg_test.go.
//
// SATU HAL YANG BERKAS INI TIDAK MEMBUKTIKAN: apa pun tentang lapangan. Riwayat
// di sini DISEMAI, bukan diamati; ia bukti perangkat lunak. Batas satu-node
// D-015/D-011 tetap berlaku dan Fase F tetap pemilik bukti lapangan.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/banana-pixel/quakealert/server/internal/store"
)

// pgTimelineSeed menyemai satu event beserta riwayatnya lewat penulis PRODUKSI
// (UpsertEvent + AppendStateLog), dalam urutan yang FK tuntut: induk lebih dulu.
//
// event_id setiap baris riwayat DITULIS ULANG ke eventID: pemanggil boleh
// membawa riwayat dari pembangun fixture, dan FK event_state_log ->
// earthquake_events tidak memaafkan ketidakcocokan itu.
func pgTimelineSeed(t *testing.T, st *store.Store, pool *pgxpool.Pool, eventID string, hist []store.EventStateLog) {
	t.Helper()
	ctx := context.Background()
	pgCleanupEvent(t, pool, eventID)

	for i := range hist {
		hist[i].EventID = eventID
	}
	last := hist[len(hist)-1]
	row := &store.EarthquakeEvent{
		EventID: eventID, Status: "HAPPENING",
		CentroidLat: -6.9034443, CentroidLon: 107.6431173,
		LocationName: "Bandung", MMIScale: "V", IntensityLabel: "Sedang",
		MaxPGA: 42.5, TriggeredNodes: last.NodeCount,
		StartedAtMs: hist[0].DecidedAt,
		EventState:  last.ToState, Revision: last.Revision,
		OriginTS: hist[0].DecidedAt - 500, OriginTSSource: OnsetSourceSensor,
		IndependentCellCount: last.IndependentCells, AlgoVer: last.AlgoVer,
	}
	if err := st.UpsertEvent(ctx, row); err != nil {
		t.Fatalf("UpsertEvent: %v", err)
	}
	for i := range hist {
		if err := st.AppendStateLog(ctx, &hist[i]); err != nil {
			t.Fatalf("AppendStateLog(rev %d): %v", hist[i].Revision, err)
		}
	}
}

// pgTimelineObs menulis satu observasi lewat InsertObservation. seq <= 0 berarti
// obs_seq NULL — kasus v1 legacy yang wajib tetap tercatat.
//
// Pembersihannya lewat kolam VERIFIKASI, bukan lewat st: kolam st ditutup oleh
// pemiliknya, dan pembersihan yang bergantung padanya akan gagal diam-diam.
func pgTimelineObs(t *testing.T, st *store.Store, pool *pgxpool.Pool, nodeID string, pga float64, receivedTS, seq int64, verify string, withLoc bool) int64 {
	t.Helper()
	proto := int16(2)
	onset := receivedTS - 500
	upper := receivedTS - 480
	o := &store.Observation{
		NodeID: nodeID, SourceClass: "FIXED_ESP32", Phase: PhaseFinal,
		ProtoVer: &proto,
		PGAGal:   pga, DurMs: 300,
		PublishTS: receivedTS - 20, ReceivedTS: receivedTS,
		OnsetTS: &onset, OnsetTSUpperBound: &upper, OnsetTSSource: OnsetSourceSensor,
		VerifyResult: verify,
	}
	if seq > 0 {
		o.ObsSeq = &seq
	}
	if withLoc {
		lat, lon := -6.9034443, 107.6431173
		o.Lat, o.Lon = &lat, &lon
	}
	ctx := context.Background()
	if err := st.InsertObservation(ctx, o); err != nil {
		t.Fatalf("InsertObservation(%s @ %d): %v", nodeID, receivedTS, err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM sensor_observations WHERE node_id = $1`, nodeID)
	})
	// observation_id adalah BIGSERIAL: urutannya urutan PENULISAN dan hanya basis
	// data yang mengetahuinya. Sebuah uji yang menentukan id sendiri akan menguji
	// ketikannya sendiri, bukan ORDER BY.
	var id int64
	if err := pool.QueryRow(ctx, `
		SELECT observation_id FROM sensor_observations
		WHERE node_id = $1 AND received_ts = $2
		ORDER BY observation_id DESC LIMIT 1`, nodeID, receivedTS).Scan(&id); err != nil {
		t.Fatalf("baca observation_id: %v", err)
	}
	return id
}

// Rakitan LENGKAP terhadap Postgres nyata: keempat keluaran wajib terbaca dari
// baris yang benar-benar ditulis produksi.
//
// Yang dicegah regresinya ada di lapisan yang tidak dapat dilihat dari Go: JSONB
// menormalkan apa yang ditulisnya. EvidenceSummary.JSON() masuk sebagai satu
// deret byte, keluar dengan spasi berbeda dan urutan kunci yang tidak dijamin,
// dan historicEvidence harus tetap membacanya. Kegagalan di situ berbunyi seperti
// TEMUAN — "seluruh transisi tanpa kontributor" — alih-alih seperti bug.
func TestPGTimelineFourOutputsAgainstRealSchema(t *testing.T) {
	dsn := pgTestDBURL(t)
	pool := pgVerifyPool(t, dsn)
	st, err := store.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(st.Close)
	ctx := context.Background()

	const eventID = "00000000-0000-4000-8000-0000000006b1"
	// Riwayat SINTETIS yang sama dengan fixture, ditulis ke skema sungguhan.
	// Sintetis, dan karena itu ia bukti perangkat lunak: keempat revisi ini tidak
	// pernah diamati di lapangan, dan CONFIRMED di sini tidak memvalidasi apa pun
	// tentang kuorum nyata pada fleet satu-node.
	hist := syntheticTimelineHistory()
	pgTimelineSeed(t, st, pool, eventID, hist)

	// Observasi: satu kandidat per node kontributor, semuanya di dalam jendela
	// rev2, ditambah satu baris yang tidak diverifikasi dan satu tanpa lokasi.
	// node_id di dalam evidence_summary harus SAMA dengan node_id observasi,
	// karena keanggotaan justru relasi itu. Riwayat fixture memakai NODE-A..D,
	// jadi observasinya ditulis dengan nama itu.
	idA := pgTimelineObs(t, st, pool, "NODE-A", 22.5, fxRev1DecidedAt-400, 1000, "OK", true)
	idB := pgTimelineObs(t, st, pool, "NODE-B", 31.0, fxRev2DecidedAt-500, 1001, "OK", true)
	// obs_seq NULL (v1 legacy): anotasinya harus UNAVAILABLE, bukan perbandingan nol.
	idLegacy := pgTimelineObs(t, st, pool, "NODE-C", 29.9, fxRev2DecidedAt-300, 0, "OK", true)
	// Terkecuali, tetapi TERBACA — penyebutnya harus utuh.
	idBad := pgTimelineObs(t, st, pool, "NODE-D", 27.4, fxRev3DecidedAt-100, 1003, "ErrBadSignature", true)

	// ---- ketiga pembaca M6′, read-only ----
	row, err := st.EventByID(ctx, eventID)
	if err != nil {
		t.Fatalf("EventByID: %v", err)
	}
	got, err := st.ListStateLogForEvent(ctx, eventID)
	if err != nil {
		t.Fatalf("ListStateLogForEvent: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("riwayat = %d baris; mau 4", len(got))
	}

	prof := tlProfile()
	nodes := TimelineContributorNodes(got)
	if len(nodes) != 4 {
		t.Fatalf("node kontributor = %v; mau 4 — evidence_summary harus terurai "+
			"SETELAH bolak-balik JSONB, dan bila tidak, keanggotaan hilang seluruhnya", nodes)
	}
	fromTS, toTS, ok := TimelineWindowBounds(got, prof)
	if !ok {
		t.Fatal("batas jendela tidak terhitung dari riwayat yang tidak kosong")
	}
	obs, err := st.ListObservationsForNodesInWindow(ctx, nodes, fromTS, toTS)
	if err != nil {
		t.Fatalf("ListObservationsForNodesInWindow: %v", err)
	}

	tl := BuildTimeline(eventID, row, got, obs, nil, prof)

	// ---- keempat keluaran ----
	if tl.Coverage.EventRowStatus != OutputObserved {
		t.Errorf("keluaran 1 = %s; mau %s", tl.Coverage.EventRowStatus, OutputObserved)
	}
	if tl.Coverage.StateLogStatus != OutputObserved || tl.Coverage.StateLogRows != 4 {
		t.Errorf("keluaran 2 = %s (%d baris); mau %s dengan 4",
			tl.Coverage.StateLogStatus, tl.Coverage.StateLogRows, OutputObserved)
	}
	if tl.Coverage.EvidenceStatus != OutputObserved || tl.Coverage.RevisionsEvidenceBroken != 0 {
		t.Errorf("keluaran 3 = %s (rusak %d); mau %s tanpa yang rusak — JSONB bolak-balik",
			tl.Coverage.EvidenceStatus, tl.Coverage.RevisionsEvidenceBroken, OutputObserved)
	}
	if tl.Coverage.ObservationsStatus != OutputObserved {
		t.Errorf("keluaran 4 = %s; mau %s", tl.Coverage.ObservationsStatus, OutputObserved)
	}
	// Emisi TIDAK dibaca di sini, dan keempat keluaran tetap OBSERVED: itu
	// justru sifat "opsional" dalam bentuk yang dapat diperiksa (D-015 batasan 1).
	if tl.Emissions != nil {
		t.Errorf("Emissions = %v; mau nil ketika tidak dibaca", tl.Emissions)
	}

	// Keempat baris yang ditulis TERBACA, dan tidak satu pun hilang diam-diam.
	if tl.Coverage.ObservationRowsRead != 4 {
		for _, o := range obs {
			t.Logf("terbaca: id=%d node=%s ts=%d verify=%s", o.ObservationID, o.NodeID, o.ReceivedTS, o.VerifyResult)
		}
		t.Fatalf("ObservationRowsRead = %d; mau 4", tl.Coverage.ObservationRowsRead)
	}
	if tl.Coverage.ExcludedRows != 1 {
		t.Errorf("ExcludedRows = %d; mau 1 (ErrBadSignature) — barisnya harus TERBACA "+
			"lalu DIHITUNG sebagai yang dibuang", tl.Coverage.ExcludedRows)
	}

	byID := make(map[int64]ObservationCandidate, len(tl.Observations))
	for _, c := range tl.Observations {
		byID[c.ObservationID] = c
	}
	if _, ok := byID[idBad]; ok {
		t.Errorf("obs %d (ErrBadSignature) menjadi kandidat; ia harus terkecuali", idBad)
	}
	// Ambiguitas di sini adalah keadaan NORMAL, bukan kasus tepi: jendela
	// keanggotaan-dan-waktu selebar CorrelationWindowMs + toleransi (22 s) dan
	// keempat revisi berjarak 3000/1200 ms, jadi tiga jendela pertama bertumpang-
	// tindih lebar. Yang diperiksa karena itu bukan "satu revisi" melainkan
	// bahwa daftar revisinya dilaporkan UTUH.
	assertAttribution(t, byID, idA, TraceAmbiguous, []int{1, 2, 3})
	assertAttribution(t, byID, idB, TraceAmbiguous, []int{2, 3})
	// obs_seq NULL melewati batas basis data sebagai NULL.
	if c, ok := byID[idLegacy]; !ok {
		t.Errorf("obs %d (obs_seq NULL) bukan kandidat", idLegacy)
	} else {
		if c.ObsSeq != nil {
			t.Errorf("obs %d: ObsSeq = %d; mau nil — kolomnya ditulis NULL", idLegacy, *c.ObsSeq)
		}
		if c.ObsSeqLink != ObsSeqUnavailable {
			t.Errorf("obs %d: ObsSeqLink = %s; mau %s — tanpa obs_seq tidak ada yang "+
				"dapat dibandingkan, dan nol bukan jawabannya", idLegacy, c.ObsSeqLink, ObsSeqUnavailable)
		}
	}

	// Provenance toleransi terbawa ke cakupan, dan nilainya nilai M1′ — bukan
	// toleransi ilmiah baru (D-015 batasan 2).
	if tl.Coverage.LinkToleranceMs != defaultLinkToleranceMs ||
		tl.Coverage.ToleranceProvenance != TolFromM1Default {
		t.Errorf("toleransi = %d ms (%s); mau %d ms (%s)",
			tl.Coverage.LinkToleranceMs, tl.Coverage.ToleranceProvenance,
			defaultLinkToleranceMs, TolFromM1Default)
	}
	if len(tl.Coverage.AlgoVersRow) != 1 || tl.Coverage.AlgoVersRow[0] != rsAlgoVer {
		t.Errorf("AlgoVersRow = %v; mau [%s] — algo_ver PER BARIS terbawa dari kolomnya",
			tl.Coverage.AlgoVersRow, rsAlgoVer)
	}
	if tl.Coverage.TerminalState != string(StateResolved) {
		t.Errorf("TerminalState = %q; mau RESOLVED — barisnya memang ada di riwayat",
			tl.Coverage.TerminalState)
	}
}

// Emisi terbaca dari alert_emissions NYATA, dan kedua kekuatan buktinya berbeda.
//
// Ini yang membedakan uji ini dari fixture: baris pasca-000008 membawa
// event_id + event_revision, baris pra-000008 tidak membawa keduanya, dan
// perbedaan itu adalah KOLOM NULLABLE — hanya Postgres yang dapat membuktikan
// bahwa NULL tiba sebagai NULL alih-alih sebagai nol atau string kosong. Sebuah
// event_revision yang terbaca 0 akan tampak menunjuk revisi 0, yang tidak pernah
// ada.
//
// Emisi tetap OPSIONAL: keempat keluaran wajib sudah OBSERVED tanpa satu pun
// baris di sini (D-015 batasan 1), dan bagian ini tidak boleh mengubahnya.
func TestPGTimelineEmissionsAreOptionalEvidenceNotACriterion(t *testing.T) {
	dsn := pgTestDBURL(t)
	pool := pgVerifyPool(t, dsn)
	st, err := store.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(st.Close)
	ctx := context.Background()

	const eventID = "00000000-0000-4000-8000-0000000006b2"
	hist := syntheticTimelineHistory()
	pgTimelineSeed(t, st, pool, eventID, hist)

	// Baris EKSAK (pasca-000008): event_id dan event_revision keduanya terisi.
	evState := string(StateUnconfirmed)
	rev := 1
	ws := 7
	eid := eventID
	exact := &store.AlertEmission{
		EventID: &eid, AlertType: "EARTHQUAKE_ADVISORY", Status: "SENT",
		NodeCount: 1, IsSevere: false, Audience: "GEO_TOPIC_ALL",
		DecidedAt: fxRev1DecidedAt, AlgoVer: rsAlgoVer,
		WSClientCount: &ws, EventState: &evState, EventRevision: &rev,
	}
	if err := st.InsertAlertEmission(ctx, exact); err != nil {
		t.Fatalf("InsertAlertEmission (eksak): %v", err)
	}
	// Baris PRA-000008: tanpa event_id, tanpa event_state, tanpa event_revision,
	// dan tanpa ws_client_count. Ia tetap bukti bahwa sebuah frame diputuskan.
	legacy := &store.AlertEmission{
		AlertType: "EARTHQUAKE_ADVISORY", Status: "SENT",
		NodeCount: 1, IsSevere: false, Audience: "GEO_TOPIC_ALL",
		DecidedAt: fxRev2DecidedAt, AlgoVer: "phase1-1.0",
	}
	if err := st.InsertAlertEmission(ctx, legacy); err != nil {
		t.Fatalf("InsertAlertEmission (legacy): %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM alert_emissions WHERE decided_at = ANY($1)`,
			[]int64{fxRev1DecidedAt, fxRev2DecidedAt})
	})

	got, err := st.ListStateLogForEvent(ctx, eventID)
	if err != nil {
		t.Fatalf("ListStateLogForEvent: %v", err)
	}
	prof := tlProfile()
	tol := prof.linkTolerance()
	// Jendela emisi SENGAJA lebih sempit dari jendela observasi: sebuah emisi
	// diputuskan PADA transisinya, jadi melebarkannya sepanjang satu jendela
	// korelasi tidak punya alasan.
	emis, err := st.ListEmissionsForTrace(ctx, got[0].DecidedAt-tol, got[len(got)-1].DecidedAt+tol)
	if err != nil {
		t.Fatalf("ListEmissionsForTrace: %v", err)
	}
	if len(emis) < 2 {
		t.Fatalf("baris emisi = %d; mau >= 2 (keduanya baru ditulis)", len(emis))
	}

	tl := BuildTimeline(eventID, nil, got, nil, emis, prof)

	// Keempat keluaran tidak bergantung pada bagian ini. Keluaran 1 NOT_OBSERVABLE
	// di sini karena barisnya sengaja tidak dibaca — dan keluaran 2 dan 3 tetap
	// OBSERVED, yang adalah keseluruhan pokoknya.
	if tl.Coverage.StateLogStatus != OutputObserved || tl.Coverage.EvidenceStatus != OutputObserved {
		t.Errorf("keluaran 2/3 = %s/%s; mau keduanya %s — emisi tidak boleh menjadi "+
			"gerbang keluaran wajib", tl.Coverage.StateLogStatus, tl.Coverage.EvidenceStatus, OutputObserved)
	}
	if len(tl.Emissions) != 4 {
		t.Fatalf("tautan emisi = %d; mau 4 (satu per revisi)", len(tl.Emissions))
	}

	// rev1 tertaut EKSAK: event_id dan event_revision keduanya cocok.
	if tl.Emissions[0].Outcome != EmissionByID {
		t.Errorf("rev1 outcome = %s; mau %s — barisnya membawa event_id DAN "+
			"event_revision", tl.Emissions[0].Outcome, EmissionByID)
	}
	if tl.Emissions[0].WSClientCount == nil || *tl.Emissions[0].WSClientCount != 7 {
		t.Errorf("rev1 ws_client_count = %v; mau 7", tl.Emissions[0].WSClientCount)
	}
	// rev2 hanya dapat tertaut lewat WAKTU: barisnya tidak membawa identitas.
	if tl.Emissions[1].Outcome != EmissionByTimeOnly {
		t.Errorf("rev2 outcome = %s; mau %s — baris pra-000008 tidak punya "+
			"event_revision untuk dicocokkan", tl.Emissions[1].Outcome, EmissionByTimeOnly)
	}
	if tl.Emissions[1].WSClientCount != nil {
		t.Errorf("rev2 ws_client_count = %d; mau nil — NULL berarti hasil kirim TIDAK "+
			"PERNAH DILAPORKAN, bukan nol klien", *tl.Emissions[1].WSClientCount)
	}
	// rev3 berjarak 1200 ms dari rev2, di dalam toleransi 2000 ms: baris legacy
	// yang sama sah diklaim KEDUA revisi, dan itu harus DITANDAI — bukan dihitung
	// sebagai dua emisi.
	if tl.Emissions[2].Outcome == EmissionByTimeOnly {
		if !tl.Emissions[1].SharedTimeOnlyLink || !tl.Emissions[2].SharedTimeOnlyLink {
			t.Errorf("tautan hanya-waktu yang dibagi tidak ditandai: rev2=%v rev3=%v — "+
				"satu baris emisi tidak boleh terbaca sebagai dua",
				tl.Emissions[1].SharedTimeOnlyLink, tl.Emissions[2].SharedTimeOnlyLink)
		}
		if tl.Emissions[1].EmissionID != tl.Emissions[2].EmissionID {
			t.Errorf("emission_id rev2=%d rev3=%d; penandaan bersama hanya bermakna bila "+
				"barisnya memang sama", tl.Emissions[1].EmissionID, tl.Emissions[2].EmissionID)
		}
	}
	// rev4 jatuh 90 s kemudian: tidak ada frame, dan itu BUKAN kegagalan.
	if tl.Emissions[3].Outcome != EmissionMissing {
		t.Errorf("rev4 outcome = %s; mau %s — hanya transisi yang menghasilkan frame "+
			"yang punya baris emisi", tl.Emissions[3].Outcome, EmissionMissing)
	}
}
