package event

import (
	"testing"

	"github.com/banana-pixel/quakealert/server/internal/store"
)

// Uji P4-M6′. Yang diperiksa berkas ini, dan hanya ini:
//
//  1. EMPAT KELUARAN WAJIB dilaporkan masing-masing dengan statusnya sendiri, dan
//     EMPTY tidak pernah dikonflasikan dengan NOT_OBSERVABLE.
//  2. RIWAYAT terurut revision ASC apa adanya, lubang dihitung HANYA di dalam
//     rentang yang ada, dan riwayat yang tidak mulai dari rev1 BUKAN lubang.
//  3. TAUTAN keanggotaan-dan-waktu: kandidat dilabeli TRACED atau AMBIGUOUS,
//     ambiguitas tumpang-tindih dilaporkan alih-alih dipilih, dan penyaring
//     bekerja SETELAH keanggotaan sehingga baris tersaring tetap terhitung ADA.
//  4. TOLERANSI milik M1′, dan provenance-nya dilaporkan.
//  5. TIDAK ADA PENILAIAN: emisi opsional, dan tidak ada field lulus/gagal.

// ---- pembangun -------------------------------------------------------------

// tlObs membangun baris ledger lewat rpRow (replay_test.go), supaya ketiga alat
// P4 melihat bentuk baris yang sama persis.
func tlObs(rows ...rpRow) []store.ReplayObservation { return rpObs(rows...) }

// tlRev membangun satu baris event_state_log dengan evidence_summary yang
// diserialkan penulis PRODUKSI (EvidenceSummary.JSON()), bukan JSON tulisan
// tangan. obs_seq tercatat = 1000+i, sama seperti trTransition.
func tlRev(eventID string, rev int, decidedAt int64, from, to, reason string, nodes ...string) store.EventStateLog {
	ev := EvidenceSummary{OriginTSSource: OnsetSourceSensor}
	cells := map[CellID]struct{}{}
	for i, n := range nodes {
		seq := int64(1000 + i)
		cell := CellID{X: int32(i), Y: 0}
		ev.Contributors = append(ev.Contributors, ContributorEvidence{
			NodeID: n, PeakPGA: 20, Phase: PhaseFinal,
			OnsetTS: decidedAt - 500, OnsetSource: OnsetSourceSensor, ObsSeq: &seq,
			Cell: cell,
		})
		cells[cell] = struct{}{}
		ev.CellIDs = append(ev.CellIDs, cell)
	}
	ev.IndependentCells = len(cells)
	f := from
	peak := 20.0
	row := store.EventStateLog{
		EventID: eventID, Revision: rev,
		ToState: to, Reason: reason,
		DecidedAt: decidedAt, NodeCount: len(nodes), IndependentCells: len(cells),
		PeakPGA: &peak, EvidenceSummary: ev.JSON(), AlgoVer: rsAlgoVer,
	}
	if from != "" {
		row.FromState = &f
	}
	return row
}

func tlUnconfirmed(eventID string, rev int, decidedAt int64, nodes ...string) store.EventStateLog {
	return tlRev(eventID, rev, decidedAt, string(StateDetected), string(StateUnconfirmed), ReasonFloorMet, nodes...)
}

func tlEventRow(eventID string, rev int, state string) *store.EarthquakeEvent {
	return &store.EarthquakeEvent{
		EventID: eventID, Status: "HAPPENING", EventState: state, Revision: rev,
		CentroidLat: -6.8, CentroidLon: 107.5, LocationName: "Bandung",
		MaxPGA: 20, TriggeredNodes: 1, StartedAtMs: 1000,
		OriginTS: 900, OriginTSSource: OnsetSourceSensor,
		IndependentCellCount: 1, AlgoVer: rsAlgoVer,
	}
}

func tlProfile() TraceProfile { return TraceProfile{Options: defaultOptions()} }

func tlAdvisory(id int64, eventID string, rev int, decidedAt int64, wsClients *int) store.TraceEmission {
	return trAdvisory(id, eventID, rev, decidedAt, wsClients)
}

// ---- 1. empat keluaran wajib -----------------------------------------------

// Keempat status dilaporkan, dan keempatnya OBSERVED ketika keempat bahannya ada.
func TestTimelineReportsFourRequiredOutputs(t *testing.T) {
	hist := []store.EventStateLog{tlUnconfirmed("e1", 1, 5000, "A")}
	obs := tlObs(rpRow{id: 1, node: "A", pga: 20, received: 4800, upper: 4700, seq: 1000, lat: -6.8, lon: 107.5})

	tl := BuildTimeline("e1", tlEventRow("e1", 1, string(StateUnconfirmed)), hist, obs, nil, tlProfile())

	if tl.Coverage.EventRowStatus != OutputObserved {
		t.Fatalf("keluaran 1 = %s, mau %s", tl.Coverage.EventRowStatus, OutputObserved)
	}
	if tl.Coverage.StateLogStatus != OutputObserved {
		t.Fatalf("keluaran 2 = %s, mau %s", tl.Coverage.StateLogStatus, OutputObserved)
	}
	if tl.Coverage.EvidenceStatus != OutputObserved {
		t.Fatalf("keluaran 3 = %s, mau %s", tl.Coverage.EvidenceStatus, OutputObserved)
	}
	if tl.Coverage.ObservationsStatus != OutputObserved {
		t.Fatalf("keluaran 4 = %s, mau %s", tl.Coverage.ObservationsStatus, OutputObserved)
	}
	if len(tl.Observations) != 1 {
		t.Fatalf("observasi = %d, mau 1", len(tl.Observations))
	}
}

