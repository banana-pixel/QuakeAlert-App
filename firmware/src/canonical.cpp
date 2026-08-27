/**
 * QuakeAlert ESP32 - Implementasi string kanonik (v1 & v2).
 *
 * Tanpa Arduino, tanpa mbedTLS: hanya snprintf, agar berkas ini dapat
 * dikompilasi di host dan diuji terhadap vektor yang sama dengan server Go.
 */

#include "canonical.h"

#include <stdio.h>

namespace {
// formatPga menuliskan pga dengan 4 desimal fixed — identik dengan
// strconv.FormatFloat(pga,'f',4,64) di Go dan dipakai oleh KEDUA versi, karena
// satu-satunya cara agar keduanya tidak menyimpang adalah tidak menuliskannya
// dua kali.
bool formatPga(char* out, size_t outSize, float pgaGal) {
    const int n = snprintf(out, outSize, "%.4f", (double)pgaGal);
    return n > 0 && (size_t)n < outSize;
}
}  // namespace

size_t buildCanonicalString(char* out, size_t outSize,
                            const char* nodeId, float pgaGal,
                            uint32_t durMs, int64_t tsMs) {
    if (out == nullptr || outSize == 0 || nodeId == nullptr) {
        return 0;
    }

    // snprintf "%.4f" membulatkan sesuai libc; nilai PGA seismik (<2000 gal)
    // aman dari perbedaan pembulatan pada 4 desimal.
    char pgaStr[24];
    if (!formatPga(pgaStr, sizeof(pgaStr), pgaGal)) {
        return 0;
    }

    // ts & dur sebagai integer desimal. PRId64 tidak selalu tersedia di
    // toolchain Arduino; gunakan format eksplisit.
    const int written = snprintf(out, outSize, "%s|%s|%lu|%lld",
                                nodeId, pgaStr,
                                (unsigned long)durMs,
                                (long long)tsMs);
    if (written <= 0 || (size_t)written >= outSize) {
        return 0;
    }
    return (size_t)written;
}

size_t buildCanonicalStringV2(char* out, size_t outSize,
                              int protoVer, const char* nodeId,
                              const char* phase, int64_t obsSeq,
                              uint8_t attemptNo, float pgaGal, uint32_t durMs,
                              int64_t onsetTsMs, int64_t detriggerTsMs,
                              int64_t tsMs) {
    if (out == nullptr || outSize == 0 || nodeId == nullptr || phase == nullptr) {
        return 0;
    }

    char pgaStr[24];
    if (!formatPga(pgaStr, sizeof(pgaStr), pgaGal)) {
        return 0;
    }

    const int written = snprintf(out, outSize, "%d|%s|%s|%lld|%u|%s|%lu|%lld|%lld|%lld",
                                protoVer, nodeId, phase,
                                (long long)obsSeq,
                                (unsigned)attemptNo,
                                pgaStr,
                                (unsigned long)durMs,
                                (long long)onsetTsMs,
                                (long long)detriggerTsMs,
                                (long long)tsMs);
    if (written <= 0 || (size_t)written >= outSize) {
        return 0;
    }
    return (size_t)written;
}

int64_t composeObsSeq(int32_t bootCount, uint16_t inBootSeq) {
    // bootCount negatif tidak dapat terjadi (penghitung NVS monotonik), tetapi
    // menggesernya ke kiri bila terjadi adalah undefined behaviour; jepit ke 0
    // agar obs_seq tetap sebuah angka dan bukan sebuah kejutan.
    const int64_t boots = bootCount > 0 ? (int64_t)bootCount : 0;
    return (boots << 16) | (int64_t)inBootSeq;
}
