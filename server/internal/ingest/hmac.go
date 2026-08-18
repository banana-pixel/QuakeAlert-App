// Package ingest menangani subscriber MQTT, verifikasi HMAC, dan anti-replay
// sebelum trigger diproses oleh engine konsensus.
package ingest

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
)

// CanonicalString membangun string kanonik yang ditandatangani, IDENTIK
// byte-per-byte dengan firmware ESP32 (contracts/mqtt + .clinerules/30 #5):
//
//	node_id|pga|dur_ms|ts
//
// dengan:
//   - pemisah '|'
//   - pga fixed 4 desimal (strconv 'f', 4)
//   - dur_ms & ts sebagai integer desimal
//
// Perubahan format apa pun WAJIB dicerminkan di firmware & diuji silang.
func CanonicalString(nodeID string, pga float64, durMs int64, ts int64) string {
	// strconv.FormatFloat 'f' dengan presisi 4 => "413.1300", konsisten lintas platform.
	pgaStr := strconv.FormatFloat(pga, 'f', 4, 64)
	return nodeID + "|" + pgaStr + "|" +
		strconv.FormatInt(durMs, 10) + "|" +
		strconv.FormatInt(ts, 10)
}

// ComputeHMAC menghitung HMAC-SHA256 dan mengembalikannya sebagai hex lowercase 64-char.
func ComputeHMAC(secret []byte, canonical string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(canonical))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyHMAC membandingkan signature yang diterima (hex) dengan yang dihitung,
// menggunakan perbandingan constant-time untuk mencegah timing attack.
func VerifyHMAC(secret []byte, canonical, receivedHex string) bool {
	expected := ComputeHMAC(secret, canonical)
	// hmac.Equal = constant time. Bandingkan bentuk hex (keduanya lowercase).
	return hmac.Equal([]byte(expected), []byte(receivedHex))
}
