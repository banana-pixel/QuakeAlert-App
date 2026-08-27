package dispatch

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/banana-pixel/quakealert/server/internal/consensus"
	"github.com/banana-pixel/quakealert/server/internal/ledger"
	"github.com/banana-pixel/quakealert/server/internal/store"
)

// Tipe payload FCM/WS sesuai kontrak (contracts/fcm/alert_payload.json).
const (
	TypeAlert    = "EARTHQUAKE_ALERT"    // >= 3 node CONFIRMED
	TypeAdvisory = "EARTHQUAKE_ADVISORY" // 1-2 node
	TypeResolved = "EVENT_RESOLVED"      // all-clear (state machine SYSTEM_SPEC)
	// TypeBroadcast adalah pengumuman operator: bukan hasil konsensus, tidak
	// pernah berbunyi seperti sirene, dan dirender klien pada kanal notifikasi
	// ber-importance rendah miliknya sendiri.
	TypeBroadcast = "ADMIN_BROADCAST"
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

// tokenFinder mengabstraksi pencarian token FCM bertarget. rangeKm adalah radius
// peringatan itu sendiri (AlertRadiusKm) — store tidak lagi menyempitkannya per
// user. Interface terpisah dari eventSaver dan dideteksi lewat type assertion
// agar penyedia lama (dan fake pada test) tetap kompatibel: yang tidak
// mengimplementasikannya jatuh ke broadcast topic.
type tokenFinder interface {
	FCMTokensWithin(ctx context.Context, lat, lon float64, rangeKm int) ([]string, error)
}

// regionTokenFinder mengabstraksi pencarian token FCM per wilayah administratif
// (siaran admin). Terpisah dari tokenFinder dan dideteksi lewat type assertion
// dengan alasan yang sama: penyedia yang tidak mengimplementasikannya jatuh ke
// broadcast topic alih-alih gagal.
type regionTokenFinder interface {
	FCMTokensInRegion(ctx context.Context, regionCode string) ([]string, error)
}

// AlertRadiusKm adalah radius peringatan TETAP: 200 km dari centroid.
//
// Tetap, dan bukan pilihan pengguna, karena praktik EEW menempatkan keputusan
// ini pada sistem: seseorang yang memilih 50 km untuk mengurangi notifikasi
// telah membuat keputusan keselamatan yang tidak ia sadari sedang dibuat, dan
// satu-satunya yang tahu ia salah adalah orang itu sendiri — setelah gempanya.
// 200 km mencakup jarak kerusakan gempa merusak di wilayah Indonesia sekaligus
// tetap sempit untuk tidak melatih pengguna mengabaikan sirene.
//
// Nilai yang SAMA dipakai gate Haversine di klien (domain/SafetyPolicy.kt) agar
// server dan perangkat tidak pernah berbeda pendapat tentang siapa yang
// dibangunkan.
const AlertRadiusKm = 200

// maxFCMConcurrency membatasi request FCM paralel per event. HTTP v1 tidak
// punya endpoint batch, jadi satu event berarti satu request per token; batas
// ini menjaga fan-out tidak menghabiskan koneksi keluar.
const maxFCMConcurrency = 16

// emissionWriter menerima satu baris keputusan dispatch. Implementasi:
// *ledger.Writer. Interface, dan bukan tipe konkret, dengan alasan yang sama
// seperti eventSaver: dispatcher tidak boleh punya cara untuk menulis ke basis
// data secara sinkron.
type emissionWriter interface {
	RecordEmission(e *ledger.Emission)
}

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

	// ledger mencatat KEPUTUSAN dispatch (bukan hasil pengiriman). Nil =
	// dinonaktifkan. Penulisannya asinkron dan berbatas; tidak ada jalur di file
	// ini yang menunggunya.
	ledger emissionWriter

	// singleNodeGeoTopicGuard mencegah kluster satu-node memilih topik nasional.
	// Default aktif; lihat guardBlocksGeoTopic.
	singleNodeGeoTopicGuard bool

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
		saver:                   saver,
		hub:                     hub,
		fcm:                     fcm,
		log:                     log,
		resolveAfter:            resolveAfter,
		active:                  make(map[string]*AlertMessage),
		singleNodeGeoTopicGuard: true,
	}
}

