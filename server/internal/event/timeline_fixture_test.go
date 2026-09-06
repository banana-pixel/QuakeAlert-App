package event

import (
	"reflect"
	"testing"

	"github.com/banana-pixel/quakealert/server/internal/store"
)

// Fixture sintetis multi-revisi untuk P4-M6′.
//
// D-015 menjadikan fixture ini SYARAT untuk `IMPLEMENTED`, bukan sesuatu yang
// ditunda ke validasi, dengan alasan yang tegas: fleet satu-node fisik TIDAK
// DAPAT menghasilkan enam bentuk di bawah. Quorum butuh >=3 kontributor
// terverifikasi di >=2 sel independensi, dan itu fakta KERAPATAN JARINGAN (S2),
// bukan cacat. Jadi bentuk-bentuk itu dibangun di sini, dan apa yang dibuktikan
// dibatasi dengan tepat:
//
//	INI BUKTI PERANGKAT LUNAK, BUKAN VALIDASI LAPANGAN.
//
// CONFIRMED dan riwayat multi-kontributor TIDAK tervalidasi-lapangan oleh M6′ dan
// tidak dapat diklaim darinya (D-015 § batas satu-node, D-011 batasan 2, S9).
// Fase F yang memiliki bukti lapangan, dan ia tetap terpisah.
//
// Enam bentuk yang diperiksa, semuanya di dalam SATU garis waktu supaya
// interaksinya ikut teruji:
//
//	1. riwayat CONFIRMED
//	2. contributors[] multi-node
//	3. independent_cells >= 2
//	4. mixed_provenance
//	5. baris terminal RESOLVED (dan CANCELLED pada varian kedua)
//	6. ambiguitas jendela bertumpang-tindih
//
// DETERMINISTIK: seluruh cap waktu dan id adalah konstanta, evidence_summary
// diserialkan penulis PRODUKSI (EvidenceSummary.JSON()), dan tidak ada jam, tidak
// ada acak, tidak ada map yang diiterasi. Dua jalan menghasilkan angka yang sama.

// Cap waktu fixture. Berjarak sengaja: rev2 dan rev3 hanya 1200 ms terpisah —
// lebih dekat daripada satu jendela korelasi — sehingga jendela keduanya WAJIB
// bertumpang-tindih dan bentuk 6 muncul dengan sendirinya, bukan dipaksakan.
const (
	fxT0 = int64(1767225600000) // 2026-01-01T00:00:00Z

	fxRev1DecidedAt = fxT0 + 3000  // DETECTED -> UNCONFIRMED
	fxRev2DecidedAt = fxT0 + 6000  // UNCONFIRMED -> CONFIRMED
	fxRev3DecidedAt = fxT0 + 7200  // CONFIRMED -> CONFIRMED (bukti bertambah)
	fxRev4DecidedAt = fxT0 + 97200 // CONFIRMED -> RESOLVED (ResolveAfterMs kemudian)

	fxEventID = "00000000-0000-4000-8000-000000000f16"
)

// syntheticTimelineHistory membangun riwayat empat revisi.
//
// Sel independensi diberikan EKSPLISIT alih-alih dihitung dari koordinat: M6′
// tidak menghitung sel — ia membaca apa yang tercatat pada potretnya — dan
// menghitungnya di sini akan menguji consensus, bukan garis waktu.
func syntheticTimelineHistory() []store.EventStateLog {
	// rev1: satu node, satu sel. Bentuk yang MEMANG dapat dicapai satu node fisik.
	rev1 := tlUnconfirmed(fxEventID, 1, fxRev1DecidedAt, "NODE-A")

	// rev2: quorum. Tiga node, tiga sel -> bentuk 1, 2 dan 3.
	rev2 := tlRev(fxEventID, 2, fxRev2DecidedAt,
		string(StateUnconfirmed), string(StateConfirmed), ReasonQuorumMet,
		"NODE-A", "NODE-B", "NODE-C")

	// rev3: bukti bertambah, dan asal onsetnya menjadi CAMPURAN -> bentuk 4.
	// decided_at-nya hanya 1200 ms setelah rev2 -> bentuk 6.
	rev3 := tlMixedProvenance(tlRev(fxEventID, 3, fxRev3DecidedAt,
		string(StateConfirmed), string(StateConfirmed), ReasonQuorumMet,
		"NODE-A", "NODE-B", "NODE-C", "NODE-D"))

	// rev4: terminal RESOLVED lewat sweep -> bentuk 5. Tidak dipicu observasi.
	rev4 := tlRev(fxEventID, 4, fxRev4DecidedAt,
		string(StateConfirmed), string(StateResolved), ReasonNoNewEvidence,
		"NODE-A", "NODE-B", "NODE-C", "NODE-D")

	return []store.EventStateLog{rev1, rev2, rev3, rev4}
}