// Baris event yang HILANG adalah NOT_OBSERVABLE, dan ia TIDAK mematikan tiga
// keluaran lainnya: riwayatnya tetap membuktikan transisinya.
func TestTimelineMissingEventRowDoesNotBlockHistory(t *testing.T) {
	hist := []store.EventStateLog{tlUnconfirmed("e1", 3, 5000, "A")}

	tl := BuildTimeline("e1", nil, hist, nil, nil, tlProfile())

	if tl.Coverage.EventRowStatus != OutputNotObservable {
		t.Fatalf("keluaran 1 = %s, mau %s", tl.Coverage.EventRowStatus, OutputNotObservable)
	}
	if tl.Coverage.StateLogStatus != OutputObserved {
		t.Fatalf("keluaran 2 = %s, mau %s (riwayat tetap ada)", tl.Coverage.StateLogStatus, OutputObserved)
	}
	if tl.Coverage.EvidenceStatus != OutputObserved {
		t.Fatalf("keluaran 3 = %s, mau %s", tl.Coverage.EvidenceStatus, OutputObserved)
	}
	if tl.Event != nil {
		t.Fatal("Event harus nil ketika barisnya tidak ada")
	}
}

// NOL observasi adalah EMPTY, bukan NOT_OBSERVABLE. Pembedaan itu inti D-015:
// pembacaannya berhasil, hasilnya nol baris, dan itu bukan bukti ketiadaan.
func TestTimelineEmptyIsNotNotObservable(t *testing.T) {
	hist := []store.EventStateLog{tlUnconfirmed("e1", 1, 5000, "A")}

	tl := BuildTimeline("e1", tlEventRow("e1", 1, string(StateUnconfirmed)), hist, nil, nil, tlProfile())

	if tl.Coverage.ObservationsStatus != OutputEmpty {
		t.Fatalf("keluaran 4 = %s, mau %s", tl.Coverage.ObservationsStatus, OutputEmpty)
	}
	if tl.Coverage.ObservationsStatus == OutputNotObservable {
		t.Fatal("EMPTY dikonflasikan dengan NOT_OBSERVABLE")
	}
}

// evidence_summary yang TIDAK SATU PUN terurai adalah NOT_OBSERVABLE — kolomnya
// ada, hanya tidak terbaca — dan barisnya TETAP dihitung sebagai revisi nyata.
func TestTimelineBrokenEvidenceIsNotObservableButRowsCount(t *testing.T) {
	row := tlUnconfirmed("e1", 1, 5000, "A")
	row.EvidenceSummary = []byte(`{"contributors":`) // JSON terpotong
	hist := []store.EventStateLog{row}

	tl := BuildTimeline("e1", nil, hist, nil, nil, tlProfile())

	if tl.Coverage.EvidenceStatus != OutputNotObservable {
		t.Fatalf("keluaran 3 = %s, mau %s", tl.Coverage.EvidenceStatus, OutputNotObservable)
	}
	if tl.Coverage.StateLogRows != 1 {
		t.Fatalf("StateLogRows = %d, mau 1 (baris rusak TETAP nyata)", tl.Coverage.StateLogRows)
	}
	if tl.Coverage.RevisionsEvidenceBroken != 1 {
		t.Fatalf("RevisionsEvidenceBroken = %d, mau 1", tl.Coverage.RevisionsEvidenceBroken)
	}
	if tl.Revisions[0].EvidenceError == "" {
		t.Fatal("EvidenceError kosong: galatnya harus dilaporkan, bukan ditelan")
	}
	if len(tl.Unattributed) != 1 || !tl.Unattributed[0].NoContributors {
		t.Fatal("revisi dengan bukti rusak harus muncul sebagai NoContributors")
	}
}

// ---- 2. riwayat, urutan dan lubang -----------------------------------------

