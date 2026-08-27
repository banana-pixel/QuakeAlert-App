/**
 * QuakeAlert ESP32 - MQTT Logic Implementation
 *
 * Contract-first: contracts/mqtt/trigger.schema.json & heartbeat.schema.json.
 * Topik dibangun runtime "sensor/<station_id>/<suffix>". Pengiriman adalah QoS 0
 * (PubSubClient tidak menegakkan lebih dari itu); ketahanan trigger datang dari
 * retry di handleAlerts() plus obs_seq+phase yang membuat retry aman
 * dideduplikasi server. trigger ditandatangani HMAC-SHA256 (crypto.cpp) memakai
 * secret per-node dari NVS (utils::getHmacKeyCopy).
 */

#include "mqtt.h"
#include "config.h"
#include "state.h"
#include "utils.h"
#include "crypto.h"
#include "mqtt_tls.h"

#include <WiFi.h>
#include <PubSubClient.h>
#include <ArduinoJson.h>
#include <stdio.h>
#include <string.h>

namespace {
bool serializeDocToBuffer(JsonDocument& doc, char* buffer, size_t bufferSize, size_t& outLength) {
    outLength = serializeJson(doc, buffer, bufferSize);
    if (outLength == 0 || outLength >= bufferSize || doc.overflowed()) {
        Serial.println("MQTT JSON serialization failed or payload too large");
        return false;
    }
    return true;
}

bool publishBuffer(const char* topic, const char* payload, size_t payloadLength) {
    if (topic == nullptr || payload == nullptr || payloadLength == 0) {
        return false;
    }
    if (!mqttClient.connected()) {
        return false;
    }

    // PubSubClient::publish hanya mendukung QoS 0. Jalur "QoS 1" yang dulu ada di
    // sini memakai beginPublish/write/endPublish dan TIDAK menghasilkan QoS 1:
    // tidak ada PUBACK yang ditunggu, dan endPublish() mengembalikan 1 tanpa
    // syarat sehingga pemeriksaannya vacuous. Yang tersisa hanyalah dua jalur
    // kode untuk satu perilaku, dan satu di antaranya berbohong.
    return mqttClient.publish(topic, reinterpret_cast<const uint8_t*>(payload), payloadLength, false);
}
}  // namespace

size_t buildTopic(char* out, size_t outSize, const char* suffix) {
    if (out == nullptr || outSize == 0 || suffix == nullptr) {
        return 0;
    }

    char stationId[STATION_ID_BUFFER_SIZE];
    getStationIdCopy(stationId, sizeof(stationId));

    const int written = snprintf(out, outSize, "%s%s%s", MQTT_TOPIC_PREFIX, stationId, suffix);
    if (written < 0 || static_cast<size_t>(written) >= outSize) {
        out[0] = '\0';
        return 0;
    }
    return static_cast<size_t>(written);
}

bool mqttPublishJson(const char* topic, const char* payload, size_t payloadLength) {
    return publishBuffer(topic, payload, payloadLength);
}

bool mqttPayloadToCString(const byte* payload, unsigned int length, char* output, size_t outputSize) {
    if (output == nullptr || outputSize == 0) {
        return false;
    }
    if (payload == nullptr) {
        output[0] = '\0';
        return false;
    }

    const size_t copyLength = (length < (outputSize - 1)) ? length : (outputSize - 1);
    memcpy(output, payload, copyLength);
    output[copyLength] = '\0';
    return copyLength == length;
}

