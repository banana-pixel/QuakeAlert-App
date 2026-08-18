package crypto

import (
	"bytes"
	"testing"
)

func testKey() [32]byte {
	var k [32]byte
	for i := range k {
		k[i] = byte(i)
	}
	return k
}

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	c, err := New(testKey())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	plaintext := []byte("node-hmac-secret-abc123")

	ct, nonce, err := c.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if len(nonce) != NonceSize {
		t.Fatalf("nonce size %d != %d", len(nonce), NonceSize)
	}
	if bytes.Equal(ct, plaintext) {
		t.Fatal("ciphertext sama dengan plaintext")
	}

	got, err := c.Decrypt(ct, nonce)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("round-trip mismatch: got=%q want=%q", got, plaintext)
	}
}

func TestDecrypt_TamperedCiphertextFails(t *testing.T) {
	c, _ := New(testKey())
	ct, nonce, _ := c.Encrypt([]byte("rahasia"))
	ct[0] ^= 0xFF // tamper

	if _, err := c.Decrypt(ct, nonce); err == nil {
		t.Fatal("Decrypt harus gagal untuk ciphertext yang dirusak (GCM auth)")
	}
}

func TestDecrypt_WrongNonceFails(t *testing.T) {
	c, _ := New(testKey())
	ct, nonce, _ := c.Encrypt([]byte("rahasia"))
	nonce[0] ^= 0xFF

	if _, err := c.Decrypt(ct, nonce); err == nil {
		t.Fatal("Decrypt harus gagal untuk nonce yang salah")
	}
}

func TestDecrypt_BadNonceSize(t *testing.T) {
	c, _ := New(testKey())
	if _, err := c.Decrypt([]byte("x"), []byte("short")); err == nil {
		t.Fatal("Decrypt harus menolak nonce dengan panjang salah")
	}
}
