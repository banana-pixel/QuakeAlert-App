package dispatch

import (
	"context"
	"log/slog"

	"github.com/banana-pixel/quakealert/server/internal/consensus"
	"github.com/banana-pixel/quakealert/server/internal/store"
)

// Tipe payload FCM/WS sesuai kontrak (contracts/fcm/alert_payload.json).
const (
	TypeAlert    = "EARTHQUAKE_ALERT"    // >= 3 node CONFIRMED
	TypeAdvisory = "EARTHQUAKE_ADVISORY" // 1-2 node
)

// GeoTopic default untuk broadcast FCM (topik geo global; segmentasi lebih
// halus dapat ditambahkan kemudian).
const GeoTopic = "geo_alert_all"

// eventSaver mengabstraksi persistensi event agar dispatcher dapat diuji.
type eventSaver interface {
	SaveEvent(ctx context.Context, e *store.EarthquakeEvent) (string, error)
}

// Dispatcher menyatukan output Consensus Engine ke tiga kanal: persistensi
// PostGIS (hanya CONFIRMED), WebSocket Hub, dan FCM. Dirancang non-blocking
// pada jalur life-safety: kegagalan satu kanal tidak menghentikan kanal lain.
type Dispatcher struct {
	saver eventSaver
	hub   *Hub
	fcm   FCMSender
	log   *slog.Logger
}

// NewDispatcher membuat dispatcher. fcm boleh nil (mis. saat kredensial FCM
// belum dikonfigurasi di pengembangan) — WS tetap berjalan.
func NewDispatcher(saver eventSaver, hub *Hub, fcm FCMSender, log *slog.Logger) *Dispatcher {
	return &Dispatcher{saver: saver, hub: hub, fcm: fcm, log: log}
}

// Dispatch adalah consensus.EventSink: dipanggil engine untuk setiap event.
// CONFIRMED -> persist + WS + FCM (priority HIGH). ADVISORY -> WS + FCM advisory
// (silent yellow banner), tanpa persistensi.
func (d *Dispatcher) Dispatch(ctx context.Context, ev *consensus.Event) {
	msg := &AlertMessage{
		MMI:            ev.MMIScale,
		IntensityLabel: ev.IntensityLabel,
		PGAGal:         ev.MaxPGA,
		CentroidLat:    ev.Centroid.Lat,
		CentroidLon:    ev.Centroid.Lon,
		Timestamp:      ev.CreatedAtMs,
		NodeCount:      ev.NodeCount,
	}

	switch ev.Status {
	case consensus.StatusConfirmed:
		msg.Type = TypeAlert
		// Persist event; event_id dari DB dipakai pada payload agar klien dapat
		// deduplikasi & korelasi.
		if d.saver != nil {
			id, err := d.saver.SaveEvent(ctx, &store.EarthquakeEvent{
				Status:         "HAPPENING",
				CentroidLat:    ev.Centroid.Lat,
				CentroidLon:    ev.Centroid.Lon,
				LocationName:   msg.LocationName,
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
			}
		}
	case consensus.StatusAdvisory:
		msg.Type = TypeAdvisory
	default:
		d.log.Warn("status event tak dikenal, diabaikan", "status", ev.Status)
		return
	}

	// 1. WebSocket broadcast (foreground clients).
	d.hub.Broadcast(msg)

	// 2. FCM (background delivery). Priority HIGH untuk life-safety.
	if d.fcm != nil {
		fmsg := &FCMMessage{
			Topic:    GeoTopic,
			Data:     BuildAlertData(msg),
			Priority: "HIGH",
		}
		if err := d.fcm.Send(ctx, fmsg); err != nil {
			d.log.Error("gagal kirim FCM", "err", err, "event_id", msg.EventID)
		}
	}

	d.log.Info("event didispatch",
		"status", ev.Status, "mmi", ev.MMIScale, "pga_gal", ev.MaxPGA,
		"nodes", ev.NodeCount, "event_id", msg.EventID)
}