// ---------------------------------------------------------------------------
// publishTrigger — contracts/mqtt/trigger.schema.json, protokol v2
// Payload: { proto_ver, node_id, phase, obs_seq, attempt_no, pga, dur_ms,
//            onset_ts, detrigger_ts?, ts, signature }
// signature = HMAC-SHA256 hex atas string kanonik v2 (canonical.cpp).
//
// detrigger_ts DIHILANGKAN dari JSON pada PRELIM (kontrak: "harus tidak ada,
// bukan 0") tetapi diserialisasi sebagai 0 di dalam string kanonik, karena string
// kanonik ber-arity tetap: sebuah field yang hilang di sana akan menggeser seluruh
// field sesudahnya dan mengubah arti tanda tangan.
// ---------------------------------------------------------------------------
bool publishTrigger(const TriggerPublish& obs) {
    if (!mqttClient.connected()) {
        return false;
    }

    // ts kanonik: ms epoch UTC. Tanpa NTP sinkron tidak bisa menandatangani
    // sesuai kontrak (server menolak ts menyimpang), jadi batalkan.
    const int64_t tsMs = getEpochMillis();
    if (tsMs <= 0) {
        Serial.println("publishTrigger aborted: NTP not synced (no valid ts)");
        return false;
    }

    // onset_ts adalah field yang DITANDATANGANI dan wajib pada v2. Tanpa onset
    // yang valid observasi ini tidak dapat dikirim sebagai v2 sama sekali, dan
    // mengirimnya sebagai v1 justru akan membuang informasi yang seluruh fase ini
    // ada untuk menyediakannya — jadi batalkan dan biarkan retry mencoba lagi
    // setelah NTP sinkron.
    if (obs.onsetTsMs <= 0 || obs.onsetTsMs > tsMs) {
        Serial.printf("publishTrigger aborted: onset_ts invalid (onset=%lld ts=%lld)\n",
                      static_cast<long long>(obs.onsetTsMs), static_cast<long long>(tsMs));
        return false;
    }

    char nodeId[STATION_ID_BUFFER_SIZE];
    getStationIdCopy(nodeId, sizeof(nodeId));

    // Ambil HMAC secret per-node dari NVS.
    char hmacKey[HMAC_KEY_MAX_LEN];
    const size_t keyLen = getHmacKeyCopy(hmacKey, sizeof(hmacKey));
    if (keyLen == 0) {
        Serial.println("publishTrigger aborted: HMAC key not provisioned");
        return false;
    }

    // String kanonik + signature (byte-identik dgn server Go).
    char canonical[CANONICAL_BUFFER_SIZE];
    const size_t canonLen = buildCanonicalStringV2(canonical, sizeof(canonical),
                                                   PROTO_VER_V2, nodeId, obs.phase,
                                                   obs.obsSeq, obs.attemptNo,
                                                   obs.pgaGal, obs.durMs,
                                                   obs.onsetTsMs, obs.detriggerTsMs, tsMs);
    if (canonLen == 0) {
        Serial.println("publishTrigger aborted: canonical string overflow");
        return false;
    }

    char signature[HMAC_HEX_LENGTH + 1];
    if (!computeHmacHex(reinterpret_cast<const uint8_t*>(hmacKey), keyLen,
                        canonical, canonLen, signature, sizeof(signature))) {
        Serial.println("publishTrigger aborted: HMAC computation failed");
        return false;
    }

    // pga diserialisasi sebagai number dengan 4 desimal fixed agar konsisten
    // byte-per-byte dengan string yang ditandatangani.
    char pgaBuf[16];
    snprintf(pgaBuf, sizeof(pgaBuf), "%.4f", obs.pgaGal);

    StaticJsonDocument<MQTT_TRIGGER_JSON_CAPACITY> doc;
    doc["proto_ver"]  = PROTO_VER_V2;
    doc["node_id"]    = nodeId;
    doc["phase"]      = obs.phase;
    doc["obs_seq"]    = obs.obsSeq;
    doc["attempt_no"] = obs.attemptNo;
    doc["pga"]        = serialized(pgaBuf);
    doc["dur_ms"]     = obs.durMs;
    doc["onset_ts"]   = obs.onsetTsMs;
    if (obs.detriggerTsMs > 0) {
        doc["detrigger_ts"] = obs.detriggerTsMs;
    }
    doc["ts"]         = tsMs;
    doc["signature"]  = signature;

    char jsonBuffer[MQTT_TRIGGER_BUFFER_SIZE];
    size_t jsonLength = 0;
    if (!serializeDocToBuffer(doc, jsonBuffer, sizeof(jsonBuffer), jsonLength)) {
        return false;
    }

    char topic[MQTT_TOPIC_BUFFER_SIZE];
    if (buildTopic(topic, sizeof(topic), MQTT_TOPIC_SUFFIX_TRIGGER) == 0) {
        return false;
    }

    if (mqttPublishJson(topic, jsonBuffer, jsonLength)) {
        Serial.printf("Trigger published (%s obs_seq=%lld attempt=%u pga=%.4f dur=%lu onset=%lld ts=%lld)\n",
                      obs.phase, static_cast<long long>(obs.obsSeq),
                      static_cast<unsigned>(obs.attemptNo), obs.pgaGal,
                      static_cast<unsigned long>(obs.durMs),
                      static_cast<long long>(obs.onsetTsMs),
                      static_cast<long long>(tsMs));
        return true;
    }

    Serial.println("Trigger publish failed!");
    return false;
}

