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

// CanonicalStringV2 membangun string kanonik protokol v2, IDENTIK byte-per-byte
// dengan firmware ESP32 (buildCanonicalStringV2 di firmware/src/crypto.cpp):
//
//	proto_ver|node_id|phase|obs_seq|attempt_no|pga|dur_ms|onset_ts|detrigger_ts|ts
//
// Fungsi TERPISAH, bukan CanonicalString yang diberi parameter. Alasannya bukan
// gaya: penanda tangan yang dapat diparameterkan hanya berjarak satu refactor
// dari menandatangani string yang berbeda dengan yang ditandatangani perangkat,
// dan bentuk v1 adalah dependensi keamanan yang byte-exact (§12.2). Keduanya
// hidup berdampingan permanen — node yang firmware-nya dirollback kembali
// menjadi node v1 dan tetap diterima.
//
// Aritasnya TETAP: setiap field selalu muncul, bahkan yang tidak ada. detriggerTS
// yang tidak ada (phase PRELIM) diserialisasi sebagai 0, BUKAN dihilangkan —
// arita yang berubah-ubah membuat dua payload berbeda dapat menghasilkan string
// kanonik yang sama.
//
// attemptNo dan detriggerTS berada DI DALAM string ini dengan sengaja: penghitung
// percobaan yang tidak ditandatangani adalah metadata yang dapat dikendalikan
// penyerang tentang laporan yang ditandatangani.
func CanonicalStringV2(
	protoVer int,
	nodeID string,
	phase string,
	obsSeq int64,
	attemptNo int,
	pga float64,
	durMs int64,
	onsetTS int64,
	detriggerTS int64,
	ts int64,
) string {
	// pga: 'f' presisi 4 => "413.1300", sama seperti v1 dan sama seperti
	// snprintf("%.4f") di firmware.
	return strconv.Itoa(protoVer) + "|" +
		nodeID + "|" +
		phase + "|" +
		strconv.FormatInt(obsSeq, 10) + "|" +
		strconv.Itoa(attemptNo) + "|" +
		strconv.FormatFloat(pga, 'f', 4, 64) + "|" +
		strconv.FormatInt(durMs, 10) + "|" +
		strconv.FormatInt(onsetTS, 10) + "|" +
		strconv.FormatInt(detriggerTS, 10) + "|" +
		strconv.FormatInt(ts, 10)
}

// canonicalFor memilih bentuk kanonik menurut ADA/TIDAKNYA proto_ver — satu
// titik pemilihan, sehingga tidak ada jalur verifikasi yang dapat memilih
// bentuk yang salah. Tanpa proto_ver payload adalah v1 legacy; nilai v2 yang
// tidak ada pada payload v1 tidak pernah ikut ditandatangani karena
// ParseTrigger sudah menolak payload yang membawanya tanpa proto_ver.
func canonicalFor(t *Trigger) string {
	if !t.IsV2() {
		return CanonicalString(t.NodeID, t.PGA, t.DurMs, t.TS)
	}
	var detriggerTS int64 // 0 = tidak ada (PRELIM); lihat CanonicalStringV2.
	if t.DetriggerTS != nil {
		detriggerTS = *t.DetriggerTS
	}
	return CanonicalStringV2(
		*t.ProtoVer, t.NodeID, t.Phase, *t.ObsSeq, *t.AttemptNo,
		t.PGA, t.DurMs, *t.OnsetTS, detriggerTS, t.TS,
	)
}
