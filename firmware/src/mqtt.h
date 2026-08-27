/**
 * QuakeAlert ESP32 - MQTT Logic
 *
 * Contract-first (contracts/mqtt/*.schema.json):
 *   - Topik dibangun runtime dari StationID: "sensor/<station_id>/<suffix>".
 *   - trigger  : event-driven, QoS 0, payload v2 {proto_ver,node_id,phase,
 *                obs_seq,attempt_no,pga,dur_ms,onset_ts,detrigger_ts,ts,
 *                signature} dgn HMAC-SHA256 atas string kanonik v2.
 *   - heartbeat: periodik 60s, QoS 0, payload {id,rssi,uptime_s,ts,clock_source,
 *                clock_offset_ms} — TIDAK ditandatangani, diagnostik saja.
 *   - status/command: kanal operasional (diagnostik & remote control).
 */

#ifndef MQTT_H
#define MQTT_H

#include <Arduino.h>
#include <stddef.h>

#include "canonical.h"

// Satu observasi yang akan dipublish. Struct, bukan delapan parameter posisional:
// tiga di antaranya adalah int64 timestamp yang saling dapat ditukar tanpa satu pun
// peringatan kompilator, dan salah satunya akan diam-diam menandatangani angka yang
// salah.
struct TriggerPublish {
    const char* phase;      // PHASE_PRELIM atau PHASE_FINAL (canonical.h)
    int64_t obsSeq;         // (boot_count << 16) | in_boot_seq
    uint8_t attemptNo;      // 1 = publikasi pertama; ikut ditandatangani
    float pgaGal;           // gal (cm/s^2)
    uint32_t durMs;
    int64_t onsetTsMs;      // onset menurut jam sensor, ms epoch UTC
    int64_t detriggerTsMs;  // 0 = tidak ada (WAJIB 0 pada PRELIM)
};

// Publikasikan trigger v2 sesuai contracts/mqtt/trigger.schema.json. ts (waktu
// publish) diambil dari getEpochMillis() di dalam fungsi ini dan distempel ULANG
// pada setiap percobaan. Menandatangani payload dgn HMAC key dari NVS.
// Mengembalikan true bila publish sukses.
bool publishTrigger(const TriggerPublish& obs);

// Heartbeat periodik sesuai contracts/mqtt/heartbeat.schema.json.
void sendHeartbeat();

void sendMqttStartupMessage();
void mqttCallback(char* topic, byte* payload, unsigned int length);
void checkMqttConnection();

bool mqttPublishJson(const char* topic, const char* payload, size_t payloadLength);
bool mqttPayloadToCString(const byte* payload, unsigned int length, char* output, size_t outputSize);

// Bangun topik penuh "sensor/<station_id><suffix>" ke buffer. Mengembalikan
// panjang (>0) atau 0 bila buffer terlalu kecil.
size_t buildTopic(char* out, size_t outSize, const char* suffix);

#endif  // MQTT_H