// ---------------------------------------------------------------------------
// sendHeartbeat — contracts/mqtt/heartbeat.schema.json
// Payload: { id, rssi, uptime_s, ts?, clock_source, clock_offset_ms? }
//
// ts DIHILANGKAN, dan bukan dikirim sebagai 0, ketika jam belum tersinkronisasi
// (O4). Node seperti itu tidak dapat menandatangani trigger sama sekali, jadi
// heartbeat adalah satu-satunya cara ia terlihat; ts=0 akan ditolak sebagai ts
// tidak wajar dan node-nya kembali menjadi tidak dapat dibedakan dari node mati.
// Payload ini TIDAK ditandatangani dan karena itu diagnostik saja.
// ---------------------------------------------------------------------------
void sendHeartbeat() {
    if (!mqttClient.connected()) {
        return;
    }

    char nodeId[STATION_ID_BUFFER_SIZE];
    getStationIdCopy(nodeId, sizeof(nodeId));

    const int32_t rssi = static_cast<int32_t>(WiFi.RSSI());
    const uint32_t uptimeSec = millis() / 1000UL;
    const int64_t tsMs = getEpochMillis();

    StaticJsonDocument<MQTT_HEARTBEAT_JSON_CAPACITY> doc;
    doc["id"]       = nodeId;
    doc["rssi"]     = rssi;
    doc["uptime_s"] = uptimeSec;
    if (tsMs > 0) {
        doc["ts"] = tsMs;
    }
    doc["clock_source"] = getClockSource();
    int64_t offsetMs = 0;
    if (getClockOffsetMs(offsetMs)) {
        // Hanya dikirim bila benar-benar terukur: sinkronisasi PERTAMA tidak punya
        // pembanding, dan 0 di sana berarti "tidak ada drift" — sebuah klaim yang
        // tidak pernah diukur.
        doc["clock_offset_ms"] = offsetMs;
    }

    char jsonBuffer[MQTT_HEARTBEAT_BUFFER_SIZE];
    size_t jsonLength = 0;
    if (!serializeDocToBuffer(doc, jsonBuffer, sizeof(jsonBuffer), jsonLength)) {
        return;
    }

    char topic[MQTT_TOPIC_BUFFER_SIZE];
    if (buildTopic(topic, sizeof(topic), MQTT_TOPIC_SUFFIX_HEARTBEAT) == 0) {
        return;
    }

    mqttPublishJson(topic, jsonBuffer, jsonLength);
}

void sendMqttStartupMessage() {
    if (!mqttClient.connected()) {
        return;
    }

    char nodeId[STATION_ID_BUFFER_SIZE];
    char lokasi[LOCATION_TEXT_BUFFER_SIZE];
    getStationIdCopy(nodeId, sizeof(nodeId));
    getLokasiAlatCopy(lokasi, sizeof(lokasi));

    StaticJsonDocument<MQTT_ALERT_JSON_CAPACITY> doc;
    doc["event"]     = "startup";
    doc["stationId"] = nodeId;
    doc["lokasi"]    = lokasi;
    doc["version"]   = FIRMWARE_VERSION;
    doc["restarts"]  = bootCount;

    char jsonBuffer[MQTT_ALERT_BUFFER_SIZE];
    size_t jsonLength = 0;
    if (!serializeDocToBuffer(doc, jsonBuffer, sizeof(jsonBuffer), jsonLength)) {
        return;
    }

    char topic[MQTT_TOPIC_BUFFER_SIZE];
    if (buildTopic(topic, sizeof(topic), MQTT_TOPIC_SUFFIX_STATUS) == 0) {
        return;
    }
    mqttPublishJson(topic, jsonBuffer, jsonLength);
}

