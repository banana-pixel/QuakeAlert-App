/**
 * Host test untuk src/canonical.cpp — dijalankan dengan g++ biasa, TANPA ESP32.
 *
 * Kompilasi & jalankan: ./scripts/canonical-host-test.sh
 *
 * Yang diuji di sini adalah satu-satunya hal yang tidak dapat diuji dari sisi
 * server: bahwa firmware membangun string kanonik yang SAMA byte-per-byte dengan
 * yang diharapkan server. Vektornya DIPATOK sebagai literal dan identik dengan
 * server/internal/ingest/hmac_v2_golden_test.go — bila salah satu sisi berubah,
 * yang gagal harus sebuah test, bukan sebuah node di lapangan.
 *
 * Digest HMAC tidak dihitung di sini (mbedTLS tidak ada di host); yang dijaga
 * adalah string masukannya, dan itulah bagian yang dapat menyimpang tanpa
 * terdeteksi.
 */

#include "../src/canonical.h"

#include <cstdio>
#include <cstring>
#include <string>

// CANONICAL_BUFFER_SIZE disuntik oleh scripts/canonical-host-test.sh yang
// membacanya dari src/config.h. Disuntik, bukan disalin: sebuah salinan di sini
// akan tetap lulus setelah nilai di config.h dikecilkan.
#ifndef CANONICAL_BUFFER_SIZE
#error "CANONICAL_BUFFER_SIZE harus disuntik dari src/config.h (-DCANONICAL_BUFFER_SIZE=...)"
#endif

static int failures = 0;

static void expectStr(const char* what, const std::string& got, const std::string& want) {
    if (got != want) {
        std::printf("FAIL %s\n  got  \"%s\"\n  want \"%s\"\n", what, got.c_str(), want.c_str());
        ++failures;
    }
}

static void expectI64(const char* what, long long got, long long want) {
    if (got != want) {
        std::printf("FAIL %s\n  got  %lld\n  want %lld\n", what, got, want);
        ++failures;
    }
}

// --- v1: bentuk yang dibekukan; satu vektor cukup untuk mengunci formatnya. ---
static void testV1() {
    char buf[CANONICAL_BUFFER_SIZE];
    const size_t n = buildCanonicalString(buf, sizeof(buf), "NODE-00000001", 1.0f, 0, 1700000000000LL);
    expectI64("panjang v1", (long long)n, (long long)std::strlen(buf));
    expectStr("kanonik v1", buf, "NODE-00000001|1.0000|0|1700000000000");
}

// --- v2: vektor yang sama dengan golden test server. ---
static void testV2GoldenVectors() {
    char buf[CANONICAL_BUFFER_SIZE];

    // PRELIM: detrigger_ts tidak ada dan diserialisasi sebagai 0.
    buildCanonicalStringV2(buf, sizeof(buf), PROTO_VER_V2, "NODE-0A1B2C3D",
                           PHASE_PRELIM, 196609, 1, 0.4215f, 300,
                           1700000004700LL, 0, 1700000005000LL);
    expectStr("kanonik v2 PRELIM", buf,
              "2|NODE-0A1B2C3D|PRELIM|196609|1|0.4215|300|1700000004700|0|1700000005000");

    buildCanonicalStringV2(buf, sizeof(buf), PROTO_VER_V2, "NODE-0A1B2C3D",
                           PHASE_FINAL, 196609, 1, 1.8842f, 2800,
                           1700000002200LL, 1700000005000LL, 1700000005000LL);
    expectStr("kanonik v2 FINAL", buf,
              "2|NODE-0A1B2C3D|FINAL|196609|1|1.8842|2800|1700000002200|1700000005000|1700000005000");

    // Batas: obs_seq 0 (event pertama pada boot pertama) SAH, attempt_no 255,
    // pga bulat tetap membawa empat desimal.
    buildCanonicalStringV2(buf, sizeof(buf), PROTO_VER_V2, "NODE-FFFFFFFF",
                           PHASE_FINAL, 0, 255, 16.0f, 0,
                           1700000000000LL, 1700000000000LL, 1700000000000LL);
    expectStr("kanonik v2 batas", buf,
              "2|NODE-FFFFFFFF|FINAL|0|255|16.0000|0|1700000000000|1700000000000|1700000000000");
}

