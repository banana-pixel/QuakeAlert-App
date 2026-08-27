package event

import (
	"testing"

	"github.com/banana-pixel/quakealert/server/internal/ingest"
	"github.com/banana-pixel/quakealert/server/internal/ledger"
)

// Nilai onset_source di paket ini adalah salinan nilai kolom
// sensor_observations.onset_source yang dimiliki paket ledger. Uji ini satu-satunya
// yang menahan keduanya tetap sama; menyimpang berarti satu baris ledger dan satu
// baris earthquake_events akan menyebut asal yang sama dengan dua nama.
func TestOnsetSourceConstantsMatchLedger(t *testing.T) {
	if OnsetSourceSensor != ledger.OnsetSourceSensor {
		t.Errorf("SENSOR menyimpang: event=%q ledger=%q", OnsetSourceSensor, ledger.OnsetSourceSensor)
	}
	if OnsetSourcePublish != ledger.OnsetSourcePublish {
		t.Errorf("PUBLISH_BOUND menyimpang: event=%q ledger=%q", OnsetSourcePublish, ledger.OnsetSourcePublish)
	}
	if PhasePrelim != ledger.PhasePrelim || PhaseFinal != ledger.PhaseFinal {
		t.Errorf("phase menyimpang: event=%q/%q ledger=%q/%q",
			PhasePrelim, PhaseFinal, ledger.PhasePrelim, ledger.PhaseFinal)
	}
}

func ptrI64(v int64) *int64 { return &v }
func ptrInt(v int) *int     { return &v }

// v1: tidak ada onset di kabel, jadi jangkarnya adalah ts - dur_ms dan asalnya
// PUBLISH_BOUND. Sebuah BATAS, dan kolomnya mengatakan demikian.
func TestObservationFromV1UsesPublishBound(t *testing.T) {
	tr := &ingest.Trigger{
		NodeID: "NODE-00000001",
		PGA:    42.5,
		DurMs:  8000,
		TS:     1_700_000_000_000,
	}

	in := ObservationFrom(tr)

	if in.OnsetTS != 1_699_999_992_000 {
		t.Errorf("OnsetTS = %d, mau ts - dur_ms = 1699999992000", in.OnsetTS)
	}
	if in.OnsetSource != OnsetSourcePublish {
		t.Errorf("OnsetSource = %q, mau PUBLISH_BOUND", in.OnsetSource)
	}
	if in.Phase != PhaseFinal {
		t.Errorf("Phase = %q, mau FINAL: v1 dipublish saat event sudah selesai", in.Phase)
	}
	if in.ObsSeq != nil || in.AttemptNo != nil || in.DetriggerTS != nil {
		t.Errorf("field v2 harus nil untuk v1: obs_seq=%v attempt=%v detrigger=%v",
			in.ObsSeq, in.AttemptNo, in.DetriggerTS)
	}
	if in.PublishTS != tr.TS || in.PGA != tr.PGA || in.DurMs != tr.DurMs || in.NodeID != tr.NodeID {
		t.Errorf("field yang diteruskan apa adanya berubah: %+v", in)
	}
	if in.Lat != 0 || in.Lon != 0 || in.LocationName != "" {
		t.Error("ObservationFrom tidak boleh mengarang lokasi; itu tugas GetNodeLocation di Tracker")
	}
}