// Riwayat dilaporkan APA ADANYA dalam urutan yang diberikan pembaca (revision
// ASC). BuildTimeline tidak mengurutkan ulang: urutannya dijamin kueri, dan
// pengurutan kedua di sini akan menyembunyikan pembaca yang rusak.
func TestTimelinePreservesRevisionOrder(t *testing.T) {
	hist := []store.EventStateLog{
		tlUnconfirmed("e1", 2, 5000, "A"),
		tlRev("e1", 3, 9000, string(StateUnconfirmed), string(StateConfirmed), ReasonQuorumMet, "A", "B", "C"),
		tlRev("e1", 4, 99000, string(StateConfirmed), string(StateResolved), ReasonNoNewEvidence, "A", "B", "C"),
	}

	tl := BuildTimeline("e1", nil, hist, nil, nil, tlProfile())

	if len(tl.Revisions) != 3 {
		t.Fatalf("revisi = %d, mau 3", len(tl.Revisions))
	}
	for i, want := range []int{2, 3, 4} {
		if got := tl.Revisions[i].Row.Revision; got != want {
			t.Fatalf("revisi[%d] = %d, mau %d", i, got, want)
		}
	}
	if tl.Coverage.FirstRevision != 2 || tl.Coverage.LastRevision != 4 {
		t.Fatalf("rentang = %d..%d, mau 2..4", tl.Coverage.FirstRevision, tl.Coverage.LastRevision)
	}
}

// Riwayat yang TIDAK dimulai dari rev1 BUKAN lubang: DETECTED tidak pernah
// menjadi baris (§9.5), jadi menyebutnya lubang akan melaporkan kehilangan yang
// tidak pernah terjadi.
func TestTimelineHistoryNotStartingAtOneIsNotAGap(t *testing.T) {
	hist := []store.EventStateLog{
		tlUnconfirmed("e1", 4, 5000, "A"),
		tlRev("e1", 5, 9000, string(StateUnconfirmed), string(StateConfirmed), ReasonQuorumMet, "A", "B", "C"),
	}

	tl := BuildTimeline("e1", nil, hist, nil, nil, tlProfile())

	if len(tl.Coverage.RevisionGaps) != 0 {
		t.Fatalf("lubang = %v, mau kosong (rev1..3 tidak pernah menjadi baris)", tl.Coverage.RevisionGaps)
	}
	if tl.Coverage.FirstRevision != 4 {
		t.Fatalf("FirstRevision = %d, mau 4", tl.Coverage.FirstRevision)
	}
}

// Nomor yang hilang DI DALAM rentang adalah bukti nyata satuan persistensi yang
// dibuang (D17/D30), dan ia dilaporkan sebagai nomornya, bukan sebagai jumlah.
func TestTimelineGapInsideRangeIsReported(t *testing.T) {
	hist := []store.EventStateLog{
		tlUnconfirmed("e1", 1, 5000, "A"),
		// rev2 HILANG
		tlRev("e1", 3, 9000, string(StateUnconfirmed), string(StateConfirmed), ReasonQuorumMet, "A", "B", "C"),
		// rev4 HILANG
		tlRev("e1", 5, 99000, string(StateConfirmed), string(StateResolved), ReasonNoNewEvidence, "A", "B", "C"),
	}

	tl := BuildTimeline("e1", nil, hist, nil, nil, tlProfile())

	want := []int{2, 4}
	if len(tl.Coverage.RevisionGaps) != len(want) {
		t.Fatalf("lubang = %v, mau %v", tl.Coverage.RevisionGaps, want)
	}
	for i, w := range want {
		if tl.Coverage.RevisionGaps[i] != w {
			t.Fatalf("lubang = %v, mau %v", tl.Coverage.RevisionGaps, want)
		}
	}
}

// State terminal dilaporkan ketika tercatat, dan KOSONG ketika tidak — bukan
// "masih terbuka", karena tabelnya tidak dapat membedakan event yang masih hidup
// dari transisi terminal yang tidak sampai ke disk.
func TestTimelineTerminalStateReportedOnlyWhenRecorded(t *testing.T) {
	open := []store.EventStateLog{tlUnconfirmed("e1", 1, 5000, "A")}
	if got := BuildTimeline("e1", nil, open, nil, nil, tlProfile()).Coverage.TerminalState; got != "" {
		t.Fatalf("TerminalState = %q, mau kosong", got)
	}

	for _, term := range []string{string(StateResolved), string(StateCancelled)} {
		hist := []store.EventStateLog{
			tlUnconfirmed("e1", 1, 5000, "A"),
			tlRev("e1", 2, 99000, string(StateUnconfirmed), term, ReasonNoNewEvidence, "A"),
		}
		if got := BuildTimeline("e1", nil, hist, nil, nil, tlProfile()).Coverage.TerminalState; got != term {
			t.Fatalf("TerminalState = %q, mau %q", got, term)
		}
	}
}

// ---- 3. tautan keanggotaan-dan-waktu ---------------------------------------

