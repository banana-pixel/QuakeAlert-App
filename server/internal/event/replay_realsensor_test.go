package event

import (
	"context"
	"testing"

	"github.com/banana-pixel/quakealert/server/internal/store"
)

// Validasi P4-M4′ langkah 6: pemutaran ulang atas JENDELA LEDGER SENSOR NYATA
// satu-satunya yang dimiliki proyek ini.
//
// Fixture ini BUKAN data buatan. Setiap nilai di bawah dibaca dari basis data
// produksi pada sesi drill perangkat sebelumnya:
//
//	earthquake_events 3adf752d-48f1-4f81-b98e-d31e3775c923
//	  origin_ts=1788255602923 origin_ts_source=SENSOR max_pga=73.0537
//	  triggered_nodes_count=1 independent_cell_count=1
//	  algo_ver=phase3-1.1/ic=5
//	event_state_log id 43: rev1 DETECTED->UNCONFIRMED FLOOR_MET      decided_at=1788255609089
//	event_state_log id 44: rev2 UNCONFIRMED->RESOLVED NO_NEW_EVIDENCE decided_at=1788255701061
//	sensor_observations 28: NODE-52960B47 PRELIM proto_ver=2 obs_seq=1507330 attempt_no=1
//	  pga=2.2952  dur=300  publish=1788255603229 received=1788255603253
//	  onset_ts=1788255602923 upper=1788255602929 SENSOR verify=OK
//	sensor_observations 29: NODE-52960B47 FINAL  proto_ver=2 obs_seq=1507330 attempt_no=1
//	  pga=73.0537 dur=6138 publish=1788255609070 received=1788255609087
//	  onset_ts=1788255602923 upper=1788255602932 SENSOR verify=OK
//	node_location snapshot: lat=-6.856209299999984 lon=107.52896220000001
//
// BATASNYA, dan ia harus dinyatakan di setiap laporan yang mengutip uji ini:
// kolom evidence_summary kedua baris riwayat TIDAK terekam pada sesi itu, jadi
// riwayat di bawah MENYUSUNNYA dari skalar yang terekam. Akibatnya perbandingan
// atas field bukti (kontributor, cell_ids) BUKAN bukti independen — ia
// tautologi. Yang benar-benar diperiksa terhadap produksi adalah kolom skalar
// yang memang terbaca: revision, from_state, to_state, reason, node_count,
// independent_cells, peak_pga, algo_ver, dan decided_at sebagai delta.
//
// S2 juga berlaku: fleet satu node membuat CONFIRMED tak terjangkau lewat
// kerapatan. UNCONFIRMED lalu RESOLVED adalah perilaku yang BENAR di sini, bukan
// cacat yang direproduksi.
const (
	rsEventID  = "3adf752d-48f1-4f81-b98e-d31e3775c923"
	rsNodeID   = "NODE-52960B47"
	rsOnset    = int64(1788255602923)
	rsObsSeq   = int64(1507330)
	rsLat      = -6.856209299999984
	rsLon      = 107.52896220000001
	rsAlgoVer  = "phase3-1.1/ic=5"
	rsPeakPGA  = 73.0537
	rsDecided1 = int64(1788255609089)
	rsDecided2 = int64(1788255701061)
	rsRecv1    = int64(1788255603253)
	rsRecv2    = int64(1788255609087)
)

func rsObservations() []store.ReplayObservation {
	seq := rsObsSeq
	onset := rsOnset
	proto := int16(2)
	attempt := int16(1)
	up1, up2 := int64(1788255602929), int64(1788255602932)
	lat1, lon1 := rsLat, rsLon
	lat2, lon2 := rsLat, rsLon
	det := int64(1788255609061)

	return []store.ReplayObservation{
		{
			ObservationID: 28, NodeID: rsNodeID, Phase: PhasePrelim,
			ProtoVer: &proto, ObsSeq: &seq, AttemptNo: &attempt,
			PGAGal: 2.2952, DurMs: 300,
			PublishTS: 1788255603229, ReceivedTS: rsRecv1,
			OnsetTS: &onset, OnsetTSUpperBound: &up1, OnsetTSSource: OnsetSourceSensor,
			Lat: &lat1, Lon: &lon1, VerifyResult: "OK",
		},
		{
			ObservationID: 29, NodeID: rsNodeID, Phase: PhaseFinal,
			ProtoVer: &proto, ObsSeq: &seq, AttemptNo: &attempt,
			PGAGal: rsPeakPGA, DurMs: 6138,
			PublishTS: 1788255609070, ReceivedTS: rsRecv2,
			OnsetTS: &onset, OnsetTSUpperBound: &up2, OnsetTSSource: OnsetSourceSensor,
			DetriggerTS: &det,
			Lat:         &lat2, Lon: &lon2, VerifyResult: "OK",
		},
	}
}