// SetLedger memasang penulis alert_emissions (pola Set* yang sama dengan
// apiSrv.Set*). Nil menonaktifkan pencatatan.
func (d *Dispatcher) SetLedger(w emissionWriter) { d.ledger = w }

// SetSingleNodeGeoTopicGuard mengaktifkan/menonaktifkan guard satu-node.
// Default AKTIF; dapat dimatikan lewat SINGLE_NODE_GEO_TOPIC_GUARD=false untuk
// pengujian lapangan pada instalasi yang benar-benar hanya punya satu node.
func (d *Dispatcher) SetSingleNodeGeoTopicGuard(enabled bool) {
	d.singleNodeGeoTopicGuard = enabled
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
// Tiga jalur yang saling eksklusif, dipilih dalam urutan ini:
//  1. Event parah (IsSevere) -> GeoTopic, TANPA filter jarak. Ini satu-satunya
//     kejadian di mana bangun-nasional adalah jawaban yang benar.
//  2. Token dalam AlertRadiusKm dari centroid.
//  3. GeoTopic sebagai fallback bila tidak ada satu pun token yang cocok.
//
// Eksklusif karena topic tidak bisa dikecualikan per pelanggan — mengirim topic
// bersamaan dengan token bertarget akan membangunkan seluruh perangkat nasional
// pada setiap gempa kecil, persis yang penargetan ini hilangkan.
func (d *Dispatcher) dispatchFCM(msg *AlertMessage) {
	// decidedAt diambil SINKRON: ini waktu keputusan, bukan waktu pengiriman.
	// Membacanya di dalam goroutine akan mencatat kapan FCM kebetulan
	// terjadwal, yang bukan hal yang sedang diukur.
	decidedAt := time.Now().UnixMilli()

	if d.fcm == nil {
		d.recordEmission(msg, ledger.AudienceNone, decidedAt)
		return
	}
	data := BuildAlertData(msg)
	severe := IsSevere(msg.MMI, msg.PGAGal)
	guarded := d.guardBlocksGeoTopic(msg)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), fcmTimeout)
		defer cancel()

		if severe && !guarded {
			// Override intensitas: jarak tidak diperiksa sama sekali. Klien pun
			// tidak akan menahannya — SafetyPolicy menerapkan aturan yang sama
			// sebelum sirene, jadi keduanya sepakat tanpa perlu flag di payload.
			d.log.Warn("event parah: FCM disiarkan tanpa filter jarak",
				"event_id", msg.EventID, "mmi", msg.MMI, "pga_gal", msg.PGAGal)
			if err := d.fcm.Send(ctx, &FCMMessage{
				Topic:    GeoTopic,
				Data:     data,
				Priority: "HIGH",
			}); err != nil {
				d.log.Error("gagal kirim FCM topic (severe)", "err", err, "event_id", msg.EventID)
			}
			d.recordEmission(msg, ledger.AudienceGeoTopicAll, decidedAt)
			return
		}

		if tokens := d.nearbyTokens(ctx, msg); len(tokens) > 0 {
			d.sendToTokens(ctx, tokens, data, msg)
			d.recordEmission(msg, ledger.AudienceTokensRadius, decidedAt)
			return
		}

		// Guard satu-node: fallback topik nasional TIDAK diambil. Satu sensor
		// tidak dapat dibedakan dari satu sensor yang rusak, dan tidak ada
		// perangkat dalam radius yang perlu dibangunkan, jadi jawaban yang benar
		// adalah tidak mengirim apa pun — bukan membangunkan seluruh negeri.
		if guarded {
			d.log.Warn("guard satu-node: FCM tidak dikirim (tanpa token dalam radius, topik nasional ditahan)",
				"event_id", msg.EventID, "type", msg.Type, "nodes", msg.NodeCount,
				"mmi", msg.MMI, "pga_gal", msg.PGAGal, "severe", severe)
			d.recordEmission(msg, ledger.AudienceNone, decidedAt)
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
		d.recordEmission(msg, ledger.AudienceGeoTopicAll, decidedAt)
	}()
}