void mqttCallback(char* topic, byte* payload, unsigned int length) {
    // Kanal command operasional: sensor/<id>/command.
    char commandTopic[MQTT_TOPIC_BUFFER_SIZE];
    if (buildTopic(commandTopic, sizeof(commandTopic), MQTT_TOPIC_SUFFIX_COMMAND) == 0) {
        return;
    }
    if (topic == nullptr || strcmp(topic, commandTopic) != 0) {
        return;
    }

    char message[64];
    mqttPayloadToCString(payload, length, message, sizeof(message));

    char statusTopic[MQTT_TOPIC_BUFFER_SIZE];
    buildTopic(statusTopic, sizeof(statusTopic), MQTT_TOPIC_SUFFIX_STATUS);

    if (strcmp(message, "ping") == 0) {
        char nodeId[STATION_ID_BUFFER_SIZE];
        char lokasi[LOCATION_TEXT_BUFFER_SIZE];
        char uptime[32];
        char currentTime[TIME_TEXT_BUFFER_SIZE];
        char eventTime[TIME_TEXT_BUFFER_SIZE];
        char wifiStrength[24];
        char chipTemp[16];

        getStationIdCopy(nodeId, sizeof(nodeId));
        getLokasiAlatCopy(lokasi, sizeof(lokasi));
        getUptimeString(uptime, sizeof(uptime));
        getWaktuString(currentTime, sizeof(currentTime));
        getLastEventTimeCopy(eventTime, sizeof(eventTime));

        const long rssi = WiFi.RSSI();
        snprintf(
            wifiStrength,
            sizeof(wifiStrength),
            "%s",
            (rssi > -67) ? "Bagus" : (rssi > -80) ? "Cukup" : "Lemah"
        );

        float tempC = 0.0f;
        bool sensorConnected = false;
        if (xSemaphoreTake(i2cMutex, pdMS_TO_TICKS(100)) == pdTRUE) {
            tempC = (mpu.getTemperature() / 340.0f) + 36.53f;
            sensorConnected = mpu.testConnection();
            xSemaphoreGive(i2cMutex);
        }
        snprintf(chipTemp, sizeof(chipTemp), "%.1f", tempC);

        StaticJsonDocument<MQTT_STATUS_JSON_CAPACITY> doc;
        doc["stationId"] = nodeId;
        doc["lokasi"] = lokasi;
        doc["uptime"] = uptime;
        doc["heap"] = ESP.getFreeHeap();
        doc["currentTime"] = currentTime;
        doc["ntpStatus"] = isNtpSynced ? "Tersinkronisasi" : "Gagal";
        doc["wifiRssi"] = rssi;
        doc["wifiStrength"] = wifiStrength;
        doc["sensorStatus"] = sensorConnected ? "Terhubung" : "Gagal";
        doc["dmpStatus"] = DMPReady ? "Siap" : "Gagal";
        doc["chipTemp"] = chipTemp;
        doc["lastEventTime"] = eventTime;
        doc["lastPga"] = lastPgaStr;
        doc["mpuOverflows"] = mpuOverflowCount;
        doc["restarts"] = bootCount;

        char output[MQTT_STATUS_BUFFER_SIZE];
        size_t outLen = 0;
        if (serializeDocToBuffer(doc, output, sizeof(output), outLen)) {
            mqttPublishJson(statusTopic, output, outLen);
        }

    } else if (strcmp(message, "reboot") == 0) {
        rebootRequestReceived = true;

    } else if (strcmp(message, "stats") == 0) {
        char nodeId[STATION_ID_BUFFER_SIZE];
        getStationIdCopy(nodeId, sizeof(nodeId));

        StaticJsonDocument<MQTT_STATUS_JSON_CAPACITY> doc;
        doc["stationId"] = nodeId;
        doc["firmware"] = FIRMWARE_VERSION;
        doc["mpuErrors"] = mpuErrorCounter;
        doc["mpuOverflows"] = mpuOverflowCount;
        doc["totalEvents"] = totalEventsDetected;
        doc["minHeapEver"] = minHeapSeen;
        doc["maxFragmentation"] = maxHeapFragmentationSeen;
        doc["currentHeap"] = ESP.getFreeHeap();
        doc["ntpSynced"] = isNtpSynced;
        doc["restarts"] = bootCount;

        char output[MQTT_STATUS_BUFFER_SIZE];
        size_t outLen = 0;
        if (serializeDocToBuffer(doc, output, sizeof(output), outLen)) {
            mqttPublishJson(statusTopic, output, outLen);
        }
    }
}