// syntheticTimelineObservations membangun ledger fixture.
//
// obs 3001 diletakkan pada fxRev2DecidedAt-500, yaitu DI DALAM jendela rev2 DAN
// jendela rev3 sekaligus. Itu bentuk 6, dan ia harus keluar sebagai
// AMBIGUOUS_MULTIPLE_TRANSITIONS — bukan dipilih ke revisi terdekat.
func syntheticTimelineObservations() []store.ReplayObservation {
	return tlObs(
		// Sebelum rev1: kandidat rev1 saja.
		rpRow{id: 3000, node: "NODE-A", pga: 22.5, received: fxRev1DecidedAt - 400,
			upper: fxRev1DecidedAt - 500, seq: 1000, lat: -6.90, lon: 107.60},

		// Bertumpang-tindih rev2 dan rev3 (bentuk 6).
		rpRow{id: 3001, node: "NODE-B", pga: 31.0, received: fxRev2DecidedAt - 500,
			upper: fxRev2DecidedAt - 600, seq: 1001, lat: -6.95, lon: 107.65},

		// Kandidat rev3 saja: setelah tepi atas rev2.
		rpRow{id: 3002, node: "NODE-D", pga: 27.4, received: fxRev3DecidedAt - 100,
			upper: fxRev3DecidedAt - 200, seq: 1003, lat: -7.05, lon: 107.75},

		// Di bawah lantai: anggota, di dalam jendela, BUKAN pemicu.
		rpRow{id: 3003, node: "NODE-C", pga: 3.2, received: fxRev2DecidedAt - 300,
			upper: fxRev2DecidedAt - 400, seq: 1002, lat: -7.00, lon: 107.70},

		// Terkecuali: tanda tangan ditolak verifier. Ledger TETAP mencatatnya (§16).
		rpRow{id: 3004, node: "NODE-C", pga: 29.9, received: fxRev2DecidedAt - 200,
			upper: fxRev2DecidedAt - 300, seq: 1002, lat: -7.00, lon: 107.70,
			verify: "ErrBadSignature"},

		// Terkecuali: node_location NULL saat ingest.
		rpRow{id: 3005, node: "NODE-D", pga: 24.1, received: fxRev3DecidedAt - 50,
			upper: fxRev3DecidedAt - 150, seq: 1003, noLoc: true},

		// Node anggota, waktu DI LUAR seluruh jendela: terbaca, bukan kandidat.
		rpRow{id: 3006, node: "NODE-A", pga: 25.0, received: fxRev4DecidedAt - 60000,
			upper: fxRev4DecidedAt - 60100, seq: 1004, lat: -6.90, lon: 107.60},
	)
}

// syntheticTimelineEmissions: satu emisi EKSAK (pasca-000008) dan satu emisi
// HANYA-WAKTU (pra-000008), supaya kedua kekuatan bukti terwakili.
func syntheticTimelineEmissions() []store.TraceEmission {
	return []store.TraceEmission{
		tlAdvisory(9001, fxEventID, 1, fxRev1DecidedAt, intp(7)),
		trAdvisoryLegacy(9002, fxRev2DecidedAt),
	}
}

func syntheticTimeline(t *testing.T) *EventTimeline {
	t.Helper()
	row := tlEventRow(fxEventID, 4, string(StateResolved))
	row.Status = "RESOLVED"
	row.TriggeredNodes = 4
	row.IndependentCellCount = 4
	return BuildTimeline(fxEventID, row,
		syntheticTimelineHistory(), syntheticTimelineObservations(),
		syntheticTimelineEmissions(), tlProfile())
}

