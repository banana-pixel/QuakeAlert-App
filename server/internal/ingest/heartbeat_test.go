package ingest

import (
	"errors"
	"io"
	"log/slog"
	"strconv"
	"testing"
	"time"
)

func hbTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// fixedNow adalah waktu server acuan pada seluruh test heartbeat.
var fixedNow = time.UnixMilli(1_723_891_234_000).UTC()

func newTestHeartbeatValidator() *HeartbeatValidator {
	v := NewHeartbeatValidator(hbTestLogger())
	v.now = func() time.Time { return fixedNow }
	return v
}

func TestParseHeartbeat_Valid(t *testing.T) {
	raw := []byte(`{"id":"NODE-163A149F","rssi":-61,"uptime_s":86400,"ts":1723891234000}`)
	h, err := ParseHeartbeat(raw)
	if err != nil {
		t.Fatalf("err = %v, mau nil", err)
	}
	if h.ID != "NODE-163A149F" || h.RSSI != -61 || h.UptimeS != 86400 || h.TS != 1723891234000 {
		t.Fatalf("hasil parse salah: %+v", h)
	}
}

func TestParseHeartbeat_Invalid(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want error
	}{
		{"json rusak", `{"id":`, ErrHeartbeatInvalidJSON},
		{"id tanpa prefix", `{"id":"X-163A149F","rssi":-61,"uptime_s":1,"ts":1723891234000}`, ErrHeartbeatInvalidID},
		{"id hex kecil", `{"id":"NODE-163a149f","rssi":-61,"uptime_s":1,"ts":1723891234000}`, ErrHeartbeatInvalidID},
		{"id terlalu pendek", `{"id":"NODE-163A149","rssi":-61,"uptime_s":1,"ts":1723891234000}`, ErrHeartbeatInvalidID},
		{"id absen", `{"rssi":-61,"uptime_s":1,"ts":1723891234000}`, ErrHeartbeatInvalidID},
		{"rssi < -120", `{"id":"NODE-163A149F","rssi":-121,"uptime_s":1,"ts":1723891234000}`, ErrHeartbeatInvalidRSSI},
		{"rssi > 0", `{"id":"NODE-163A149F","rssi":1,"uptime_s":1,"ts":1723891234000}`, ErrHeartbeatInvalidRSSI},
		{"uptime negatif", `{"id":"NODE-163A149F","rssi":-61,"uptime_s":-1,"ts":1723891234000}`, ErrHeartbeatInvalidUptime},
		{"ts di bawah batas kontrak", `{"id":"NODE-163A149F","rssi":-61,"uptime_s":1,"ts":1699999999999}`, ErrHeartbeatInvalidTS},
		{"ts detik bukan ms", `{"id":"NODE-163A149F","rssi":-61,"uptime_s":1,"ts":1723891234}`, ErrHeartbeatInvalidTS},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseHeartbeat([]byte(tc.raw)); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, mau %v", err, tc.want)
			}
		})
	}
}

// TestParseHeartbeat_RSSIBoundaries memastikan batas kontrak inklusif.
func TestParseHeartbeat_RSSIBoundaries(t *testing.T) {
	for _, rssi := range []string{"-120", "0"} {
		raw := []byte(`{"id":"NODE-163A149F","rssi":` + rssi + `,"uptime_s":0,"ts":1723891234000}`)
		if _, err := ParseHeartbeat(raw); err != nil {
			t.Fatalf("rssi=%s harus diterima, err = %v", rssi, err)
		}
	}
}

func TestHeartbeatValidator_LatencyDanClockSkew(t *testing.T) {
	nowMs := fixedNow.UnixMilli()
	skewLimitMs := int64(MaxClockSkew / time.Millisecond) // 30_000

	cases := []struct {
		name        string
		ts          int64
		wantErr     error
		wantLatency int
	}{
		{"ts = waktu server", nowMs, nil, 0},
		{"telat 250ms", nowMs - 250, nil, 250},
		{"telat tepat 30s (batas diterima)", nowMs - skewLimitMs, nil, 30_000},
		{"telat 30s + 1ms", nowMs - skewLimitMs - 1, ErrHeartbeatClockSkew, 0},
		{"ts di masa depan 5s → latency di-clamp 0", nowMs + 5_000, nil, 0},
		{"ts di masa depan tepat 30s", nowMs + skewLimitMs, nil, 0},
		{"ts di masa depan 30s + 1ms", nowMs + skewLimitMs + 1, ErrHeartbeatClockSkew, 0},
	}

	v := newTestHeartbeatValidator()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := []byte(`{"id":"NODE-163A149F","rssi":-61,"uptime_s":10,"ts":` +
				strconv.FormatInt(tc.ts, 10) + `}`)
			h, latency, err := v.Validate(raw)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, mau %v", err, tc.wantErr)
			}
			if tc.wantErr != nil {
				if h != nil {
					t.Fatalf("heartbeat harus nil saat ditolak, dapat %+v", h)
				}
				return
			}
			if latency != tc.wantLatency {
				t.Fatalf("latency = %d ms, mau %d ms", latency, tc.wantLatency)
			}
			if h.ID != "NODE-163A149F" {
				t.Fatalf("station_id = %q", h.ID)
			}
		})
	}
}

// TestHeartbeatValidator_PayloadInvalid memastikan error struktur diteruskan
// apa adanya (validator tidak menelan penyebab penolakan).
func TestHeartbeatValidator_PayloadInvalid(t *testing.T) {
	v := newTestHeartbeatValidator()
	_, _, err := v.Validate([]byte(`{"id":"NODE-163A149F","rssi":-200,"uptime_s":1,"ts":1723891234000}`))
	if !errors.Is(err, ErrHeartbeatInvalidRSSI) {
		t.Fatalf("err = %v, mau %v", err, ErrHeartbeatInvalidRSSI)
	}
}

func TestStationIDFromTopic(t *testing.T) {
	cases := []struct {
		topic  string
		wantID string
		wantOK bool
	}{
		{"sensor/NODE-163A149F/heartbeat", "NODE-163A149F", true},
		{"sensor/NODE-163A149F/trigger", "NODE-163A149F", true},
		{"sensor//heartbeat", "", false},
		{"sensor/NODE-163A149F", "", false},
		{"other/NODE-163A149F/heartbeat", "", false},
		{"sensor/NODE-163A149F/heartbeat/extra", "", false},
		{"", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.topic, func(t *testing.T) {
			id, ok := StationIDFromTopic(tc.topic)
			if ok != tc.wantOK || id != tc.wantID {
				t.Fatalf("(%q, %v), mau (%q, %v)", id, ok, tc.wantID, tc.wantOK)
			}
		})
	}
}