void checkMqttConnection() {
    if (WiFi.status() != WL_CONNECTED) {
        return;
    }
    if (mqttClient.connected()) {
        return;
    }

    const unsigned long now = millis();
    if (now - lastMqttAttempt <= MQTT_RECONNECT_INTERVAL_MS) {
        return;
    }
    lastMqttAttempt = now;

    // Sertifikat broker diverifikasi terhadap jam dinding, dan ESP32 boot pada
    // 1970. Mencoba handshake sebelum NTP hanya menghasilkan rc=-2 yang terlihat
    // seperti broker tidak dapat dihubungi. Ditahan di sini, bukan di dalam
    // configureMqttTls(), karena syaratnya berubah selama runtime. Jalur
    // plaintext (mqtt_use_tls = false, dev) tidak menilai sertifikat apa pun,
    // jadi menunggu jam hanya menunda tanpa alasan.
    if (mqtt_use_tls && !mqttTlsClockReady()) {
        return;
    }

    char clientId[40];
    snprintf(clientId, sizeof(clientId), "%s%04X", MQTT_CLIENT_ID_PREFIX,
             static_cast<unsigned int>(random(0x10000)));

    // --- Last Will & Testament (sensor/<id>/status, QoS 1) ---
    // Broker mempublikasikan LWT bila sensor putus tak terduga sehingga server
    // menandai node offline.
    char nodeId[STATION_ID_BUFFER_SIZE];
    getStationIdCopy(nodeId, sizeof(nodeId));

    char statusTopic[MQTT_TOPIC_BUFFER_SIZE];
    buildTopic(statusTopic, sizeof(statusTopic), MQTT_TOPIC_SUFFIX_STATUS);

    StaticJsonDocument<256> lwtDoc;
    lwtDoc["id"]     = nodeId;
    lwtDoc["status"] = "offline";
    char lwtBuffer[256];
    const size_t lwtLen = serializeJson(lwtDoc, lwtBuffer, sizeof(lwtBuffer));
    if (lwtLen == 0 || lwtLen >= sizeof(lwtBuffer)) {
        Serial.println("MQTT LWT serialization failed");
        return;
    }

    // PubSubClient tidak memiliki method setWill(); konfigurasi LWT diteruskan
    // langsung sebagai parameter connect():
    // connect(id, user, pass, willTopic, willQos, willRetain, willMessage).
    // willMessage harus null-terminated (serializeJson ke char* sudah menjamin).
    if (mqttClient.connect(clientId, mqtt_user, mqtt_password,
                           statusTopic, /*willQos=*/1, /*willRetain=*/false,
                           lwtBuffer)) {
        char commandTopic[MQTT_TOPIC_BUFFER_SIZE];
        buildTopic(commandTopic, sizeof(commandTopic), MQTT_TOPIC_SUFFIX_COMMAND);
        mqttClient.subscribe(commandTopic);
        Serial.println("MQTT Connected");
        if (!startupMessageSent) {
            sendMqttStartupMessage();
            startupMessageSent = true;
        }
    } else {
        Serial.printf("MQTT failed, rc=%d\n", mqttClient.state());
    }
}
