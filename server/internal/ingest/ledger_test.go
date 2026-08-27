package ingest

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/banana-pixel/quakealert/server/internal/consensus"
	"github.com/banana-pixel/quakealert/server/internal/ledger"
	"github.com/banana-pixel/quakealert/server/internal/store"
)

// fakeObsWriter menangkap baris ledger tanpa basis data.
type fakeObsWriter struct{ rows []*ledger.Observation }

func (f *fakeObsWriter) RecordObservation(o *ledger.Observation) { f.rows = append(f.rows, o) }

// ---------------------------------------------------------------------------
// §20.1 — pemetaan ledger
// ---------------------------------------------------------------------------

func TestTriggerObservation_Mapping(t *testing.T) {
	cases := []struct {
		name           string
		pga            float64
		durMs          int64
		ts             int64
		wantUpperBound int64
	}{
		{"nominal", 413.13, 8000, 1_700_000_005_000, 1_699_999_997_000},
		// dur_ms = 0: batas atas berimpit dengan waktu publish. Legal — event
		// dengan durasi nol tercatat, bukan ditolak.
		{"durasi nol", 16.6, 0, 1_700_000_005_000, 1_700_000_005_000},
		// Batas: publish_ts - dur_ms mendahului 1700000000000. Tidak ada lantai
		// pada nilai ini; ia memang boleh menunjuk ke masa sebelum publish.
		{"batas atas mendahului 1.7e12", 20.0, 6000, 1_700_000_001_000, 1_699_999_995_000},
		{"durasi lebih besar dari epoch parsial", 20.0, 60000, 1_700_000_010_000, 1_699_999_950_000},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tr := &Trigger{
				NodeID:    "NODE-0A1B2C3D",
				PGA:       tc.pga,
				DurMs:     tc.durMs,
				TS:        tc.ts,
				Signature: "a1b2c3",
			}
			o := TriggerObservation(tr, 1_700_000_005_123, ledger.VerifyResultOK)

			if o.SourceClass != "FIXED_ESP32" {
				t.Errorf("source_class = %q, want FIXED_ESP32", o.SourceClass)
			}
			if o.Phase != "FINAL" {
				t.Errorf("phase = %q, want FINAL", o.Phase)
			}
			if o.OnsetTSSource != "PUBLISH_BOUND" {
				t.Errorf("onset_ts_source = %q, want PUBLISH_BOUND", o.OnsetTSSource)
			}
			if o.OnsetTSUpperBound == nil || *o.OnsetTSUpperBound != tc.wantUpperBound {
				t.Errorf("onset_ts_upper_bound = %v, want %d", o.OnsetTSUpperBound, tc.wantUpperBound)
			}
			if o.PublishTS != tc.ts {
				t.Errorf("publish_ts = %d, want %d", o.PublishTS, tc.ts)
			}
			// dur_ms DICATAT. Bahwa ia tidak pernah menjadi masukan konsensus
			// dijamin oleh tanda tangan Engine.Ingest, yang diuji di bawah.
			if o.DurMs != tc.durMs {
				t.Errorf("dur_ms = %d, want %d", o.DurMs, tc.durMs)
			}

			// Kolom Fase 2 harus NULL pada v1, bukan diisi nilai tebakan.
			if o.ProtoVer != nil {
				t.Errorf("proto_ver = %v, want nil pada v1", *o.ProtoVer)
			}
			if o.ObsSeq != nil {
				t.Errorf("obs_seq = %v, want nil sampai Fase 2", *o.ObsSeq)
			}
			if o.OnsetTS != nil {
				t.Errorf("onset_ts = %v, want nil (onset sebenarnya tak diketahui di v1)", *o.OnsetTS)
			}
			// Lokasi di-snapshot oleh writer, bukan di pemetaan.
			if o.Lat != nil || o.Lon != nil {
				t.Error("pemetaan tidak boleh mengisi node_location (di-snapshot di luar hot path)")
			}
		})
	}
}