// Satu revisi yang memuat node itu -> TRACED, dengan lag dan anotasi obs_seq.
func TestTimelineSingleRevisionIsTraced(t *testing.T) {
	hist := []store.EventStateLog{tlUnconfirmed("e1", 1, 5000, "A")}
	obs := tlObs(rpRow{id: 7, node: "A", pga: 20, received: 4800, upper: 4700, seq: 1000, lat: -6.8, lon: 107.5})

	tl := BuildTimeline("e1", nil, hist, obs, nil, tlProfile())

	if len(tl.Observations) != 1 {
		t.Fatalf("observasi = %d, mau 1", len(tl.Observations))
	}
	c := tl.Observations[0]
	if c.Attribution != TraceTraced {
		t.Fatalf("atribusi = %s, mau %s", c.Attribution, TraceTraced)
	}
	if c.LagMs != 200 {
		t.Fatalf("LagMs = %d, mau 200 (decided_at - received_ts)", c.LagMs)
	}
	if c.ObsSeqLink != ObsSeqExact {
		t.Fatalf("ObsSeqLink = %s, mau %s (obs_seq 1000 = yang tercatat)", c.ObsSeqLink, ObsSeqExact)
	}
	if len(tl.Revisions[0].Candidates) != 1 {
		t.Fatalf("kandidat rev1 = %d, mau 1", len(tl.Revisions[0].Candidates))
	}
}

// Node yang TIDAK ada di contributors[] tidak pernah menjadi kandidat, meski
// waktunya di dalam jendela: keanggotaan adalah separuh predikatnya.
func TestTimelineNonMemberNodeIsNeverACandidate(t *testing.T) {
	hist := []store.EventStateLog{tlUnconfirmed("e1", 1, 5000, "A")}
	obs := tlObs(rpRow{id: 7, node: "Z", pga: 20, received: 4800, upper: 4700, lat: -6.8, lon: 107.5})

	tl := BuildTimeline("e1", nil, hist, obs, nil, tlProfile())

	if len(tl.Observations) != 0 {
		t.Fatalf("observasi = %d, mau 0 (node Z bukan kontributor)", len(tl.Observations))
	}
	if tl.Coverage.ObservationRowsRead != 1 {
		t.Fatalf("ObservationRowsRead = %d, mau 1 (barisnya TETAP terbaca)", tl.Coverage.ObservationRowsRead)
	}
}

// Waktu di LUAR jendela tidak menjadi kandidat, meski node-nya anggota. Jendela
// bawahnya CorrelationWindowMs + toleransi, atasnya toleransi saja.
func TestTimelineWindowBoundsAreClosedAndAsymmetric(t *testing.T) {
	p := tlProfile()
	tol, _ := EffectiveLinkTolerance(p)
	decided := int64(50000)
	lo := decided - (p.Options.CorrelationWindowMs + tol)
	hi := decided + tol

	hist := []store.EventStateLog{tlUnconfirmed("e1", 1, decided, "A")}
	obs := tlObs(
		rpRow{id: 1, node: "A", pga: 20, received: lo - 1, upper: lo - 2, lat: -6.8, lon: 107.5}, // di luar
		rpRow{id: 2, node: "A", pga: 20, received: lo, upper: lo - 1, lat: -6.8, lon: 107.5},     // tepat di tepi
		rpRow{id: 3, node: "A", pga: 20, received: hi, upper: hi - 1, lat: -6.8, lon: 107.5},     // tepat di tepi
		rpRow{id: 4, node: "A", pga: 20, received: hi + 1, upper: hi, lat: -6.8, lon: 107.5},     // di luar
	)

	tl := BuildTimeline("e1", nil, hist, obs, nil, p)

	if len(tl.Observations) != 2 {
		t.Fatalf("observasi = %d, mau 2 (kedua tepi masuk, kedua luar tidak)", len(tl.Observations))
	}
	for _, c := range tl.Observations {
		if c.ObservationID != 2 && c.ObservationID != 3 {
			t.Fatalf("obs %d masuk; hanya baris tepi (2,3) yang boleh", c.ObservationID)
		}
	}
	e := tl.Revisions[0]
	if e.WindowFromTS != lo || e.WindowToTS != hi {
		t.Fatalf("jendela = [%d..%d], mau [%d..%d]", e.WindowFromTS, e.WindowToTS, lo, hi)
	}
}

// Jendela dua revisi yang berdekatan BERTUMPANG-TINDIH, jadi satu observasi dapat
// menjadi kandidat keduanya. Itu AMBIGUOUS_MULTIPLE_TRANSITIONS — bukan lulus,
// bukan gagal, dan TIDAK dipilih salah satunya.
func TestTimelineOverlappingWindowsReportAmbiguity(t *testing.T) {
	hist := []store.EventStateLog{
		tlUnconfirmed("e1", 1, 5000, "A"),
		tlRev("e1", 2, 6000, string(StateUnconfirmed), string(StateConfirmed), ReasonQuorumMet, "A", "B", "C"),
	}
	obs := tlObs(rpRow{id: 9, node: "A", pga: 20, received: 4900, upper: 4800, seq: 1000, lat: -6.8, lon: 107.5})

	tl := BuildTimeline("e1", nil, hist, obs, nil, tlProfile())

	if len(tl.Observations) != 1 {
		t.Fatalf("observasi = %d, mau 1 baris unik", len(tl.Observations))
	}
	c := tl.Observations[0]
	if c.Attribution != TraceAmbiguous {
		t.Fatalf("atribusi = %s, mau %s", c.Attribution, TraceAmbiguous)
	}
	if len(c.AttributedTo) != 2 || c.AttributedTo[0] != 1 || c.AttributedTo[1] != 2 {
		t.Fatalf("AttributedTo = %v, mau [1 2]", c.AttributedTo)
	}
	if tl.Coverage.AmbiguousCandidates != 1 {
		t.Fatalf("AmbiguousCandidates = %d, mau 1", tl.Coverage.AmbiguousCandidates)
	}
	// Ia muncul pada KEDUA revisi: laporan per revisi tetap lengkap.
	if len(tl.Revisions[0].Candidates) != 1 || len(tl.Revisions[1].Candidates) != 1 {
		t.Fatal("kandidat ambigu harus muncul pada KEDUA revisi")
	}
	if tl.Coverage.CandidateRows != 2 {
		t.Fatalf("CandidateRows = %d, mau 2 (pasangan revisi, bukan baris unik)", tl.Coverage.CandidateRows)
	}
}

