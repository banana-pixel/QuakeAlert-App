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

const char* intensityToText(float pgaValue);
float calculateHeapFragmentationPercent(uint32_t freeHeap, uint32_t maxAllocHeap);

#endif  // UTILS_H