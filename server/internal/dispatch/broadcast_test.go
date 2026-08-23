package dispatch

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

// fakeRegionSaver menambahkan regionTokenFinder ke fakeSaver sehingga
// dispatchBroadcastFCM memilih jalur token per wilayah (type assertion di
// regionTokens).
type fakeRegionSaver struct {
	fakeSaver
	tokens []string
	err    error

	mu      sync.Mutex
	regions []string
}

func (f *fakeRegionSaver) FCMTokensInRegion(_ context.Context, regionCode string) ([]string, error) {
	f.mu.Lock()
	f.regions = append(f.regions, regionCode)
	f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	return f.tokens, nil
}

func (f *fakeRegionSaver) asked() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.regions...)
}

func testBroadcast() *BroadcastMessage {
	return &BroadcastMessage{
		BroadcastID: "bcast-1",
		Title:       "Uji sistem",
		Body:        "Ini pengumuman uji.",
		Timestamp:   time.Unix(1_781_913_558, 0).UnixMilli(),
	}
}

// Siaran nasional memakai topic pembaruan, BUKAN GeoTopic: seseorang yang
// berhenti berlangganan kabar tidak boleh ikut kehilangan sirene.
func TestDispatchBroadcast_NationalUsesTheUpdatesTopic(t *testing.T) {
	fcm := &fakeFCM{}
	h := testHub()
	d := NewDispatcher(&fakeSaver{}, h, fcm, 0, testLogger())

	d.DispatchBroadcast(testBroadcast())

	waitForSends(t, fcm, 1)
	_, topics := fcm.targets()
	if len(topics) != 1 || topics[0] != UpdatesTopic {
		t.Fatalf("topic = %v, mau [%s]", topics, UpdatesTopic)
	}
	if fcm.sends[0].Priority != "NORMAL" {
		t.Fatalf("priority = %q, mau NORMAL: pengumuman tidak boleh berbunyi seperti alert",
			fcm.sends[0].Priority)
	}
	if fcm.sends[0].Data["type"] != TypeBroadcast {
		t.Fatalf("data type = %q", fcm.sends[0].Data["type"])
	}
}

// Siaran regional hanya ke token wilayah itu, dan TIDAK pernah jatuh ke topic
// nasional saat wilayahnya kosong: sebuah pengumuman satu provinsi yang
// membangunkan seluruh negeri lebih buruk daripada pengumuman yang tidak
// terkirim sebagai push (barisnya tetap terbaca di daftar Pembaruan).
func TestDispatchBroadcast_RegionalTargetsTokensOnly(t *testing.T) {
	saver := &fakeRegionSaver{tokens: []string{"tok-a", "tok-b"}}
	fcm := &fakeFCM{}
	d := NewDispatcher(saver, testHub(), fcm, 0, testLogger())

	msg := testBroadcast()
	msg.RegionCode = "ID-jawa-barat"
	d.DispatchBroadcast(msg)

	waitForSends(t, fcm, 2)
	tokens, topics := fcm.targets()
	if len(tokens) != 2 {
		t.Fatalf("token = %v, mau 2", tokens)
	}
	if len(topics) != 0 {
		t.Fatalf("topic = %v, mau kosong", topics)
	}
	if asked := saver.asked(); len(asked) != 1 || asked[0] != "ID-jawa-barat" {
		t.Fatalf("wilayah yang dicari = %v", asked)
	}
}

func TestDispatchBroadcast_RegionalWithNoTokensSendsNothing(t *testing.T) {
	saver := &fakeRegionSaver{} // wilayah tanpa satu pun token aktif
	fcm := &fakeFCM{}
	d := NewDispatcher(saver, testHub(), fcm, 0, testLogger())

	msg := testBroadcast()
	msg.RegionCode = "ID-papua-selatan"
	d.DispatchBroadcast(msg)

	// Tidak ada yang bisa ditunggu; beri kesempatan goroutine FCM berjalan.
	time.Sleep(50 * time.Millisecond)
	if n := fcm.count(); n != 0 {
		t.Fatalf("%d pengiriman, mau 0: tidak ada fallback nasional untuk siaran regional", n)
	}
}

// WebSocket menyusul jalur yang sama: nasional ke semua klien, regional hanya ke
// anggota kanal wilayah itu — kanal yang sama dengan chat regional.
func TestDispatchBroadcast_WebSocketFollowsTheSameTargeting(t *testing.T) {
	h := testHub()
	inRegion := addClient(h, "global", "ID-jawa-barat")
	elsewhere := addClient(h, "global", "ID-jawa-tengah")
	d := NewDispatcher(&fakeSaver{}, h, nil, 0, testLogger())

	msg := testBroadcast()
	msg.RegionCode = "ID-jawa-barat"
	d.DispatchBroadcast(msg)

	if len(inRegion.send) != 1 {
		t.Fatalf("anggota wilayah menerima %d frame, mau 1", len(inRegion.send))
	}
	if len(elsewhere.send) != 0 {
		t.Fatalf("wilayah lain menerima %d frame, mau 0", len(elsewhere.send))
	}

	var got BroadcastMessage
	if err := json.Unmarshal(<-inRegion.send, &got); err != nil {
		t.Fatal(err)
	}
	// Type diisi dispatcher, bukan pemanggil: klien merutekan frame berdasarkan
	// bidang ini, jadi ia tidak boleh bergantung pada kedisiplinan penelepon.
	if got.Type != TypeBroadcast || got.Title != "Uji sistem" {
		t.Fatalf("frame = %+v", got)
	}

	national := testBroadcast()
	d.DispatchBroadcast(national)
	if len(inRegion.send) != 1 || len(elsewhere.send) != 1 {
		t.Fatalf("siaran nasional: %d dan %d frame, mau 1 dan 1",
			len(inRegion.send), len(elsewhere.send))
	}
}

// waitForSends menunggu jalur FCM asinkron mencapai n pengiriman.
func waitForSends(t *testing.T, fcm *fakeFCM, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if fcm.count() >= n {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("hanya %d pengiriman FCM setelah menunggu, mau %d", fcm.count(), n)
}
