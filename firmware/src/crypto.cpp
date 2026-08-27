/**
 * QuakeAlert ESP32 - HMAC-SHA256 implementation (mbedTLS bundled in ESP32
 * Arduino framework — no external dependency).
 *
 * String kanoniknya sendiri ada di canonical.cpp.
 */

#include "crypto.h"

#include <mbedtls/md.h>

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
