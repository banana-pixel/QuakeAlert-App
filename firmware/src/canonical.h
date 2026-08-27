/**
 * QuakeAlert ESP32 - String kanonik yang ditandatangani.
 *
 * DUA bentuk hidup berdampingan permanen (contracts/mqtt/trigger.schema.json):
 *
 *   v1: "node_id|pga|dur_ms|ts"
 *   v2: "proto_ver|node_id|phase|obs_seq|attempt_no|pga|dur_ms|onset_ts|
 *        detrigger_ts|ts"
 *
 * dengan pemisah '|', pga 4 desimal fixed (snprintf "%.4f"), sisanya integer
 * desimal, dan semua timestamp ms epoch UTC.
 *
 * Berkas ini SENGAJA tidak menyentuh Arduino maupun mbedTLS: hanya snprintf.
 * Itulah yang membuatnya dapat dikompilasi dan diuji di host (lihat
 * scripts/canonical-host-test.sh), dan pemeriksaan silang byte-per-byte
 * terhadap server Go adalah satu-satunya cara mengetahui bahwa kedua sisi
 * menandatangani string yang sama.
 *
 * Implementasi ini WAJIB byte-identik dengan server Go
 * (server/internal/ingest/hmac.go).
 */

#ifndef CANONICAL_H
#define CANONICAL_H

#include <stddef.h>
#include <stdint.h>

// Nilai proto_ver satu-satunya yang dikenal kontrak.
#define PROTO_VER_V2 2

// Nilai phase kontrak. PRELIM dipublish pada konfirmasi onset, FINAL saat event
// ditutup — TEPAT DUA publikasi per event.
#define PHASE_PRELIM "PRELIM"
#define PHASE_FINAL  "FINAL"

// Bangun string kanonik v1 ke dalam buffer. Mengembalikan panjang string (>0)
// atau 0 bila buffer terlalu kecil. pga diformat 4 desimal fixed.
size_t buildCanonicalString(char* out, size_t outSize,
                            const char* nodeId, float pgaGal,
                            uint32_t durMs, int64_t tsMs);

// Bangun string kanonik v2. Aritasnya TETAP: setiap field selalu muncul, bahkan
// yang tidak ada. detriggerTsMs untuk phase PRELIM diserialisasi sebagai 0,
// BUKAN dihilangkan — arita yang berubah-ubah membuat dua payload berbeda dapat
// menghasilkan string kanonik yang sama.
//
// attemptNo dan detriggerTsMs berada DI DALAM string ini dengan sengaja:
// penghitung percobaan yang tidak ditandatangani adalah metadata yang dapat
// dikendalikan penyerang tentang laporan yang ditandatangani.
size_t buildCanonicalStringV2(char* out, size_t outSize,
                              int protoVer, const char* nodeId,
                              const char* phase, int64_t obsSeq,
                              uint8_t attemptNo, float pgaGal, uint32_t durMs,
                              int64_t onsetTsMs, int64_t detriggerTsMs,
                              int64_t tsMs);

// obs_seq = (boot_count << 16) | in_boot_seq.
//
// boot_count di bit tinggi berarti node yang reboot dan memulai ulang penghitung
// per-boot-nya tidak bertabrakan dengan riwayatnya sendiri, TANPA satu pun
// penulisan NVS per event — dan penulisan NVS per event adalah hal yang akan
// menghabiskan flash tepat pada node yang paling sering melaporkan gempa.
int64_t composeObsSeq(int32_t bootCount, uint16_t inBootSeq);

#endif  // CANONICAL_H
