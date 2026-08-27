package dispatch

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/banana-pixel/quakealert/server/internal/ledger"
)

// failingFCM menggagalkan pengiriman yang tokennya (atau topiknya) memuat
// failMatch. Fake terpisah dari fakeFCM, yang selalu berhasil: yang diuji di
// sini justru selisih antara "dicoba" dan "berhasil", dan fake yang tidak pernah
// gagal tidak dapat membedakan keduanya.
type failingFCM struct {
	failMatch string

	mu    sync.Mutex
	sends []*FCMMessage
}

func (f *failingFCM) Send(_ context.Context, m *FCMMessage) error {
	f.mu.Lock()
	f.sends = append(f.sends, m)
	f.mu.Unlock()

	target := m.Token + m.Topic
	if f.failMatch != "" && strings.Contains(target, f.failMatch) {
		return errors.New("fcm ditolak (fake)")
	}
	return nil
}

func (f *failingFCM) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sends)
}

// deliveryFixture sama seperti guardFixture tetapi dengan FCM yang dapat gagal.
func deliveryFixture(t *testing.T, tokens []string, failMatch string) (*Dispatcher, *failingFCM, *fakeEmissionWriter, *client) {
	t.Helper()
	fcm := &failingFCM{failMatch: failMatch}
	h := NewHub(testLogger(), func(*http.Request) bool { return true })
	d := NewDispatcher(&fakeTargetedSaver{tokens: tokens}, h, fcm, time.Hour, testLogger())
	w := &fakeEmissionWriter{}
	d.SetLedger(w)
	return d, fcm, w, registerClient(h)
}

// firstRow menunggu satu baris emisi dan mengembalikannya.
func firstRow(t *testing.T, w *fakeEmissionWriter) *ledger.Emission {
	t.Helper()
	if !waitFor(func() bool { return len(w.snapshot()) == 1 }) {
		t.Fatalf("timeout menunggu baris emisi (got %d)", len(w.snapshot()))
	}
	return w.snapshot()[0]
}

// ---------------------------------------------------------------------------
// Hub.Broadcast: yang dihitung adalah frame yang benar-benar masuk antrean
// ---------------------------------------------------------------------------

func TestBroadcastCountsOnlyEnqueuedClients(t *testing.T) {
	h := NewHub(testLogger(), nil)
	a := registerClient(h)
	registerClient(h)

	if got := h.Broadcast(&AlertMessage{Type: TypeAlert}); got != 2 {
		t.Fatalf("jumlah penerima = %d, want 2", got)
	}
	// Kuras milik a saja, lalu penuhi antreannya sampai kapasitas: klien lambat
	// tidak menerima peringatan berikutnya dan tidak boleh dihitung.
	<-a.send
	for len(a.send) < cap(a.send) {
		a.send <- []byte("penuh")
	}
	if got := h.Broadcast(&AlertMessage{Type: TypeAlert}); got != 1 {
		t.Errorf("jumlah penerima = %d, want 1 (klien lambat tidak dihitung)", got)
	}
}

// ---------------------------------------------------------------------------
// §6.4/§13.3 — hasil pengiriman pada baris alert_emissions
// ---------------------------------------------------------------------------

// Jalur bertarget: attempted = jumlah token, succeeded = yang tidak error, dan
// ws_client_count = klien yang benar-benar menerima frame.
func TestDeliveryOutcomeRecordedForTargetedTokens(t *testing.T) {
	d, fcm, w, c := deliveryFixture(t, []string{"token-a", "token-b"}, "")

	before := time.Now().UnixMilli()
	d.Dispatch(t.Context(), nonSevereEvent(3))
	readMessage(t, c.send)

	if !waitFor(func() bool { return fcm.count() == 2 }) {
		t.Fatalf("pengiriman = %d, want 2", fcm.count())
	}
	row := firstRow(t, w)
	if row.Audience != ledger.AudienceTokensRadius {
		t.Fatalf("audience = %q, want %q", row.Audience, ledger.AudienceTokensRadius)
	}
	assertCounts(t, row, 1, 2, 2)
	if row.DeliveryAt == nil {
		t.Fatal("delivery_at NULL padahal pengiriman teramati")
	}
	if *row.DeliveryAt < before || *row.DeliveryAt < row.DecidedAt {
		t.Errorf("delivery_at = %d, harus >= decided_at %d", *row.DeliveryAt, row.DecidedAt)
	}
}

