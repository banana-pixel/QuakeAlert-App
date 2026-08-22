# Firmware Audit — ESP32 vs Kontrak QuakeAlert

Tanggal: 2026-08-18. Basis: `contracts/mqtt/*.schema.json`, ADR-0003, `.clinerules/30-firmware-esp32.md`.
Status: **AUDIT (belum ada perubahan kode firmware).** Dokumen ini adalah daftar temuan + rencana perbaikan berurutan. Prinsip contract-first (ADR-0004): kontrak menang, firmware harus menyesuaikan.

## Ringkasan

Firmware saat ini fungsional dan hemat memori (StaticJsonDocument, zero-heap di hot path, STA/LTA adaptif, LWT, mutex I2C). Namun **payload dan topik menyimpang dari kontrak**, dan **tidak ada HMAC** — pelanggaran life-safety integrity (Aturan Emas #3, ADR-0003).

Severity: 🔴 blocker (integritas/keamanan/kontrak), 🟡 penting, 🟢 minor.

---

## Temuan

### 🔴 F-1 — Tidak ada HMAC-SHA256 pada trigger
- **Sekarang:** `sendMqttAlert()` / `sendMqttReport()` publish JSON tanpa signature.
- **Kontrak:** `trigger.schema.json` mewajibkan `signature` = HMAC-SHA256 hex 64-char atas string kanonik `node_id|pga|dur_ms|ts` (pga 4 desimal fixed). `.clinerules/30` #5.
- **Dampak:** Server tidak bisa memverifikasi keaslian trigger → rentan spoofing → false positive alarm (life-safety).
- **Aksi:** Tambah `src/hmac.cpp` (kanonikalisasi byte-identik + HMAC via mbedTLS `mbedtls_md_hmac`). Secret dari `Preferences`/NVS (bukan hardcode).

### 🔴 F-2 — Topik & skema payload tidak sesuai kontrak
- **Sekarang:** `seismo/alert`, `seismo/report`, `seismo/heartbeat`, `seismo/status`, `seismo/command`. Field alert: `stationId, lokasi, lat, lon, waktu, intensitas, pga`.
- **Kontrak:** trigger → `sensor/<station_id>/trigger` dengan `{node_id, pga, dur_ms, ts, signature}`; heartbeat → `sensor/<station_id>/heartbeat` dengan `{id, rssi, uptime_s, ts}`.
- **Dampak:** Server (subscriber ingest) tidak akan mengenali pesan. Integrasi gagal total.
- **Aksi:** Ganti konstanta topik di `config.h` ke pola `sensor/<station_id>/...`. Selaraskan field payload. Keputusan: `alert` vs `report` dilebur menjadi satu event `trigger` sesuai kontrak (lihat F-6).

### 🔴 F-3 — Timestamp `ts` (ms epoch UTC) tidak dikirim
- **Sekarang:** payload memakai `waktu` string terformat (human-readable), bukan `ts` int64 ms epoch.
- **Kontrak:** `ts` = int64 ms epoch UTC, dipakai server untuk anti-replay (`ts <= last_seen_ts` ditolak) & toleransi drift 30s.
- **Dampak:** Anti-replay server mustahil; string waktu tidak deterministik untuk HMAC.
- **Aksi:** Kirim `ts` = `epochMillis` dari NTP-synced clock. Format HMAC pakai `ts` mentah (ms).

### 🟡 F-4 — Satuan `pga` sudah gal (benar), tapi presisi tidak dikunci
- **Sekarang:** alert `"%.2f"`, heartbeat/report `"%.4f"` — campur.
- **Kontrak:** untuk string kanonik HMAC, `pga` **fixed 4 desimal**. Nilai numerik payload tetap gal.
- **Aksi:** Standarkan `snprintf(pga, "%.4f", pga_gal)` untuk yang masuk HMAC. Satuan gal sudah benar (`DATA_RATIO` = 980.665/8192).

### 🟡 F-5 — Heartbeat field tidak match & QoS salah
- **Sekarang:** heartbeat QoS 0, field `{id, version, lat, lon, lokasi, pga, rssi, uptime}`.
- **Kontrak:** `{id, rssi, uptime_s, ts}`, dan `.clinerules/00` #3 menyiratkan QoS 1 untuk keandalan life-safety (trigger wajib QoS 1; heartbeat minimal konsisten dgn spec — verifikasi di SYSTEM_SPEC Bab 6.C).
- **Aksi:** Rename `uptime`→`uptime_s`, tambah `ts`, buang field non-kontrak (atau pindah ke pesan status terpisah). Konfirmasi QoS heartbeat vs spec sebelum ubah.

### 🟡 F-6 — Dua pesan (alert QoS?/report) vs satu `trigger`
- **Sekarang:** `sendMqttAlert` (ringkas) + `sendMqttReport` (lengkap) dua publish terpisah.
- **Kontrak:** satu event `trigger` (QoS 1). `.clinerules/30` #4: debounce ≥60s setelah trigger pertama (sudah ada `EVENT_COOLDOWN_PERIOD_MS=60000` ✅).
- **Aksi:** Konsolidasi menjadi satu `publishTrigger()` QoS 1 sesuai skema. `dur_ms` = durasi event STA/LTA (sudah dihitung, cap `MAX_EVENT_DURATION_MS`).

### ✅ F-7 — TLS/MQTTS 8883 perlu verifikasi CA — SELESAI
- **Sekarang:** `configureMqttTls()` (`src/mqtt_tls.cpp`) mem-pin akar ISRG X1 + X2 dari `src/mqtt_ca.h` lewat `espClient.setCACert()`. `setInsecure()` tidak lagi ada di jalur default; ia hanya terkompilasi bila `secrets.h` mendefinisikan `SECRET_MQTT_ALLOW_INSECURE_TLS`, dan build itu mengumumkan dirinya di setiap boot.
- **Kontrak:** ADR-0003 & `.clinerules/30` #6: MQTTS 8883 dengan validasi CA; plaintext 1883 dilarang di produksi.
- **Catatan:** yang dipin adalah AKAR, bukan leaf. Sertifikat broker diterbitkan ACME lewat Caddy dan diperbarui tiap ~60 hari (`deploy/scripts/sync-mqtt-certs.sh`), jadi mem-pin leaf berarti me-reflash fleet setiap perpanjangan. Kedua akar disertakan karena rantai RSA berujung di X1 sementara rantai ECDSA dapat berujung di X2.
- **Konsekuensi runtime:** verifikasi masa berlaku butuh jam dinding, dan ESP32 boot pada 1970. `checkMqttConnection()` menahan koneksi sampai `mqttTlsClockReady()` true, sehingga node tidak membakar siklus reconnect pada handshake yang pasti gagal sebelum NTP.
- **Broker dengan CA sendiri:** `SECRET_MQTT_CA_CERT` di `secrets.h` menggantikan akar publik tanpa mengubah kode.

### 🟡 F-8 — `node_id` format
- **Sekarang:** `STATION_ID_BUFFER_SIZE 16`, prefix client `ESP32-Seismo-`.
- **Kontrak:** `node_id` pola `^NODE-[0-9A-F]{8}$` (13 char). Buffer 16 cukup, tapi format generator harus `NODE-XXXXXXXX` (mis. dari `chipId`/MAC).
- **Aksi:** Pastikan `getStationIdCopy()` menghasilkan format `NODE-<8 hex uppercase>`.

### 🟢 F-9 — Provisioning SoftAP & NVS
- **`.clinerules/30` #2/#3:** Wi-Fi creds & `station_id` via `Preferences.h` (NVS); reset via BOOT 5s → SoftAP `QuakeNode-Setup` → `POST /setup`.
- **Aksi:** Verifikasi keberadaan di `network.cpp`; jika hardcode via `secrets.h`, jadikan fallback saja.

### 🟢 F-10 — Zero-block loop
- **`.clinerules/30` #1:** dilarang `delay()` di loop utama. Firmware sudah pakai FreeRTOS task + `millis()` interval ✅. Pertahankan.

---

## Dependency (platformio.ini)

Saat ini ter-pin dengan baik (bagus, sesuai Aturan Emas #6):
- `knolleary/PubSubClient@^2.8`
- `bblanchon/ArduinoJson@^6.21.5` (v6 dipertahankan agar `StaticJsonDocument` stack-allocated)
- `electroniccats/MPU6050@^1.4.3`

Untuk HMAC (F-1): **tidak perlu lib eksternal** — pakai `mbedtls/md.h` yang sudah tersedia di framework ESP32 Arduino. Hindari menambah dependency baru.

---

## Urutan Perbaikan yang Disarankan (kontrak dulu, baru kode)

1. F-1 + F-3: buat `src/hmac.cpp/.h`, tambah `ts` ms epoch, string kanonik `node_id|pga|dur_ms|ts`.
2. F-2 + F-6: ubah topik ke `sensor/<station_id>/trigger`, konsolidasi ke satu `publishTrigger()` QoS 1.
3. F-4 + F-8: kunci presisi `pga` 4 desimal, format `node_id` `NODE-XXXXXXXX`.
4. F-5: selaraskan heartbeat ke `{id, rssi, uptime_s, ts}`.
5. ~~F-7~~ + F-9: TLS CA sudah ter-pin (lihat F-7 di atas); sisa: provisioning NVS/SoftAP.

Semua perubahan firmware harus disertai uji vektor HMAC yang **identik byte** dengan implementasi server Go (uji silang) sebelum dianggap selesai.