// TestSyntheticTimelineCoversAllSixShapes adalah gerbang fixture D-015. Ia
// memeriksa keenam bentuk pada SATU garis waktu, karena yang perlu dibuktikan
// bukan hanya bahwa masing-masing dapat dibangun, melainkan bahwa keenamnya
// terbaca benar ketika hadir bersama.
func TestSyntheticTimelineCoversAllSixShapes(t *testing.T) {
	tl := syntheticTimeline(t)

	// Keempat keluaran wajib teramati.
	for name, got := range map[string]string{
		"1 baris event": tl.Coverage.EventRowStatus,
		"2 riwayat":     tl.Coverage.StateLogStatus,
		"3 evidence":    tl.Coverage.EvidenceStatus,
		"4 observasi":   tl.Coverage.ObservationsStatus,
	} {
		if got != OutputObserved {
			t.Fatalf("keluaran %s = %s, mau %s", name, got, OutputObserved)
		}
	}

	// Bentuk 1: riwayat CONFIRMED — dan bukan hanya baris event yang menyebutnya.
	var confirmed int
	for i := range tl.Revisions {
		if tl.Revisions[i].Row.ToState == string(StateConfirmed) {
			confirmed++
		}
	}
	if confirmed != 2 {
		t.Fatalf("baris CONFIRMED = %d, mau 2 (rev2 dan rev3)", confirmed)
	}

	// Bentuk 2 + 3: multi-kontributor dan independent_cells >= 2.
	rev2 := tl.Revisions[1]
	if len(rev2.Evidence.Contributors) != 3 {
		t.Fatalf("kontributor rev2 = %d, mau 3", len(rev2.Evidence.Contributors))
	}
	if rev2.Evidence.IndependentCells < 2 {
		t.Fatalf("independent_cells rev2 = %d, mau >= 2", rev2.Evidence.IndependentCells)
	}
	rev3 := tl.Revisions[2]
	if len(rev3.Evidence.Contributors) != 4 {
		t.Fatalf("kontributor rev3 = %d, mau 4", len(rev3.Evidence.Contributors))
	}
	if tl.Coverage.ContributorNodes != 4 {
		t.Fatalf("union node = %d, mau 4", tl.Coverage.ContributorNodes)
	}
	if tl.Coverage.SingleNodeContributors {
		t.Fatal("SingleNodeContributors benar: fixture ini sengaja BUKAN satu node")
	}

	// Bentuk 4: mixed_provenance, pada rev3 dan bukan pada rev2.
	if rev2.Evidence.MixedProvenance {
		t.Fatal("rev2 mixed_provenance benar padahal seluruh kontributornya SENSOR")
	}
	if !rev3.Evidence.MixedProvenance {
		t.Fatal("rev3 mixed_provenance salah padahal asal onsetnya campuran")
	}

	// Bentuk 5: terminal RESOLVED, dan riwayatnya utuh tanpa lubang.
	if tl.Coverage.TerminalState != string(StateResolved) {
		t.Fatalf("TerminalState = %q, mau %s", tl.Coverage.TerminalState, StateResolved)
	}
	if len(tl.Coverage.RevisionGaps) != 0 {
		t.Fatalf("lubang = %v, mau kosong", tl.Coverage.RevisionGaps)
	}
	if tl.Coverage.FirstRevision != 1 || tl.Coverage.LastRevision != 4 {
		t.Fatalf("rentang = %d..%d, mau 1..4", tl.Coverage.FirstRevision, tl.Coverage.LastRevision)
	}

	// Bentuk 6: ambiguitas jendela bertumpang-tindih, dilaporkan bukan dipilih.
	//
	// DUA baris ambigu, bukan satu, dan angka itu bukan kebetulan fixture: jendela
	// korelasi 20 s sementara rev1..rev3 berjarak 3000 ms dan 1200 ms, jadi jendela
	// ketiganya BERTUMPANG-TINDIH lebar. Untuk event nyata yang berumur pendek itu
	// keadaan NORMAL, bukan kasus tepi — dan karena itu ambiguitas harus dilaporkan
	// apa adanya alih-alih diputuskan ke revisi terdekat (D-015 batasan 3).
	if tl.Coverage.AmbiguousCandidates != 2 {
		t.Fatalf("AmbiguousCandidates = %d, mau 2 (obs 3000 dan 3001)", tl.Coverage.AmbiguousCandidates)
	}
	byID := map[int64]ObservationCandidate{}
	for _, c := range tl.Observations {
		byID[c.ObservationID] = c
	}
	// obs 3000 (NODE-A) anggota rev1, rev2 DAN rev3: node itu kontributor ketiganya.
	assertAttribution(t, byID, 3000, TraceAmbiguous, []int{1, 2, 3})
	// obs 3001 (NODE-B) bukan kontributor rev1, jadi hanya rev2 dan rev3.
	assertAttribution(t, byID, 3001, TraceAmbiguous, []int{2, 3})
	// obs 3002 (NODE-D) hanya rev3: NODE-D belum kontributor pada rev2 dan jendela
	// rev4 jauh setelahnya. Inilah satu-satunya TRACED tanpa syarat di fixture.
	assertAttribution(t, byID, 3002, TraceTraced, []int{3})
}

