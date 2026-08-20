// Package api mengimplementasikan REST API tier QuakeAlert (contract-first,
// contracts/openapi/openapi.yaml). Router chi + auth JWT HS256 + handler.
package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// Klaim JWT anonymous minimal. sub = user_id (UUID), exp = expiry (detik epoch).
type jwtClaims struct {
	Sub string `json:"sub"`
	Exp int64  `json:"exp"`
	Iat int64  `json:"iat"`
}

type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

var (
	errMalformedToken = errors.New("token JWT malformed")
	errBadSignature   = errors.New("signature JWT tidak valid")
	errWrongAlg       = errors.New("algoritma JWT bukan HS256")
	errExpired        = errors.New("token JWT kedaluwarsa")
	errMissingExp     = errors.New("klaim exp wajib ada")
	errEmptySubject   = errors.New("klaim sub kosong")
	errEmptySecret    = errors.New("secret HS256 kosong")
	errNonPositiveTTL = errors.New("ttl token harus > 0")
)

// verifyHS256 memvalidasi token JWT HS256 secara byte-safe (constant-time
// signature compare) dan mengembalikan klaim bila valid. Hanya menerima
// alg=HS256 untuk mencegah alg-confusion (mis. "none").
func verifyHS256(token string, secret []byte, now time.Time) (*jwtClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errMalformedToken
	}
	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, errMalformedToken
	}
	var h jwtHeader
	if err := json.Unmarshal(headerJSON, &h); err != nil {
		return nil, errMalformedToken
	}
	if h.Alg != "HS256" {
		return nil, errWrongAlg
	}

	signingInput := parts[0] + "." + parts[1]
	expectedSig := signHS256([]byte(signingInput), secret)
	gotSig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, errMalformedToken
	}
	if !hmac.Equal(expectedSig, gotSig) {
		return nil, errBadSignature
	}

	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errMalformedToken
	}
	var c jwtClaims
	if err := json.Unmarshal(claimsJSON, &c); err != nil {
		return nil, errMalformedToken
	}
	if c.Sub == "" {
		return nil, errEmptySubject
	}
	// exp WAJIB ada: token tanpa batas waktu efektif (exp=0) akan hidup selamanya
	// bila secret bocor. Jalur mint selalu menetapkan exp; verifier menegakkannya
	// sebagai pertahanan berlapis untuk jalur mint lain di masa depan.
	if c.Exp == 0 {
		return nil, errMissingExp
	}
	if now.Unix() >= c.Exp {
		return nil, errExpired
	}
	return &c, nil
}

func signHS256(signingInput, secret []byte) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write(signingInput)
	return mac.Sum(nil)
}

// mintHS256 membuat token JWT HS256 (dipakai di test & bootstrap dev).
func mintHS256(claims jwtClaims, secret []byte) string {
	header := jwtHeader{Alg: "HS256", Typ: "JWT"}
	hb, _ := json.Marshal(header)
	cb, _ := json.Marshal(claims)
	h := base64.RawURLEncoding.EncodeToString(hb)
	c := base64.RawURLEncoding.EncodeToString(cb)
	sig := signHS256([]byte(h+"."+c), secret)
	return h + "." + c + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// MintHS256 menerbitkan token JWT HS256 untuk userID dengan masa hidup ttl.
// Ini jalur produksi yang dipakai HandleAnonymousAuth; klaim yang dihasilkan
// adalah `sub` (=user_id), `iat`, dan `exp` (detik epoch UTC) — bentuk yang
// sama dengan yang divalidasi verifyHS256, sehingga token terbitan sendiri
// selalu lolos middleware auth.
//
// Argumen ditolak eksplisit alih-alih diperbaiki diam-diam: subject kosong,
// secret kosong, atau ttl <= 0 akan menghasilkan token yang tidak berguna
// (atau langsung kedaluwarsa) dan menyembunyikan salah-konfigurasi.
func MintHS256(userID string, secret []byte, ttl time.Duration) (string, error) {
	if userID == "" {
		return "", errEmptySubject
	}
	if len(secret) == 0 {
		return "", errEmptySecret
	}
	if ttl <= 0 {
		return "", errNonPositiveTTL
	}
	now := time.Now()
	return mintHS256(jwtClaims{
		Sub: userID,
		Iat: now.Unix(),
		Exp: now.Add(ttl).Unix(),
	}, secret), nil
}
