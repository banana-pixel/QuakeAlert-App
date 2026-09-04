package event

import (
	"testing"

	"github.com/banana-pixel/quakealert/server/internal/dispatch"
	"github.com/banana-pixel/quakealert/server/internal/store"
)

// Uji P4-M1′. Yang diperiksa berkas ini, dan hanya ini:
//
//  1. PENYEBUT: setiap baris jendela masuk TEPAT satu keranjang — di bawah
//     lantai, terkecuali, atau tertelusuri — dan ketiganya berjumlah TotalRows.
//     Tidak ada baris yang boleh hilang diam-diam.
//  2. TAUTAN: satu transisi UNCONFIRMED ditemukan, banyak dilaporkan AMBIGU, nol
//     dilaporkan NO_UNCONFIRMED_TRANSITION — dan pemetaan N:1 tidak dihitung
//     sebagai kegagalan.
//  3. EMISI: tautan eksak (event_id+revision) dibedakan dari tautan hanya-waktu
//     (pra-000008), dan ws_client_count NULL dibedakan dari nol.
//  4. BATAS: counter yang tidak diketahui tidak pernah menjadi nol, transisi
//     bertepi-jendela ditandai, dan fleet satu-node dinyatakan.

// ---- pembangun ------------------------------------------------------------

// trObs membangun satu baris ledger. rpRow dipakai ulang (replay_test.go) supaya
// kedua alat P4 melihat bentuk baris yang sama persis.
func trObs(rows ...rpRow) []store.ReplayObservation { return rpObs(rows...) }

// trUnconfirmed membangun satu baris event_state_log yang masuk ke UNCONFIRMED,
// dengan evidence_summary yang diserialkan oleh EvidenceSummary.JSON() — penulis
// PRODUKSI, bukan JSON yang ditulis tangan.
func trUnconfirmed(eventID string, rev int, decidedAt int64, nodes ...string) store.EventStateLog {
	return trTransition(eventID, rev, decidedAt, string(StateUnconfirmed), nodes...)
}

func trTransition(eventID string, rev int, decidedAt int64, to string, nodes ...string) store.EventStateLog {
	ev := EvidenceSummary{OriginTSSource: OnsetSourceSensor}
	for i, n := range nodes {
		seq := int64(1000 + i)
		ev.Contributors = append(ev.Contributors, ContributorEvidence{
			NodeID: n, PeakPGA: 20, Phase: PhaseFinal,
			OnsetTS: decidedAt - 500, OnsetSource: OnsetSourceSensor, ObsSeq: &seq,
		})
	}
	ev.IndependentCells = len(nodes)
	from := string(StateDetected)
	peak := 20.0
	return store.EventStateLog{
		EventID: eventID, Revision: rev,
		FromState: &from, ToState: to, Reason: ReasonFloorMet,
		DecidedAt: decidedAt, NodeCount: len(nodes), IndependentCells: len(nodes),
		PeakPGA: &peak, EvidenceSummary: ev.JSON(), AlgoVer: rsAlgoVer,
	}
}

// trAdvisory membangun baris alert_emissions PASCA-000008: beridentitas.
func trAdvisory(id int64, eventID string, rev int, decidedAt int64, wsClients *int) store.TraceEmission {
	e, r := eventID, rev
	state := string(StateUnconfirmed)
	return store.TraceEmission{
		EmissionID: id, EventID: &e, AlertType: dispatch.TypeAdvisory,
		Status: "ADVISORY", Audience: "NONE", DecidedAt: decidedAt, AlgoVer: rsAlgoVer,
		EventState: &state, EventRevision: &r, WSClientCount: wsClients,
	}
}

// trAdvisoryLegacy membangun baris PRA-000008: tanpa event_id dan tanpa
// event_state/event_revision. Ia hanya dapat ditautkan menurut waktu.
func trAdvisoryLegacy(id int64, decidedAt int64) store.TraceEmission {
	return store.TraceEmission{
		EmissionID: id, AlertType: dispatch.TypeAdvisory,
		Status: "ADVISORY", Audience: "NONE", DecidedAt: decidedAt, AlgoVer: "phase1-1.0",
	}
}

func intp(v int) *int { return &v }