// assertAttribution memeriksa label DAN daftar revisinya bersama. Keduanya harus
// diperiksa sekaligus: sebuah label TRACED dengan dua revisi, atau AMBIGUOUS
// dengan satu, adalah laporan yang saling bertentangan.
func assertAttribution(t *testing.T, byID map[int64]ObservationCandidate, id int64, attrib string, revs []int) {
	t.Helper()
	c, ok := byID[id]
	if !ok {
		t.Fatalf("obs %d tidak ada di daftar kandidat", id)
	}
	if c.Attribution != attrib {
		t.Fatalf("obs %d: atribusi = %s, mau %s", id, c.Attribution, attrib)
	}
	if len(c.AttributedTo) != len(revs) {
		t.Fatalf("obs %d: AttributedTo = %v, mau %v", id, c.AttributedTo, revs)
	}
	for i := range revs {
		if c.AttributedTo[i] != revs[i] {
			t.Fatalf("obs %d: AttributedTo = %v, mau %v", id, c.AttributedTo, revs)
		}
	}
}

// Penyebutnya utuh: setiap baris yang terbaca masuk tepat satu keranjang, dan
// tidak satu pun hilang diam-diam.
func TestSyntheticTimelineDenominatorIsWhole(t *testing.T) {
	tl := syntheticTimeline(t)
	cov := tl.Coverage

	if cov.ObservationRowsRead != 7 {
		t.Fatalf("ObservationRowsRead = %d, mau 7", cov.ObservationRowsRead)
	}
	// 3000, 3001, 3002 memenuhi syarat; 3003 di bawah lantai; 3004 dan 3005
	// terkecuali; 3006 anggota tetapi di luar jendela.
	if len(tl.Observations) != 3 {
		t.Fatalf("memenuhi syarat = %d, mau 3", len(tl.Observations))
	}
	if cov.BelowFloorRows != 1 {
		t.Fatalf("BelowFloorRows = %d, mau 1", cov.BelowFloorRows)
	}
	if cov.ExcludedRows != 2 {
		t.Fatalf("ExcludedRows = %d, mau 2", cov.ExcludedRows)
	}
	nonMember := cov.ObservationRowsRead - len(tl.Observations) - cov.BelowFloorRows - cov.ExcludedRows
	if nonMember != 1 {
		t.Fatalf("di luar jendela = %d, mau 1 (obs 3006)", nonMember)
	}
	// Pasangan revisi > baris unik, dan selisihnya bukan penghitungan ganda:
	// obs 3000 adalah kandidat rev1+rev2+rev3, obs 3001 rev2+rev3, obs 3002 rev3.
	// 3 + 2 + 1 = 6. Baris uniknya tetap 3 — kedua angka dilaporkan terpisah justru
	// supaya tidak satu pun dari keduanya dibaca sebagai yang lain.
	if cov.CandidateRows != 6 {
		t.Fatalf("CandidateRows = %d, mau 6", cov.CandidateRows)
	}
}

// Emisi: kedua kekuatan bukti terwakili, dan revisi tanpa emisi dilaporkan
// MISSING tanpa menggugurkan keempat keluaran wajib.
func TestSyntheticTimelineEmissionsCarryBothStrengths(t *testing.T) {
	tl := syntheticTimeline(t)

	if len(tl.Emissions) != 4 {
		t.Fatalf("Emissions = %d, mau 4 (satu per revisi)", len(tl.Emissions))
	}
	// rev1 EKSAK (emisi 9001 membawa event_id + event_revision). rev2 dan rev3
	// keduanya HANYA-WAKTU atas emisi 9002 yang SAMA: baris pra-000008 tidak
	// membawa event_revision, dan rev3 hanya 1200 ms setelah rev2 — lebih dekat
	// daripada toleransi 2000 ms. Ia tidak dapat dipisahkan, jadi kedua sisinya
	// ditandai berbagi alih-alih satu di antaranya dibuang. rev4 MISSING.
	want := []string{EmissionByID, EmissionByTimeOnly, EmissionByTimeOnly, EmissionMissing}
	for i, w := range want {
		if tl.Emissions[i].Outcome != w {
			t.Fatalf("rev%d outcome = %s, mau %s", i+1, tl.Emissions[i].Outcome, w)
		}
	}
	if tl.Emissions[1].EmissionID != 9002 || tl.Emissions[2].EmissionID != 9002 {
		t.Fatalf("rev2/rev3 emission_id = %d/%d, mau 9002 keduanya",
			tl.Emissions[1].EmissionID, tl.Emissions[2].EmissionID)
	}
	if !tl.Emissions[1].SharedTimeOnlyLink || !tl.Emissions[2].SharedTimeOnlyLink {
		t.Fatal("tautan hanya-waktu yang DIBAGI tidak ditandai: satu emisi akan terhitung dua")
	}
	if tl.Emissions[0].SharedTimeOnlyLink {
		t.Fatal("tautan EKSAK ditandai berbagi: eksak tidak pernah ambigu")
	}
	if tl.Emissions[0].WSClientCount == nil || *tl.Emissions[0].WSClientCount != 7 {
		t.Fatalf("rev1 ws_client_count = %v, mau 7", tl.Emissions[0].WSClientCount)
	}
	if tl.Emissions[1].WSClientCount != nil {
		t.Fatal("emisi pra-000008 membawa ws_client_count: NULL dikonflasikan dengan nol")
	}
}

