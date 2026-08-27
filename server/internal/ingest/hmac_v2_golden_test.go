package ingest

import "testing"

// §20.5 — golden vector protokol v2.
//
// Seperti vektor v1, string dan digest di bawah DIPATOK sebagai literal dan
// TIDAK dihitung ulang dari CanonicalStringV2. Alasannya sama, dan pada v2 lebih
// tajam: bentuk kanonik ini ditandatangani BERSAMA firmware ESP32
// (buildCanonicalStringV2 di firmware/src/crypto.cpp), dan firmware tidak dapat
// diperbarui secepat server. Sebuah test yang membangun harapannya dengan fungsi
// yang diuji akan tetap lulus setelah formatnya berubah — dan yang gagal justru
// perangkat di lapangan.
//
// Digest diverifikasi secara independen di luar Go:
//
//	printf '%s' '2|NODE-0A1B2C3D|PRELIM|196609|1|0.4215|300|1700000004700|0|1700000005000' \
//	  | openssl dgst -sha256 -hmac 'test-key'
//
// Secret "test-key" adalah nilai uji tetap, bukan kredensial.
func TestCanonicalStringV2_GoldenVectors(t *testing.T) {
	const secret = "test-key"

	cases := []struct {
		name        string
		protoVer    int
		nodeID      string
		phase       string
		obsSeq      int64
		attemptNo   int
		pga         float64
		durMs       int64
		onsetTS     int64
		detriggerTS int64
		ts          int64
		canonical   string
		digest      string
	}{
		{
			// PRELIM: detrigger_ts TIDAK ADA pada payload dan diserialisasi
			// sebagai 0 — bukan dihilangkan. Arita tetap inilah yang membuat dua
			// payload berbeda tidak dapat menghasilkan satu string kanonik.
			name:      "PRELIM tanpa detrigger",
			protoVer:  2,
			nodeID:    "NODE-0A1B2C3D",
			phase:     PhasePrelim,
			obsSeq:    196609, // (3 << 16) | 1
			attemptNo: 1,
			pga:       0.4215,
			durMs:     300,
			onsetTS:   1_700_000_004_700,
			ts:        1_700_000_005_000,
			canonical: "2|NODE-0A1B2C3D|PRELIM|196609|1|0.4215|300|1700000004700|0|1700000005000",
			digest:    "7a46e2129c648deaef9b48d58b021a73d2250910ab8ec0832c173536f3a2bc3d",
		},
		{
			name:        "FINAL dengan detrigger",
			protoVer:    2,
			nodeID:      "NODE-0A1B2C3D",
			phase:       PhaseFinal,
			obsSeq:      196609,
			attemptNo:   1,
			pga:         1.8842,
			durMs:       2800,
			onsetTS:     1_700_000_002_200,
			detriggerTS: 1_700_000_005_000,
			ts:          1_700_000_005_000,
			canonical:   "2|NODE-0A1B2C3D|FINAL|196609|1|1.8842|2800|1700000002200|1700000005000|1700000005000",
			digest:      "67db9da449681b81b47e41fd1c2f7d15e900adf0f02d00cdd4e2aba54f21bd32",
		},
		{
			// Nilai batas: obs_seq 0 (event pertama pada boot pertama) dan
			// attempt_no 255. obs_seq 0 SAH, dan itulah sebabnya field v2
			// bertipe pointer: nol tidak dapat berarti "tidak ada".
			// pga bulat tetap membawa empat desimal, sama seperti v1.
			name:        "batas: obs_seq 0, attempt_no 255, pga bulat",
			protoVer:    2,
			nodeID:      "NODE-FFFFFFFF",
			phase:       PhaseFinal,
			obsSeq:      0,
			attemptNo:   255,
			pga:         16,
			durMs:       0,
			onsetTS:     1_700_000_000_000,
			detriggerTS: 1_700_000_000_000,
			ts:          1_700_000_000_000,
			canonical:   "2|NODE-FFFFFFFF|FINAL|0|255|16.0000|0|1700000000000|1700000000000|1700000000000",
			digest:      "f0d3f7068c9a5ba55378c0ab169c0e64a754297046a18533d0d1182d5ae4c8f3",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CanonicalStringV2(tc.protoVer, tc.nodeID, tc.phase, tc.obsSeq,
				tc.attemptNo, tc.pga, tc.durMs, tc.onsetTS, tc.detriggerTS, tc.ts)
			if got != tc.canonical {
				t.Fatalf("canonical v2:\n got  %q\n want %q", got, tc.canonical)
			}
			if d := ComputeHMAC([]byte(secret), got); d != tc.digest {
				t.Errorf("digest = %q, want %q", d, tc.digest)
			}
		})
	}
}

// TestCanonicalV2IsNotV1 menegaskan bahwa kedua bentuk tidak dapat bertabrakan:
// tidak ada payload v2 yang string kanoniknya sama dengan payload v1 mana pun.
// Prefiks "2|" yang tidak dapat muncul di posisi pertama v1 (node_id selalu
// "NODE-…") inilah yang menjaminnya.
func TestCanonicalV2IsNotV1(t *testing.T) {
	v1 := CanonicalString("NODE-0A1B2C3D", 0.4215, 300, 1_700_000_005_000)
	v2 := CanonicalStringV2(2, "NODE-0A1B2C3D", PhaseFinal, 1, 1,
		0.4215, 300, 1_700_000_004_700, 1_700_000_005_000, 1_700_000_005_000)
	if v1 == v2 {
		t.Fatal("bentuk kanonik v1 dan v2 bertabrakan")
	}
}