func trProfile() TraceProfile { return TraceProfile{Options: defaultOptions()} }

// ---- 1. penyebut -----------------------------------------------------------

func TestTraceBucketsSumToTotalRows(t *testing.T) {
	obs := trObs(
		rpRow{id: 1, node: "A", pga: 2.0, received: 1000, upper: 900, lat: -6.8, lon: 107.5},   // di bawah lantai
		rpRow{id: 2, node: "A", pga: 20.0, received: 2000, upper: 1900, lat: -6.8, lon: 107.5}, // tertelusuri
		rpRow{id: 3, node: "A", pga: 20.0, received: 3000, upper: 2900, noLoc: true},           // terkecuali
		rpRow{id: 4, node: "A", pga: 20.0, received: 4000, upper: 3900, lat: -6.8, lon: 107.5, verify: "ErrBadSignature"},
	)
	hist := []store.EventStateLog{trUnconfirmed("e1", 1, 2100, "A")}
	rep := Trace(obs, hist, nil, trProfile(), 4)

	if rep.TotalRows != 4 {
		t.Fatalf("TotalRows = %d; mau 4", rep.TotalRows)
	}
	if got := rep.BelowFloor + len(rep.Excluded) + len(rep.Traces); got != rep.TotalRows {
		t.Fatalf("keranjang berjumlah %d; mau %d (below=%d excluded=%d traced=%d)",
			got, rep.TotalRows, rep.BelowFloor, len(rep.Excluded), len(rep.Traces))
	}
	if rep.BelowFloor != 1 {
		t.Errorf("BelowFloor = %d; mau 1", rep.BelowFloor)
	}
	if c := rep.ExcludeCounts(); c[SkipNoLocation] != 1 || c[SkipVerifyNotOK] != 1 {
		t.Errorf("ExcludeCounts = %v; mau NODE_LOCATION_NULL=1 VERIFY_RESULT_NOT_OK=1", c)
	}
}

// Lantai adalah >=, bukan >: classify() memakai `< MinPGAGal` untuk DETECTED,
// jadi sebuah baris yang TEPAT di lantai memenuhi syarat dan harus ditelusuri.
func TestTraceFloorIsInclusive(t *testing.T) {
	obs := trObs(rpRow{id: 1, node: "A", pga: MinPGAGal, received: 2000, upper: 1900, lat: -6.8, lon: 107.5})
	rep := Trace(obs, []store.EventStateLog{trUnconfirmed("e1", 1, 2050, "A")}, nil, trProfile(), 1)
	if rep.BelowFloor != 0 {
		t.Fatalf("pga tepat di lantai dihitung di bawah lantai")
	}
	if len(rep.Traces) != 1 || rep.Traces[0].Outcome != TraceTraced {
		t.Fatalf("hasil = %+v; mau satu TRACED", rep.Traces)
	}
}

// Baris terkecuali TIDAK boleh hilang: ia dilaporkan satu per satu dengan pga dan
// waktunya, karena NODE_LOCATION_NULL adalah pemicu NYATA yang dibuang gerbang
// gagal-tertutup di Ingest.
func TestTraceExcludedRowsCarryPGAAndTime(t *testing.T) {
	obs := trObs(rpRow{id: 7, node: "A", pga: 51.25, received: 4321, upper: 4000, noLoc: true})
	rep := Trace(obs, nil, nil, trProfile(), 1)
	if len(rep.Excluded) != 1 {
		t.Fatalf("Excluded = %d; mau 1", len(rep.Excluded))
	}
	e := rep.Excluded[0]
	if e.ObservationID != 7 || e.PGAGal != 51.25 || e.ReceivedTS != 4321 || e.Reason != SkipNoLocation {
		t.Errorf("baris terkecuali = %+v", e)
	}
}

func TestTraceNoOnsetAnchorIsExcludedNotDropped(t *testing.T) {
	o := rpRow{id: 9, node: "A", pga: 30, received: 5000, lat: -6.8, lon: 107.5}.obs()
	o.OnsetTS, o.OnsetTSUpperBound = nil, nil
	rep := Trace([]store.ReplayObservation{o}, nil, nil, trProfile(), 1)
	if len(rep.Excluded) != 1 || rep.Excluded[0].Reason != SkipNoOnsetAnchor {
		t.Fatalf("Excluded = %+v; mau satu NO_ONSET_ANCHOR", rep.Excluded)
	}
}

