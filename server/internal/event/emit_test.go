package event

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/banana-pixel/quakealert/server/internal/dispatch"
)

// snapTo membangun Snapshot minimal untuk state tujuan tertentu.
func snapTo(to State, everConfirmed bool) Snapshot {
	return Snapshot{
		EventID:        "E0000000-0000-4000-8000-000000000000",
		From:           StateDetected,
		To:             to,
		Reason:         ReasonFloorMet,
		Revision:       3,
		OriginTS:       onsetBase,
		OriginTSSource: OnsetSourceSensor,
		DecidedAt:      onsetBase + 4000,
		CentroidLat:    baseLat,
		CentroidLon:    baseLon,
		LocationName:   "N1 site",
		PeakPGA:        MinPGAGal + 10,
		MMIScale:       "V",
		IntensityLabel: "moderate",

		NodeCount:        3,
		IndependentCells: 2,
		EverConfirmed:    everConfirmed,
	}
}

// Tabel §8.1 sebagai tabel: type yang diumumkan dan hak FCM, per state tujuan.
// Nilai type tidak boleh bertambah (D11), jadi ekspektasinya ditulis sebagai
// konstanta dispatch yang SUDAH ADA sejak Fase 2.
func TestFrameForFollowsEmissionTable(t *testing.T) {
	cases := []struct {
		to            State
		everConfirmed bool
		wantType      string
		wantPush      bool
		wantOK        bool
	}{
		{StateDetected, false, "", false, false},
		{StateUnconfirmed, false, dispatch.TypeAdvisory, false, true},
		{StateConfirmed, false, dispatch.TypeAlert, true, true},
		{StateResolved, true, dispatch.TypeResolved, true, true},
		{StateResolved, false, dispatch.TypeResolved, false, true},
		{StateCancelled, true, dispatch.TypeResolved, true, true},
		{StateCancelled, false, dispatch.TypeResolved, false, true},
	}

	for _, c := range cases {
		msg, push, ok := FrameFor(snapTo(c.to, c.everConfirmed))
		if ok != c.wantOK {
			t.Errorf("FrameFor(%s ever=%v) ok = %v, mau %v", c.to, c.everConfirmed, ok, c.wantOK)
			continue
		}
		if !ok {
			if msg != nil {
				t.Errorf("FrameFor(%s) mengembalikan frame walau ok=false", c.to)
			}
			continue
		}
		if msg.Type != c.wantType {
			t.Errorf("FrameFor(%s) type = %q, mau %q", c.to, msg.Type, c.wantType)
		}
		if push != c.wantPush {
			t.Errorf("FrameFor(%s ever=%v) push = %v, mau %v", c.to, c.everConfirmed, push, c.wantPush)
		}
	}
}

// Enam field aditif §8.3 harus benar-benar sampai ke frame, dan timestamp harus
// tetap waktu KEPUTUSAN — bukan onset.
func TestFrameForCarriesTheSixAdditiveFields(t *testing.T) {
	s := snapTo(StateConfirmed, false)
	msg, _, ok := FrameFor(s)
	if !ok {
		t.Fatal("FrameFor(CONFIRMED) ok = false")
	}

	if msg.EventState != string(StateConfirmed) {
		t.Errorf("event_state = %q, mau CONFIRMED", msg.EventState)
	}
	if msg.EventRevision != s.Revision {
		t.Errorf("event_revision = %d, mau %d", msg.EventRevision, s.Revision)
	}
	if msg.OriginTS != s.OriginTS {
		t.Errorf("origin_ts = %d, mau %d", msg.OriginTS, s.OriginTS)
	}
	if msg.OriginTSSource != OnsetSourceSensor {
		t.Errorf("origin_ts_source = %q, mau SENSOR", msg.OriginTSSource)
	}
	if msg.NodeCount != s.NodeCount {
		t.Errorf("node_count = %d, mau %d", msg.NodeCount, s.NodeCount)
	}
	if msg.IndependentCellCount != s.IndependentCells {
		t.Errorf("independent_cell_count = %d, mau %d", msg.IndependentCellCount, s.IndependentCells)
	}
	if msg.Timestamp != s.DecidedAt {
		t.Errorf("timestamp = %d, mau DecidedAt %d", msg.Timestamp, s.DecidedAt)
	}
	if msg.EventID != s.EventID {
		t.Errorf("event_id = %q, mau %q", msg.EventID, s.EventID)
	}
}

