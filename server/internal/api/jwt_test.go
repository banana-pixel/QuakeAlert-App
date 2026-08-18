package api

import (
	"testing"
	"time"
)

func TestVerifyHS256_Valid(t *testing.T) {
	secret := []byte("this-is-a-32-byte-minimum-secret!")
	now := time.Unix(1_700_000_000, 0)
	token := mintHS256(jwtClaims{Sub: "user-123", Iat: now.Unix(), Exp: now.Add(time.Hour).Unix()}, secret)

	claims, err := verifyHS256(token, secret, now)
	if err != nil {
		t.Fatalf("verify gagal: %v", err)
	}
	if claims.Sub != "user-123" {
		t.Fatalf("sub = %q, mau user-123", claims.Sub)
	}
}

func TestVerifyHS256_Expired(t *testing.T) {
	secret := []byte("this-is-a-32-byte-minimum-secret!")
	now := time.Unix(1_700_000_000, 0)
	token := mintHS256(jwtClaims{Sub: "u", Exp: now.Add(-time.Second).Unix()}, secret)

	if _, err := verifyHS256(token, secret, now); err != errExpired {
		t.Fatalf("err = %v, mau errExpired", err)
	}
}

func TestVerifyHS256_BadSignature(t *testing.T) {
	secret := []byte("this-is-a-32-byte-minimum-secret!")
	other := []byte("another-32-byte-minimum-secret!!")
	now := time.Unix(1_700_000_000, 0)
	token := mintHS256(jwtClaims{Sub: "u", Exp: now.Add(time.Hour).Unix()}, secret)

	if _, err := verifyHS256(token, other, now); err != errBadSignature {
		t.Fatalf("err = %v, mau errBadSignature", err)
	}
}

func TestVerifyHS256_Malformed(t *testing.T) {
	secret := []byte("this-is-a-32-byte-minimum-secret!")
	now := time.Unix(1_700_000_000, 0)
	for _, tok := range []string{"", "a.b", "a.b.c.d", "not-base64.@@@.@@@"} {
		if _, err := verifyHS256(tok, secret, now); err == nil {
			t.Fatalf("token %q seharusnya invalid", tok)
		}
	}
}

func TestVerifyHS256_RejectNoneAlg(t *testing.T) {
	secret := []byte("this-is-a-32-byte-minimum-secret!")
	now := time.Unix(1_700_000_000, 0)
	// header alg=none, tanpa signature valid → harus ditolak (alg-confusion).
	// {"alg":"none","typ":"JWT"} . {"sub":"u"} . ""
	tok := "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0.eyJzdWIiOiJ1In0."
	if _, err := verifyHS256(tok, secret, now); err == nil {
		t.Fatal("alg=none harus ditolak")
	}
}