// ---- 2. tautan observasi -> transisi ---------------------------------------

// Pemetaan N:1 adalah rancangan, bukan cacat: UNCONFIRMED -> UNCONFIRMED bukan
// transisi yang sah, jadi banyak observasi dari satu node di dalam satu jendela
// korelasi BERBAGI satu transisi. Ketiganya harus TRACED, dan transisi
// berbedanya harus satu.
func TestTraceManyObservationsShareOneTransition(t *testing.T) {
	obs := trObs(
		rpRow{id: 1, node: "A", pga: 20, received: 2000, upper: 1900, lat: -6.8, lon: 107.5},
		rpRow{id: 2, node: "A", pga: 40, received: 2400, upper: 1900, lat: -6.8, lon: 107.5},
		rpRow{id: 3, node: "A", pga: 73, received: 2800, upper: 1900, lat: -6.8, lon: 107.5},
	)
	hist := []store.EventStateLog{trUnconfirmed("e1", 1, 2100, "A")}
	rep := Trace(obs, hist, nil, trProfile(), 3)

	if got := rep.Outcomes()[TraceTraced]; got != 3 {
		t.Fatalf("TRACED = %d; mau 3 (hasil=%v)", got, rep.Outcomes())
	}
	if got := rep.DistinctTransitions(); got != 1 {
		t.Errorf("DistinctTransitions = %d; mau 1", got)
	}
	if len(rep.Unattributed) != 0 {
		t.Errorf("Unattributed = %+v; mau kosong", rep.Unattributed)
	}
}

// Lag boleh NEGATIF: baris yang tiba SETELAH event sudah UNCONFIRMED bertransisi
// lebih dulu. Itu bukan anomali dan tidak boleh membatalkan tautan.
func TestTraceNegativeLagIsStillTraced(t *testing.T) {
	obs := trObs(rpRow{id: 2, node: "A", pga: 40, received: 5000, upper: 4900, lat: -6.8, lon: 107.5})
	hist := []store.EventStateLog{trUnconfirmed("e1", 1, 2100, "A")}
	rep := Trace(obs, hist, nil, trProfile(), 1)
	if rep.Traces[0].Outcome != TraceTraced {
		t.Fatalf("hasil = %s; mau TRACED", rep.Traces[0].Outcome)
	}
	if got := rep.Traces[0].LagMs; got != 2100-5000 {
		t.Errorf("LagMs = %d; mau %d", got, 2100-5000)
	}
}

// Dua transisi berbeda yang keduanya memuat node ini di dalam jendela: tautannya
// TIDAK DAPAT DIPUTUSKAN. Bukan TRACED, bukan NO_TRANSITION.
func TestTraceMultipleCandidatesIsAmbiguous(t *testing.T) {
	obs := trObs(rpRow{id: 1, node: "A", pga: 40, received: 3000, upper: 2900, lat: -6.8, lon: 107.5})
	hist := []store.EventStateLog{
		trUnconfirmed("e1", 1, 2500, "A"),
		trUnconfirmed("e2", 1, 3400, "A"),
	}
	rep := Trace(obs, hist, nil, trProfile(), 1)
	tr := rep.Traces[0]
	if tr.Outcome != TraceAmbiguous {
		t.Fatalf("hasil = %s; mau %s", tr.Outcome, TraceAmbiguous)
	}
	if len(tr.Candidates) != 2 || tr.Candidates[0] != "e1#1" || tr.Candidates[1] != "e2#1" {
		t.Errorf("Candidates = %v; mau [e1#1 e2#1]", tr.Candidates)
	}
	if tr.EmissionOutcome != EmissionNotApplicable {
		t.Errorf("EmissionOutcome = %s; mau %s", tr.EmissionOutcome, EmissionNotApplicable)
	}
	// Kedua kandidat TERATRIBUSI: keduanya dijangkau observasi ini.
	if len(rep.Unattributed) != 0 {
		t.Errorf("Unattributed = %+v; mau kosong", rep.Unattributed)
	}
}