// rsHistory menyusun kedua baris event_state_log dari skalar yang terekam.
//
// evidence_summary DIREKONSTRUKSI — lihat catatan batas di atas berkas. Ia
// dibangun dari bentuk yang sama yang Tracker hasilkan (satu kontributor, fase
// FINAL, jangkar SENSOR, sel independensi dihitung dengan independenceCell yang
// sama), jadi pemeriksaan atas field bukti di uji ini adalah pemeriksaan
// KONSISTENSI, bukan pemeriksaan terhadap produksi.
func rsHistory() []store.EventStateLog {
	peak := rsPeakPGA
	seq := rsObsSeq
	ev := EvidenceSummary{
		Contributors: []ContributorEvidence{{
			NodeID:      rsNodeID,
			PeakPGA:     rsPeakPGA,
			Phase:       PhaseFinal,
			OnsetTS:     rsOnset,
			OnsetSource: OnsetSourceSensor,
			ObsSeq:      &seq,
			Cell:        rsCell(),
		}},
		IndependentCells: 1,
		CellIDs:          []CellID{rsCell()},
		OriginTSSource:   OnsetSourceSensor,
		MixedProvenance:  false,
	}
	detected, unconfirmed := string(StateDetected), string(StateUnconfirmed)

	return []store.EventStateLog{
		{
			EventID: rsEventID, Revision: 1,
			FromState: &detected, ToState: string(StateUnconfirmed),
			Reason: ReasonFloorMet, DecidedAt: rsDecided1,
			NodeCount: 1, IndependentCells: 1, PeakPGA: &peak,
			EvidenceSummary: ev.JSON(), AlgoVer: rsAlgoVer,
		},
		{
			EventID: rsEventID, Revision: 2,
			FromState: &unconfirmed, ToState: string(StateResolved),
			Reason: ReasonNoNewEvidence, DecidedAt: rsDecided2,
			NodeCount: 1, IndependentCells: 1, PeakPGA: &peak,
			EvidenceSummary: ev.JSON(), AlgoVer: rsAlgoVer,
		},
	}
}

// rsCell memakai fungsi PRODUKSI, bukan angka yang disalin: sebuah label sel yang
// ditulis tangan akan menguji ketikannya sendiri.
func rsCell() CellID {
	k := independenceCell(rsLat, rsLon, 5)
	return CellID{X: k.X, Y: k.Y}
}

// Gerbang: label produksi harus DITERIMA oleh biner ini. Bila uji ini gagal,
// basisnya sudah naik dan jendela nyata tidak boleh lagi diputar tanpa perhatian.
func TestRealSensorAlgoVerAccepted(t *testing.T) {
	if err := CheckAlgoVer(rsAlgoVer, DefaultReplayProfile()); err != nil {
		t.Fatalf("label produksi %q ditolak: %v", rsAlgoVer, err)
	}
}

