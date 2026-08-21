package dispatch

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/banana-pixel/quakealert/server/internal/consensus"
	"github.com/banana-pixel/quakealert/server/internal/store"
)

// Tipe payload FCM/WS sesuai kontrak (contracts/fcm/alert_payload.json).
const (
	TypeAlert    = "EARTHQUAKE_ALERT"    // >= 3 node CONFIRMED
	TypeAdvisory = "EARTHQUAKE_ADVISORY" // 1-2 node
	TypeResolved = "EVENT_RESOLVED"      // all-clear (state machine SYSTEM_SPEC)
)

// GeoTopic default untuk broadcast FCM (topik geo global; segmentasi lebih
// halus dapat ditambahkan kemudian).
const GeoTopic = "geo_alert_all"

// fcmTimeout membatasi durasi satu pengiriman FCM HTTP v1. Dipakai pada jalur
// ASYNC (goroutine terpisah) agar kegagalan/lambatnya FCM tidak pernah
// memblokir jalur konsensus life-safety.
const fcmTimeout = 10 * time.Second

// defaultResolveAfter adalah waktu COOLDOWN_RUNNING -> RESOLVED bila tidak
// dikonfigurasi (SYSTEM_SPEC: 90 detik).
const defaultResolveAfter = 90 * time.Second

// eventSaver mengabstraksi persistensi event agar dispatcher dapat diuji.
type eventSaver interface {
	SaveEvent(ctx context.Context, e *store.EarthquakeEvent) (string, error)
	ResolveEvent(ctx context.Context, eventID string) error
}

// tokenFinder mengabstraksi pencarian token FCM bertarget. Interface terpisah
// dari eventSaver dan dideteksi lewat type assertion agar penyedia lama (dan
// fake pada test) tetap kompatibel: yang tidak mengimplementasikannya jatuh ke
// broadcast topic seperti sebelumnya.
type tokenFinder interface {
	FCMTokensWithin(ctx context.Context, lat, lon float64, rangeKm int) ([]string, error)
}

// dispatchRadiusKm adalah radius pencarian token untuk satu event.
//
// Sengaja lebar: klien menyaring sendiri dengan gate Haversine memakai radius
// pilihan user (maksimum 300 km di Settings) sebelum sirene berbunyi, jadi
// mengirim lebih luas hanya berbiaya satu notifikasi yang di-drop klien —
// sedangkan mengirim terlalu sempit berarti perangkat yang seharusnya berbunyi
// tetap diam.
const dispatchRadiusKm = 350

// maxFCMConcurrency membatasi request FCM paralel per event. HTTP v1 tidak
// punya endpoint batch, jadi satu event berarti satu request per token; batas
// ini menjaga fan-out tidak menghabiskan koneksi keluar.
const maxFCMConcurrency = 16

// Dispatcher menyatukan output Consensus Engine ke tiga kanal: persistensi
// PostGIS (hanya CONFIRMED), WebSocket Hub, dan FCM. Dirancang non-blocking
// pada jalur life-safety: kegagalan satu kanal tidak menghentikan kanal lain.
//
// FCM dikirim ASYNC di goroutine terpisah (timeout fcmTimeout). WebSocket
// broadcast sendiri sudah non-blocking (send buffer per klien). Satu-satunya IO
// sinkron di hot path adalah SaveEvent (wajib untuk mendapatkan event_id yang
// stabil sebelum broadcast).
type Dispatcher struct {
	saver        eventSaver
	hub          *Hub
	fcm          FCMSender
	log          *slog.Logger
	resolveAfter time.Duration

	mu     sync.Mutex
	active map[string]*AlertMessage // event_id yang menunggu resolusi (dedup timer)
}

// NewDispatcher membuat dispatcher. fcm boleh nil (mis. saat kredensial FCM
// belum dikonfigurasi di pengembangan) — WS tetap berjalan. resolveAfter <= 0
// memakai defaultResolveAfter (90s).
func NewDispatcher(saver eventSaver, hub *Hub, fcm FCMSender, resolveAfter time.Duration, log *slog.Logger) *Dispatcher {
	if resolveAfter <= 0 {
		resolveAfter = defaultResolveAfter
	}
	return &Dispatcher{
		saver:        saver,
		hub:          hub,
		fcm:          fcm,
		log:          log,
		resolveAfter: resolveAfter,
		active:       make(map[string]*AlertMessage),
	}
}

