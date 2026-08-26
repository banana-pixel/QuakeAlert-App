/**
 * QuakeAlert ESP32 - Core Utilities Implementation
 */

#include "utils.h"
#include "config.h"
#include "state.h"

#include <Preferences.h>
#include <stdarg.h>
#include <stdio.h>
#include <string.h>
#include <time.h>
#include <sys/time.h>



namespace {
bool copyStringLocked(char* destination, size_t destinationSize, const char* source) {
    if (destination == nullptr || destinationSize == 0) {
        return false;
    }

    const char* safeSource = (source != nullptr) ? source : "";
    const int written = snprintf(destination, destinationSize, "%s", safeSource);
    return written >= 0 && static_cast<size_t>(written) < destinationSize;
}
}  // namespace

void monitorHeap() {
    if (millis() - lastHeapCheck <= 10000) {
        return;
    }

    lastHeapCheck = millis();

    const uint32_t freeHeap = ESP.getFreeHeap();
    const uint32_t maxAllocHeap = ESP.getMaxAllocHeap();

    if (freeHeap < minHeapSeen) {
        minHeapSeen = freeHeap;
    }

    const float fragmentation = calculateHeapFragmentationPercent(freeHeap, maxAllocHeap);
    if (fragmentation > maxHeapFragmentationSeen) {
        maxHeapFragmentationSeen = fragmentation;
    }

    if (freeHeap < 15000) {
        Serial.printf(
            "WARNING: Heap low: free=%lu maxAlloc=%lu fragmentation=%.2f%%\n",
            static_cast<unsigned long>(freeHeap),
            static_cast<unsigned long>(maxAllocHeap),
            fragmentation
        );
    }

    if (fragmentation > MAX_FRAGMENTATION_PERCENT) {
        Serial.printf(
            "WARNING: Heap fragmentation high: %.2f%% (free=%lu maxAlloc=%lu)\n",
            fragmentation,
            static_cast<unsigned long>(freeHeap),
            static_cast<unsigned long>(maxAllocHeap)
        );
    }
}

bool copyStringSafe(char* destination, size_t destinationSize, const char* source) {
    return copyStringLocked(destination, destinationSize, source);
}

bool formatStringSafe(char* destination, size_t destinationSize, const char* format, ...) {
    if (destination == nullptr || destinationSize == 0 || format == nullptr) {
        return false;
    }

    va_list args;
    va_start(args, format);
    const int written = vsnprintf(destination, destinationSize, format, args);
    va_end(args);

    if (written < 0) {
        destination[0] = '\0';
        return false;
    }

    if (static_cast<size_t>(written) >= destinationSize) {
        destination[destinationSize - 1] = '\0';
        return false;
    }

    return true;
}

void setLokasiAlat(const char* lokasi) {
    if (stateMutex != nullptr) {
        xSemaphoreTake(stateMutex, portMAX_DELAY);
    }

    copyStringLocked(lokasiAlat, sizeof(lokasiAlat), lokasi);
    locationResolved = (lokasi != nullptr && lokasi[0] != '\0');

    if (stateMutex != nullptr) {
        xSemaphoreGive(stateMutex);
    }
}

void setLocationStatusUnknown() {
    setLokasiAlat("Lokasi Tidak Diketahui (API Error)");
    if (stateMutex != nullptr) {
        xSemaphoreTake(stateMutex, portMAX_DELAY);
        locationResolved = false;
        xSemaphoreGive(stateMutex);
    } else {
        locationResolved = false;
    }
}

void setLocationStatusSearching() {
    setLokasiAlat("Mencari lokasi...");
    if (stateMutex != nullptr) {
        xSemaphoreTake(stateMutex, portMAX_DELAY);
        locationResolved = false;
        xSemaphoreGive(stateMutex);
    } else {
        locationResolved = false;
    }
}

void setLocationStatusWifiDisconnected() {
    setLokasiAlat("WiFi terputus");
    if (stateMutex != nullptr) {
        xSemaphoreTake(stateMutex, portMAX_DELAY);
        locationResolved = false;
        xSemaphoreGive(stateMutex);
    } else {
        locationResolved = false;
    }
}

void setLastEventTime(const char* waktu) {
    if (stateMutex != nullptr) {
        xSemaphoreTake(stateMutex, portMAX_DELAY);
    }

    copyStringLocked(lastEventTime, sizeof(lastEventTime), waktu);

    if (stateMutex != nullptr) {
        xSemaphoreGive(stateMutex);
    }
}

void setLastIntensity(const char* intensity) {
    if (stateMutex != nullptr) {
        xSemaphoreTake(stateMutex, portMAX_DELAY);
    }

    copyStringLocked(lastIntensity, sizeof(lastIntensity), intensity);

    if (stateMutex != nullptr) {
        xSemaphoreGive(stateMutex);
    }
}

void setLastPga(const char* pgaText) {
    if (stateMutex != nullptr) {
        xSemaphoreTake(stateMutex, portMAX_DELAY);
    }

    copyStringLocked(lastPgaStr, sizeof(lastPgaStr), pgaText);

    if (stateMutex != nullptr) {
        xSemaphoreGive(stateMutex);
    }
}

void setNtpSyncStatus(bool synced) {
    if (stateMutex != nullptr) {
        xSemaphoreTake(stateMutex, portMAX_DELAY);
    }

    isNtpSynced = synced;

    if (stateMutex != nullptr) {
        xSemaphoreGive(stateMutex);
    }
}

bool getLokasiAlatCopy(char* destination, size_t destinationSize) {
    if (destination == nullptr || destinationSize == 0) {
        return false;
    }

    bool ok;
    if (stateMutex != nullptr) {
        xSemaphoreTake(stateMutex, portMAX_DELAY);
    }

    ok = copyStringLocked(destination, destinationSize, lokasiAlat);

    if (stateMutex != nullptr) {
        xSemaphoreGive(stateMutex);
    }

    return ok;
}

