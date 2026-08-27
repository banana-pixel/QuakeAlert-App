package ingest

import (
	"errors"
	"testing"
)

// Test di file ini menguji ParseTrigger, yang tidak menyentuh HMAC; karena itu
// setiap payload memakai zeroSig — bentuknya sah, cocoknya tidak pernah.

func TestParseTrigger_V1WithV2FieldRejected(t *testing.T) {
	// Setiap field v2 secara sendiri-sendiri, tanpa proto_ver. Semuanya harus
	// DITOLAK, bukan diabaikan: tanpa proto_ver tidak satu pun dari field ini
	// ikut ditandatangani.
	fields := []string{
		`"phase":"FINAL"`,
		`"obs_seq":1`,
		`"attempt_no":1`,
		`"onset_ts":1700000004700`,
		`"detrigger_ts":1700000005000`,
	}
	for _, f := range fields {
		t.Run(f, func(t *testing.T) {
			raw := `{"node_id":"NODE-0A1B2C3D","pga":0.4215,"dur_ms":300,` +
				`"ts":1700000005000,"signature":"` + zeroSig + `",` + f + `}`
			_, err := ParseTrigger([]byte(raw))
			if !errors.Is(err, ErrUnsignedV2Field) {
				t.Fatalf("err = %v, want ErrUnsignedV2Field", err)
			}
		})
	}
}

func TestParseTrigger_V1StillAccepted(t *testing.T) {
	raw := `{"node_id":"NODE-0A1B2C3D","pga":413.13,"dur_ms":8000,` +
		`"ts":1700000005000,"signature":"` + zeroSig + `"}`
	tr, err := ParseTrigger([]byte(raw))
	if err != nil {
		t.Fatalf("payload v1 ditolak: %v", err)
	}
	if tr.IsV2() {
		t.Error("payload tanpa proto_ver dikenali sebagai v2")
	}
	// v1 selalu FINAL: ia dipublish saat event sudah selesai.
	if tr.EffectivePhase() != PhaseFinal {
		t.Errorf("EffectivePhase = %q, want FINAL", tr.EffectivePhase())
	}
}

