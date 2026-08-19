package ingest

import (
	"encoding/json"
	"errors"
	"log/slog"
	"regexp"
	"time"
)

// stationIDPattern mencerminkan contracts/mqtt/heartbeat.schema.json
// (^NODE-[0-9A-F]{8}$). Sama dengan node_id pada trigger; dipisah agar
// perubahan salah satu kontrak tidak diam-diam menular ke yang lain.
var stationIDPattern = regexp.MustCompile(`^NODE-[0-9A-F]{8}$`)

// Heartbeat adalah payload periodik (60s, QoS 1) sesuai
// contracts/mqtt/heartbeat.schema.json. Satuan kanonik: rssi=dBm,
// uptime_s=detik, ts=ms epoch UTC.
type Heartbeat struct {
	ID      string `json:"id"`
	RSSI    int    `json:"rssi"`
	UptimeS int64  `json:"uptime_s"`
	TS      int64  `json:"ts"`
}

// Batas kontrak heartbeat.
const (
	hbMinRSSI = -120
	hbMaxRSSI = 0
	// hbMinTS: 1.7e12 ms ~ 2023-11, batas bawah `ts` di skema.
	hbMinTS = 1700000000000
)

var (
	ErrHeartbeatInvalidJSON   = errors.New("payload heartbeat bukan JSON valid")
	ErrHeartbeatInvalidID     = errors.New("id tidak sesuai pola NODE-XXXXXXXX")
	ErrHeartbeatInvalidRSSI   = errors.New("rssi di luar rentang [-120,0] dBm")
	ErrHeartbeatInvalidUptime = errors.New("uptime_s negatif")
	ErrHeartbeatInvalidTS     = errors.New("ts bukan ms epoch UTC yang wajar")
	ErrHeartbeatClockSkew     = errors.New("ts heartbeat menyimpang > 30s dari waktu server")
)

// ParseHeartbeat meng-unmarshal dan memvalidasi struktur payload terhadap
// kontrak (belum termasuk clock skew, yang butuh waktu server).
func ParseHeartbeat(raw []byte) (*Heartbeat, error) {
	var h Heartbeat
	if err := json.Unmarshal(raw, &h); err != nil {
		return nil, ErrHeartbeatInvalidJSON
	}
	if !stationIDPattern.MatchString(h.ID) {
		return nil, ErrHeartbeatInvalidID
	}
	if h.RSSI < hbMinRSSI || h.RSSI > hbMaxRSSI {
		return nil, ErrHeartbeatInvalidRSSI
	}
	if h.UptimeS < 0 {
		return nil, ErrHeartbeatInvalidUptime
	}
	if h.TS < hbMinTS {
		return nil, ErrHeartbeatInvalidTS
	}
	return &h, nil
}

// HeartbeatValidator memvalidasi heartbeat (struktur + clock skew) dan
// menghitung latency satu-arah node→server.
//
// CATATAN KEAMANAN: kontrak heartbeat TIDAK memuat signature, jadi payload ini
// tidak terautentikasi — heartbeat hanya boleh memutakhirkan telemetri liveness
// (rssi/latency/last_heartbeat), JANGAN dipakai untuk keputusan life-safety.
// Anti-spoof pada level transport: MQTTS + auth broker (ADR-0003) dan
// pemeriksaan kecocokan station_id pada topik (lihat Subscriber).
type HeartbeatValidator struct {
	log *slog.Logger
	now func() time.Time // diinjeksi untuk test
}

// NewHeartbeatValidator membuat validator heartbeat.
func NewHeartbeatValidator(log *slog.Logger) *HeartbeatValidator {
	return &HeartbeatValidator{log: log, now: time.Now}
}

// Validate mem-parse payload, menolak ts yang menyimpang > MaxClockSkew, dan
// mengembalikan latency satu-arah dalam ms (now - ts, di-clamp >= 0 agar drift
// kecil ke depan tidak tersimpan sebagai nilai negatif).
func (v *HeartbeatValidator) Validate(raw []byte) (*Heartbeat, int, error) {
	h, err := ParseHeartbeat(raw)
	if err != nil {
		return nil, 0, err
	}

	nowMs := v.now().UnixMilli()
	skewMs := nowMs - h.TS
	maxSkewMs := int64(MaxClockSkew / time.Millisecond)
	if skewMs > maxSkewMs || skewMs < -maxSkewMs {
		v.log.Warn("heartbeat ditolak: clock skew", "station_id", h.ID, "ts", h.TS, "server_ms", nowMs)
		return nil, 0, ErrHeartbeatClockSkew
	}

	latencyMs := skewMs
	if latencyMs < 0 {
		latencyMs = 0
	}
	return h, int(latencyMs), nil
}