bool getStationIdCopy(char* destination, size_t destinationSize) {
    if (destination == nullptr || destinationSize == 0) {
        return false;
    }

    bool ok;
    if (stateMutex != nullptr) {
        xSemaphoreTake(stateMutex, portMAX_DELAY);
    }

    ok = copyStringLocked(destination, destinationSize, StationID);

    if (stateMutex != nullptr) {
        xSemaphoreGive(stateMutex);
    }

    return ok;
}

size_t getHmacKeyCopy(char* destination, size_t destinationSize) {
    if (destination == nullptr || destinationSize == 0) {
        return 0;
    }

    // HMAC secret disimpan di NVS namespace "quake-app" saat provisioning.
    // Baca read-only agar tidak menulis ulang partisi NVS.
    Preferences prefs;
    if (!prefs.begin("quake-app", true)) {
        destination[0] = '\0';
        return 0;
    }

    size_t len = prefs.getString(NVS_KEY_HMAC, destination, destinationSize);
    prefs.end();

    if (len == 0 || len >= destinationSize) {
        destination[0] = '\0';
        return 0;
    }

    // Preferences::getString returns the length INCLUDING the NUL terminator.
    // The NUL is not part of the secret; strip it so HMAC uses the exact
    // key bytes the server stores (otherwise HMAC block-padding differs and
    // every signature is rejected as "HMAC invalid").
    if (destination[len - 1] == '\0') {
        len -= 1;
    }
    return len;
}

bool getLastEventTimeCopy(char* destination, size_t destinationSize) {

    if (destination == nullptr || destinationSize == 0) {
        return false;
    }

    bool ok;
    if (stateMutex != nullptr) {
        xSemaphoreTake(stateMutex, portMAX_DELAY);
    }

    ok = copyStringLocked(destination, destinationSize, lastEventTime);

    if (stateMutex != nullptr) {
        xSemaphoreGive(stateMutex);
    }

    return ok;
}

bool getLastIntensityCopy(char* destination, size_t destinationSize) {
    if (destination == nullptr || destinationSize == 0) {
        return false;
    }

    bool ok;
    if (stateMutex != nullptr) {
        xSemaphoreTake(stateMutex, portMAX_DELAY);
    }

    ok = copyStringLocked(destination, destinationSize, lastIntensity);

    if (stateMutex != nullptr) {
        xSemaphoreGive(stateMutex);
    }

    return ok;
}

bool getWaktuString(char* destination, size_t destinationSize) {
    if (destination == nullptr || destinationSize == 0) {
        return false;
    }

    bool ntpReady = isNtpSynced;
    if (stateMutex != nullptr) {
        xSemaphoreTake(stateMutex, portMAX_DELAY);
        ntpReady = isNtpSynced;
        xSemaphoreGive(stateMutex);
    }

    if (!ntpReady) {
        return copyStringLocked(destination, destinationSize, "N/A");
    }

    struct tm timeinfo;
    if (!getLocalTime(&timeinfo)) {
        return copyStringLocked(destination, destinationSize, "N/A");
    }

    const size_t written = strftime(destination, destinationSize, "%Y-%m-%d %H:%M:%S", &timeinfo);
    return written > 0;
}

bool getUptimeString(char* destination, size_t destinationSize) {
    if (destination == nullptr || destinationSize == 0) {
        return false;
    }

    const unsigned long uptimeMillis = millis();
    const unsigned long days = uptimeMillis / 86400000UL;
    const unsigned long hours = (uptimeMillis % 86400000UL) / 3600000UL;
    const unsigned long minutes = (uptimeMillis % 3600000UL) / 60000UL;

    return formatStringSafe(destination, destinationSize, "%lud %luh %lum", days, hours, minutes);
}

int64_t getEpochMillis() {
    bool ntpReady = isNtpSynced;
    if (stateMutex != nullptr) {
        xSemaphoreTake(stateMutex, portMAX_DELAY);
        ntpReady = isNtpSynced;
        xSemaphoreGive(stateMutex);
    }

    if (!ntpReady) {
        return 0;
    }

    struct timeval tv;
    if (gettimeofday(&tv, nullptr) != 0) {
        return 0;
    }

    // Guard terhadap clock yang belum benar-benar disetel (masih ~1970).
    if (tv.tv_sec < 1000000000L) {  // sebelum 2001-09-09
        return 0;
    }

    return (int64_t)tv.tv_sec * 1000LL + (int64_t)(tv.tv_usec / 1000);
}

const char* intensityToText(float pgaValue) {

    if (pgaValue < 0.5f) {
        return "I (Tidak Terasa)";
    }
    if (pgaValue < 2.8f) {
        return "II-III (Lemah)";
    }
    if (pgaValue < 6.2f) {
        return "IV (Ringan)";
    }
    if (pgaValue < 12.0f) {
        return "V (Sedang)";
    }
    if (pgaValue < 22.0f) {
        return "VI (Kuat)";
    }
    if (pgaValue < 40.0f) {
        return "VII (Sangat Kuat)";
    }
    if (pgaValue < 75.0f) {
        return "VIII (Merusak)";
    }
    if (pgaValue < 139.0f) {
        return "IX (Hebat)";
    }
    return "X+ (Ekstrem)";
}

float calculateHeapFragmentationPercent(uint32_t freeHeap, uint32_t maxAllocHeap) {
    if (freeHeap == 0) {
        return 100.0f;
    }

    return 100.0f * (1.0f - (static_cast<float>(maxAllocHeap) / static_cast<float>(freeHeap)));
}