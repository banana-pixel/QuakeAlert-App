#include "mqtt_tls.h"

#include "secrets.h"
#include "state.h"
#include "mqtt_ca.h"

#include <Arduino.h>
#include <time.h>

namespace {

// 1 Januari 2025 UTC. Ambang "jam sudah masuk akal", bukan ambang keamanan:
// yang dijaga adalah agar verifikasi masa berlaku tidak dijalankan terhadap
// jam 1970. Dipilih di masa lalu supaya node dengan NTP yang melenceng
// beberapa jam tetap dapat terhubung — mbedTLS yang menilai masa berlaku
// sebenarnya, bukan konstanta ini.
constexpr time_t MQTT_TLS_MIN_PLAUSIBLE_EPOCH = 1735689600;

// Pesan "menunggu jam" dibatasi lajunya: checkMqttConnection() dipanggil dari
// loop(), dan tanpa ini serial dipenuhi satu baris identik per iterasi.
constexpr unsigned long CLOCK_NOTICE_INTERVAL_MS = 10000;

}  // namespace

void configureMqttTls() {
#if defined(SECRET_MQTT_ALLOW_INSECURE_TLS) && SECRET_MQTT_ALLOW_INSECURE_TLS
    // Jalur dev melawan broker lokal yang sertifikatnya tidak dikenal siapa pun.
    // Sengaja berisik: build yang lolos ke lapangan dengan baris ini akan
    // mengatakannya di setiap boot.
    espClient.setInsecure();
    Serial.println("MQTT TLS: PERINGATAN - validasi sertifikat DIMATIKAN "
                   "(SECRET_MQTT_ALLOW_INSECURE_TLS). Jangan pakai di lapangan.");
#else
#if defined(SECRET_MQTT_CA_CERT)
    // Broker dengan CA sendiri: PEM dari secrets.h menggantikan akar publik.
    espClient.setCACert(SECRET_MQTT_CA_CERT);
    Serial.println("MQTT TLS: memakai CA dari secrets.h");
#else
    espClient.setCACert(MQTT_ROOT_CA_BUNDLE);
    Serial.println("MQTT TLS: memakai akar ISRG (Let's Encrypt), verifikasi aktif");
#endif
#endif
}

bool mqttTlsClockReady() {
#if defined(SECRET_MQTT_ALLOW_INSECURE_TLS) && SECRET_MQTT_ALLOW_INSECURE_TLS
    // Tanpa verifikasi tidak ada masa berlaku yang dinilai, jadi menahan
    // koneksi hanya akan menunda node tanpa menambah keamanan apa pun.
    return true;
#else
    // time(nullptr), bukan isNtpSynced: flag itu kembali false setiap
    // NTP_SYNC_INTERVAL_MS untuk memicu sinkronisasi berikutnya, sementara jam
    // yang sudah pernah di-set tetap sah. Menahan MQTT pada flag itu akan
    // memutus node yang sehat setiap jam.
    const time_t nowEpoch = time(nullptr);
    if (nowEpoch >= MQTT_TLS_MIN_PLAUSIBLE_EPOCH) {
        return true;
    }

    static unsigned long lastNotice = 0;
    const unsigned long nowMs = millis();
    if (lastNotice == 0 || nowMs - lastNotice >= CLOCK_NOTICE_INTERVAL_MS) {
        lastNotice = nowMs;
        Serial.println("MQTT TLS: menunggu NTP - sertifikat tidak dapat "
                       "diverifikasi terhadap jam boot 1970");
    }
    return false;
#endif
}
