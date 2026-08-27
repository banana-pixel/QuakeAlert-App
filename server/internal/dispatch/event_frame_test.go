package dispatch

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/banana-pixel/quakealert/server/internal/ledger"
)

// frame membangun frame Fase 3 yang sudah selesai diputuskan Tracker. Sengaja di
// bawah SeverePGAGal supaya jalur token bertarget yang diambil, bukan override
// intensitas.
func frame(alertType, state string, revision int) *AlertMessage {
	return &AlertMessage{
		Type: alertType, EventID: "E1", MMI: "VI", IntensityLabel: "strong",
		PGAGal: 120, CentroidLat: -6.9, CentroidLon: 107.6,
		LocationName: "Bandung, West Java, ID", Timestamp: time.Now().UnixMilli(),
		NodeCount: 3, EventState: state, EventRevision: revision,
		OriginTS: 1_700_000_000_000, OriginTSSource: "SENSOR", IndependentCellCount: 2,
	}
}

// fcmData membaca data pengiriman ke-i di bawah kunci fakeFCM: pengiriman terjadi
// di goroutine, jadi membacanya langsung akan menjadi race.
func fcmData(f *fakeFCM, i int) map[string]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sends[i].Data
}

func eventFrameFixture(t *testing.T, tokens []string, fcm FCMSender) (*Dispatcher, *fakeEmissionWriter, *client) {
	t.Helper()
	h := NewHub(testLogger(), func(*http.Request) bool { return true })
	d := NewDispatcher(&fakeTargetedSaver{tokens: tokens}, h, fcm, time.Hour, testLogger())
	w := &fakeEmissionWriter{}
	d.SetLedger(w)
	return d, w, registerClient(h)
}

// Frame tanpa hak FCM (UNCONFIRMED, D10) disiarkan lewat WebSocket dan TIDAK
// menyentuh FCM sama sekali — namun tetap meninggalkan baris provenance §8.5.
func TestDispatchEventFrameWithoutPushBroadcastsOnly(t *testing.T) {
	fcm := &fakeFCM{}
	d, w, c := eventFrameFixture(t, []string{"token-a"}, fcm)

	d.DispatchEventFrame(t.Context(), frame(TypeAdvisory, "UNCONFIRMED", 1), false)

	raw := readMessage(t, c.send)
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("frame WS bukan JSON: %v", err)
	}
	if got["event_state"] != "UNCONFIRMED" {
		t.Errorf("event_state pada frame WS = %v, mau UNCONFIRMED", got["event_state"])
	}

	if fcm.count() != 0 {
		t.Errorf("pengiriman FCM = %d, mau 0: UNCONFIRMED tidak membangunkan siapa pun", fcm.count())
	}
	if !waitFor(func() bool { return len(w.snapshot()) == 1 }) {
		t.Fatal("timeout menunggu baris emisi")
	}
	row := w.snapshot()[0]
	if row.Audience != ledger.AudienceNone {
		t.Errorf("audience = %q, mau %q", row.Audience, ledger.AudienceNone)
	}
	// FCM ADA tapi sengaja tidak dipakai: nol yang teramati, bukan NULL.
	if row.FCMAttempted == nil || *row.FCMAttempted != 0 {
		t.Errorf("fcm_attempted = %v, mau 0", row.FCMAttempted)
	}
	if row.WSClientCount == nil || *row.WSClientCount != 1 {
		t.Errorf("ws_client_count = %v, mau 1", row.WSClientCount)
	}
	if row.EventState == nil || *row.EventState != "UNCONFIRMED" {
		t.Errorf("event_state = %v, mau UNCONFIRMED", row.EventState)
	}
	if row.EventRevision == nil || *row.EventRevision != 1 {
		t.Errorf("event_revision = %v, mau 1", row.EventRevision)
	}
}

