# GAP ANALYSIS — QuakeAlert Ecosystem

Dokumen ini melacak selisih antara **arsitektur target** ([`SYSTEM_SPEC.md`](./SYSTEM_SPEC.md)) dan **kondisi kode aktual** di repo. Diperbarui setiap milestone.

**Tanggal audit:** 2026-08-18
**Commit basis:** `ac26212`

---

## 1. Ringkasan Status per Komponen

| Komponen | Target | Status Aktual | Kematangan |
|---|---|---|---|
| Android UI (Compose) | Full flow (onboarding, warning, sensors, history, chat, settings) | Ada, UI-only dengan mock/StateFlow lokal | 🟡 ~60% (UI selesai, tanpa backend nyata) |
| Android — FCM / EmergencyService | Data-only handler + full-screen intent + siren | Belum ada `FirebaseMessagingService`, belum ada `WarningActivity` full-screen | 🔴 Belum ada |
| Android — WebSocket client | WSS listener foreground | Belum ada | 🔴 Belum ada |
| Android — Haversine & coverage | Native distance gating sebelum alarm | Belum ada | 🔴 Belum ada |
| Server (Go) | Monolith: MQTT ingest, konsensus, FCM, WS, REST | Belum ada direktori `server/` | 🔴 Belum ada |
| Firmware (ESP32) | PlatformIO, HMAC, MQTTS, NVS provisioning | Belum ada direktori `firmware/` | 🔴 Belum ada |
| Contracts | OpenAPI + JSON Schema MQTT/FCM + DDL migrasi | Belum ada direktori `contracts/` | 🔴 Belum ada |
| Deploy | docker-compose, Mosquitto, Postgres+PostGIS, Redis | Belum ada direktori `deploy/` | 🔴 Belum ada |
| Docs & ADR | Spec + ADR + gap analysis | Sedang dibangun (fase ini) | 🟡 In-progress |

---

## 2. Isu Teknis pada Draf Spec Awal (Sudah Diperbaiki)

Selama audit spec awal ditemukan 12 kontradiksi/kesalahan yang berpotensi jadi bug lintas komponen. Semua telah dikoreksi di `SYSTEM_SPEC.md`:

| # | Isu | Draf Awal | Perbaikan | Rasional |
|---|---|---|---|---|
| 1 | Penyimpanan secret HMAC | `secret_key_hash` (hash) | `secret_key_enc` + `secret_key_nonce` (AES-GCM) | Verifikasi HMAC butuh key mentah; hash tak bisa di-reverse. |
| 2 | Nama lokasi event | `epicenter` | `estimated_centroid` | Weighted centroid ≠ episenter seismologis. |
| 3 | Satuan PGA | ambigu (g vs gal) | gal (`cm/s²`) kanonik + tabel konversi | Cegah bug lintas komponen. |
| 4 | Tipe `max_pga` | terlalu kecil | `NUMERIC(8,4)` | Menampung gempa kuat > 100 gal. |
| 5 | Window konsensus | 2.5 detik | 8 detik (`CONSENSUS_WINDOW_MS`) | Radius 50km / Vs ~3.5km/s butuh ~14s; 2.5s melewatkan node. |
| 6 | Default coverage radius | 500 km | 50 km | 500km memicu false-alarm massal. |
| 7 | Signature length | contoh 32-hex (MD5) | 64-hex (SHA-256) | Konsisten dengan HMAC-SHA256. |
| 8 | Timestamp | detik vs ms ambigu | ms epoch UTC (`int64`) kanonik | Anti-replay & konsistensi. |
| 9 | Transport | 1883 plaintext disebut | MQTTS 8883 + HTTPS + WSS wajib | Life-safety, cegah spoof. |
| 10 | TTL chat | "auto-expire" tak jelas | pg_cron / partisi / goroutine eksplisit | Postgres tak punya TTL native. |
| 11 | Istilah "mesh" chat | `mesh_chat_messages` | `chat_messages` | Arsitektur server-backed, bukan mesh P2P. |
| 12 | Full-screen intent | tak menyebut Android 14+ | `USE_FULL_SCREEN_INTENT` + `canUseFullScreenIntent()` | Batasan API 34. |

---

## 3. Prioritas Backlog (Urutan Rekomendasi)

1. **Contracts-first** — tulis OpenAPI, JSON Schema MQTT/FCM, DDL migrasi (sumber kebenaran).
2. **Deploy skeleton** — docker-compose (Postgres+PostGIS, Redis, Mosquitto) untuk dev lokal.
3. **Server monolith** — ingest MQTT → verifikasi HMAC → konsensus → FCM/WS.
4. **Firmware** — trigger + HMAC + MQTTS + provisioning SoftAP.
5. **Android wiring** — ganti mock dengan REST/WS nyata, tambah `EmergencyMessagingService` + `WarningActivity` + Haversine gating.

---

## 4. Risiko Terbuka

- **Konsensus window 8s belum tervalidasi lapangan** — perlu kalibrasi dengan data sensor nyata.
- **Weighted centroid bukan episenter** — perlu disclaimer jelas di UI agar tidak menyesatkan.
- **Box 1GB RAM** — Postgres+PostGIS+Redis+Mosquitto+Go bersaing memori; perlu tuning `shared_buffers`, `maxmemory` Redis, dan footprint Go (lihat ADR-0001).
- **Full-screen intent Android 14+** — bergantung izin user; perlu fallback heads-up notification.
