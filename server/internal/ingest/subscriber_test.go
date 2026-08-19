package ingest

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// --- Fakes ---

// fakeMessage adalah mqtt.Message minimal untuk menguji callback subscriber
// tanpa broker nyata.
type fakeMessage struct {
	topic   string
	payload []byte
}

func (m fakeMessage) Duplicate() bool   { return false }
func (m fakeMessage) Qos() byte         { return 1 }
func (m fakeMessage) Retained() bool    { return false }
func (m fakeMessage) Topic() string     { return m.topic }
func (m fakeMessage) MessageID() uint16 { return 1 }
func (m fakeMessage) Payload() []byte   { return m.payload }
func (m fakeMessage) Ack()              {}

var _ mqtt.Message = fakeMessage{}

// fakeVerifier mencatat apakah pipa verifikasi (HMAC + IO DB) sampai dipanggil.
type fakeVerifier struct {
	calls   int
	lastID  string
	failErr error
}

func (f *fakeVerifier) VerifyTrigger(_ context.Context, t *Trigger) error {
	f.calls++
	f.lastID = t.NodeID
	return f.failErr
}

// subFixture merangkai Subscriber dengan verifier fake dan merekam trigger yang
// lolos sampai handler. Client MQTT nil aman karena test memanggil callback
// (onMessage/onHeartbeat) secara langsung, bukan via broker.
type subFixture struct {
	sub        *Subscriber
	verifier   *fakeVerifier
	triggers   []*Trigger
	heartbeats []string
}

func newSubFixture(verifyErr error) *subFixture {
	f := &subFixture{verifier: &fakeVerifier{failErr: verifyErr}}
	f.sub = NewSubscriber(nil, f.verifier, func(_ context.Context, t *Trigger) {
		f.triggers = append(f.triggers, t)
	}, hbTestLogger(), 2*time.Second)
	return f
}

// withHeartbeat mengaktifkan jalur heartbeat dengan waktu server tetap (fixedNow).
func (f *subFixture) withHeartbeat() *subFixture {
	v := NewHeartbeatValidator(hbTestLogger())
	v.now = func() time.Time { return fixedNow }
	f.sub.WithHeartbeat(v, func(_ context.Context, h *Heartbeat, _ int) {
		f.heartbeats = append(f.heartbeats, h.ID)
	})
	return f
}

// triggerPayload membangun payload trigger yang valid secara struktur. Signature
// hanya perlu berpola 64 hex — kebenaran HMAC-nya diuji di hmac_test.go.
func triggerPayload(nodeID string, tsMs int64) []byte {
	const sig = "b26a6f9e1a18d02a347a1d8605eedf8f37e229933336f739075874ac92185128"
	return []byte(`{"node_id":"` + nodeID + `","pga":413.13,"dur_ms":8000,"ts":` +
		strconv.FormatInt(tsMs, 10) + `,"signature":"` + sig + `"}`)
}

func heartbeatPayload(stationID string, tsMs int64) []byte {
	return []byte(`{"id":"` + stationID + `","rssi":-61,"uptime_s":10,"ts":` +
		strconv.FormatInt(tsMs, 10) + `}`)
}

// --- Tests: HIGH-3 cross-check topik vs payload pada trigger ---

// Node yang memegang kredensial broker sah tidak boleh dapat mem-publish trigger
// atas nama node LAIN dengan mengirim node_id berbeda ke topiknya sendiri.
// Penolakan wajib terjadi SEBELUM verifikasi HMAC / IO DB.
func TestOnMessage_TopicStationIDMismatch(t *testing.T) {
	f := newSubFixture(nil)

	f.sub.onMessage(nil, fakeMessage{
		topic:   "sensor/NODE-AAAAAAAA/trigger",
		payload: triggerPayload("NODE-BBBBBBBB", fixedNow.UnixMilli()),
	})

	if f.verifier.calls != 0 {
		t.Fatalf("verifier dipanggil %d kali; mismatch harus ditolak sebelum HMAC", f.verifier.calls)
	}
	if len(f.triggers) != 0 {
		t.Fatalf("handler dipanggil %d kali; trigger mismatch harus di-drop", len(f.triggers))
	}
}

func TestOnMessage_TopicStationIDMatch(t *testing.T) {
	f := newSubFixture(nil)

	const id = "NODE-163A149F"
	f.sub.onMessage(nil, fakeMessage{
		topic:   "sensor/" + id + "/trigger",
		payload: triggerPayload(id, fixedNow.UnixMilli()),
	})

	if f.verifier.calls != 1 {
		t.Fatalf("verifier dipanggil %d kali, mau 1", f.verifier.calls)
	}
	if f.verifier.lastID != id {
		t.Fatalf("node_id yang diverifikasi = %q, mau %q", f.verifier.lastID, id)
	}
	if len(f.triggers) != 1 || f.triggers[0].NodeID != id {
		t.Fatalf("handler tidak menerima trigger yang cocok: %+v", f.triggers)
	}
}