// Penyaring bekerja SETELAH keanggotaan: baris di bawah lantai dan baris
// terkecuali tetap terhitung ADA pada revisinya, jadi "tanpa kandidat" tidak
// pernah terbaca sebagai "tidak ada observasi".
func TestTimelineFiltersRunAfterMembership(t *testing.T) {
	hist := []store.EventStateLog{tlUnconfirmed("e1", 1, 5000, "A")}
	obs := tlObs(
		rpRow{id: 1, node: "A", pga: 2.0, received: 4700, upper: 4600, lat: -6.8, lon: 107.5},                             // di bawah lantai
		rpRow{id: 2, node: "A", pga: 20.0, received: 4800, upper: 4700, noLoc: true},                                      // terkecuali: tanpa lokasi
		rpRow{id: 3, node: "A", pga: 20.0, received: 4900, upper: 4800, lat: -6.8, lon: 107.5, verify: "ErrBadSignature"}, // terkecuali: verify
	)

	tl := BuildTimeline("e1", nil, hist, obs, nil, tlProfile())

	if len(tl.Observations) != 0 {
		t.Fatalf("observasi memenuhi syarat = %d, mau 0", len(tl.Observations))
	}
	if tl.Coverage.BelowFloorRows != 1 {
		t.Fatalf("BelowFloorRows = %d, mau 1", tl.Coverage.BelowFloorRows)
	}
	if tl.Coverage.ExcludedRows != 2 {
		t.Fatalf("ExcludedRows = %d, mau 2", tl.Coverage.ExcludedRows)
	}
	e := tl.Revisions[0]
	if e.BelowFloor != 1 || len(e.ExcludedCandidates) != 2 {
		t.Fatalf("per revisi: lantai=%d terkecuali=%d, mau 1 dan 2", e.BelowFloor, len(e.ExcludedCandidates))
	}
	// Alasan terkecuali dilaporkan dengan kosakata TERTUTUP milik M1′.
	seen := map[string]bool{}
	for _, ex := range e.ExcludedCandidates {
		seen[ex.Reason] = true
	}
	if !seen[SkipNoLocation] || !seen[SkipVerifyNotOK] {
		t.Fatalf("alasan terkecuali = %v, mau %s dan %s", seen, SkipNoLocation, SkipVerifyNotOK)
	}
	// Revisi tanpa kandidat TETAPI dengan baris tersaring harus dapat dibedakan.
	if len(tl.Unattributed) != 1 {
		t.Fatalf("Unattributed = %d, mau 1", len(tl.Unattributed))
	}
	u := tl.Unattributed[0]
	if u.NoContributors {
		t.Fatal("NoContributors benar padahal contributors[] terisi")
	}
	if u.MemberRowsFiltered != 3 {
		t.Fatalf("MemberRowsFiltered = %d, mau 3", u.MemberRowsFiltered)
	}
}

// Transisi yang lahir dari penjadwal atau pencabutan ditandai BUKAN dipicu
// observasi, sehingga jendela kandidat yang kosong tidak terbaca sebagai
// observasi yang hilang.
func TestTimelineSweepBornRevisionsAreMarkedNotObservationDriven(t *testing.T) {
	for _, reason := range []string{ReasonNoNewEvidence, ReasonEvidenceInvalid, ReasonOperatorRetracted} {
		hist := []store.EventStateLog{
			tlRev("e1", 1, 200000, string(StateUnconfirmed), string(StateResolved), reason, "A"),
		}
		tl := BuildTimeline("e1", nil, hist, nil, nil, tlProfile())
		if len(tl.Unattributed) != 1 {
			t.Fatalf("%s: Unattributed = %d, mau 1", reason, len(tl.Unattributed))
		}
		if !tl.Unattributed[0].NotObservationDriven {
			t.Fatalf("%s: NotObservationDriven salah, mau benar", reason)
		}
	}

	// Sebaliknya: alasan yang MEMANG dipicu observasi tidak boleh ditandai.
	for _, reason := range []string{ReasonFloorMet, ReasonQuorumMet, ReasonFirstObservation} {
		hist := []store.EventStateLog{tlRev("e1", 1, 5000, "", string(StateUnconfirmed), reason, "A")}
		tl := BuildTimeline("e1", nil, hist, nil, nil, tlProfile())
		if tl.Unattributed[0].NotObservationDriven {
			t.Fatalf("%s: NotObservationDriven benar, mau salah", reason)
		}
	}
}

