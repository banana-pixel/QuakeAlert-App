/**
 * Host test untuk src/onset.cpp — dijalankan dengan g++ biasa, TANPA ESP32.
 *
 * Kompilasi & jalankan: ./scripts/canonical-host-test.sh
 *
 * Yang diuji di sini adalah satu-satunya aritmetika di jalur onset yang dapat
 * salah tanpa terlihat: pemetaan tick monotonik onset menjadi ms epoch UTC.
 * Bila pemetaan itu meleset, node tetap mempublikasikan angka yang lolos skema,
 * lolos HMAC, dan disimpan server sebagai onset_ts_source=SENSOR — yaitu sebuah
 * angka salah yang berlabel pengukuran. Itu bentuk kegagalan paling buruk yang
 * mungkin di sini, jadi ia diuji dengan T0/T1 yang DIPATOK, bukan dengan jam.
 *
 * Yang secara sengaja TIDAK diuji sebagai "onset": publish_ts - dur_ms. Itu
 * batas atas, bukan pengukuran; salah satu test di bawah justru menuntut kedua
 * angka itu BERBEDA, supaya sebuah implementasi yang diam-diam menghitung onset
 * dari publish_ts gagal di sini alih-alih lolos dengan nama SENSOR.
 */

#include "../src/canonical.h"
#include "../src/onset.h"

#include <cstdio>
#include <cstring>
#include <string>

#ifndef CANONICAL_BUFFER_SIZE
#error "CANONICAL_BUFFER_SIZE harus disuntik dari src/config.h (-DCANONICAL_BUFFER_SIZE=...)"
#endif

// Toleransi koherensi onset server (server/internal/ingest/verifier.go:
// MaxOnsetSkew = 2 * time.Second). Dipatok di sini dengan sengaja: bila server
// mengecilkannya, test ini harus ikut ditinjau.
static const long long MAX_ONSET_SKEW_MS = 2000;

static int failures = 0;

static void expectI64(const char* what, long long got, long long want) {
    if (got != want) {
        std::printf("FAIL %s\n  got  %lld\n  want %lld\n", what, got, want);
        ++failures;
    }
}

static void expectTrue(const char* what, bool cond) {
    if (!cond) {
        std::printf("FAIL %s\n", what);
        ++failures;
    }
}

// --- Skenario yang dipatok ---------------------------------------------------
//
// Satu event, dua publikasi, seluruh angka ditulis tangan supaya jawaban yang
// benar diketahui tanpa menjalankan apa pun:
//
//   millis() 100000  onset (pelintasan ambang STA/LTA pertama)  <- T0
//   millis() 100300  konfirmasi; slot PRELIM diisi, dur_ms=300
//   millis() 100336  PRELIM dipublish                            <- T1 PRELIM
//   millis() 109407  de-trigger; slot FINAL diisi, dur_ms=9407
//   millis() 109431  FINAL dipublish                             <- T1 FINAL
//
// Jam dinding pada instan publish FINAL adalah EPOCH_AT_FINAL_PUBLISH.
static const uint32_t M_ONSET          = 100000;
static const uint32_t M_PRELIM_PUBLISH = 100336;
static const uint32_t M_DETRIGGER      = 109407;
static const uint32_t M_FINAL_PUBLISH  = 109431;
static const uint32_t DUR_PRELIM_MS    = 300;
static const uint32_t DUR_FINAL_MS     = 9407;

static const int64_t EPOCH_AT_PRELIM_PUBLISH = 1767225590905LL;
static const int64_t EPOCH_AT_FINAL_PUBLISH  = 1767225600000LL;

// Onset yang benar: 1767225600000 - (109431 - 100000) = 1767225590569.
static const int64_t WANT_ONSET_TS = 1767225590569LL;

// TEST WAJIB: onset = T0, publikasi = T1, T1 > T0, dan onset_ts YANG DILAPORKAN
// adalah T0 — bukan T1, bukan T1 - dur_ms.
static void testOnsetIsTriggerInstantNotPublishInstant() {
    const int64_t onsetTs = epochFromMonotonic(EPOCH_AT_FINAL_PUBLISH,
                                               M_FINAL_PUBLISH, M_ONSET);
    const int64_t publishTs = EPOCH_AT_FINAL_PUBLISH;

    expectI64("onset_ts == T0", onsetTs, WANT_ONSET_TS);
    expectI64("publish_ts == T1", publishTs, EPOCH_AT_FINAL_PUBLISH);
    expectTrue("onset_ts < publish_ts", onsetTs < publishTs);

    // Provenance SENSOR ditentukan server dari BENTUK payload: proto_ver=2 dan
    // onset_ts ada (server/internal/ingest/verifier.go TriggerObservation ->
    // OnsetSourceSensor). Yang dapat dijaga di sisi firmware adalah bahwa angka
    // itu memang ada, positif, dan bukan jam publish.
    expectTrue("onset_ts terisi (prasyarat onset_ts_source=SENSOR)", onsetTs > 0);
    expectTrue("onset_ts bukan publish_ts", onsetTs != publishTs);
}