// Tanpa FCM terkonfigurasi, hitungan FCM tetap NULL: nol yang tidak teramati
// tidak boleh berpura-pura menjadi nol yang teramati.
func TestDispatchEventFrameWithoutFCMConfiguredLeavesCountsNull(t *testing.T) {
	d, w, c := eventFrameFixture(t, nil, nil)

	d.DispatchEventFrame(t.Context(), frame(TypeAdvisory, "UNCONFIRMED", 1), false)
	readMessage(t, c.send)

	if !waitFor(func() bool { return len(w.snapshot()) == 1 }) {
		t.Fatal("timeout menunggu baris emisi")
	}
	if row := w.snapshot()[0]; row.FCMAttempted != nil {
		t.Errorf("fcm_attempted = %v, mau NULL tanpa FCM terkonfigurasi", *row.FCMAttempted)
	}
}

// Frame yang berhak atas FCM (CONFIRMED) mengambil jalur pengiriman biasa, dan
// state Fase 3 ikut sampai ke data FCM maupun ke baris provenance.
func TestDispatchEventFrameWithPushSendsFCM(t *testing.T) {
	fcm := &fakeFCM{}
	d, w, c := eventFrameFixture(t, []string{"token-a", "token-b"}, fcm)

	d.DispatchEventFrame(t.Context(), frame(TypeAlert, "CONFIRMED", 2), true)
	readMessage(t, c.send)

	waitForSends(t, fcm, 2)
	tokens, topics := fcm.targets()
	if len(topics) != 0 {
		t.Errorf("topic tersentuh = %v, mau tidak ada pada jalur bertarget", topics)
	}
	if len(tokens) != 2 {
		t.Errorf("token terkirim = %v, mau 2", tokens)
	}
	if data := fcmData(fcm, 0); data["event_state"] != "CONFIRMED" || data["event_revision"] != "2" {
		t.Errorf("data FCM = %v, mau event_state CONFIRMED / event_revision 2", data)
	}

	if !waitFor(func() bool { return len(w.snapshot()) == 1 }) {
		t.Fatal("timeout menunggu baris emisi")
	}
	row := w.snapshot()[0]
	if row.Audience != ledger.AudienceTokensRadius {
		t.Errorf("audience = %q, mau %q", row.Audience, ledger.AudienceTokensRadius)
	}
	if row.EventState == nil || *row.EventState != "CONFIRMED" {
		t.Errorf("event_state = %v, mau CONFIRMED", row.EventState)
	}
	if row.EventRevision == nil || *row.EventRevision != 2 {
		t.Errorf("event_revision = %v, mau 2", row.EventRevision)
	}
}

// Baris Fase 2 tetap NULL pada kedua kolom baru: frame yang tidak mengumumkan
// state apa pun tidak boleh mengarang kepastian di kolom itu (§8.5).
func TestPhase2EmissionRowLeavesEventStateNull(t *testing.T) {
	d, w, c := eventFrameFixture(t, nil, nil)

	msg := frame(TypeAlert, "", 0)
	msg.OriginTS, msg.OriginTSSource, msg.IndependentCellCount = 0, "", 0
	d.DispatchEventFrame(t.Context(), msg, false)
	readMessage(t, c.send)

	if !waitFor(func() bool { return len(w.snapshot()) == 1 }) {
		t.Fatal("timeout menunggu baris emisi")
	}
	row := w.snapshot()[0]
	if row.EventState != nil || row.EventRevision != nil {
		t.Errorf("event_state/event_revision = %v/%v, mau NULL keduanya", row.EventState, row.EventRevision)
	}
}

// Frame nil tidak boleh menyiarkan apa pun maupun panik.
func TestDispatchEventFrameNilIsNoop(t *testing.T) {
	fcm := &fakeFCM{}
	d, w, c := eventFrameFixture(t, []string{"token-a"}, fcm)

	d.DispatchEventFrame(t.Context(), nil, true)

	select {
	case b := <-c.send:
		t.Fatalf("frame tersiar = %s, mau tidak ada", b)
	default:
	}
	if fcm.count() != 0 || len(w.snapshot()) != 0 {
		t.Errorf("FCM = %d, baris = %d, mau nol keduanya", fcm.count(), len(w.snapshot()))
	}
}
