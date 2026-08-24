/**
 * QuakeAlert ESP32 - MQTT Logic Implementation
 *
 * Contract-first: contracts/mqtt/trigger.schema.json & heartbeat.schema.json.
 * Topik dibangun runtime "sensor/<station_id>/<suffix>". trigger & heartbeat
 * publish QoS 1. trigger ditandatangani HMAC-SHA256 (crypto.cpp) memakai secret
 * per-node dari NVS (utils::getHmacKeyCopy).
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

bool publishBuffer(const char* topic, const char* payload, size_t payloadLength, uint8_t qos) {
    if (topic == nullptr || payload == nullptr || payloadLength == 0) {
        return false;
    }
    if (!mqttClient.connected()) {
        return false;
    }

    // PubSubClient::publish tanpa QoS hanya mendukung QoS 0. Untuk QoS 1 gunakan
    // beginPublish/write/endPublish yang menyetel flag QoS via header.
    if (qos == 0) {
        return mqttClient.publish(topic, reinterpret_cast<const uint8_t*>(payload), payloadLength, false);
    }

    // QoS 1 path: PubSubClient tidak menyimpan state PUBACK, namun broker tetap
    // memproses DUP/PUBACK. beginPublish menandai QoS via retained/qos bit.
    if (!mqttClient.beginPublish(topic, payloadLength, false)) {
        return false;
    }
    const size_t written = mqttClient.write(reinterpret_cast<const uint8_t*>(payload), payloadLength);
    if (!mqttClient.endPublish()) {
        return false;
    }
    return written == payloadLength;
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

bool mqttPublishJson(const char* topic, const char* payload, size_t payloadLength, uint8_t qos) {
    return publishBuffer(topic, payload, payloadLength, qos);
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
// publishTrigger — contracts/mqtt/trigger.schema.json (QoS 1)
// Payload: { node_id, pga, dur_ms, ts, signature }
// signature = HMAC-SHA256 hex atas "node_id|pga|dur_ms|ts" (pga 4 desimal).
// ---------------------------------------------------------------------------
bool publishTrigger(float pgaGal, uint32_t durMs) {
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
    const size_t canonLen = buildCanonicalString(canonical, sizeof(canonical), nodeId, pgaGal, durMs, tsMs);
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
    snprintf(pgaBuf, sizeof(pgaBuf), "%.4f", pgaGal);

    StaticJsonDocument<MQTT_TRIGGER_JSON_CAPACITY> doc;
    doc["node_id"]   = nodeId;
    doc["pga"]       = serialized(pgaBuf);
    doc["dur_ms"]    = durMs;
    doc["ts"]        = tsMs;
    doc["signature"] = signature;

    char jsonBuffer[MQTT_TRIGGER_BUFFER_SIZE];
    size_t jsonLength = 0;
    if (!serializeDocToBuffer(doc, jsonBuffer, sizeof(jsonBuffer), jsonLength)) {
        return false;
    }

    char topic[MQTT_TOPIC_BUFFER_SIZE];
    if (buildTopic(topic, sizeof(topic), MQTT_TOPIC_SUFFIX_TRIGGER) == 0) {
        return false;
    }

    if (mqttPublishJson(topic, jsonBuffer, jsonLength, MQTT_TRIGGER_QOS)) {
        Serial.printf("Trigger published (pga=%.4f dur=%lu ts=%lld)\n",
                      pgaGal, static_cast<unsigned long>(durMs),
                      static_cast<long long>(tsMs));
        return true;
    }

    Serial.println("Trigger publish failed!");
    return false;
}

// ---------------------------------------------------------------------------
// sendHeartbeat — contracts/mqtt/heartbeat.schema.json (QoS 1)
// Payload: { id, rssi, uptime_s, ts }
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
    doc["ts"]       = tsMs;

    char jsonBuffer[MQTT_HEARTBEAT_BUFFER_SIZE];
    size_t jsonLength = 0;
    if (!serializeDocToBuffer(doc, jsonBuffer, sizeof(jsonBuffer), jsonLength)) {
        return;
    }

    char topic[MQTT_TOPIC_BUFFER_SIZE];
    if (buildTopic(topic, sizeof(topic), MQTT_TOPIC_SUFFIX_HEARTBEAT) == 0) {
        return;
    }

    mqttPublishJson(topic, jsonBuffer, jsonLength, MQTT_HEARTBEAT_QOS);
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
    mqttPublishJson(topic, jsonBuffer, jsonLength, 0);
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
        char intensity[INTENSITY_TEXT_BUFFER_SIZE];
        char wifiStrength[24];
        char chipTemp[16];

        getStationIdCopy(nodeId, sizeof(nodeId));
        getLokasiAlatCopy(lokasi, sizeof(lokasi));
        getUptimeString(uptime, sizeof(uptime));
        getWaktuString(currentTime, sizeof(currentTime));
        getLastEventTimeCopy(eventTime, sizeof(eventTime));
        getLastIntensityCopy(intensity, sizeof(intensity));

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
        doc["lastIntensity"] = intensity;
        doc["lastPga"] = lastPgaStr;
        doc["mpuOverflows"] = mpuOverflowCount;
        doc["restarts"] = bootCount;

        char output[MQTT_STATUS_BUFFER_SIZE];
        size_t outLen = 0;
        if (serializeDocToBuffer(doc, output, sizeof(output), outLen)) {
            mqttPublishJson(statusTopic, output, outLen, 0);
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
            mqttPublishJson(statusTopic, output, outLen, 0);
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