// Dispatch adalah consensus.EventSink: dipanggil engine untuk setiap event yang
// lolos cooldown. CONFIRMED -> persist + WS + FCM (priority HIGH). ADVISORY ->
// WS + FCM advisory (silent yellow banner), tanpa persistensi.
func (d *Dispatcher) Dispatch(ctx context.Context, ev *consensus.Event) {
	msg := &AlertMessage{
		MMI:            ev.MMIScale,
		IntensityLabel: ev.IntensityLabel,
		PGAGal:         ev.MaxPGA,
		CentroidLat:    ev.Centroid.Lat,
		CentroidLon:    ev.Centroid.Lon,
		LocationName:   ev.LocationName,
		Timestamp:      ev.CreatedAtMs,
		NodeCount:      ev.NodeCount,
	}

	switch ev.Status {
	case consensus.StatusConfirmed:
		msg.Type = TypeAlert
		// Persist event; event_id dari DB dipakai pada payload agar klien dapat
		// deduplikasi & korelasi. Cooldown di engine menjamin SATU persistensi
		// per gempa -> event_id stabil untuk seluruh siklus alert -> resolved.
		if d.saver != nil {
			id, err := d.saver.SaveEvent(ctx, &store.EarthquakeEvent{
				Status:         "HAPPENING",
				CentroidLat:    ev.Centroid.Lat,
				CentroidLon:    ev.Centroid.Lon,
				LocationName:   ev.LocationName,
				MMIScale:       ev.MMIScale,
				IntensityLabel: ev.IntensityLabel,
				MaxPGA:         ev.MaxPGA,
				TriggeredNodes: ev.NodeCount,
				StartedAtMs:    ev.CreatedAtMs,
			})
			if err != nil {
				d.log.Error("gagal simpan event", "err", err)
			} else {
				msg.EventID = id
				d.trackResolution(id, msg)
			}
		}
	case consensus.StatusAdvisory:
		msg.Type = TypeAdvisory
	default:
		d.log.Warn("status event tak dikenal, diabaikan", "status", ev.Status)
		return
	}

	// 1. WebSocket broadcast (foreground clients) — non-blocking (send buffer).
	d.hub.Broadcast(msg)

	// 2. FCM (background delivery). ASYNC agar jalur konsensus tak pernah
	//    diblokir lambatnya Google API. Priority HIGH untuk life-safety.
	d.dispatchFCM(msg)

	d.log.Info("event didispatch",
		"status", ev.Status, "mmi", ev.MMIScale, "pga_gal", ev.MaxPGA,
		"nodes", ev.NodeCount, "event_id", msg.EventID, "location", msg.LocationName)
}

// dispatchFCM mengirim FCM secara asinkron di goroutine terpisah dengan timeout
// fcmTimeout (10s). Tidak pernah memblokir jalur konsensus / broadcast.
//
// Dua jalur yang saling eksklusif: token bertarget dalam dispatchRadiusKm dari
// centroid bila ada, jika tidak broadcast ke GeoTopic. Eksklusif karena topic
// tidak bisa dikecualikan per pelanggan — mengirim keduanya akan mengembalikan
// bangun-nasional yang justru dihilangkan penargetan ini.
func (d *Dispatcher) dispatchFCM(msg *AlertMessage) {
	if d.fcm == nil {
		return
	}
	data := BuildAlertData(msg)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), fcmTimeout)
		defer cancel()

		if tokens := d.nearbyTokens(ctx, msg); len(tokens) > 0 {
			d.sendToTokens(ctx, tokens, data, msg)
			return
		}
		// Fallback, bukan tambahan: topic tidak bisa dikecualikan per pelanggan,
		// jadi mengirimnya bersamaan dengan token bertarget akan membangunkan
		// seluruh perangkat nasional lagi — persis yang penargetan ini hilangkan.
		// Dipakai hanya bila tidak ada satu pun token dalam radius (belum ada
		// user yang menyinkronkan posisi, atau store tidak mendukung pencarian).
		if err := d.fcm.Send(ctx, &FCMMessage{
			Topic:    GeoTopic,
			Data:     data,
			Priority: "HIGH",
		}); err != nil {
			d.log.Error("gagal kirim FCM topic", "err", err, "event_id", msg.EventID, "type", msg.Type)
		}
	}()
}