// Transisi yang node_id-nya lain sama sekali: tidak ada tautan, dan tidak ada
// kandidat terdekat yang pantas dilaporkan.
func TestTraceNoTransitionWhenNodeNeverContributed(t *testing.T) {
	obs := trObs(rpRow{id: 1, node: "A", pga: 40, received: 3000, upper: 2900, lat: -6.8, lon: 107.5})
	hist := []store.EventStateLog{trUnconfirmed("e1", 1, 3050, "B")}
	rep := Trace(obs, hist, nil, trProfile(), 1)
	tr := rep.Traces[0]
	if tr.Outcome != TraceNoTransition {
		t.Fatalf("hasil = %s; mau %s", tr.Outcome, TraceNoTransition)
	}
	if tr.NearestCandidate != "" {
		t.Errorf("NearestCandidate = %q; mau kosong (node tak pernah menyumbang)", tr.NearestCandidate)
	}
}

// Node cocok tetapi transisinya JAUH di luar jendela: dilaporkan sebagai
// NO_TRANSITION dengan kandidat terdekat, supaya "tidak ada transisi" dapat
// dibedakan dari "jendela tautan terlalu sempit".
func TestTraceNearestCandidateDistinguishesNarrowWindow(t *testing.T) {
	obs := trObs(rpRow{id: 1, node: "A", pga: 40, received: 100000, upper: 99000, lat: -6.8, lon: 107.5})
	hist := []store.EventStateLog{trUnconfirmed("e1", 1, 1000, "A")}
	rep := Trace(obs, hist, nil, trProfile(), 1)
	tr := rep.Traces[0]
	if tr.Outcome != TraceNoTransition {
		t.Fatalf("hasil = %s; mau %s", tr.Outcome, TraceNoTransition)
	}
	if tr.NearestCandidate != "e1#1" {
		t.Errorf("NearestCandidate = %q; mau e1#1", tr.NearestCandidate)
	}
	if tr.NearestCandidateOffMs != 1000-100000 {
		t.Errorf("NearestCandidateOffMs = %d; mau %d", tr.NearestCandidateOffMs, 1000-100000)
	}
}

// Hanya transisi ke UNCONFIRMED yang dipertimbangkan. Sebuah CONFIRMED atau
// RESOLVED pada node yang sama bukan tautan yang dicari kriteria ini.
func TestTraceIgnoresNonUnconfirmedTransitions(t *testing.T) {
	obs := trObs(rpRow{id: 1, node: "A", pga: 40, received: 3000, upper: 2900, lat: -6.8, lon: 107.5})
	hist := []store.EventStateLog{
		trTransition("e1", 2, 3050, string(StateConfirmed), "A"),
		trTransition("e1", 3, 3080, string(StateResolved), "A"),
	}
	rep := Trace(obs, hist, nil, trProfile(), 1)
	if rep.Traces[0].Outcome != TraceNoTransition {
		t.Fatalf("hasil = %s; mau %s", rep.Traces[0].Outcome, TraceNoTransition)
	}
	if rep.StateLogRows != 2 {
		t.Errorf("StateLogRows = %d; mau 2 (baris mentah tetap dilaporkan)", rep.StateLogRows)
	}
}

// evidence_summary yang tidak dapat diurai TIDAK membuat barisnya hilang: ia
// muncul sebagai transisi tak-teratribusi. Baris yang rusak harus terlihat.
func TestTraceUnparseableEvidenceBecomesUnattributed(t *testing.T) {
	obs := trObs(rpRow{id: 1, node: "A", pga: 40, received: 3000, upper: 2900, lat: -6.8, lon: 107.5})
	row := trUnconfirmed("e1", 1, 3050, "A")
	row.EvidenceSummary = []byte("{bukan json")
	rep := Trace(obs, []store.EventStateLog{row}, nil, trProfile(), 1)

	if rep.Traces[0].Outcome != TraceNoTransition {
		t.Errorf("hasil = %s; mau %s", rep.Traces[0].Outcome, TraceNoTransition)
	}
	if len(rep.Unattributed) != 1 || rep.Unattributed[0].EventID != "e1" {
		t.Fatalf("Unattributed = %+v; mau satu e1", rep.Unattributed)
	}
}

