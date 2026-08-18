package ingest

import "testing"

func TestCanonicalString_Format(t *testing.T) {
	// pga harus fixed 4 desimal, byte-identik dengan firmware.
	got := CanonicalString("NODE-0A1B2C3D", 413.13, 8000, 1700000005000)
	want := "NODE-0A1B2C3D|413.1300|8000|1700000005000"
	if got != want {
		t.Fatalf("canonical mismatch:\n got=%q\nwant=%q", got, want)
	}
}

func TestCanonicalString_PGARounding(t *testing.T) {
	// Nilai dengan >4 desimal dibulatkan ke 4.
	got := CanonicalString("NODE-00000001", 12.34567, 1000, 1700000000000)
	want := "NODE-00000001|12.3457|1000|1700000000000"
	if got != want {
		t.Fatalf("got=%q want=%q", got, want)
	}
}

func TestVerifyHMAC_RoundTrip(t *testing.T) {
	secret := []byte("super-secret-node-key")
	canonical := CanonicalString("NODE-0A1B2C3D", 413.13, 8000, 1700000005000)
	sig := ComputeHMAC(secret, canonical)

	if len(sig) != 64 {
		t.Fatalf("signature harus 64 hex char, dapat %d", len(sig))
	}
	if !VerifyHMAC(secret, canonical, sig) {
		t.Fatal("VerifyHMAC gagal untuk signature yang benar")
	}
	// Ubah char terakhir ke nilai yang PASTI berbeda (hindari kasus char == '0').
	last := sig[63]
	repl := byte('0')
	if last == '0' {
		repl = '1'
	}
	tampered := sig[:63] + string(repl)
	if VerifyHMAC(secret, canonical, tampered) {
		t.Fatal("VerifyHMAC menerima signature yang salah")
	}

	if VerifyHMAC([]byte("kunci-lain"), canonical, sig) {
		t.Fatal("VerifyHMAC menerima secret yang salah")
	}
}

func TestComputeHMAC_KnownVector(t *testing.T) {
	// Known-answer test (KAT) untuk uji silang dengan firmware ESP32.
	// secret="test", canonical="NODE-00000001|1.0000|0|1700000000000".
	// `want` diverifikasi terhadap OpenSSL:
	//   printf '%s' 'NODE-00000001|1.0000|0|1700000000000' \
	//     | openssl dgst -sha256 -hmac 'test'
	// Firmware WAJIB menghasilkan hex yang identik byte-per-byte.
	secret := []byte("test")
	canonical := "NODE-00000001|1.0000|0|1700000000000"
	const want = "b26a6f9e1a18d02a347a1d8605eedf8f37e229933336f739075874ac92185128"

	sig := ComputeHMAC(secret, canonical)
	if len(sig) != 64 {
		t.Fatalf("panjang hmac salah: %d", len(sig))
	}
	if sig != ComputeHMAC(secret, canonical) {
		t.Fatal("HMAC tidak deterministik")
	}
	if sig != want {
		t.Fatalf("HMAC known-vector mismatch:\n got=%q\nwant=%q", sig, want)
	}
}