// TestObservationHasNoPhase2Columns menjaga agar kolom Fase 2 tidak menyelinap
// masuk lebih awal. attempt_no dan detrigger_ts TIDAK DAPAT diisi oleh payload
// v1 sama sekali (tidak ada nomor percobaan di kabel), jadi keberadaan fieldnya
// akan menjadi kolom yang selamanya NULL sambil terlihat seperti data.
func TestObservationHasNoPhase2Columns(t *testing.T) {
	forbidden := map[string]bool{
		"AttemptNo": true, "DetriggerTS": true, "IngestSeq": true, "CorrelationKey": true,
	}
	typ := reflect.TypeOf(ledger.Observation{})
	for i := 0; i < typ.NumField(); i++ {
		if forbidden[typ.Field(i).Name] {
			t.Errorf("Observation punya field Fase 2 %q — Fase 1 tidak boleh membawanya", typ.Field(i).Name)
		}
	}
}

// TestDurMsIsNotAConsensusInput menegaskan D21 secara struktural: satu-satunya
// bentuk masukan konsensus (consensus.Reading) tidak membawa dur_ms, jadi tidak
// ada cara mendiamkan atau meloloskan observasi berdasarkan durasi pada Fase 1.
// dur_ms DICATAT di ledger; ia bukan gerbang keputusan.
func TestDurMsIsNotAConsensusInput(t *testing.T) {
	typ := reflect.TypeOf(consensus.Reading{})
	for _, f := range []string{"DurMs", "Duration", "DurationMs"} {
		if _, ok := typ.FieldByName(f); ok {
			t.Errorf("consensus.Reading punya %q — dur_ms tidak boleh menjadi masukan keputusan pada Fase 1", f)
		}
	}
}

// zeroSig adalah tanda tangan hex yang bentuknya valid namun tidak pernah cocok.
const zeroSig = "0000000000000000000000000000000000000000000000000000000000000000"

func itoa(v int64) string { return strconv.FormatInt(v, 10) }

// ---------------------------------------------------------------------------
// §20.2 — pencatatan kegagalan verifikasi
// ---------------------------------------------------------------------------

func TestVerifyTrigger_RecordsEveryOutcome(t *testing.T) {
	const validTS = int64(1_700_000_005_000)
	secret := []byte("test-key")

	cases := []struct {
		name    string
		node    *store.NodeSecret
		payload []byte
		wantErr error
		want    string
	}{
		{
			name:    "diterima",
			node:    &store.NodeSecret{StationID: "NODE-0A1B2C3D", IsActive: true, Verified: true},
			payload: validTrigger(t, secret),
			want:    "OK",
		},
		{
			name:    "node non-aktif",
			node:    &store.NodeSecret{StationID: "NODE-0A1B2C3D", IsActive: false, Verified: true},
			payload: validTrigger(t, secret),
			wantErr: ErrNodeInactive,
			want:    "ErrNodeInactive",
		},
		{
			name:    "node belum terverifikasi",
			node:    &store.NodeSecret{StationID: "NODE-0A1B2C3D", IsActive: true, Verified: false},
			payload: validTrigger(t, secret),
			wantErr: ErrNodeUnverified,
			want:    "ErrNodeUnverified",
		},
		{
			name: "clock skew",
			node: &store.NodeSecret{StationID: "NODE-0A1B2C3D", IsActive: true, Verified: true},
			// 95 detik DI DEPAN jam server: di luar +30s MaxClockSkew. Arah masa
			// depan, karena ParseTrigger menolak ts di bawah 1.7e12 sebelum
			// gerbang freshness sempat dijalankan.
			payload: []byte(`{"node_id":"NODE-0A1B2C3D","pga":10,"dur_ms":1000,"ts":1700000100000,"signature":"` + zeroSig + `"}`),
			wantErr: ErrClockSkew,
			want:    "ErrClockSkew",
		},
		{
			name: "replay",
			node: &store.NodeSecret{
				StationID: "NODE-0A1B2C3D", IsActive: true, Verified: true,
				LastSeenTS: validTS,
			},
			payload: validTrigger(t, secret),
			wantErr: ErrReplay,
			want:    "ErrReplay",
		},
		{
			name:    "HMAC tidak valid",
			node:    &store.NodeSecret{StationID: "NODE-0A1B2C3D", IsActive: true, Verified: true},
			payload: []byte(`{"node_id":"NODE-0A1B2C3D","pga":10,"dur_ms":1000,"ts":1700000005000,"signature":"` + zeroSig + `"}`),
			wantErr: ErrBadSignature,
			want:    "ErrBadSignature",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := &fakeNodeSource{node: tc.node}
			v := newTestVerifier(t, src)
			w := &fakeObsWriter{}
			v.WithLedger(w)

			_, err := v.Verify(context.Background(), tc.payload)
			if tc.wantErr == nil && err != nil {
				t.Fatalf("verifikasi gagal tak terduga: %v", err)
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}

			if len(w.rows) != 1 {
				t.Fatalf("baris ledger = %d, want tepat 1", len(w.rows))
			}
			if got := w.rows[0].VerifyResult; got != tc.want {
				t.Errorf("verify_result = %q, want %q", got, tc.want)
			}
			if w.rows[0].NodeID != "NODE-0A1B2C3D" {
				t.Errorf("node_id = %q", w.rows[0].NodeID)
			}
		})
	}
}

