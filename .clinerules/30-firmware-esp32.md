# 30 — Firmware (ESP32) Rules

Firmware ESP32 + MPU6050 di `firmware/` (PlatformIO, C++). Lihat SYSTEM_SPEC Bab 6.C & ADR-0003.

## Wajib

1. **Zero-block:** Dilarang `delay()` di loop utama. Gunakan state machine berbasis `millis()`.
2. **Persistent config:** Simpan Wi-Fi credentials & `station_id` via `Preferences.h` (NVS), bukan EEPROM usang.
3. **Provisioning SoftAP:** Reset NVS via tombol BOOT 5 detik → SoftAP `QuakeNode-Setup` → terima config via HTTP `POST /setup`.
4. **Debounce:** Debounce getaran minimal 60 detik setelah trigger pertama terkirim.
5. **HMAC kanonik:** String yang ditandatangani identik byte-per-byte dengan `/contracts/mqtt` — urutan `node_id|pga|dur_ms|ts`, pemisah `|`, `pga` fixed 4 desimal, `ts` ms epoch. Output 64-hex.
6. **TLS:** MQTTS 8883 dengan validasi CA certificate. Plaintext 1883 dilarang.
7. **Satuan:** PGA dalam gal (`cm/s²`), `ts` ms epoch UTC, `dur_ms` ms, RSSI dBm.

## Struktur disarankan

```
firmware/
  platformio.ini
  src/main.cpp
  src/config.h        # topik, interval heartbeat, threshold
  src/hmac.cpp        # kanonikalisasi + HMAC-SHA256
  lib/
```