func TestParseTrigger_V2FieldValidation(t *testing.T) {
	cases := []struct {
		name    string
		fields  string
		wantErr error
	}{
		{
			name:   "PRELIM lengkap",
			fields: `"proto_ver":2,"phase":"PRELIM","obs_seq":196609,"attempt_no":1,"onset_ts":1700000004700`,
		},
		{
			name: "FINAL lengkap",
			fields: `"proto_ver":2,"phase":"FINAL","obs_seq":196609,"attempt_no":1,` +
				`"onset_ts":1700000004700,"detrigger_ts":1700000005000`,
		},
		{
			// obs_seq 0 SAH: event pertama pada boot pertama.
			name:   "obs_seq 0 sah",
			fields: `"proto_ver":2,"phase":"PRELIM","obs_seq":0,"attempt_no":1,"onset_ts":1700000004700`,
		},
		{
			name:    "proto_ver tak dikenal",
			fields:  `"proto_ver":3,"phase":"PRELIM","obs_seq":1,"attempt_no":1,"onset_ts":1700000004700`,
			wantErr: ErrInvalidProtoVer,
		},
		{
			name:    "phase tidak ada",
			fields:  `"proto_ver":2,"obs_seq":1,"attempt_no":1,"onset_ts":1700000004700`,
			wantErr: ErrInvalidPhase,
		},
		{
			name:    "phase tak dikenal",
			fields:  `"proto_ver":2,"phase":"UPDATE","obs_seq":1,"attempt_no":1,"onset_ts":1700000004700`,
			wantErr: ErrInvalidPhase,
		},
		{
			name:    "obs_seq tidak ada",
			fields:  `"proto_ver":2,"phase":"PRELIM","attempt_no":1,"onset_ts":1700000004700`,
			wantErr: ErrInvalidObsSeq,
		},
		{
			name:    "obs_seq negatif",
			fields:  `"proto_ver":2,"phase":"PRELIM","obs_seq":-1,"attempt_no":1,"onset_ts":1700000004700`,
			wantErr: ErrInvalidObsSeq,
		},
		{
			name:    "attempt_no tidak ada",
			fields:  `"proto_ver":2,"phase":"PRELIM","obs_seq":1,"onset_ts":1700000004700`,
			wantErr: ErrInvalidAttemptNo,
		},
		{
			name:    "attempt_no 0",
			fields:  `"proto_ver":2,"phase":"PRELIM","obs_seq":1,"attempt_no":0,"onset_ts":1700000004700`,
			wantErr: ErrInvalidAttemptNo,
		},
		{
			name:    "attempt_no 256",
			fields:  `"proto_ver":2,"phase":"PRELIM","obs_seq":1,"attempt_no":256,"onset_ts":1700000004700`,
			wantErr: ErrInvalidAttemptNo,
		},
		{
			name:    "onset_ts tidak ada",
			fields:  `"proto_ver":2,"phase":"PRELIM","obs_seq":1,"attempt_no":1`,
			wantErr: ErrInvalidOnsetTS,
		},
		{
			name:    "onset_ts nol",
			fields:  `"proto_ver":2,"phase":"PRELIM","obs_seq":1,"attempt_no":1,"onset_ts":0`,
			wantErr: ErrInvalidOnsetTS,
		},
		{
			name:    "FINAL tanpa detrigger_ts",
			fields:  `"proto_ver":2,"phase":"FINAL","obs_seq":1,"attempt_no":1,"onset_ts":1700000004700`,
			wantErr: ErrInvalidDetrigger,
		},
		{
			// 0 bukan "tidak ada": ia akan lolos setiap pemeriksaan keberadaan
			// dan tercatat sebagai instan di tahun 1970.
			name: "FINAL dengan detrigger_ts 0",
			fields: `"proto_ver":2,"phase":"FINAL","obs_seq":1,"attempt_no":1,` +
				`"onset_ts":1700000004700,"detrigger_ts":0`,
			wantErr: ErrInvalidDetrigger,
		},
		{
			name: "PRELIM membawa detrigger_ts",
			fields: `"proto_ver":2,"phase":"PRELIM","obs_seq":1,"attempt_no":1,` +
				`"onset_ts":1700000004700,"detrigger_ts":1700000005000`,
			wantErr: ErrInvalidDetrigger,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := `{"node_id":"NODE-0A1B2C3D","pga":0.4215,"dur_ms":300,` +
				`"ts":1700000005000,"signature":"` + zeroSig + `",` + tc.fields + `}`
			tr, err := ParseTrigger([]byte(raw))
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("payload sah ditolak: %v", err)
				}
				if !tr.IsV2() {
					t.Error("payload dengan proto_ver tidak dikenali sebagai v2")
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

// TestParseTrigger_KeepsV1Bounds menegaskan bahwa batas v1 tidak melunak karena
// v2 ada: field bersama tetap divalidasi dengan aturan yang sama pada kedua versi.
func TestParseTrigger_KeepsV1Bounds(t *testing.T) {
	// Batas v1 tidak berubah karena v2 ada. pga di luar rentang tetap ditolak
	// meski payloadnya v2.
	raw := `{"node_id":"NODE-0A1B2C3D","pga":2000.1,"dur_ms":300,"ts":1700000005000,` +
		`"signature":"` + zeroSig + `","proto_ver":2,"phase":"PRELIM","obs_seq":1,` +
		`"attempt_no":1,"onset_ts":1700000004700}`
	if _, err := ParseTrigger([]byte(raw)); !errors.Is(err, ErrInvalidPGA) {
		t.Fatalf("err = %v, want ErrInvalidPGA", err)
	}
}