// --- §20.10: CANONICAL_BUFFER_SIZE harus menampung kasus TERBURUK. ---
//
// Buffer yang terlalu kecil tidak menghasilkan tanda tangan yang salah — ia
// menghasilkan publikasi yang dibatalkan, diam-diam, hanya pada nilai ekstrem.
// Karena itu batasnya diuji, bukan diperkirakan.
static void testWorstCaseFitsBuffer() {
    char buf[CANONICAL_BUFFER_SIZE];

    // node_id sepanjang yang dapat disimpan STATION_ID_BUFFER_SIZE (16) = 15
    // char; phase terpanjang = "PRELIM"; obs_seq = int64 maksimum; attempt_no
    // 255; pga 2000 gal (batas kontrak); dur_ms 60000 (batas kontrak); ketiga
    // timestamp 13 digit.
    const char* longestNodeId = "NODE-0123456789";
    const size_t n = buildCanonicalStringV2(buf, sizeof(buf), PROTO_VER_V2, longestNodeId,
                                            PHASE_PRELIM, 9223372036854775807LL, 255,
                                            2000.0f, 60000,
                                            9999999999999LL, 9999999999999LL, 9999999999999LL);
    if (n == 0) {
        std::printf("FAIL CANONICAL_BUFFER_SIZE (%d) terlalu kecil untuk kasus terburuk\n",
                    (int)CANONICAL_BUFFER_SIZE);
        ++failures;
        return;
    }
    std::printf("kasus terburuk = %zu byte + NUL, buffer = %d\n", n, (int)CANONICAL_BUFFER_SIZE);
    if (n + 1 > CANONICAL_BUFFER_SIZE) {
        std::printf("FAIL kasus terburuk tidak muat\n");
        ++failures;
    }

    // Dan buffer yang benar-benar terlalu kecil harus GAGAL, bukan memotong:
    // string kanonik yang terpotong akan ditandatangani dengan benar dan ditolak
    // server sebagai HMAC invalid — kegagalan yang menyalahkan pihak yang salah.
    char tiny[16];
    if (buildCanonicalStringV2(tiny, sizeof(tiny), PROTO_VER_V2, longestNodeId,
                               PHASE_FINAL, 1, 1, 1.0f, 1, 1700000000000LL,
                               1700000000000LL, 1700000000000LL) != 0) {
        std::printf("FAIL buffer kecil harus mengembalikan 0, bukan memotong\n");
        ++failures;
    }
}

// --- obs_seq = (boot_count << 16) | in_boot_seq ---
static void testComposeObsSeq() {
    expectI64("obs_seq boot 3 seq 1", composeObsSeq(3, 1), 196609);
    expectI64("obs_seq boot 0 seq 0", composeObsSeq(0, 0), 0);
    // Penghitung per-boot yang berputar tidak bertabrakan dengan boot lain:
    // itulah gunanya boot_count berada di bit tinggi.
    expectI64("obs_seq boot 1 seq 65535", composeObsSeq(1, 65535), 131071);
    expectI64("obs_seq boot 2 seq 0", composeObsSeq(2, 0), 131072);
    if (composeObsSeq(1, 65535) >= composeObsSeq(2, 0)) {
        std::printf("FAIL obs_seq boot berikutnya harus melampaui boot sebelumnya\n");
        ++failures;
    }
}

int main() {
    testV1();
    testV2GoldenVectors();
    testWorstCaseFitsBuffer();
    testComposeObsSeq();

    if (failures != 0) {
        std::printf("\n%d pemeriksaan GAGAL\n", failures);
        return 1;
    }
    std::printf("semua pemeriksaan kanonik host lulus\n");
    return 0;
}
