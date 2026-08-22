/**
 * QuakeAlert ESP32 - Konfigurasi TLS untuk koneksi MQTT.
 *
 * Dipisahkan dari mqtt.cpp karena satu-satunya tempat yang boleh menyertakan
 * mqtt_ca.h adalah implementasi di sini: header itu mendefinisikan bundel PEM
 * sebagai array static, jadi menyertakannya dari dua unit kompilasi akan
 * menggandakan ~2,7 KB flash tanpa alasan.
 */

#ifndef QUAKEALERT_MQTT_TLS_H
#define QUAKEALERT_MQTT_TLS_H

/**
 * Memasang trust anchor pada espClient. Dipanggil sekali dari setup(), SEBELUM
 * percobaan koneksi pertama — setCACert pada klien yang sudah terhubung tidak
 * berlaku untuk sesi yang sedang berjalan.
 */
void configureMqttTls();

/**
 * true bila jam dinding sudah cukup masuk akal untuk memverifikasi masa berlaku
 * sertifikat.
 *
 * ESP32 boot dengan waktu 1 Januari 1970. Terhadap jam itu setiap sertifikat
 * yang sah terlihat "belum berlaku", jadi handshake gagal dengan alasan yang
 * tidak ada hubungannya dengan penyerang. checkMqttConnection() menahan diri
 * sampai fungsi ini true, alih-alih menghabiskan siklus reconnect pada
 * kegagalan yang sudah pasti.
 */
bool mqttTlsClockReady();

#endif  // QUAKEALERT_MQTT_TLS_H
