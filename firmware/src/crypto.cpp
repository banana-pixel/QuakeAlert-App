/**
 * QuakeAlert ESP32 - HMAC-SHA256 implementation (mbedTLS bundled in ESP32
 * Arduino framework — no external dependency).
 *
 * Canonical byte-stream must match server/internal/ingest/hmac.go exactly.
 */

#include "crypto.h"

#include <string.h>
#include <stdio.h>
#include <mbedtls/md.h>

size_t buildCanonicalString(char* out, size_t outSize,
                            const char* nodeId, float pgaGal,
                            uint32_t durMs, int64_t tsMs) {
    if (out == nullptr || outSize == 0 || nodeId == nullptr) {
        return 0;
    }

    // pga fixed 4 desimal — identik dengan strconv.FormatFloat(pga,'f',4,64) di Go.
    // snprintf "%.4f" membulatkan half-to-even/half-away sesuai libc; nilai PGA
    // seismik (<2000 gal) aman dari perbedaan pembulatan pada 4 desimal.
    char pgaStr[24];
    int n = snprintf(pgaStr, sizeof(pgaStr), "%.4f", (double)pgaGal);
    if (n <= 0 || (size_t)n >= sizeof(pgaStr)) {
        return 0;
    }

    // ts & dur sebagai integer desimal. PRId64 tidak selalu tersedia di
    // toolchain Arduino; gunakan format eksplisit.
    int written = snprintf(out, outSize, "%s|%s|%lu|%lld",
                           nodeId, pgaStr,
                           (unsigned long)durMs,
                           (long long)tsMs);
    if (written <= 0 || (size_t)written >= outSize) {
        return 0;
    }
    return (size_t)written;
}

bool computeHmacHex(const uint8_t* key, size_t keyLen,
                    const char* canonical, size_t canonicalLen,
                    char* outHex, size_t outHexSize) {
    if (key == nullptr || canonical == nullptr || outHex == nullptr) {
        return false;
    }
    // 32-byte SHA256 => 64 hex + NUL.
    if (outHexSize < 65) {
        return false;
    }

    const mbedtls_md_info_t* info = mbedtls_md_info_from_type(MBEDTLS_MD_SHA256);
    if (info == nullptr) {
        return false;
    }

    uint8_t raw[32];
    int rc = mbedtls_md_hmac(info,
                             key, keyLen,
                             reinterpret_cast<const uint8_t*>(canonical), canonicalLen,
                             raw);
    if (rc != 0) {
        return false;
    }

    static const char* hexChars = "0123456789abcdef";
    for (size_t i = 0; i < 32; ++i) {
        outHex[i * 2]     = hexChars[(raw[i] >> 4) & 0x0F];
        outHex[i * 2 + 1] = hexChars[raw[i] & 0x0F];
    }
    outHex[64] = '\0';
    return true;
}
