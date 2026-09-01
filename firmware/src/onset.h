/**
 * QuakeAlert ESP32 - Pemetaan tick monotonik onset ke ms epoch UTC.
 *
 * Onset getaran DIUKUR pada jam monotonik (millis()) di sensor.cpp: ia adalah
 * pelintasan ambang STA/LTA yang pertama, dicatat sebelum event dikonfirmasi.
 * Yang dikirim ke server harus ms epoch UTC, jadi tick itu perlu dipetakan ke
 * epoch — dan pemetaan itulah satu-satunya aritmetika di jalur onset yang dapat
 * salah tanpa terlihat.
 *
 * Berkas ini SENGAJA tidak menyentuh Arduino: tidak ada millis(), tidak ada
 * gettimeofday, tidak ada mutex. Jam dibaca oleh pemanggilnya (utils.cpp) dan
 * DIMASUKKAN sebagai argumen. Itulah yang membuat pemetaannya dapat diuji di
 * host dengan T0/T1 yang dipatok (lihat scripts/canonical-host-test.sh); sebuah
 * fungsi yang memanggil millis() sendiri hanya dapat diuji di atas perangkat,
 * yang berarti dalam praktiknya tidak diuji.
 *
 * Yang TIDAK dilakukan di sini: onset TIDAK PERNAH dihitung sebagai
 * publish_ts - dur_ms. Itu adalah batas atas yang galatnya adalah keterlambatan
 * publish, dan server sudah menyimpannya sendiri sebagai onset_ts_upper_bound
 * dengan onset_ts_source=PUBLISH_BOUND. Angka dari berkas ini adalah pengukuran,
 * dan hanya karena itu ia boleh dilaporkan sebagai SENSOR.
 */

#ifndef ONSET_H
#define ONSET_H

#include <stdint.h>

// Petakan sebuah tick millis() ke ms epoch UTC.
//
//   nowEpochMs  jam dinding SEKARANG (ms epoch UTC), 0 bila belum tersinkron.
//   nowMillis   nilai millis() yang dibaca pada instan yang SAMA dengan nowEpochMs.
//   atMillis    tick yang ingin dipetakan, mis. onset atau de-trigger.
//
// Mengembalikan 0 bila nowEpochMs <= 0 — tanpa jam dinding, tick monotonik tidak
// dapat dinyatakan dalam epoch, dan mengarangnya akan berarti melabeli angka
// palsu sebagai pengukuran sensor. Nol adalah nilai yang ditolak publishTrigger().
//
// Selisih millis() dihitung pada tipe unsigned 32-bit dengan sengaja: ia benar
// melewati wrap-around ~49,7 hari, sedangkan mengurangkan dua nilai yang sudah
// dilebarkan ke int64 tidak.
int64_t epochFromMonotonic(int64_t nowEpochMs, uint32_t nowMillis, uint32_t atMillis);

#endif  // ONSET_H