// Kontributor banyak node, independent_cells >= 2, dan mixed_provenance terbaca
// dari evidence_summary apa adanya — dan fleet satu-node dinyatakan terpisah.
func TestTimelineMultiContributorEvidenceIsDecoded(t *testing.T) {
	hist := []store.EventStateLog{
		tlUnconfirmed("e1", 1, 5000, "A"),
		tlRev("e1", 2, 9000, string(StateUnconfirmed), string(StateConfirmed), ReasonQuorumMet, "A", "B", "C"),
	}

	tl := BuildTimeline("e1", nil, hist, nil, nil, tlProfile())

	conf := tl.Revisions[1]
	if !conf.EvidenceParsed {
		t.Fatal("evidence CONFIRMED tidak terurai")
	}
	if len(conf.Evidence.Contributors) != 3 {
		t.Fatalf("kontributor = %d, mau 3", len(conf.Evidence.Contributors))
	}
	if conf.Evidence.IndependentCells < 2 {
		t.Fatalf("independent_cells = %d, mau >= 2", conf.Evidence.IndependentCells)
	}
	if len(conf.ContributorNodes) != 3 {
		t.Fatalf("ContributorNodes = %v, mau 3 node", conf.ContributorNodes)
	}
	if tl.Coverage.ContributorNodes != 3 {
		t.Fatalf("union node = %d, mau 3", tl.Coverage.ContributorNodes)
	}
	if tl.Coverage.SingleNodeContributors {
		t.Fatal("SingleNodeContributors benar padahal ada 3 node")
	}

	// Satu node saja -> dinyatakan, karena quorum tidak terjangkau (S2).
	solo := BuildTimeline("e1", nil, []store.EventStateLog{tlUnconfirmed("e1", 1, 5000, "A")}, nil, nil, tlProfile())
	if !solo.Coverage.SingleNodeContributors {
		t.Fatal("SingleNodeContributors salah padahal hanya node A")
	}
}

// tlMixedProvenance menandai satu revisi sebagai bukti dengan asal onset CAMPURAN
// lewat penulis PRODUKSI: satu kontributor terukur sensor, satu batas dari waktu
// publish. Ditulis ulang, bukan disunting JSON-nya, supaya bentuk barisnya tetap
// bentuk yang benar-benar ditulis Tracker.
func tlMixedProvenance(row store.EventStateLog) store.EventStateLog {
	ev, err := historicEvidence(row.EvidenceSummary)
	if err != nil {
		panic(err)
	}
	if len(ev.Contributors) < 2 {
		panic("butuh >= 2 kontributor untuk asal campuran")
	}
	ev.Contributors[1].OnsetSource = OnsetSourcePublish
	ev.MixedProvenance = true
	row.EvidenceSummary = ev.JSON()
	return row
}

// mixed_provenance dibaca apa adanya dari potretnya. Ia dibawa eksplisit justru
// supaya tidak ada pembaca lampau yang perlu menebak apakah jangkar event
// campuran adalah pengukuran.
func TestTimelineMixedProvenanceIsDecoded(t *testing.T) {
	conf := tlRev("e1", 2, 9000, string(StateUnconfirmed), string(StateConfirmed), ReasonQuorumMet, "A", "B", "C")
	hist := []store.EventStateLog{
		tlUnconfirmed("e1", 1, 5000, "A"),
		tlMixedProvenance(conf),
	}

	tl := BuildTimeline("e1", nil, hist, nil, nil, tlProfile())

	if tl.Revisions[0].Evidence.MixedProvenance {
		t.Fatal("rev1 mixed_provenance benar padahal seluruh kontributornya SENSOR")
	}
	if !tl.Revisions[1].Evidence.MixedProvenance {
		t.Fatal("rev2 mixed_provenance salah padahal asal onsetnya campuran")
	}
	sources := map[string]bool{}
	for _, c := range tl.Revisions[1].Evidence.Contributors {
		sources[c.OnsetSource] = true
	}
	if !sources[OnsetSourceSensor] || !sources[OnsetSourcePublish] {
		t.Fatalf("asal onset = %v, mau SENSOR dan PUBLISH_BOUND", sources)
	}
}

