/**
 * QuakeAlert ESP32 - HMAC-SHA256 canonicalization & signing.
 *
 * Kontrak (contracts/mqtt/trigger.schema.json + .clinerules/30 #5):
 *   String kanonik = "node_id|pga|dur_ms|ts"
 *     - pemisah '|'
 *     - pga fixed 4 desimal (snprintf "%.4f")
 *     - dur_ms & ts integer desimal (ts = ms epoch UTC)
 *   signature = HMAC-SHA256 hex lowercase 64-char.
 *
 * Implementasi ini WAJIB byte-identik dengan server Go
 * (server/internal/ingest/hmac.go). Known-answer test:
 *   secret="test", canonical="NODE-00000001|1.0000|0|1700000000000"
 *   => b26a6f9e1a18d02a347a1d8605eedf8f37e229933336f739075874ac92185128
 */

#ifndef CRYPTO_H
#define CRYPTO_H

#include <Arduino.h>
#include <stddef.h>
#include <stdint.h>

// Bangun string kanonik ke dalam buffer. Mengembalikan panjang string (>0)
// atau 0 bila buffer terlalu kecil. pga diformat 4 desimal fixed.
size_t buildCanonicalString(char* out, size_t outSize,
                            const char* nodeId, float pgaGal,
                            uint32_t durMs, int64_t tsMs);

// Hitung HMAC-SHA256 atas `canonical` dan tulis 64 char hex lowercase + NUL
// ke `outHex` (butuh >= 65 byte). Mengembalikan true bila sukses.
bool computeHmacHex(const uint8_t* key, size_t keyLen,
                    const char* canonical, size_t canonicalLen,
                    char* outHex, size_t outHexSize);

#endif  // CRYPTO_H