// v2 PRELIM: onset terukur sensor, detrigger belum ada.
func TestObservationFromV2PrelimUsesSensorOnset(t *testing.T) {
	tr := &ingest.Trigger{
		NodeID:    "NODE-00000002",
		PGA:       18.0,
		DurMs:     1300,
		TS:        1_700_000_001_500,
		ProtoVer:  ptrInt(ingest.ProtoVerV2),
		Phase:     ingest.PhasePrelim,
		ObsSeq:    ptrI64(7),
		AttemptNo: ptrInt(1),
		OnsetTS:   ptrI64(1_700_000_000_300),
	}

	in := ObservationFrom(tr)

	if in.OnsetTS != 1_700_000_000_300 {
		t.Errorf("OnsetTS = %d, mau onset_ts payload", in.OnsetTS)
	}
	if in.OnsetSource != OnsetSourceSensor {
		t.Errorf("OnsetSource = %q, mau SENSOR", in.OnsetSource)
	}
	if in.Phase != PhasePrelim {
		t.Errorf("Phase = %q, mau PRELIM", in.Phase)
	}
	if in.ObsSeq == nil || *in.ObsSeq != 7 {
		t.Errorf("ObsSeq = %v, mau 7", in.ObsSeq)
	}
	if in.AttemptNo == nil || *in.AttemptNo != 1 {
		t.Errorf("AttemptNo = %v, mau 1", in.AttemptNo)
	}
	if in.DetriggerTS != nil {
		t.Errorf("DetriggerTS = %v, mau nil pada PRELIM", in.DetriggerTS)
	}
	// Onset v2 TIDAK boleh dihitung dari ts - dur_ms, walau keduanya tersedia.
	if in.OnsetTS == tr.TS-tr.DurMs {
		t.Error("onset v2 tampak dihitung dari batas publish, bukan dibaca dari payload")
	}
}

// v2 FINAL: detrigger diteruskan.
func TestObservationFromV2FinalCarriesDetrigger(t *testing.T) {
	tr := &ingest.Trigger{
		NodeID:      "NODE-00000003",
		PGA:         210.0,
		DurMs:       9000,
		TS:          1_700_000_010_000,
		ProtoVer:    ptrInt(ingest.ProtoVerV2),
		Phase:       ingest.PhaseFinal,
		ObsSeq:      ptrI64(7),
		AttemptNo:   ptrInt(2),
		OnsetTS:     ptrI64(1_700_000_000_300),
		DetriggerTS: ptrI64(1_700_000_009_300),
	}

	in := ObservationFrom(tr)

	if in.Phase != PhaseFinal {
		t.Errorf("Phase = %q, mau FINAL", in.Phase)
	}
	if in.DetriggerTS == nil || *in.DetriggerTS != 1_700_000_009_300 {
		t.Errorf("DetriggerTS = %v, mau 1700000009300", in.DetriggerTS)
	}
	if in.OnsetSource != OnsetSourceSensor {
		t.Errorf("OnsetSource = %q, mau SENSOR", in.OnsetSource)
	}
}

// Retry v2 dari episode yang sama membawa ts yang lebih baru tetapi onset yang
// SAMA — properti yang membuat first-bound-wins (D29) sepele untuk v2 dan penting
// untuk v1.
func TestObservationFromV2RetryKeepsSameOnset(t *testing.T) {
	base := ingest.Trigger{
		NodeID: "NODE-00000004", PGA: 30, DurMs: 3000,
		ProtoVer: ptrInt(ingest.ProtoVerV2), Phase: ingest.PhasePrelim,
		ObsSeq: ptrI64(11), OnsetTS: ptrI64(1_700_000_000_000),
	}
	first := base
	first.TS = 1_700_000_003_000
	first.AttemptNo = ptrInt(1)
	retry := base
	retry.TS = 1_700_000_030_000
	retry.AttemptNo = ptrInt(3)

	a, b := ObservationFrom(&first), ObservationFrom(&retry)
	if a.OnsetTS != b.OnsetTS {
		t.Fatalf("onset berubah antar-retry: %d lalu %d", a.OnsetTS, b.OnsetTS)
	}
	if a.PublishTS == b.PublishTS {
		t.Fatal("uji ini tidak berarti bila kedua publish_ts sama")
	}
}

// Sebaliknya untuk v1: retry membawa batas yang LEBIH LONGGAR, dan itu alasan
// first-bound-wins ada.
func TestObservationFromV1RetryProducesLooserBound(t *testing.T) {
	first := &ingest.Trigger{NodeID: "NODE-00000005", PGA: 30, DurMs: 3000, TS: 1_700_000_003_000}
	retry := &ingest.Trigger{NodeID: "NODE-00000005", PGA: 30, DurMs: 3000, TS: 1_700_000_030_000}

	a, b := ObservationFrom(first), ObservationFrom(retry)
	if !(b.OnsetTS > a.OnsetTS) {
		t.Fatalf("batas retry %d seharusnya lebih baru (lebih longgar) dari %d", b.OnsetTS, a.OnsetTS)
	}
}