// Arah kebalikan: transisi di tepi bawah jendela ditandai AtWindowEdge, karena
// pada jendela N-baris terakhir penyebab paling sering justru sah — observasi
// pemicunya ada SEBELUM tepi jendela.
func TestTraceUnattributedAtWindowEdgeIsFlagged(t *testing.T) {
	obs := trObs(rpRow{id: 1, node: "A", pga: 40, received: 100000, upper: 99000, lat: -6.8, lon: 107.5})
	hist := []store.EventStateLog{
		trUnconfirmed("edge", 1, 100000, "B"), // di dalam jendela+toleransi dari FromTS
		trUnconfirmed("far", 1, 9000000, "B"), // jauh setelahnya
	}
	rep := Trace(obs, hist, nil, trProfile(), 1)
	if len(rep.Unattributed) != 2 {
		t.Fatalf("Unattributed = %d; mau 2", len(rep.Unattributed))
	}
	byID := map[string]UnattributedTransition{}
	for _, u := range rep.Unattributed {
		byID[u.EventID] = u
	}
	if !byID["edge"].AtWindowEdge {
		t.Errorf("transisi tepi TIDAK ditandai AtWindowEdge")
	}
	if byID["far"].AtWindowEdge {
		t.Errorf("transisi jauh salah ditandai AtWindowEdge")
	}
	if got := byID["edge"].NodeIDs; len(got) != 1 || got[0] != "B" {
		t.Errorf("NodeIDs = %v; mau [B]", got)
	}
}

// ---- 3. tautan transisi -> emisi -------------------------------------------

func TestTraceEmissionMatchedByEventIDAndRevision(t *testing.T) {
	obs := trObs(rpRow{id: 1, node: "A", pga: 40, received: 3000, upper: 2900, lat: -6.8, lon: 107.5})
	hist := []store.EventStateLog{trUnconfirmed("e1", 1, 3050, "A")}
	emis := []store.TraceEmission{trAdvisory(11, "e1", 1, 3055, intp(2))}

	rep := Trace(obs, hist, emis, trProfile(), 1)
	tr := rep.Traces[0]
	if tr.EmissionOutcome != EmissionByID {
		t.Fatalf("EmissionOutcome = %s; mau %s", tr.EmissionOutcome, EmissionByID)
	}
	if tr.EmissionID != 11 {
		t.Errorf("EmissionID = %d; mau 11", tr.EmissionID)
	}
	if tr.WSClientCount == nil || *tr.WSClientCount != 2 {
		t.Errorf("WSClientCount = %v; mau 2", tr.WSClientCount)
	}
}

// Revisi yang SALAH bukan kecocokan: sebuah baris emisi untuk rev2 tidak
// membuktikan frame rev1 pernah dikirim.
func TestTraceEmissionWrongRevisionIsMissing(t *testing.T) {
	obs := trObs(rpRow{id: 1, node: "A", pga: 40, received: 3000, upper: 2900, lat: -6.8, lon: 107.5})
	hist := []store.EventStateLog{trUnconfirmed("e1", 1, 3050, "A")}
	emis := []store.TraceEmission{trAdvisory(11, "e1", 2, 3055, intp(2))}

	rep := Trace(obs, hist, emis, trProfile(), 1)
	if got := rep.Traces[0].EmissionOutcome; got != EmissionMissing {
		t.Fatalf("EmissionOutcome = %s; mau %s", got, EmissionMissing)
	}
}

// Baris pra-000008 tidak membawa identitas apa pun; ia hanya dapat ditautkan
// menurut waktu, dan HARUS dilabeli sebagai bukti yang lebih lemah.
func TestTraceEmissionLegacyRowIsTimeOnly(t *testing.T) {
	obs := trObs(rpRow{id: 1, node: "A", pga: 40, received: 3000, upper: 2900, lat: -6.8, lon: 107.5})
	hist := []store.EventStateLog{trUnconfirmed("e1", 1, 3050, "A")}
	emis := []store.TraceEmission{trAdvisoryLegacy(9, 3060)}

	rep := Trace(obs, hist, emis, trProfile(), 1)
	tr := rep.Traces[0]
	if tr.EmissionOutcome != EmissionByTimeOnly {
		t.Fatalf("EmissionOutcome = %s; mau %s", tr.EmissionOutcome, EmissionByTimeOnly)
	}
	if tr.EmissionID != 9 {
		t.Errorf("EmissionID = %d; mau 9", tr.EmissionID)
	}
	if tr.WSClientCount != nil {
		t.Errorf("WSClientCount = %v; mau nil (kolom 000007 belum ada)", tr.WSClientCount)
	}
}

