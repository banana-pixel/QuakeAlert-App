/**
 * QuakeAlert ESP32 - HMAC-SHA256 signing.
 *
 * String kanonik yang ditandatangani hidup di canonical.h/.cpp — dipisahkan
 * karena ia dapat (dan harus) diuji di host, sedangkan berkas ini bergantung
 * pada mbedTLS. Header ini tetap meneruskan canonical.h agar pemanggil yang ada
 * tidak berubah.
 *
 * Kontrak (contracts/mqtt/trigger.schema.json + .clinerules/30 #5):
 *   signature = HMAC-SHA256 hex lowercase 64-char atas string kanonik versi
 *   yang sesuai.
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

#include "canonical.h"

// Hitung HMAC-SHA256 atas `canonical` dan tulis 64 char hex lowercase + NUL
// ke `outHex` (butuh >= 65 byte). Mengembalikan true bila sukses.
bool computeHmacHex(const uint8_t* key, size_t keyLen,
                    const char* canonical, size_t canonicalLen,
                    char* outHex, size_t outHexSize);

#endif  // CRYPTO_H
