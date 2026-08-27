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

// Heartbeat adalah payload periodik (60s) sesuai
// contracts/mqtt/heartbeat.schema.json. Satuan kanonik: rssi=dBm,
// uptime_s=detik, ts=ms epoch UTC.
//
// TS bertipe pointer karena ia BOLEH tidak ada: node yang jamnya belum
// tersinkronisasi tidak punya timestamp untuk dilaporkan, dan kontrak menyuruhnya
// menghilangkan field itu alih-alih mengirim 0 (O4). Nilai 0 akan ditolak sebagai
// ts tidak wajar dan node-nya kembali menjadi tidak terlihat — yaitu persis
// kegagalan yang O4 ada untuk menutup.
type Heartbeat struct {
	ID      string `json:"id"`
	RSSI    int    `json:"rssi"`
	UptimeS int64  `json:"uptime_s"`
	TS      *int64 `json:"ts,omitempty"`

	// ClockSource adalah kualitas jam MENURUT NODE ITU SENDIRI. Kosong = node
	// firmware lama; server menganggapnya NTP karena ia mengirim ts yang lolos
	// gerbang skew.
	ClockSource string `json:"clock_source,omitempty"`
	// ClockOffsetMs adalah koreksi terakhir yang node terapkan pada jamnya.
	// Diagnostik saja; bukan gerbang apa pun.
	ClockOffsetMs *int64 `json:"clock_offset_ms,omitempty"`
}

// Nilai clock_source yang dikenal kontrak. RTC sudah dicantumkan meski perangkat
// keras saat ini belum memilikinya, agar penambahannya bukan perubahan kontrak.
const (
	ClockSourceNTP  = "NTP"
	ClockSourceRTC  = "RTC"
	ClockSourceNone = "NONE"
)

// Batas kontrak heartbeat.
const (
	hbMinRSSI = -120
	hbMaxRSSI = 0
	// hbMinTS: 1.7e12 ms ~ 2023-11, batas bawah `ts` di skema.
	hbMinTS = 1700000000000
)

// HasClock melaporkan apakah node ini mengaku punya jam yang dapat dipercaya.
// Node dengan clock_source NONE TIDAK punya, dan karena itu tidak mengirim ts,
// tidak dapat diukur latency-nya, dan tidak dapat menandatangani trigger.
func (h *Heartbeat) HasClock() bool { return h.ClockSource != ClockSourceNone }

var (
	ErrHeartbeatInvalidJSON   = errors.New("payload heartbeat bukan JSON valid")
	ErrHeartbeatInvalidID     = errors.New("id tidak sesuai pola NODE-XXXXXXXX")
	ErrHeartbeatInvalidRSSI   = errors.New("rssi di luar rentang [-120,0] dBm")
	ErrHeartbeatInvalidUptime = errors.New("uptime_s negatif")
	ErrHeartbeatInvalidTS     = errors.New("ts bukan ms epoch UTC yang wajar")
	ErrHeartbeatClockSkew     = errors.New("ts heartbeat menyimpang > 30s dari waktu server")

	// ErrHeartbeatInvalidClockSource menolak nilai clock_source di luar enum.
	// Ditolak, bukan dinormalkan menjadi NONE: sebuah nilai yang tidak dikenal
	// berarti server dan firmware tidak sepakat tentang arti field ini, dan
	// menebak artinya adalah cara paling cepat menyembunyikan ketidaksepakatan itu.
	ErrHeartbeatInvalidClockSource = errors.New("clock_source bukan NTP, RTC, atau NONE")

	// ErrHeartbeatUnexpectedTS menolak ts yang hadir bersama clock_source NONE.
	// Node yang mengaku tidak punya jam namun tetap melaporkan waktu sedang
	// melaporkan angka yang ia sendiri nyatakan tidak bermakna.
	ErrHeartbeatUnexpectedTS = errors.New("ts hadir padahal clock_source NONE")
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
	switch h.ClockSource {
	case "", ClockSourceNTP, ClockSourceRTC:
		// ts WAJIB: node mengaku punya jam, jadi ia punya waktu untuk dilaporkan.
		if h.TS == nil || *h.TS < hbMinTS {
			return nil, ErrHeartbeatInvalidTS
		}
	case ClockSourceNone:
		// ts harus TIDAK ADA. Inilah satu-satunya jalan bagi node tanpa jam untuk
		// tetap terlihat: tanpa pengecualian ini ia diam total dan tidak dapat
		// dibedakan dari node yang mati (O4).
		if h.TS != nil {
			return nil, ErrHeartbeatUnexpectedTS
		}
	default:
		return nil, ErrHeartbeatInvalidClockSource
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
//
// latency nil berarti TIDAK TERUKUR, bukan nol: node dengan clock_source NONE
// tidak mengirim ts, jadi tidak ada apa pun untuk dikurangkan dari jam server.
// Heartbeatnya TETAP diterima — ia satu-satunya bukti bahwa node itu hidup, dan
// menolaknya karena jamnya rusak akan membuat kerusakan jam terlihat identik
// dengan kematian perangkat (O4).
func (v *HeartbeatValidator) Validate(raw []byte) (*Heartbeat, *int, error) {
	h, err := ParseHeartbeat(raw)
	if err != nil {
		return nil, nil, err
	}

	if !h.HasClock() {
		v.log.Warn("heartbeat tanpa jam tersinkronisasi",
			"station_id", h.ID, "clock_source", h.ClockSource, "uptime_s", h.UptimeS)
		return h, nil, nil
	}

	nowMs := v.now().UnixMilli()
	skewMs := nowMs - *h.TS
	maxSkewMs := int64(MaxClockSkew / time.Millisecond)
	if skewMs > maxSkewMs || skewMs < -maxSkewMs {
		v.log.Warn("heartbeat ditolak: clock skew", "station_id", h.ID, "ts", *h.TS, "server_ms", nowMs)
		return nil, nil, ErrHeartbeatClockSkew
	}

	latencyMs := int(skewMs)
	if latencyMs < 0 {
		latencyMs = 0
	}
	return h, &latencyMs, nil
}
