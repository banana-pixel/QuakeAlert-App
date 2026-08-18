package ingest

import (
	"context"
	"log/slog"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// TriggerTopic adalah wildcard topik trigger sesuai kontrak: sensor/<station_id>/trigger.
const TriggerTopic = "sensor/+/trigger"

// TriggerHandler dipanggil untuk setiap trigger yang LOLOS verifikasi.
type TriggerHandler func(ctx context.Context, t *Trigger)

// Subscriber menghubungkan MQTT ke pipeline verifikasi.
type Subscriber struct {
	client    mqtt.Client
	verifier  *Verifier
	handler   TriggerHandler
	log       *slog.Logger
	ioTimeout time.Duration
}

// NewSubscriber membuat subscriber. Client dibuat di caller (main) agar opsi
// TLS/kredensial terpusat.
func NewSubscriber(client mqtt.Client, v *Verifier, h TriggerHandler, log *slog.Logger, ioTimeout time.Duration) *Subscriber {
	return &Subscriber{client: client, verifier: v, handler: h, log: log, ioTimeout: ioTimeout}
}

// Start berlangganan topik trigger dengan QoS 1 (life-safety, at-least-once).
func (s *Subscriber) Start() error {
	token := s.client.Subscribe(TriggerTopic, 1, s.onMessage)
	token.Wait()
	if err := token.Error(); err != nil {
		return err
	}
	s.log.Info("subscribed", "topic", TriggerTopic, "qos", 1)
	return nil
}

// onMessage adalah callback hot-path. Minimalkan alokasi; verifikasi lalu delegasikan.
func (s *Subscriber) onMessage(_ mqtt.Client, msg mqtt.Message) {
	// Context per-pesan dengan timeout IO (Aturan Server #3: <= 2s).
	ctx, cancel := context.WithTimeout(context.Background(), s.ioTimeout)
	defer cancel()

	t, err := s.verifier.Verify(ctx, msg.Payload())
	if err != nil {
		// Penolakan sudah di-log di verifier pada level yang tepat; di sini debug.
		s.log.Debug("trigger ditolak", "topic", msg.Topic(), "err", err)
		return
	}

	s.log.Info("trigger diterima", "node_id", t.NodeID, "pga", t.PGA, "ts", t.TS)
	s.handler(ctx, t)
}