// Topik di luar bentuk kontrak "sensor/<station_id>/trigger" ditolak meski
// payload valid: StationIDFromTopic gagal, jadi tidak ada identitas topik yang
// bisa dicocokkan — fail closed.
func TestOnMessage_TopicMalformed(t *testing.T) {
	const id = "NODE-163A149F"
	topics := []string{
		"sensor/" + id,
		"other/" + id + "/trigger",
		"sensor//trigger",
		"sensor/" + id + "/trigger/extra",
		"",
	}
	for _, topic := range topics {
		t.Run(topic, func(t *testing.T) {
			f := newSubFixture(nil)
			f.sub.onMessage(nil, fakeMessage{
				topic:   topic,
				payload: triggerPayload(id, fixedNow.UnixMilli()),
			})
			if f.verifier.calls != 0 || len(f.triggers) != 0 {
				t.Fatalf("topik %q harus ditolak (verifier=%d, handler=%d)",
					topic, f.verifier.calls, len(f.triggers))
			}
		})
	}
}

// Payload yang rusak strukturnya di-drop sebelum apa pun; verifier tidak dipanggil.
func TestOnMessage_PayloadInvalid(t *testing.T) {
	f := newSubFixture(nil)

	f.sub.onMessage(nil, fakeMessage{
		topic:   "sensor/NODE-163A149F/trigger",
		payload: []byte(`{"node_id":"NODE-163A149F","pga":`),
	})

	if f.verifier.calls != 0 || len(f.triggers) != 0 {
		t.Fatalf("payload rusak harus di-drop (verifier=%d, handler=%d)",
			f.verifier.calls, len(f.triggers))
	}
}

// Trigger dengan topik cocok tetapi gagal verifikasi (mis. HMAC invalid) tidak
// diteruskan ke handler.
func TestOnMessage_VerifierMenolak(t *testing.T) {
	f := newSubFixture(ErrBadSignature)

	const id = "NODE-163A149F"
	f.sub.onMessage(nil, fakeMessage{
		topic:   "sensor/" + id + "/trigger",
		payload: triggerPayload(id, fixedNow.UnixMilli()),
	})

	if f.verifier.calls != 1 {
		t.Fatalf("verifier dipanggil %d kali, mau 1", f.verifier.calls)
	}
	if len(f.triggers) != 0 {
		t.Fatalf("handler dipanggil meski verifikasi gagal: %+v", f.triggers)
	}
}

// --- Tests: cross-check heartbeat (jalur tanpa signature) ---

func TestOnHeartbeat_TopicStationIDMismatch(t *testing.T) {
	f := newSubFixture(nil).withHeartbeat()

	f.sub.onHeartbeat(nil, fakeMessage{
		topic:   "sensor/NODE-AAAAAAAA/heartbeat",
		payload: heartbeatPayload("NODE-BBBBBBBB", fixedNow.UnixMilli()),
	})
	if len(f.heartbeats) != 0 {
		t.Fatalf("heartbeat mismatch harus di-drop, dapat %v", f.heartbeats)
	}

	// Kontrol positif: topik yang cocok tetap diterima.
	f.sub.onHeartbeat(nil, fakeMessage{
		topic:   "sensor/NODE-AAAAAAAA/heartbeat",
		payload: heartbeatPayload("NODE-AAAAAAAA", fixedNow.UnixMilli()),
	})
	if len(f.heartbeats) != 1 || f.heartbeats[0] != "NODE-AAAAAAAA" {
		t.Fatalf("heartbeat yang cocok harus diterima, dapat %v", f.heartbeats)
	}
}

// --- Test: Verify(raw) tetap menolak struktur invalid sebelum pipa kripto ---

func TestVerify_ParseGagalSebelumPipa(t *testing.T) {
	// store & cipher sengaja nil: payload rusak harus ditolak ParseTrigger jauh
	// sebelum ada akses DB/kripto — bila tidak, test ini panic.
	v := &Verifier{log: hbTestLogger(), now: func() time.Time { return fixedNow }}
	if _, err := v.Verify(context.Background(), []byte(`{`)); !errors.Is(err, ErrInvalidJSON) {
		t.Fatalf("err = %v, mau %v", err, ErrInvalidJSON)
	}
}
