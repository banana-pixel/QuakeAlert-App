package dispatch

import (
	"encoding/json"
	"testing"
	"time"
)

func testDrill() *AlertMessage {
	return &AlertMessage{
		EventID:        "test-abc",
		MMI:            "VII",
		IntensityLabel: "strong",
		PGAGal:         300,
		CentroidLat:    -6.9175,
		CentroidLon:    107.6191,
		LocationName:   "LATIHAN — bukan gempa sungguhan",
		NodeCount:      3,
		Timestamp:      time.Unix(1_781_913_558, 0).UnixMilli(),
	}
}

// Drill hanya ke topic latihan. Yang diperiksa di sini adalah ketidakhadiran
// GeoTopic dan token bertarget: sebuah drill yang sampai ke perangkat pengguna
// sungguhan mengajari orang bahwa sirene aplikasi ini kadang tidak berarti
// apa-apa, dan tidak ada rilis berikutnya yang dapat mencabut pelajaran itu.
func TestDispatchTestAlert_OnlyReachesTheTestTopic(t *testing.T) {
	saver := &fakeTargetedSaver{tokens: []string{"tok-a", "tok-b"}}
	fcm := &fakeFCM{}
	d := NewDispatcher(saver, testHub(), fcm, 0, testLogger())

	d.DispatchTestAlert(testDrill())

	waitForSends(t, fcm, 1)
	tokens, topics := fcm.targets()
	if len(tokens) != 0 {
		t.Fatalf("token = %v, mau kosong: drill tidak pernah bertarget perangkat", tokens)
	}
	if len(topics) != 1 || topics[0] != TestAlertsTopic {
		t.Fatalf("topic = %v, mau [%s]", topics, TestAlertsTopic)
	}
	// PGA 300 gal melewati SeverePGAGal, jadi jalur alert sungguhan akan
	// menyiarkannya secara nasional. Drill tetap tidak.
	if topics[0] == GeoTopic {
		t.Fatal("drill parah pun tidak boleh menyentuh topic nasional")
	}
	if fcm.sends[0].Data["is_test"] != "true" {
		t.Fatalf("data is_test = %q, mau \"true\"", fcm.sends[0].Data["is_test"])
	}
	// HIGH karena yang sedang diuji justru apakah sebuah peringatan mampu
	// membangunkan perangkat dari Doze.
	if fcm.sends[0].Priority != "HIGH" {
		t.Fatalf("priority = %q, mau HIGH", fcm.sends[0].Priority)
	}
}

// Drill tidak menulis satu baris pun: riwayat gempa yang memuat gempa yang
// tidak pernah terjadi tidak dapat dipercaya untuk apa pun.
func TestDispatchTestAlert_PersistsNothing(t *testing.T) {
	saver := &fakeSaver{}
	d := NewDispatcher(saver, testHub(), &fakeFCM{}, 0, testLogger())

	d.DispatchTestAlert(testDrill())

	saved, resolved := saver.snapshot()
	if len(saved) != 0 || len(resolved) != 0 {
		t.Fatalf("tersimpan = %d, resolved = %d, mau 0/0", len(saved), len(resolved))
	}
	// Dan tidak menyita slot resolusi yang mungkin dibutuhkan gempa sungguhan
	// beberapa detik kemudian.
	d.mu.Lock()
	active := len(d.active)
	d.mu.Unlock()
	if active != 0 {
		t.Fatalf("active = %d, mau 0: drill tidak boleh memakai state machine resolusi", active)
	}
}

// Frame WS-nya membawa is_test, karena itulah pagar kedua: build rilis
// menjatuhkan frame ini alih-alih menampilkan sirene (docs/CLIENT_SPEC.md §5.8).
func TestDispatchTestAlert_WebSocketFrameIsMarked(t *testing.T) {
	h := testHub()
	c := registerClient(h)
	d := NewDispatcher(&fakeSaver{}, h, nil, 0, testLogger())

	d.DispatchTestAlert(testDrill())

	select {
	case raw := <-c.send:
		var frame map[string]any
		if err := json.Unmarshal(raw, &frame); err != nil {
			t.Fatal(err)
		}
		if frame["is_test"] != true {
			t.Fatalf("frame is_test = %v, mau true", frame["is_test"])
		}
		if frame["type"] != TypeAlert {
			t.Fatalf("frame type = %v, mau %s", frame["type"], TypeAlert)
		}
	case <-time.After(time.Second):
		t.Fatal("tidak ada frame WS untuk drill")
	}
}

// Peringatan sungguhan tidak boleh berubah bentuk hanya karena flag ini ada:
// klien yang sudah terpasang membaca kontrak yang lama.
func TestBuildAlertData_OmitsIsTestForARealEvent(t *testing.T) {
	data := BuildAlertData(&AlertMessage{Type: TypeAlert, EventID: "event-1"})

	if _, ok := data["is_test"]; ok {
		t.Fatalf("payload gempa sungguhan tidak boleh memuat is_test: %v", data)
	}
}