// nearbyTokens mengembalikan token dalam dispatchRadiusKm dari centroid, atau
// nil bila store tidak mendukung pencarian itu / query gagal. Kegagalan di sini
// bukan kegagalan dispatch: pemanggil tetap melanjutkan ke topic broadcast.
func (d *Dispatcher) nearbyTokens(ctx context.Context, msg *AlertMessage) []string {
	finder, ok := d.saver.(tokenFinder)
	if !ok {
		return nil
	}
	tokens, err := finder.FCMTokensWithin(ctx, msg.CentroidLat, msg.CentroidLon, dispatchRadiusKm)
	if err != nil {
		d.log.Error("gagal cari token FCM terdekat", "err", err, "event_id", msg.EventID)
		return nil
	}
	return tokens
}

// sendToTokens mengirim satu message per token dengan paralelisme terbatas.
// Kegagalan per token hanya dicatat: satu token mati (UNREGISTERED) tidak boleh
// menghentikan pengiriman ke perangkat lain pada event yang sama.
func (d *Dispatcher) sendToTokens(ctx context.Context, tokens []string, data map[string]string, msg *AlertMessage) {
	sem := make(chan struct{}, maxFCMConcurrency)
	var wg sync.WaitGroup
	var failed atomic.Int64

	for _, token := range tokens {
		wg.Add(1)
		sem <- struct{}{}
		go func(token string) {
			defer wg.Done()
			defer func() { <-sem }()
			err := d.fcm.Send(ctx, &FCMMessage{
				Token:    token,
				Data:     data,
				Priority: "HIGH",
			})
			if err != nil {
				failed.Add(1)
				// Token tidak ikut dicatat: itu identifier perangkat.
				d.log.Warn("gagal kirim FCM ke token", "err", err, "event_id", msg.EventID)
			}
		}(token)
	}
	wg.Wait()

	d.log.Info("FCM bertarget terkirim",
		"event_id", msg.EventID, "type", msg.Type,
		"tokens", len(tokens), "gagal", failed.Load(), "radius_km", dispatchRadiusKm)
}

// trackResolution memulai state machine resolusi untuk event CONFIRMED yang baru
// dipersistensi: setelah resolveAfter (90s) -> update DB RESOLVED + broadcast
// EVENT_RESOLVED dengan event_id yang SAMA (all-clear). Dedup via map active agar
// timer ganda tidak pernah dibuat.
func (d *Dispatcher) trackResolution(eventID string, orig *AlertMessage) {
	d.mu.Lock()
	if _, ok := d.active[eventID]; ok {
		d.mu.Unlock()
		return
	}
	d.active[eventID] = orig
	d.mu.Unlock()

	go func() {
		timer := time.NewTimer(d.resolveAfter)
		defer timer.Stop()
		<-timer.C
		d.resolve(eventID, orig)
	}()
}

// resolve menandai event selesai (all-clear): update status di DB lalu broadcast
// EVENT_RESOLVED (WS + FCM) memakai event_id yang sama dengan alert awal.
func (d *Dispatcher) resolve(eventID string, orig *AlertMessage) {
	ctx, cancel := context.WithTimeout(context.Background(), fcmTimeout)
	defer cancel()

	if d.saver != nil {
		if err := d.saver.ResolveEvent(ctx, eventID); err != nil {
			d.log.Warn("gagal menandai event RESOLVED di DB", "event_id", eventID, "err", err)
		}
	}

	msg := &AlertMessage{
		Type:           TypeResolved,
		EventID:        eventID,
		MMI:            orig.MMI,
		IntensityLabel: orig.IntensityLabel,
		PGAGal:         orig.PGAGal,
		CentroidLat:    orig.CentroidLat,
		CentroidLon:    orig.CentroidLon,
		LocationName:   orig.LocationName,
		Timestamp:      time.Now().UnixMilli(),
		NodeCount:      orig.NodeCount,
	}

	// Broadcast tetap berjalan walau update DB gagal (idempoten; klien hanya
	// butuh event_id untuk mematikan status siaga).
	d.hub.Broadcast(msg)
	d.dispatchFCM(msg)

	d.mu.Lock()
	delete(d.active, eventID)
	d.mu.Unlock()

	d.log.Info("event resolved (all-clear) didispatch", "event_id", eventID)
}