// Keempat anotasi obs_seq dilaporkan sebagai dirinya sendiri. Anotasi, BUKAN
// penyaring: obs_seq yang lebih tinggi dari yang tercatat adalah NORMAL — barisnya
// diserap setelah transisi — jadi ia tidak pernah membatalkan kandidat.
func TestTimelineObsSeqAnnotationsNeverFilter(t *testing.T) {
	hist := []store.EventStateLog{tlUnconfirmed("e1", 1, 5000, "A")} // obs_seq tercatat = 1000
	obs := tlObs(
		rpRow{id: 1, node: "A", pga: 20, received: 4700, upper: 4600, seq: 999, lat: -6.8, lon: 107.5},
		rpRow{id: 2, node: "A", pga: 20, received: 4800, upper: 4700, seq: 1000, lat: -6.8, lon: 107.5},
		rpRow{id: 3, node: "A", pga: 20, received: 4900, upper: 4800, seq: 1001, lat: -6.8, lon: 107.5},
		rpRow{id: 4, node: "A", pga: 20, received: 4950, upper: 4850, lat: -6.8, lon: 107.5}, // v1: tanpa obs_seq
	)

	tl := BuildTimeline("e1", nil, hist, obs, nil, tlProfile())

	if len(tl.Observations) != 4 {
		t.Fatalf("observasi = %d, mau 4 — obs_seq TIDAK boleh menyaring", len(tl.Observations))
	}
	want := map[int64]string{1: ObsSeqAbsorbedLE, 2: ObsSeqExact, 3: ObsSeqLaterGT, 4: ObsSeqUnavailable}
	for _, c := range tl.Observations {
		if c.ObsSeqLink != want[c.ObservationID] {
			t.Fatalf("obs %d: ObsSeqLink = %s, mau %s", c.ObservationID, c.ObsSeqLink, want[c.ObservationID])
		}
	}
}

// ---- 4. toleransi milik M1′ ------------------------------------------------

// Bawaan berasal dari M1′ (defaultLinkToleranceMs) dan dilaporkan sebagai
// M1_DEFAULT; nilai yang diassersi operator dilaporkan sebagai OPERATOR_ASSERTED.
// M6′ tidak memperkenalkan toleransi ilmiah baru (D-015 batasan 2).
func TestTimelineToleranceComesFromM1WithProvenance(t *testing.T) {
	def := tlProfile()
	ms, prov := EffectiveLinkTolerance(def)
	if ms != defaultLinkToleranceMs {
		t.Fatalf("toleransi bawaan = %d, mau %d (milik M1')", ms, defaultLinkToleranceMs)
	}
	if prov != TolFromM1Default {
		t.Fatalf("provenance = %s, mau %s", prov, TolFromM1Default)
	}
	if ms != def.linkTolerance() {
		t.Fatalf("toleransi %d != linkTolerance() %d: ada aturan kedua", ms, def.linkTolerance())
	}

	op := tlProfile()
	op.LinkToleranceMs = 750
	ms, prov = EffectiveLinkTolerance(op)
	if ms != 750 {
		t.Fatalf("toleransi operator = %d, mau 750", ms)
	}
	if prov != TolFromOperator {
		t.Fatalf("provenance = %s, mau %s", prov, TolFromOperator)
	}

	// Dan ia dibawa ke laporan, bukan hanya dihitung.
	tl := BuildTimeline("e1", nil, []store.EventStateLog{tlUnconfirmed("e1", 1, 5000, "A")}, nil, nil, op)
	if tl.Coverage.LinkToleranceMs != 750 || tl.Coverage.ToleranceProvenance != TolFromOperator {
		t.Fatalf("Coverage = %d/%s, mau 750/%s",
			tl.Coverage.LinkToleranceMs, tl.Coverage.ToleranceProvenance, TolFromOperator)
	}
	if tl.Coverage.CorrelationWindowMs != op.Options.CorrelationWindowMs {
		t.Fatalf("CorrelationWindowMs = %d, mau %d", tl.Coverage.CorrelationWindowMs, op.Options.CorrelationWindowMs)
	}
}

// TimelineWindowBounds memakai decided_at TERKECIL dan TERBESAR, bukan baris
// pertama dan terakhir: urutan slice adalah urutan revisi, dan revisi tidak
// menjamin waktu — sebuah baris yang datang terlambat dapat memundurkan jamnya.
func TestTimelineWindowBoundsUseMinMaxNotFirstLast(t *testing.T) {
	p := tlProfile()
	tol, _ := EffectiveLinkTolerance(p)
	hist := []store.EventStateLog{
		tlUnconfirmed("e1", 1, 9000, "A"),
		tlRev("e1", 2, 5000, string(StateUnconfirmed), string(StateConfirmed), ReasonQuorumMet, "A"), // lebih AWAL
	}

	from, to, ok := TimelineWindowBounds(hist, p)
	if !ok {
		t.Fatal("ok salah padahal riwayatnya tidak kosong")
	}
	if want := int64(5000) - (p.Options.CorrelationWindowMs + tol); from != want {
		t.Fatalf("from = %d, mau %d (dari decided_at TERKECIL)", from, want)
	}
	if want := int64(9000) + tol; to != want {
		t.Fatalf("to = %d, mau %d (dari decided_at TERBESAR)", to, want)
	}

	if _, _, ok := TimelineWindowBounds(nil, p); ok {
		t.Fatal("ok benar untuk riwayat kosong: tidak ada pusat jendela yang dapat dipakai")
	}
}

