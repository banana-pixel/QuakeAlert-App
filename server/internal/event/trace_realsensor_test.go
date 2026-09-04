package event

import (
	"testing"

	"github.com/banana-pixel/quakealert/server/internal/store"
)

// Validasi P4-M1′ atas JENDELA LEDGER SENSOR NYATA satu-satunya yang dimiliki
// proyek ini. Fixture-nya dipakai ULANG dari replay_realsensor_test.go
// (rsObservations, rsHistory) — bukan disalin — supaya kedua alat P4 menelusuri
// baris yang sama persis dan tidak ada kesempatan bagi dua salinan menyimpang.
//
// TIGA BATAS yang harus ikut disebut setiap kali uji ini dikutip:
//
//  1. alert_emissions TIDAK terekam pada sesi drill itu. Kaki emisi karenanya
//     TIDAK DIVALIDASI terhadap produksi di sini; ia diperiksa terhadap baris
//     yang disusun uji ini sendiri, dan kasus "tanpa baris emisi" diperiksa
//     terpisah agar hasilnya MISSING — bukan diam-diam terlacak.
//
//  2. evidence_summary kedua baris riwayat juga tidak terekam (lihat catatan di
//     replay_realsensor_test.go), jadi keanggotaan node yang menjadi dasar
//     tautan di sini adalah rekonstruksi. Yang nyata: node_id, waktu, dan pga.
//
//  3. S2 berlaku. Satu node berarti CONFIRMED tak terjangkau menurut kerapatan,
//     jadi UNCONFIRMED adalah tujuan yang BENAR — bukan cacat yang tertelusuri.

// Jendela nyata: dua observasi, satu di bawah lantai (PRELIM 2.2952) dan satu di
// atasnya (FINAL 73.0537). Penyebutnya karena itu 1, bukan 2, dan itu benar:
// baris PRELIM bukan pemicu yang memenuhi syarat.
func TestRealSensorTraceQualifyingObservation(t *testing.T) {
	rep := Trace(rsObservations(), rsHistory(), nil, DefaultTraceProfile(), 2)

	if rep.TotalRows != 2 {
		t.Fatalf("TotalRows = %d; mau 2", rep.TotalRows)
	}
	if rep.BelowFloor != 1 {
		t.Fatalf("BelowFloor = %d; mau 1 (PRELIM pga=2.2952)", rep.BelowFloor)
	}
	if len(rep.Excluded) != 0 {
		t.Fatalf("Excluded = %+v; mau kosong (kedua baris verify=OK dan punya lokasi)", rep.Excluded)
	}
	if len(rep.Traces) != 1 {
		t.Fatalf("Traces = %d; mau 1", len(rep.Traces))
	}

	tr := rep.Traces[0]
	if tr.ObservationID != 29 || tr.NodeID != rsNodeID || tr.PGAGal != rsPeakPGA {
		t.Errorf("baris tertelusuri = %+v; mau observation_id=29 %s pga=%v", tr, rsNodeID, rsPeakPGA)
	}
	if tr.Outcome != TraceTraced {
		t.Fatalf("hasil = %s; mau %s", tr.Outcome, TraceTraced)
	}
	if tr.EventID != rsEventID || tr.Revision != 1 {
		t.Errorf("tautan = %s#%d; mau %s#1", tr.EventID, tr.Revision, rsEventID)
	}
	if tr.DecidedAt != rsDecided1 {
		t.Errorf("DecidedAt = %d; mau %d", tr.DecidedAt, rsDecided1)
	}
	if tr.AlgoVer != rsAlgoVer {
		t.Errorf("AlgoVer = %q; mau %q", tr.AlgoVer, rsAlgoVer)
	}
	// Lag nyata: decided_at 1788255609089 - received_ts 1788255609087 = 2 ms.
	if want := rsDecided1 - rsRecv2; tr.LagMs != want {
		t.Errorf("LagMs = %d; mau %d", tr.LagMs, want)
	}
	if tr.ObsSeqLink != ObsSeqExact {
		t.Errorf("ObsSeqLink = %s; mau %s (obs_seq 1507330 di kedua sisi)", tr.ObsSeqLink, ObsSeqExact)
	}
	if !rep.SingleNodeFleet {
		t.Error("SingleNodeFleet = false pada fleet satu-node (S2)")
	}
	t.Logf("jendela nyata: obs=%d di bawah lantai=%d tertelusuri=%d transisi berbeda=%d lag=%dms",
		rep.TotalRows, rep.BelowFloor, len(rep.Traces), rep.DistinctTransitions(), tr.LagMs)
}