// Onset TIDAK BOLEH sama dengan publish_ts - dur_ms. Selisih keduanya adalah
// keterlambatan publish (24 ms di skenario ini), dan justru selisih itulah yang
// membuat batas atas tidak dapat disebut pengukuran.
static void testOnsetDiffersFromPublishMinusDuration() {
    const int64_t onsetTs = epochFromMonotonic(EPOCH_AT_FINAL_PUBLISH,
                                               M_FINAL_PUBLISH, M_ONSET);
    const int64_t upperBound = EPOCH_AT_FINAL_PUBLISH - (int64_t)DUR_FINAL_MS;

    expectI64("batas atas publish_ts - dur_ms", upperBound, 1767225590593LL);
    expectTrue("onset_ts lebih awal dari batas atasnya", onsetTs < upperBound);
    expectI64("selisih = keterlambatan publish",
              upperBound - onsetTs, (long long)(M_FINAL_PUBLISH - M_DETRIGGER));
}

// PRELIM dan FINAL adalah dua publikasi pada waktu berbeda dengan dur_ms
// berbeda, tetapi onset event-nya SATU. Keduanya harus melaporkan onset_ts yang
// sama persis; kalau tidak, satu event akan tampak sebagai dua onset di server.
static void testPrelimAndFinalAgreeOnOnset() {
    const int64_t prelimOnset = epochFromMonotonic(EPOCH_AT_PRELIM_PUBLISH,
                                                   M_PRELIM_PUBLISH, M_ONSET);
    const int64_t finalOnset = epochFromMonotonic(EPOCH_AT_FINAL_PUBLISH,
                                                  M_FINAL_PUBLISH, M_ONSET);

    expectI64("onset_ts PRELIM", prelimOnset, WANT_ONSET_TS);
    expectI64("onset_ts FINAL", finalOnset, WANT_ONSET_TS);
}

// Pemeriksaan koherensi server (§14.4) dijalankan di sini atas angka yang
// dihasilkan firmware. Node yang gagal di pemeriksaan ini akan ditolak di
// lapangan tanpa satu pun test firmware yang berbunyi — jadi test-nya di sini.
static void testSatisfiesServerOnsetCoherence() {
    const int64_t onsetTs = epochFromMonotonic(EPOCH_AT_FINAL_PUBLISH,
                                               M_FINAL_PUBLISH, M_ONSET);
    const int64_t detriggerTs = epochFromMonotonic(EPOCH_AT_FINAL_PUBLISH,
                                                   M_FINAL_PUBLISH, M_DETRIGGER);
    const int64_t ts = EPOCH_AT_FINAL_PUBLISH;
    const long long dur = (long long)DUR_FINAL_MS;

    expectTrue("aturan 1: onset_ts <= ts", onsetTs <= ts);
    expectTrue("aturan 2: ts - onset_ts >= dur_ms - skew",
               (ts - onsetTs) >= dur - MAX_ONSET_SKEW_MS);
    expectTrue("aturan 3 (attempt_no=1): ts - onset_ts <= dur_ms + skew",
               (ts - onsetTs) <= dur + MAX_ONSET_SKEW_MS);
    expectTrue("aturan 4a: onset_ts <= detrigger_ts <= ts",
               onsetTs <= detriggerTs && detriggerTs <= ts);
    // Pemeriksaan terkuat: sepenuhnya di dalam jam sensor, tanpa keterlambatan
    // publish. Di sini ia harus EKSAK, bukan sekadar di dalam toleransi.
    expectI64("aturan 4b: detrigger_ts - onset_ts == dur_ms",
              detriggerTs - onsetTs, dur);
}

// PRELIM: dur_ms adalah waktu berjalan sejak onset saat slot diisi, bukan sejak
// publish. Selisih ts - onset_ts karena itu sedikit MELEBIHI dur_ms, dan harus
// tetap di dalam toleransi aturan 3.
static void testPrelimCoherence() {
    const int64_t onsetTs = epochFromMonotonic(EPOCH_AT_PRELIM_PUBLISH,
                                               M_PRELIM_PUBLISH, M_ONSET);
    const long long elapsed = EPOCH_AT_PRELIM_PUBLISH - onsetTs;

    expectI64("PRELIM ts - onset_ts", elapsed, (long long)(M_PRELIM_PUBLISH - M_ONSET));
    expectTrue("PRELIM elapsed >= dur_ms", elapsed >= (long long)DUR_PRELIM_MS);
    expectTrue("PRELIM elapsed <= dur_ms + skew",
               elapsed <= (long long)DUR_PRELIM_MS + MAX_ONSET_SKEW_MS);
}