// Kegagalan sebagian: satu token gagal tidak mengurangi attempted, hanya
// succeeded. Inilah satu-satunya alasan kedua kolom itu terpisah.
func TestDeliveryOutcomePartialTokenFailure(t *testing.T) {
	d, fcm, w, c := deliveryFixture(t, []string{"token-ok", "token-mati"}, "mati")

	d.Dispatch(t.Context(), nonSevereEvent(3))
	readMessage(t, c.send)

	if !waitFor(func() bool { return fcm.count() == 2 }) {
		t.Fatalf("pengiriman = %d, want 2", fcm.count())
	}
	assertCounts(t, firstRow(t, w), 1, 2, 1)
}

// GeoTopic adalah SATU request: attempted 1, dan succeeded 0 bila Send gagal.
func TestDeliveryOutcomeGeoTopicFailure(t *testing.T) {
	d, fcm, w, c := deliveryFixture(t, nil, GeoTopic)

	d.Dispatch(t.Context(), nonSevereEvent(3)) // tanpa token -> fallback topic
	readMessage(t, c.send)

	if !waitFor(func() bool { return fcm.count() == 1 }) {
		t.Fatalf("pengiriman = %d, want 1", fcm.count())
	}
	row := firstRow(t, w)
	if row.Audience != ledger.AudienceGeoTopicAll {
		t.Fatalf("audience = %q, want %q", row.Audience, ledger.AudienceGeoTopicAll)
	}
	assertCounts(t, row, 1, 1, 0)
}

// Guard satu-node menahan pengiriman: FCM DIKONFIGURASI dan keputusannya nol.
// 0, bukan NULL — nol yang teramati adalah fakta, bukan ketiadaan data.
func TestDeliveryOutcomeGuardedIsObservedZero(t *testing.T) {
	d, fcm, w, c := deliveryFixture(t, nil, "")

	d.Dispatch(t.Context(), severeEvent(1))
	readMessage(t, c.send)

	row := firstRow(t, w)
	if row.Audience != ledger.AudienceNone {
		t.Fatalf("audience = %q, want %q", row.Audience, ledger.AudienceNone)
	}
	if fcm.count() != 0 {
		t.Errorf("pengiriman = %d, want 0 (guard menahan FCM)", fcm.count())
	}
	assertCounts(t, row, 1, 0, 0)
}

// FCM tidak dikonfigurasi: tidak ada apa pun yang teramati tentang pengiriman
// FCM, jadi kedua kolomnya NULL. Jangkauan WebSocket tetap teramati dan tetap
// dicatat — 0 di sana akan menyembunyikan siaran yang benar-benar terjadi.
func TestDeliveryOutcomeNilFCMLeavesFCMCountsNull(t *testing.T) {
	h := NewHub(testLogger(), func(*http.Request) bool { return true })
	d := NewDispatcher(&fakeSaver{}, h, nil, time.Hour, testLogger())
	w := &fakeEmissionWriter{}
	d.SetLedger(w)
	c := registerClient(h)

	d.Dispatch(t.Context(), confirmedEvent())
	readMessage(t, c.send)

	row := firstRow(t, w)
	if row.FCMAttempted != nil || row.FCMSucceeded != nil {
		t.Errorf("fcm_attempted/fcm_succeeded harus NULL tanpa FCM, got %v/%v",
			row.FCMAttempted, row.FCMSucceeded)
	}
	if row.WSClientCount == nil || *row.WSClientCount != 1 {
		t.Errorf("ws_client_count = %v, want 1", row.WSClientCount)
	}
	if row.DeliveryAt == nil {
		t.Error("delivery_at NULL padahal siaran WebSocket teramati")
	}
}

// assertCounts memeriksa ketiga hitungan sekaligus; nil dilaporkan sebagai
// kegagalan karena setiap pemanggil di sini memang mengharapkan angka.
func assertCounts(t *testing.T, row *ledger.Emission, ws, attempted, succeeded int) {
	t.Helper()
	if row.WSClientCount == nil || *row.WSClientCount != ws {
		t.Errorf("ws_client_count = %v, want %d", row.WSClientCount, ws)
	}
	if row.FCMAttempted == nil || *row.FCMAttempted != attempted {
		t.Errorf("fcm_attempted = %v, want %d", row.FCMAttempted, attempted)
	}
	if row.FCMSucceeded == nil || *row.FCMSucceeded != succeeded {
		t.Errorf("fcm_succeeded = %v, want %d", row.FCMSucceeded, succeeded)
	}
}