// guardBlocksGeoTopic melaporkan apakah pesan ini dilarang memilih GeoTopic.
//
// Sebuah kluster dengan SATU node penyumbang tidak boleh membangunkan seluruh
// negeri, pada jalur severe maupun sebagai fallback tanpa token. Satu sensor
// yang berteriak keras tidak dapat dibedakan dari satu sensor yang rusak,
// terjatuh, atau tertabrak — dan justru bacaan besar itulah yang paling mungkin
// merupakan kegagalan perangkat, bukan gempa. Bila jalur bertarget tidak
// menghasilkan satu token pun, hasil yang benar adalah TANPA FCM
// (audience = NONE), bukan siaran nasional.
//
// Guard dipasang di sini, di dalam dispatchFCM, dan bukan pada masing-masing
// pemanggilan Send: dengan begitu ia mencakup KEDUA titik GeoTopic sekaligus
// kedua pemanggil (Dispatch dan resolve), dan tidak ada cabang baru yang dapat
// melewatinya. DispatchBroadcast dan DispatchTestAlert tidak melewati fungsi ini
// dan memang tidak seharusnya: keduanya tindakan operator ke UpdatesTopic, bukan
// hasil konsensus.
func (d *Dispatcher) guardBlocksGeoTopic(msg *AlertMessage) bool {
	return d.singleNodeGeoTopicGuard && msg.NodeCount <= 1
}

// recordEmission mengantre satu baris alert_emissions. Dipanggil SETELAH
// keputusan audience benar-benar dieksekusi, sehingga nilai yang tercatat adalah
// yang benar-benar dipublikasikan — bukan yang direncanakan.
//
// Hanya keputusan yang dicatat. Hasil pengiriman (jumlah klien WS, sukses/gagal
// per token FCM) BUKAN bagian dari fase ini: menunggunya berarti menahan
// pencatatan di belakang Google API, dan menulis-balik nanti berarti satu
// goroutine lagi per event untuk data yang belum ada pemakainya.
func (d *Dispatcher) recordEmission(msg *AlertMessage, audience string, decidedAt int64) {
	if d.ledger == nil {
		return
	}

	e := &ledger.Emission{
		AlertType:   msg.Type,
		Status:      emissionStatus(msg.Type),
		NodeCount:   msg.NodeCount,
		CentroidLat: &msg.CentroidLat,
		CentroidLon: &msg.CentroidLon,
		IsSevere:    IsSevere(msg.MMI, msg.PGAGal),
		Audience:    audience,
		DecidedAt:   decidedAt,
		AlgoVer:     ledger.AlgoVer,
	}
	// event_id NULL bukan data yang hilang: ADVISORY hari ini tidak
	// dipersistensi sama sekali, jadi ia tidak punya identitas event untuk
	// dirujuk.
	if msg.EventID != "" {
		id := msg.EventID
		e.EventID = &id
	}
	if msg.MMI != "" {
		mmi := msg.MMI
		e.MMI = &mmi
	}
	pga := msg.PGAGal
	e.PGAGal = &pga

	d.ledger.RecordEmission(e)
}

// emissionStatus memetakan tipe payload ke status sebagaimana diemisikan.
// EVENT_RESOLVED hanya pernah menyusul event CONFIRMED: hanya CONFIRMED yang
// dipersistensi, dan hanya event yang dipersistensi yang punya timer resolusi.
func emissionStatus(alertType string) string {
	if alertType == TypeAdvisory {
		return string(consensus.StatusAdvisory)
	}
	return string(consensus.StatusConfirmed)
}

// nearbyTokens mengembalikan token dalam AlertRadiusKm dari centroid, atau
// nil bila store tidak mendukung pencarian itu / query gagal. Kegagalan di sini
// bukan kegagalan dispatch: pemanggil tetap melanjutkan ke topic broadcast.
func (d *Dispatcher) nearbyTokens(ctx context.Context, msg *AlertMessage) []string {
	finder, ok := d.saver.(tokenFinder)
	if !ok {
		return nil
	}
	tokens, err := finder.FCMTokensWithin(ctx, msg.CentroidLat, msg.CentroidLon, AlertRadiusKm)
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
		"tokens", len(tokens), "gagal", failed.Load(), "radius_km", AlertRadiusKm)
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