// Revisi terminal lahir dari sweep, jadi jendela kandidatnya yang kosong
// DIHARAPKAN — bukan observasi yang hilang.
func TestSyntheticTimelineTerminalRevisionIsNotObservationDriven(t *testing.T) {
	tl := syntheticTimeline(t)

	if len(tl.Unattributed) != 1 {
		t.Fatalf("Unattributed = %d, mau 1 (rev4 saja)", len(tl.Unattributed))
	}
	u := tl.Unattributed[0]
	if u.Revision != 4 {
		t.Fatalf("Unattributed = rev%d, mau rev4", u.Revision)
	}
	if u.ToState != string(StateResolved) {
		t.Fatalf("ToState = %s, mau %s", u.ToState, StateResolved)
	}
	if !u.NotObservationDriven {
		t.Fatal("NotObservationDriven salah untuk baris yang lahir dari sweep")
	}
	if u.NoContributors {
		t.Fatal("NoContributors benar padahal contributors[] rev4 terisi 4 node")
	}
}

// Varian kedua: terminal CANCELLED. Dibangun ulang dari riwayat yang sama supaya
// yang berubah hanya barisnya, bukan seluruh fixture.
func TestSyntheticTimelineCancelledTerminalIsCovered(t *testing.T) {
	hist := syntheticTimelineHistory()
	hist[3] = tlRev(fxEventID, 4, fxRev4DecidedAt,
		string(StateConfirmed), string(StateCancelled), ReasonEvidenceInvalid,
		"NODE-A", "NODE-B", "NODE-C", "NODE-D")

	tl := BuildTimeline(fxEventID, nil, hist, syntheticTimelineObservations(), nil, tlProfile())

	if tl.Coverage.TerminalState != string(StateCancelled) {
		t.Fatalf("TerminalState = %q, mau %s", tl.Coverage.TerminalState, StateCancelled)
	}
	if !tl.Unattributed[0].NotObservationDriven {
		t.Fatal("EVIDENCE_INVALIDATED harus ditandai BUKAN dipicu observasi")
	}
	if tl.Emissions != nil {
		t.Fatal("Emissions bukan nil padahal emis nil")
	}
}

// DETERMINISTIK: dua jalan atas fixture yang sama menghasilkan angka yang sama,
// termasuk urutan daftar. Tanpa ini sebuah map yang diiterasi akan lolos diam-diam.
func TestSyntheticTimelineIsDeterministic(t *testing.T) {
	a, b := syntheticTimeline(t), syntheticTimeline(t)

	// DeepEqual, bukan ==: Coverage membawa slice (AlgoVersRow, RevisionGaps), dan
	// justru slice itulah tempat urutan yang tidak stabil akan muncul.
	if !reflect.DeepEqual(a.Coverage, b.Coverage) {
		t.Fatalf("Coverage berbeda antar-jalan:\n a=%+v\n b=%+v", a.Coverage, b.Coverage)
	}
	if len(a.Observations) != len(b.Observations) {
		t.Fatalf("jumlah observasi berbeda: %d vs %d", len(a.Observations), len(b.Observations))
	}
	for i := range a.Observations {
		x, y := a.Observations[i], b.Observations[i]
		if x.ObservationID != y.ObservationID || x.Attribution != y.Attribution || x.LagMs != y.LagMs {
			t.Fatalf("observasi[%d] berbeda: %+v vs %+v", i, x, y)
		}
	}
	for i := range a.Revisions {
		x, y := a.Revisions[i], b.Revisions[i]
		if len(x.ContributorNodes) != len(y.ContributorNodes) {
			t.Fatalf("revisi[%d]: jumlah node berbeda", i)
		}
		for j := range x.ContributorNodes {
			if x.ContributorNodes[j] != y.ContributorNodes[j] {
				t.Fatalf("revisi[%d]: urutan node berbeda — ada map yang diiterasi", i)
			}
		}
	}
}