// Frame Fase 2 harus tetap byte-identik: semua field baru omitempty, jadi frame
// yang tidak mengisinya tidak boleh menumbuhkan satu kunci pun — di WebSocket
// maupun di data FCM.
func TestPhase2FrameKeysUnchanged(t *testing.T) {
	msg := &dispatch.AlertMessage{
		Type: dispatch.TypeAlert, EventID: "legacy", MMI: "V", IntensityLabel: "moderate",
		PGAGal: 20, CentroidLat: baseLat, CentroidLon: baseLon,
		LocationName: "N1 site", Timestamp: onsetBase, NodeCount: 3,
	}

	b, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, k := range []string{"event_state", "event_revision", "origin_ts", "origin_ts_source", "independent_cell_count"} {
		if _, ok := got[k]; ok {
			t.Errorf("frame Fase 2 memuat %q; field aditif harus omitempty", k)
		}
	}

	data := dispatch.BuildAlertData(msg)
	for _, k := range []string{"event_state", "event_revision", "origin_ts", "origin_ts_source", "independent_cell_count"} {
		if _, ok := data[k]; ok {
			t.Errorf("data FCM Fase 2 memuat %q; field aditif harus bersyarat", k)
		}
	}
}

// Data FCM adalah peta string-ke-string; angka Fase 3 harus sampai sebagai
// string, dan sampai secara utuh.
func TestFCMDataCarriesAdditiveFieldsAsStrings(t *testing.T) {
	msg, _, ok := FrameFor(snapTo(StateConfirmed, false))
	if !ok {
		t.Fatal("FrameFor(CONFIRMED) ok = false")
	}

	data := dispatch.BuildAlertData(msg)
	want := map[string]string{
		"event_state":            "CONFIRMED",
		"event_revision":         "3",
		"origin_ts_source":       OnsetSourceSensor,
		"independent_cell_count": "2",
	}
	for k, v := range want {
		if data[k] != v {
			t.Errorf("data FCM %q = %q, mau %q", k, data[k], v)
		}
	}
	if data["origin_ts"] == "" {
		t.Error("data FCM origin_ts kosong")
	}
}

// ---- Bridge ----------------------------------------------------------------

type recSink struct {
	msgs  []*dispatch.AlertMessage
	pushs []bool
}

func (r *recSink) DispatchEventFrame(_ context.Context, msg *dispatch.AlertMessage, push bool) {
	r.msgs = append(r.msgs, msg)
	r.pushs = append(r.pushs, push)
}

// Bridge meneruskan tepat frame yang diumumkan, dan MENELAN yang tidak: sebuah
// transisi tak-publik tidak boleh menjadi panggilan sink kosong.
func TestBridgeForwardsOnlyAnnouncedTransitions(t *testing.T) {
	sink := &recSink{}
	b := NewBridge(sink)

	b.EmitTransition(context.Background(), snapTo(StateDetected, false))
	if len(sink.msgs) != 0 {
		t.Fatalf("panggilan sink = %d untuk -> DETECTED, mau 0", len(sink.msgs))
	}

	b.EmitTransition(context.Background(), snapTo(StateUnconfirmed, false))
	b.EmitTransition(context.Background(), snapTo(StateConfirmed, false))
	if len(sink.msgs) != 2 {
		t.Fatalf("panggilan sink = %d, mau 2", len(sink.msgs))
	}
	if sink.pushs[0] || !sink.pushs[1] {
		t.Errorf("push = %v, mau [false true]", sink.pushs)
	}
}

// Bridge tanpa sink membuang frame tanpa panik: server yang berjalan tanpa Hub
// maupun FCM tetap harus dapat melacak event.
func TestBridgeWithoutSinkIsSafe(t *testing.T) {
	NewBridge(nil).EmitTransition(context.Background(), snapTo(StateConfirmed, false))
	var nilBridge *Bridge
	nilBridge.EmitTransition(context.Background(), snapTo(StateConfirmed, false))
}