// TestVerifyTrigger_InfraFailureNotRecordedAsVerifyResult: kegagalan basis data
// bukan pernyataan tentang perilaku sensor. Mencatatnya sebagai verify_result
// akan menyalahkan node atas kesalahan server.
func TestVerifyTrigger_InfraFailureNotRecordedAsVerifyResult(t *testing.T) {
	src := &fakeNodeSource{getErr: errors.New("pool habis")}
	v := newTestVerifier(t, src)
	w := &fakeObsWriter{}
	v.WithLedger(w)

	if _, err := v.Verify(context.Background(), validTrigger(t, []byte("test-key"))); err == nil {
		t.Fatal("want error dari store")
	}
	if len(w.rows) != 0 {
		t.Fatalf("baris ledger = %d, want 0 untuk kegagalan infrastruktur", len(w.rows))
	}
}

// TestVerifyTrigger_NoLedgerIsSafe: ledger nonaktif tidak boleh mengubah apa pun
// pada hasil verifikasi.
func TestVerifyTrigger_NoLedgerIsSafe(t *testing.T) {
	src := &fakeNodeSource{node: &store.NodeSecret{
		StationID: "NODE-0A1B2C3D", IsActive: true, Verified: true,
	}}
	v := newTestVerifier(t, src) // tanpa WithLedger

	if _, err := v.Verify(context.Background(), validTrigger(t, []byte("test-key"))); err != nil {
		t.Fatalf("verifikasi harus lolos tanpa ledger: %v", err)
	}
}

// ---------------------------------------------------------------------------
// §20.9 — waktu
// ---------------------------------------------------------------------------

// TestReceivedTSIsServerGenerated: received_ts berasal dari jam server, bukan
// dari payload. Node yang jamnya sedikit di depan server (masih dalam
// MaxClockSkew) adalah kasus SAH, jadi tidak ada di mana pun kode produksi yang
// boleh mensyaratkan received_ts >= publish_ts — pernyataan seperti itu akan
// menolak data yang benar.
func TestReceivedTSIsServerGenerated(t *testing.T) {
	serverNow := time.UnixMilli(1_700_000_005_000)
	src := &fakeNodeSource{node: &store.NodeSecret{
		StationID: "NODE-0A1B2C3D", IsActive: true, Verified: true,
	}}
	v := newTestVerifier(t, src)
	v.now = func() time.Time { return serverNow }
	w := &fakeObsWriter{}
	v.WithLedger(w)

	// ts payload 10 detik DI DEPAN jam server, tetapi dengan tanda tangan sah.
	// Lolos gerbang skew? Tidak — dan justru itu intinya: apa pun hasilnya,
	// received_ts harus tetap jam server.
	ahead := serverNow.UnixMilli() + 10_000
	canonical := CanonicalString("NODE-0A1B2C3D", 20, 1000, ahead)
	payload := []byte(`{"node_id":"NODE-0A1B2C3D","pga":20,"dur_ms":1000,"ts":` +
		itoa(ahead) + `,"signature":"` + ComputeHMAC([]byte("test-key"), canonical) + `"}`)

	_, _ = v.Verify(context.Background(), payload)

	if len(w.rows) != 1 {
		t.Fatalf("baris ledger = %d, want 1", len(w.rows))
	}
	row := w.rows[0]
	if row.ReceivedTS != serverNow.UnixMilli() {
		t.Errorf("received_ts = %d, want jam server %d", row.ReceivedTS, serverNow.UnixMilli())
	}
	if row.PublishTS != ahead {
		t.Errorf("publish_ts = %d, want ts payload %d", row.PublishTS, ahead)
	}
	if row.ReceivedTS >= row.PublishTS {
		t.Error("fixture ini seharusnya punya publish_ts > received_ts; kasus itulah yang harus tetap tercatat")
	}
}
