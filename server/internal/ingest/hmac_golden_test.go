package ingest

import "testing"

// §20.5 — golden vector protokol v1.
//
// String dan digest di bawah ini DIPATOK sebagai literal, bukan dihitung ulang
// dari CanonicalString. Itulah seluruh maksudnya: sebuah test yang membangun
// nilai harapannya dengan fungsi yang sedang diuji akan tetap lulus setelah
// format kanoniknya berubah — dan format itu ditandatangani BERSAMA firmware,
// yang tidak dapat diperbarui secepat server.
//
// Ketika Fase 2 menambahkan CanonicalStringV2, test ini adalah pagar yang
// menahannya agar tidak menyentuh perilaku v1.
//
// Secret vektor ini adalah nilai uji tetap ("test-key"), bukan kredensial.
// Digest-nya diverifikasi secara independen di luar Go:
//
//	printf '%s' 'NODE-0A1B2C3D|413.1300|8000|1700000005000' \
//	  | openssl dgst -sha256 -hmac 'test-key'
//
// sehingga nilai patokannya tidak berasal dari implementasi yang sedang diuji.
func TestCanonicalStringV1_GoldenVectors(t *testing.T) {
	const secret = "test-key"

	cases := []struct {
		name      string
		nodeID    string
		pga       float64
		durMs     int64
		ts        int64
		canonical string
		digest    string
	}{
		{
			name:      "nominal",
			nodeID:    "NODE-0A1B2C3D",
			pga:       413.13,
			durMs:     8000,
			ts:        1_700_000_005_000,
			canonical: "NODE-0A1B2C3D|413.1300|8000|1700000005000",
			digest:    "cb2cc5d59a7ce4922e8325d9f4cb8de816a84da968c5429d0d9fcab6d9f69e7b",
		},
		{
			// pga bulat tetap membawa empat desimal. Nol di belakang itu BAGIAN
			// dari yang ditandatangani, bukan kosmetik.
			name:      "pga bulat tetap 4 desimal",
			nodeID:    "NODE-FFFFFFFF",
			pga:       16,
			durMs:     0,
			ts:        1_700_000_000_000,
			canonical: "NODE-FFFFFFFF|16.0000|0|1700000000000",
			digest:    "2a6abeac6f4eb90229c7af69b3dc508201c2874151887fa409a268fb6621c1dc",
		},
		{
			// Pembulatan ke desimal keempat, bukan pemotongan.
			name:      "pembulatan desimal keempat",
			nodeID:    "NODE-00000001",
			pga:       0.00005,
			durMs:     60000,
			ts:        1_700_000_009_999,
			canonical: "NODE-00000001|0.0001|60000|1700000009999",
			digest:    "31ad47abc7eeb401a42fec53a6626db0346672aa3fae1012fe5025be16b20cf6",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CanonicalString(tc.nodeID, tc.pga, tc.durMs, tc.ts)
			if got != tc.canonical {
				t.Fatalf("string kanonik berubah:\n got  %q\n want %q\n"+
					"Bila ini disengaja, firmware/src/crypto.cpp WAJIB berubah pada rilis yang sama.",
					got, tc.canonical)
			}
			if digest := ComputeHMAC([]byte(secret), got); digest != tc.digest {
				t.Fatalf("digest v1 berubah untuk %q:\n got  %s\n want %s", got, digest, tc.digest)
			}
			if !VerifyHMAC([]byte(secret), tc.canonical, tc.digest) {
				t.Error("VerifyHMAC menolak vektor golden-nya sendiri")
			}
		})
	}
}