// Yang EKSAK menang: bila baris beridentitas cocok ada, kehadiran baris legacy
// yang juga dekat waktunya tidak boleh menurunkan kekuatan laporan.
func TestTraceEmissionExactWinsOverTimeOnly(t *testing.T) {
	obs := trObs(rpRow{id: 1, node: "A", pga: 40, received: 3000, upper: 2900, lat: -6.8, lon: 107.5})
	hist := []store.EventStateLog{trUnconfirmed("e1", 1, 3050, "A")}
	emis := []store.TraceEmission{
		trAdvisoryLegacy(9, 3051),
		trAdvisory(11, "e1", 1, 3400, intp(0)),
	}
	rep := Trace(obs, hist, emis, trProfile(), 1)
	tr := rep.Traces[0]
	if tr.EmissionOutcome != EmissionByID || tr.EmissionID != 11 {
		t.Fatalf("hasil = %s id=%d; mau %s id=11", tr.EmissionOutcome, tr.EmissionID, EmissionByID)
	}
}

// ws_client_count = 0 BUKAN kegagalan dan BUKAN NULL: baris emisinya sendiri
// membuktikan frame diputuskan dan disiarkan; nol klien berarti tidak ada yang
// terhubung. Keduanya harus tetap dapat dibedakan di hasil.
func TestTraceZeroWSClientsIsNotMissing(t *testing.T) {
	obs := trObs(rpRow{id: 1, node: "A", pga: 40, received: 3000, upper: 2900, lat: -6.8, lon: 107.5})
	hist := []store.EventStateLog{trUnconfirmed("e1", 1, 3050, "A")}
	emis := []store.TraceEmission{trAdvisory(11, "e1", 1, 3055, intp(0))}

	rep := Trace(obs, hist, emis, trProfile(), 1)
	tr := rep.Traces[0]
	if tr.EmissionOutcome != EmissionByID {
		t.Fatalf("EmissionOutcome = %s; mau %s", tr.EmissionOutcome, EmissionByID)
	}
	if tr.WSClientCount == nil || *tr.WSClientCount != 0 {
		t.Fatalf("WSClientCount = %v; mau 0 yang BUKAN nil", tr.WSClientCount)
	}
}

// Penyaring emisi memakai alert_type, BUKAN audience: sebuah EARTHQUAKE_ALERT di
// jendela yang sama tidak boleh dihitung sebagai frame advisory.
func TestTraceEmissionIgnoresNonAdvisoryTypes(t *testing.T) {
	obs := trObs(rpRow{id: 1, node: "A", pga: 40, received: 3000, upper: 2900, lat: -6.8, lon: 107.5})
	hist := []store.EventStateLog{trUnconfirmed("e1", 1, 3050, "A")}
	alert := trAdvisory(11, "e1", 1, 3055, intp(2))
	alert.AlertType = dispatch.TypeAlert
	rep := Trace(obs, hist, []store.TraceEmission{alert}, trProfile(), 1)

	if got := rep.Traces[0].EmissionOutcome; got != EmissionMissing {
		t.Errorf("EmissionOutcome = %s; mau %s", got, EmissionMissing)
	}
	if rep.EmissionRows != 1 {
		t.Errorf("EmissionRows = %d; mau 1 (baris mentah tetap dilaporkan)", rep.EmissionRows)
	}
}

// ---- 4. batas yang wajib dinyatakan ---------------------------------------

// Counter yang tidak disediakan operator TIDAK boleh terbaca sebagai nol.
func TestTraceCountersUnknownIsNotZero(t *testing.T) {
	var c TraceCounters
	if c.Known {
		t.Fatal("TraceCounters kosong menyatakan dirinya diketahui")
	}
}