// Union node dikumpulkan dari SELURUH revisi, terurut dan tanpa duplikat, karena
// itulah daftar yang dikirim ke pembaca observasi.
func TestTimelineContributorNodesAreUnionSortedUnique(t *testing.T) {
	hist := []store.EventStateLog{
		tlUnconfirmed("e1", 1, 5000, "B"),
		tlRev("e1", 2, 9000, string(StateUnconfirmed), string(StateConfirmed), ReasonQuorumMet, "A", "B", "C"),
	}

	got := TimelineContributorNodes(hist)
	want := []string{"A", "B", "C"}
	if len(got) != len(want) {
		t.Fatalf("node = %v, mau %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("node = %v, mau %v (terurut)", got, want)
		}
	}

	if n := TimelineContributorNodes(nil); len(n) != 0 {
		t.Fatalf("node = %v, mau kosong", n)
	}
}

// ---- 5. tidak ada penilaian ------------------------------------------------

// Emisi adalah bagian KELIMA yang OPSIONAL. nil (tidak dibaca) dibedakan dari
// slice kosong (dibaca dan tidak ada), dan tidak satu pun memengaruhi keempat
// status wajib (D-015 batasan 1).
func TestTimelineEmissionsAreOptionalAndNeverGateTheFour(t *testing.T) {
	hist := []store.EventStateLog{tlUnconfirmed("e1", 1, 5000, "A")}
	obs := tlObs(rpRow{id: 1, node: "A", pga: 20, received: 4800, upper: 4700, seq: 1000, lat: -6.8, lon: 107.5})
	row := tlEventRow("e1", 1, string(StateUnconfirmed))

	// Tidak dibaca sama sekali.
	off := BuildTimeline("e1", row, hist, obs, nil, tlProfile())
	if off.Emissions != nil {
		t.Fatal("Emissions bukan nil padahal emis nil: 'tidak dibaca' harus tetap terlihat")
	}

	// Dibaca dan TIDAK ADA barisnya -> MISSING, dan keempat status tetap OBSERVED.
	empty := BuildTimeline("e1", row, hist, obs, []store.TraceEmission{}, tlProfile())
	if len(empty.Emissions) != 1 {
		t.Fatalf("Emissions = %d, mau 1 (satu per revisi)", len(empty.Emissions))
	}
	if empty.Emissions[0].Outcome != EmissionMissing {
		t.Fatalf("outcome = %s, mau %s", empty.Emissions[0].Outcome, EmissionMissing)
	}
	for _, s := range []string{
		empty.Coverage.EventRowStatus, empty.Coverage.StateLogStatus,
		empty.Coverage.EvidenceStatus, empty.Coverage.ObservationsStatus,
	} {
		if s != OutputObserved {
			t.Fatalf("status wajib = %s, mau %s: emisi yang hilang TIDAK boleh menggugurkannya", s, OutputObserved)
		}
	}
}

// Tautan EKSAK (event_id + event_revision) dibedakan dari tautan HANYA-WAKTU
// (pra-000008), eksak menang, dan ws_client_count NULL dibedakan dari nol.
func TestTimelineEmissionExactBeatsTimeOnly(t *testing.T) {
	hist := []store.EventStateLog{
		tlUnconfirmed("e1", 1, 5000, "A"),
		tlRev("e1", 2, 9000, string(StateUnconfirmed), string(StateConfirmed), ReasonQuorumMet, "A"),
	}
	emis := []store.TraceEmission{
		trAdvisoryLegacy(10, 5000),             // hanya-waktu, tepat pada rev1
		tlAdvisory(11, "e1", 1, 5000, intp(3)), // EKSAK untuk rev1
		trAdvisoryLegacy(12, 9000),             // hanya-waktu, tepat pada rev2
	}

	tl := BuildTimeline("e1", nil, hist, nil, emis, tlProfile())

	if len(tl.Emissions) != 2 {
		t.Fatalf("Emissions = %d, mau 2", len(tl.Emissions))
	}
	if tl.Emissions[0].Outcome != EmissionByID || tl.Emissions[0].EmissionID != 11 {
		t.Fatalf("rev1 = %s/%d, mau %s/11", tl.Emissions[0].Outcome, tl.Emissions[0].EmissionID, EmissionByID)
	}
	if tl.Emissions[0].WSClientCount == nil || *tl.Emissions[0].WSClientCount != 3 {
		t.Fatalf("rev1 ws_client_count = %v, mau 3", tl.Emissions[0].WSClientCount)
	}
	if tl.Emissions[1].Outcome != EmissionByTimeOnly || tl.Emissions[1].EmissionID != 12 {
		t.Fatalf("rev2 = %s/%d, mau %s/12", tl.Emissions[1].Outcome, tl.Emissions[1].EmissionID, EmissionByTimeOnly)
	}
	if tl.Emissions[1].WSClientCount != nil {
		t.Fatal("ws_client_count NULL dikonflasikan dengan nol")
	}
}