func TestRealSensorWindowReproducesDecisions(t *testing.T) {
	p := DefaultReplayProfile()
	res, err := Replay(context.Background(), rsObservations(), p)
	if err != nil {
		t.Fatalf("Replay galat: %v", err)
	}

	if res.Input.TotalRows != 2 || res.Input.FedRows != 2 || len(res.Input.Skipped) != 0 {
		t.Fatalf("masukan = %+v; mau 2/2/0", res.Input)
	}
	if res.AlgoVer != rsAlgoVer {
		t.Fatalf("algo_ver replay = %q; mau %q", res.AlgoVer, rsAlgoVer)
	}

	rep := Compare(rsHistory(), res, p)
	if !rep.Bijective() {
		t.Fatalf("tidak bijektif: historis=%v replay=%v ambigu=%v",
			rep.UnmatchedHistoric, rep.UnmatchedReplayed, rep.AmbiguousSignatures)
	}
	if !rep.DecisionsReproduced() {
		for _, e := range rep.Events {
			for _, d := range e.Diffs {
				t.Errorf("diff: %s", d)
			}
		}
		t.Fatal("keputusan jendela sensor nyata TIDAK direproduksi")
	}

	c := rep.Events[0]
	if c.HistoricEventID != rsEventID {
		t.Errorf("HistoricEventID = %q; mau %q", c.HistoricEventID, rsEventID)
	}
	// F2: pengelompokan yang direproduksi adalah node + obs_seq nyata.
	if want := rsNodeID + "#" + "1507330"; c.Signature != want {
		t.Errorf("tanda tangan = %q; mau %q", c.Signature, want)
	}
}

// F3 atas data nyata: decided_at dibandingkan sebagai DELTA, dan deltanya harus
// jatuh di dalam toleransi kuantisasi sweep.
//
// Angka yang diperiksa punya arti: riwayat produksi memutuskan RESOLVED 91.972 ms
// setelah UNCONFIRMED, sementara replay memutuskannya pada batas tik sweep
// terdekat setelah ResolveAfterMs. Selisih kedua delta itu karenanya TIDAK boleh
// nol — fase tik produksi tidak terekam — dan itulah tepatnya mengapa F3 meminta
// perbandingan relatif dengan toleransi, bukan kesetaraan.
func TestRealSensorDecidedAtIsWithinSweepTolerance(t *testing.T) {
	p := DefaultReplayProfile()
	res, err := Replay(context.Background(), rsObservations(), p)
	if err != nil {
		t.Fatalf("Replay galat: %v", err)
	}
	rep := Compare(rsHistory(), res, p)
	if len(rep.Events) != 1 {
		t.Fatalf("perbandingan event = %d; mau 1", len(rep.Events))
	}
	c := rep.Events[0]
	if len(c.Timings) != 2 {
		t.Fatalf("Timings = %d; mau 2", len(c.Timings))
	}

	if got := c.Timings[0]; got.HistoricMs != 0 || got.ReplayedMs != 0 || got.DifferenceMs != 0 {
		t.Errorf("rev1 adalah basis relatif, harus nol di kedua sisi: %+v", got)
	}
	histDelta := rsDecided2 - rsDecided1
	if c.Timings[1].HistoricMs != histDelta {
		t.Errorf("delta historis = %d; mau %d", c.Timings[1].HistoricMs, histDelta)
	}
	if !c.TimingWithinTolerance() {
		t.Errorf("delta di luar toleransi %d ms: historis=%d replay=%d selisih=%d",
			p.tolerance(), c.Timings[1].HistoricMs, c.Timings[1].ReplayedMs, c.Timings[1].DifferenceMs)
	}
	t.Logf("delta RESOLVED: historis=%dms replay=%dms selisih=%dms toleransi=%dms",
		c.Timings[1].HistoricMs, c.Timings[1].ReplayedMs, c.Timings[1].DifferenceMs, p.tolerance())
}

// Baris kedua yang diumpankan ulang (obs_seq SAMA) tidak boleh melahirkan event
// kedua maupun transisi kedua: verifier dedup mendahului ledger, dan
// isSecondEpisodeLocked mengembalikan false pada obs_seq yang sama.
func TestRealSensorRefeedProducesNoSecondEvent(t *testing.T) {
	rows := rsObservations()
	dup := rows[1]
	dup.ObservationID = 30
	dup.ReceivedTS = rows[1].ReceivedTS + 25
	rows = append(rows, dup)

	res, err := Replay(context.Background(), rows, DefaultReplayProfile())
	if err != nil {
		t.Fatalf("Replay galat: %v", err)
	}
	if len(res.Events) != 1 {
		t.Errorf("event = %d; mau 1", len(res.Events))
	}
	if len(res.Frames) != 2 {
		t.Errorf("frame = %d; mau 2 (UNCONFIRMED, RESOLVED)", len(res.Frames))
	}
}
