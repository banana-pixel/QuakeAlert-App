/**
 * QuakeAlert ESP32 - MQTT Logic
 *
 * Contract-first (contracts/mqtt/*.schema.json):
 *   - Topik dibangun runtime dari StationID: "sensor/<station_id>/<suffix>".
 *   - trigger  : event-driven, QoS 1, payload {node_id,pga,dur_ms,ts,signature}
 *                dgn HMAC-SHA256 kanonik "node_id|pga|dur_ms|ts".
 *   - heartbeat: periodik 60s, QoS 1, payload {id,rssi,uptime_s,ts}.
 *   - status/command: kanal operasional (diagnostik & remote control).
 */

#ifndef MQTT_H
#define MQTT_H

#include <Arduino.h>
#include <stddef.h>

// Publikasikan trigger seismik sesuai contracts/mqtt/trigger.schema.json.
// pgaGal dalam gal (cm/s^2), durMs durasi getaran (ms). ts diambil dari
// getEpochMillis(). Menandatangani payload dgn HMAC key dari NVS. QoS 1.
// Mengembalikan true bila publish sukses.
bool publishTrigger(float pgaGal, uint32_t durMs);

// Heartbeat periodik sesuai contracts/mqtt/heartbeat.schema.json (QoS 1).
void sendHeartbeat();

void sendMqttStartupMessage();
void mqttCallback(char* topic, byte* payload, unsigned int length);
void checkMqttConnection();

bool mqttPublishJson(const char* topic, const char* payload, size_t payloadLength, uint8_t qos = 0);
bool mqttPayloadToCString(const byte* payload, unsigned int length, char* output, size_t outputSize);

// Bangun topik penuh "sensor/<station_id><suffix>" ke buffer. Mengembalikan
// panjang (>0) atau 0 bila buffer terlalu kecil.
size_t buildTopic(char* out, size_t outSize, const char* suffix);

#endif  // MQTT_H