func TestTraceCountersFromStatsJSON(t *testing.T) {
	raw := []byte(`{
		"event_persist_dropped_total": 3,
		"event_upsert_failures_total": 1,
		"event_state_log_failures_total": 2,
		"event_state_log_skipped_total": 7,
		"event_transitions_to_unconfirmed_total": 11
	}`)
	c, err := CountersFromStatsJSON(raw)
	if err != nil {
		t.Fatalf("CountersFromStatsJSON galat: %v", err)
	}
	if !c.Known {
		t.Error("Known = false setelah pengurai berhasil")
	}
	if c.PersistDropped != 3 || c.UpsertFailures != 1 || c.StateLogFailures != 2 || c.StateLogSkipped != 7 {
		t.Errorf("counter = %+v", c)
	}
}

func TestTraceCountersFromStatsJSONRejectsGarbage(t *testing.T) {
	if _, err := CountersFromStatsJSON([]byte("bukan json")); err == nil {
		t.Fatal("JSON rusak diterima")
	}
}

// S2: fleet satu-node harus DINYATAKAN. UNCONFIRMED adalah state tujuan yang
// benar di sana, dan ketiadaan CONFIRMED bukan cacat.
func TestTraceSingleNodeFleetIsDeclared(t *testing.T) {
	obs := trObs(
		rpRow{id: 1, node: "A", pga: 40, received: 3000, upper: 2900, lat: -6.8, lon: 107.5},
		rpRow{id: 2, node: "A", pga: 45, received: 3400, upper: 2900, lat: -6.8, lon: 107.5},
	)
	rep := Trace(obs, []store.EventStateLog{trUnconfirmed("e1", 1, 3050, "A")}, nil, trProfile(), 2)
	if !rep.SingleNodeFleet {
		t.Fatal("SingleNodeFleet = false pada jendela satu node")
	}
	if len(rep.NodeIDs) != 1 || rep.NodeIDs[0] != "A" {
		t.Errorf("NodeIDs = %v; mau [A]", rep.NodeIDs)
	}
}

func TestTraceMultiNodeFleetIsNotSingle(t *testing.T) {
	obs := trObs(
		rpRow{id: 1, node: "A", pga: 40, received: 3000, upper: 2900, lat: -6.8, lon: 107.5},
		rpRow{id: 2, node: "B", pga: 45, received: 3100, upper: 2900, lat: -6.9, lon: 107.6},
	)
	rep := Trace(obs, []store.EventStateLog{trUnconfirmed("e1", 1, 3150, "A", "B")}, nil, trProfile(), 2)
	if rep.SingleNodeFleet {
		t.Fatal("SingleNodeFleet = true pada jendela dua node")
	}
	if len(rep.NodeIDs) != 2 {
		t.Errorf("NodeIDs = %v; mau dua", rep.NodeIDs)
	}
}

// RequestedN dan TotalRows berbeda bila tabel lebih pendek dari N. Pembaca harus
// tahu penyebut mana yang ia lihat.
func TestTraceRequestedNIsReportedSeparatelyFromTotalRows(t *testing.T) {
	obs := trObs(rpRow{id: 1, node: "A", pga: 40, received: 3000, upper: 2900, lat: -6.8, lon: 107.5})
	rep := Trace(obs, nil, nil, trProfile(), 500)
	if rep.RequestedN != 500 || rep.TotalRows != 1 {
		t.Fatalf("RequestedN=%d TotalRows=%d; mau 500/1", rep.RequestedN, rep.TotalRows)
	}
}

// Jendela kosong bukan galat dan bukan keberhasilan: ia laporan atas nol baris,
// dan harus dapat dibedakan dari laporan nol tautan atas jendela berisi.
func TestTraceEmptyWindowIsReportedNotFatal(t *testing.T) {
	rep := Trace(nil, nil, nil, trProfile(), 100)
	if rep.TotalRows != 0 || len(rep.Traces) != 0 {
		t.Fatalf("laporan jendela kosong = %+v", rep)
	}
	if rep.FromTS != 0 || rep.ToTS != 0 {
		t.Errorf("tepi jendela = %d..%d; mau 0..0", rep.FromTS, rep.ToTS)
	}
	if rep.SingleNodeFleet {
		t.Error("SingleNodeFleet = true pada jendela kosong")
	}
}

