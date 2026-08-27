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
	if h.ID != "NODE-163A149F" || h.RSSI != -61 || h.UptimeS != 86400 || h.TS == nil || *h.TS != 1723891234000 {
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
			if latency == nil || *latency != tc.wantLatency {
				t.Fatalf("latency = %v ms, mau %d ms", latency, tc.wantLatency)
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

// ---------------------------------------------------------------------------
// O4 — node tanpa jam tersinkronisasi harus TERLIHAT, bukan hilang
// ---------------------------------------------------------------------------

// TestHeartbeat_ClockSourceNone adalah kriteria keluar O4: node yang jamnya belum
// tersinkronisasi menghilangkan ts dan tetap diterima. Tanpa ini ia diam total
// dan tidak dapat dibedakan dari node yang mati.
func TestHeartbeat_ClockSourceNone(t *testing.T) {
	v := newTestHeartbeatValidator()
	raw := []byte(`{"id":"NODE-163A149F","rssi":-61,"uptime_s":95,"clock_source":"NONE"}`)

	h, latency, err := v.Validate(raw)
	if err != nil {
		t.Fatalf("heartbeat tanpa jam ditolak: %v", err)
	}
	if h.TS != nil {
		t.Errorf("ts = %v, want nil", *h.TS)
	}
	if h.HasClock() {
		t.Error("HasClock = true untuk clock_source NONE")
	}
	// nil, bukan 0: 0 akan terbaca sebagai latency sempurna.
	if latency != nil {
		t.Errorf("latency = %d, want nil (tidak terukur)", *latency)
	}
}

func TestHeartbeat_ClockSourceValidation(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want error
	}{
		{
			// Absen = node firmware lama; ts tetap wajib dan itulah yang membuat
			// server berhak menganggapnya tersinkronisasi.
			name: "clock_source absen, ts ada",
			raw:  `{"id":"NODE-163A149F","rssi":-61,"uptime_s":10,"ts":1723891234000}`,
		},
		{
			name: "NTP dengan ts dan offset",
			raw:  `{"id":"NODE-163A149F","rssi":-61,"uptime_s":10,"ts":1723891234000,"clock_source":"NTP","clock_offset_ms":42}`,
		},
		{
			// RTC belum ada di perangkat keras saat ini; nilainya sudah dikenal
			// agar penambahannya bukan perubahan kontrak.
			name: "RTC diterima",
			raw:  `{"id":"NODE-163A149F","rssi":-61,"uptime_s":10,"ts":1723891234000,"clock_source":"RTC"}`,
		},
		{
			name: "NTP tanpa ts",
			raw:  `{"id":"NODE-163A149F","rssi":-61,"uptime_s":10,"clock_source":"NTP"}`,
			want: ErrHeartbeatInvalidTS,
		},
		{
			// Kontrak menyuruh node tanpa jam MENGHILANGKAN ts, bukan mengirim 0.
			name: "ts 0 tetap ditolak",
			raw:  `{"id":"NODE-163A149F","rssi":-61,"uptime_s":10,"ts":0}`,
			want: ErrHeartbeatInvalidTS,
		},
		{
			name: "NONE tetapi membawa ts",
			raw:  `{"id":"NODE-163A149F","rssi":-61,"uptime_s":10,"ts":1723891234000,"clock_source":"NONE"}`,
			want: ErrHeartbeatUnexpectedTS,
		},
		{
			name: "clock_source tak dikenal",
			raw:  `{"id":"NODE-163A149F","rssi":-61,"uptime_s":10,"ts":1723891234000,"clock_source":"GPS"}`,
			want: ErrHeartbeatInvalidClockSource,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseHeartbeat([]byte(tc.raw))
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

// TestHeartbeat_ClocklessSkipsSkewGate: heartbeat tanpa jam TIDAK boleh melewati
// gerbang skew — tidak ada ts untuk dibandingkan, dan menerapkan gerbang itu
// dengan nol akan menolak setiap node tanpa jam sebagai penyimpangan 55 tahun.
func TestHeartbeat_ClocklessSkipsSkewGate(t *testing.T) {
	v := newTestHeartbeatValidator()
	// Jam server dipindah jauh; hasilnya harus tidak berubah.
	v.now = func() time.Time { return fixedNow.Add(72 * time.Hour) }

	if _, _, err := v.Validate([]byte(
		`{"id":"NODE-163A149F","rssi":-61,"uptime_s":95,"clock_source":"NONE"}`)); err != nil {
		t.Fatalf("heartbeat tanpa jam ditolak karena jam server: %v", err)
	}
}