// Tanpa baris alert_emissions, kaki emisi HARUS melapor MISSING. Ini yang menjaga
// batas 1 di kepala berkas tetap jujur: ketiadaan rekaman tidak boleh terbaca
// sebagai frame yang terbukti terkirim.
func TestRealSensorTraceEmissionUnrecordedIsMissing(t *testing.T) {
	rep := Trace(rsObservations(), rsHistory(), nil, DefaultTraceProfile(), 2)
	if got := rep.Traces[0].EmissionOutcome; got != EmissionMissing {
		t.Fatalf("EmissionOutcome = %s; mau %s", got, EmissionMissing)
	}
	if rep.EmissionRows != 0 {
		t.Errorf("EmissionRows = %d; mau 0", rep.EmissionRows)
	}
}

// Baris emisi yang BENTUKNYA seperti yang ditulis DispatchEventFrame untuk
// transisi ini menutup rantai penuh: observasi -> event -> baris state log ->
// emisi advisory. Baris emisinya DISUSUN uji ini (batas 1), jadi yang dibuktikan
// adalah rantainya tertutup di bawah bentuk baris produksi — bukan bahwa baris
// itu ada di produksi.
func TestRealSensorTraceFullChainWithProductionShapedEmission(t *testing.T) {
	ws := 1
	emis := []store.TraceEmission{trAdvisory(1, rsEventID, 1, rsDecided1, &ws)}
	rep := Trace(rsObservations(), rsHistory(), emis, DefaultTraceProfile(), 2)

	tr := rep.Traces[0]
	if tr.Outcome != TraceTraced || tr.EmissionOutcome != EmissionByID {
		t.Fatalf("rantai = %s/%s; mau %s/%s", tr.Outcome, tr.EmissionOutcome, TraceTraced, EmissionByID)
	}
	if tr.WSClientCount == nil || *tr.WSClientCount != 1 {
		t.Errorf("WSClientCount = %v; mau 1", tr.WSClientCount)
	}
	if len(rep.Unattributed) != 0 {
		t.Errorf("Unattributed = %+v; mau kosong", rep.Unattributed)
	}
	t.Logf("rantai tertutup: obs=%d -> event=%s rev%d -> state_log decided_at=%d -> emission_id=%d ws=%d",
		tr.ObservationID, tr.EventID, tr.Revision, tr.DecidedAt, tr.EmissionID, *tr.WSClientCount)
}

// Baris RESOLVED pada jendela yang sama TIDAK boleh ikut ditelusuri, dan karena
// tidak ada observasi yang dapat menjangkaunya, ia juga TIDAK boleh muncul
// sebagai transisi tak-teratribusi — hanya transisi UNCONFIRMED yang dilacak
// kriteria ini.
func TestRealSensorTraceResolvedRowIsNeitherLinkedNorUnattributed(t *testing.T) {
	rep := Trace(rsObservations(), rsHistory(), nil, DefaultTraceProfile(), 2)
	if rep.StateLogRows != 2 {
		t.Fatalf("StateLogRows = %d; mau 2", rep.StateLogRows)
	}
	if len(rep.Unattributed) != 0 {
		t.Fatalf("Unattributed = %+v; mau kosong (baris RESOLVED tidak dilacak)", rep.Unattributed)
	}
	if rep.DistinctTransitions() != 1 {
		t.Errorf("DistinctTransitions = %d; mau 1", rep.DistinctTransitions())
	}
}
