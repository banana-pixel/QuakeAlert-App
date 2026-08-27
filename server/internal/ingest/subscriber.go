package ingest

import (
	"context"
	"log/slog"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// Wildcard topik sesuai kontrak: sensor/<station_id>/{trigger,heartbeat}.
const (
	TriggerTopic   = "sensor/+/trigger"
	HeartbeatTopic = "sensor/+/heartbeat"
)

// TriggerHandler dipanggil untuk setiap trigger yang LOLOS verifikasi.
type TriggerHandler func(ctx context.Context, t *Trigger)

// TriggerVerifier memverifikasi trigger yang struktur payloadnya sudah lolos
// ParseTrigger: node aktif, clock skew, HMAC, anti-replay. Diabstraksi agar
// Subscriber dapat diuji tanpa Postgres (implementasi produksi: *Verifier).
type TriggerVerifier interface {
	VerifyTrigger(ctx context.Context, t *Trigger) error
}

// HeartbeatHandler dipanggil untuk setiap heartbeat yang lolos validasi.
// latencyMs adalah latency satu-arah node→server hasil hitung validator, atau
// nil bila node tidak mengirim ts (clock_source NONE) sehingga tidak ada apa pun
// untuk diukur. nil berarti TIDAK TERUKUR, bukan nol.
type HeartbeatHandler func(ctx context.Context, h *Heartbeat, latencyMs *int)

// Subscriber menghubungkan MQTT ke pipeline verifikasi.
type Subscriber struct {
	client    mqtt.Client
	verifier  TriggerVerifier
	handler   TriggerHandler
	log       *slog.Logger
	ioTimeout time.Duration

	// Heartbeat opsional; aktif hanya bila WithHeartbeat dipanggil.
	hbValidator *HeartbeatValidator
	hbHandler   HeartbeatHandler
}

// NewSubscriber membuat subscriber. Client dibuat di caller (main) agar opsi
// TLS/kredensial terpusat.
func NewSubscriber(client mqtt.Client, v TriggerVerifier, h TriggerHandler, log *slog.Logger, ioTimeout time.Duration) *Subscriber {
	return &Subscriber{client: client, verifier: v, handler: h, log: log, ioTimeout: ioTimeout}
}

// WithHeartbeat mengaktifkan langganan heartbeat. Mengembalikan receiver agar
// bisa dirangkai dari main.
func (s *Subscriber) WithHeartbeat(v *HeartbeatValidator, h HeartbeatHandler) *Subscriber {
	s.hbValidator, s.hbHandler = v, h
	return s
}

// Start berlangganan topik trigger (dan heartbeat bila dikonfigurasi) dengan
// QoS 1 (life-safety, at-least-once).
func (s *Subscriber) Start() error {
	if err := s.subscribe(TriggerTopic, s.onMessage); err != nil {
		return err
	}
	if s.hbValidator == nil || s.hbHandler == nil {
		s.log.Warn("heartbeat tidak dikonfigurasi — status liveness node tidak diperbarui")
		return nil
	}
	return s.subscribe(HeartbeatTopic, s.onHeartbeat)
}

func (s *Subscriber) subscribe(topic string, cb mqtt.MessageHandler) error {
	token := s.client.Subscribe(topic, 1, cb)
	token.Wait()
	if err := token.Error(); err != nil {
		return err
	}
	s.log.Info("subscribed", "topic", topic, "qos", 1)
	return nil
}

// onMessage adalah callback hot-path. Minimalkan alokasi; verifikasi lalu delegasikan.
func (s *Subscriber) onMessage(_ mqtt.Client, msg mqtt.Message) {
	// Context per-pesan dengan timeout IO (Aturan Server #3: <= 2s).
	ctx, cancel := context.WithTimeout(context.Background(), s.ioTimeout)
	defer cancel()

	// Validasi struktur lebih dulu agar node_id tersedia untuk cross-check topik
	// (payload hanya di-parse sekali; verifikasi lanjut memakai hasilnya).
	t, err := ParseTrigger(msg.Payload())
	if err != nil {
		s.log.Debug("trigger ditolak: payload invalid", "topic", msg.Topic(), "err", err)
		return
	}

	// Cross-check topik vs payload: segmen <station_id> pada topik WAJIB sama
	// dengan node_id di payload. Tanpa ini, node yang punya kredensial broker sah
	// dapat mem-publish trigger ber-node_id milik node LAIN ke topiknya sendiri —
	// dan bila ia juga memegang secret node itu, HMAC pun lolos. Dijalankan
	// SEBELUM verifikasi HMAC: cek string murah, kripto & IO DB terakhir.
	topicID, ok := StationIDFromTopic(msg.Topic())
	if !ok || topicID != t.NodeID {
		s.log.Warn("trigger ditolak: station_id topik != node_id payload",
			"topic", msg.Topic(), "payload_node_id", t.NodeID)
		return
	}

	if err := s.verifier.VerifyTrigger(ctx, t); err != nil {
		// Penolakan sudah di-log di verifier pada level yang tepat; di sini debug.
		s.log.Debug("trigger ditolak", "topic", msg.Topic(), "err", err)
		return
	}

	s.log.Info("trigger diterima", "node_id", t.NodeID, "pga", t.PGA, "ts", t.TS)
	s.handler(ctx, t)
}

// onHeartbeat memvalidasi heartbeat lalu mendelegasikan pemutakhiran status.
// Payload heartbeat tidak ber-signature, jadi minimal station_id pada payload
// wajib cocok dengan segmen topik agar satu node tidak bisa memutakhirkan
// baris node lain.
func (s *Subscriber) onHeartbeat(_ mqtt.Client, msg mqtt.Message) {
	ctx, cancel := context.WithTimeout(context.Background(), s.ioTimeout)
	defer cancel()

	h, latencyMs, err := s.hbValidator.Validate(msg.Payload())
	if err != nil {
		s.log.Debug("heartbeat ditolak", "topic", msg.Topic(), "err", err)
		return
	}
	if topicID, ok := StationIDFromTopic(msg.Topic()); !ok || topicID != h.ID {
		s.log.Warn("heartbeat ditolak: station_id topik != payload",
			"topic", msg.Topic(), "payload_id", h.ID)
		return
	}

	s.log.Debug("heartbeat diterima", "station_id", h.ID, "rssi", h.RSSI, "latency_ms", latencyMs)
	s.hbHandler(ctx, h, latencyMs)
}

// StationIDFromTopic mengambil <station_id> dari topik "sensor/<station_id>/<suffix>".
func StationIDFromTopic(topic string) (string, bool) {
	parts := strings.Split(topic, "/")
	if len(parts) != 3 || parts[0] != "sensor" || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}