// Tanpa jam dinding, pemetaan mengembalikan 0 — TIDAK menebak, tidak memakai
// uptime sebagai epoch. Nol adalah nilai yang membuat publishTrigger()
// membatalkan publikasi (src/mqtt.cpp), yaitu perilaku yang benar: lebih baik
// satu observasi hilang daripada satu onset karangan berlabel SENSOR.
static void testUnsyncedClockYieldsZero() {
    expectI64("epoch 0 -> 0", epochFromMonotonic(0, M_FINAL_PUBLISH, M_ONSET), 0);
    expectI64("epoch negatif -> 0", epochFromMonotonic(-1, M_FINAL_PUBLISH, M_ONSET), 0);
}

// NTP baru sinkron SETELAH onset: jam dinding sekarang benar, tick onset masih
// tersimpan, jadi onset tetap dapat dinyatakan dalam epoch dengan mundur.
// Inilah alasan konversi dilakukan pada saat publish, bukan pada saat onset.
static void testSyncAfterOnsetStillBackDates() {
    const int64_t onsetTs = epochFromMonotonic(EPOCH_AT_FINAL_PUBLISH,
                                               M_FINAL_PUBLISH, M_ONSET);
    expectI64("onset tetap benar meski sinkron menyusul", onsetTs, WANT_ONSET_TS);
}

// Onset pada instan yang sama dengan pembacaan jam: selisih nol, bukan negatif.
static void testZeroElapsed() {
    expectI64("elapsed 0", epochFromMonotonic(EPOCH_AT_FINAL_PUBLISH,
                                              M_FINAL_PUBLISH, M_FINAL_PUBLISH),
              EPOCH_AT_FINAL_PUBLISH);
}

// millis() berputar setiap ~49,7 hari. Onset sebelum putaran dan publish
// sesudahnya: selisih unsigned 32-bit tetap benar (66 ms), sedangkan aritmetika
// yang dilebarkan ke int64 lebih dulu akan menghasilkan ~4,29 miliar ms —
// sebuah onset di tahun 1834 yang lolos HMAC.
static void testMillisWrapAround() {
    const uint32_t atMillis = 0xFFFFFFF0u;  // 16 ms sebelum wrap
    const uint32_t nowMillis = 50u;         // 50 ms sesudah wrap
    expectI64("selisih melewati wrap = 66 ms",
              epochFromMonotonic(EPOCH_AT_FINAL_PUBLISH, nowMillis, atMillis),
              EPOCH_AT_FINAL_PUBLISH - 66);
}

// Angka yang dipetakan masuk ke string kanonik v2 pada posisi onset_ts, dan
// posisi ts tetap jam publish. Keduanya harus muncul BERBEDA di string yang
// ditandatangani; string yang membawa angka yang sama di kedua posisi berarti
// onset hilang di jalur perakitan payload, bukan di pemetaannya.
static void testOnsetReachesCanonicalStringV2() {
    const int64_t onsetTs = epochFromMonotonic(EPOCH_AT_FINAL_PUBLISH,
                                               M_FINAL_PUBLISH, M_ONSET);
    const int64_t detriggerTs = epochFromMonotonic(EPOCH_AT_FINAL_PUBLISH,
                                                   M_FINAL_PUBLISH, M_DETRIGGER);
    char buf[CANONICAL_BUFFER_SIZE];
    const size_t n = buildCanonicalStringV2(buf, sizeof(buf), PROTO_VER_V2,
                                            "NODE-0A1B2C3D", PHASE_FINAL,
                                            196609, 1, 194.1993f, DUR_FINAL_MS,
                                            onsetTs, detriggerTs,
                                            EPOCH_AT_FINAL_PUBLISH);
    expectTrue("string kanonik terbangun", n > 0);
    expectI64("kanonik v2 dengan onset terukur", (long long)n, (long long)std::strlen(buf));

    const std::string got(buf);
    expectTrue("onset_ts ada di string kanonik",
               got.find("|1767225590569|") != std::string::npos);
    expectTrue("ts publish ada di string kanonik",
               got.size() > 13 && got.compare(got.size() - 13, 13, "1767225600000") == 0);
}

int main() {
    testOnsetIsTriggerInstantNotPublishInstant();
    testOnsetDiffersFromPublishMinusDuration();
    testPrelimAndFinalAgreeOnOnset();
    testSatisfiesServerOnsetCoherence();
    testPrelimCoherence();
    testUnsyncedClockYieldsZero();
    testSyncAfterOnsetStillBackDates();
    testZeroElapsed();
    testMillisWrapAround();
    testOnsetReachesCanonicalStringV2();

    if (failures != 0) {
        std::printf("\n%d pemeriksaan GAGAL\n", failures);
        return 1;
    }
    std::printf("semua pemeriksaan onset host lulus\n");
    return 0;
}
