/**
 * QuakeAlert ESP32 - Core Utilities
 */

#ifndef UTILS_H
#define UTILS_H

#include <Arduino.h>
#include <stddef.h>

struct tm;

void monitorHeap();

bool copyStringSafe(char* destination, size_t destinationSize, const char* source);
bool formatStringSafe(char* destination, size_t destinationSize, const char* format, ...);
void setLokasiAlat(const char* lokasi);
void setLocationStatusUnknown();
void setLocationStatusSearching();
void setLocationStatusWifiDisconnected();
void setLastEventTime(const char* waktu);
void setLastIntensity(const char* intensity);
void setLastPga(const char* pgaText);
void setNtpSyncStatus(bool synced);

bool getLokasiAlatCopy(char* destination, size_t destinationSize);
bool getStationIdCopy(char* destination, size_t destinationSize);
// Salin HMAC secret per-node dari NVS (namespace "quake-app", key NVS_KEY_HMAC).
// Mengembalikan panjang key (>0) atau 0 bila belum di-provision.
size_t getHmacKeyCopy(char* destination, size_t destinationSize);

bool getLastEventTimeCopy(char* destination, size_t destinationSize);
bool getLastIntensityCopy(char* destination, size_t destinationSize);

bool getWaktuString(char* destination, size_t destinationSize);
bool getUptimeString(char* destination, size_t destinationSize);
// Waktu epoch UTC dalam milidetik (int64). Mengembalikan 0 bila NTP belum sinkron.
int64_t getEpochMillis();

// Epoch UTC (ms) untuk sebuah instan millis() di masa lalu. 0 bila NTP belum
// sinkron. Diperlukan karena seluruh pewaktuan sensor adalah millis(): tanpa
// konversi ini onset_ts hanya dapat diambil pada saat publish, yaitu justru batas
// atas yang protokol v2 ada untuk menggantikan.
int64_t epochAtMillis(unsigned long atMillis);

// Kualitas jam menurut node sendiri, nilai kontrak heartbeat: "NTP" atau "NONE".
// "RTC" tidak pernah dikembalikan — perangkat keras saat ini tidak punya RTC.
const char* getClockSource();

// Koreksi terakhir yang diterapkan pada jam node (ms, positif = jam dimajukan).
// false berarti belum ada koreksi yang terukur (sinkronisasi pertama tidak punya
// pembanding), dan heartbeat HARUS menghilangkan field-nya alih-alih mengirim 0.
bool getClockOffsetMs(int64_t& offsetMs);
// Dicatat oleh checkNtpSync() setiap kali jam yang SUDAH pernah sinkron dikoreksi.
void setClockOffsetMs(int64_t offsetMs);

const char* intensityToText(float pgaValue);
float calculateHeapFragmentationPercent(uint32_t freeHeap, uint32_t maxAllocHeap);

#endif  // UTILS_H