// TraceWindowBounds harus MELEBARKAN ke belakang sebesar satu jendela korelasi:
// observasi di tepi bawah dapat menempel ke event yang bertransisi lebih awal,
// dan baris itu ada di luar rentang received_ts observasi.
func TestTraceWindowBoundsWidenBackwardByCorrelationWindow(t *testing.T) {
	obs := trObs(
		rpRow{id: 1, node: "A", pga: 40, received: 100000, upper: 99000, lat: -6.8, lon: 107.5},
		rpRow{id: 2, node: "A", pga: 45, received: 140000, upper: 99000, lat: -6.8, lon: 107.5},
	)
	p := trProfile()
	from, to, ok := TraceWindowBounds(obs, p)
	if !ok {
		t.Fatal("ok = false pada jendela berisi")
	}
	wantFrom := int64(100000) - p.Options.CorrelationWindowMs - p.linkTolerance()
	wantTo := int64(140000) + p.linkTolerance()
	if from != wantFrom || to != wantTo {
		t.Errorf("batas = %d..%d; mau %d..%d", from, to, wantFrom, wantTo)
	}
}

func TestTraceWindowBoundsEmptyIsNotOK(t *testing.T) {
	if _, _, ok := TraceWindowBounds(nil, trProfile()); ok {
		t.Fatal("ok = true pada jendela kosong")
	}
}

// obs_seq adalah ANOTASI, tidak pernah penyaring: nilai yang lebih tinggi berarti
// observasi diserap SETELAH transisi, yang normal, dan tidak boleh membatalkan
// tautan.
func TestTraceObsSeqIsAnnotationNotFilter(t *testing.T) {
	obs := trObs(rpRow{id: 1, node: "A", pga: 40, received: 3000, upper: 2900, seq: 999999, lat: -6.8, lon: 107.5})
	hist := []store.EventStateLog{trUnconfirmed("e1", 1, 3050, "A")} // obs_seq tercatat = 1000
	rep := Trace(obs, hist, nil, trProfile(), 1)
	tr := rep.Traces[0]
	if tr.Outcome != TraceTraced {
		t.Fatalf("hasil = %s; mau TRACED walau obs_seq lebih tinggi", tr.Outcome)
	}
	if tr.ObsSeqLink != ObsSeqLaterGT {
		t.Errorf("ObsSeqLink = %s; mau %s", tr.ObsSeqLink, ObsSeqLaterGT)
	}
}

func TestTraceObsSeqExactAndUnavailable(t *testing.T) {
	hist := []store.EventStateLog{trUnconfirmed("e1", 1, 3050, "A")} // tercatat 1000

	exact := trObs(rpRow{id: 1, node: "A", pga: 40, received: 3000, upper: 2900, seq: 1000, lat: -6.8, lon: 107.5})
	if got := Trace(exact, hist, nil, trProfile(), 1).Traces[0].ObsSeqLink; got != ObsSeqExact {
		t.Errorf("ObsSeqLink = %s; mau %s", got, ObsSeqExact)
	}

	v1 := trObs(rpRow{id: 1, node: "A", pga: 40, received: 3000, upper: 2900, lat: -6.8, lon: 107.5})
	if got := Trace(v1, hist, nil, trProfile(), 1).Traces[0].ObsSeqLink; got != ObsSeqUnavailable {
		t.Errorf("ObsSeqLink v1 = %s; mau %s", got, ObsSeqUnavailable)
	}
}

func TestTraceDefaultProfileMatchesReplayDefaults(t *testing.T) {
	if DefaultTraceProfile().Options != DefaultReplayProfile().Options {
		t.Fatal("default TraceProfile menyimpang dari DefaultReplayProfile")
	}
	if DefaultTraceProfile().linkTolerance() != defaultLinkToleranceMs {
		t.Errorf("toleransi tautan bawaan = %d; mau %d",
			DefaultTraceProfile().linkTolerance(), defaultLinkToleranceMs)
	}
}
