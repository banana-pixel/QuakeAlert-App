# CLIENT SPEC — Integrasi Android Client

Panduan integrasi resmi untuk aplikasi Android (Kotlin, `id.web.quakealert`) terhadap backend QuakeAlert. Sumber kebenaran teknis: `contracts/openapi/openapi.yaml` (REST), `contracts/mqtt/*.schema.json` (MQTT), `contracts/fcm/alert_payload.json` (FCM). Dokumen ini merangkum alur wajib yang dipakai aplikasi.

**Status:** mengikuti implementasi server saat ini (commit `d7b06bb`). Diperbarui bersama kontrak.

---

## 1. Ringkasan & Base URL

| Lingkungan | Base URL | Catatan |
|---|---|---|
| Produksi | `https://api.quakealert.id` | HTTPS wajib (ADR-0003). |
| Dev lokal | `http://localhost:8080` | Stack: `server/docker-compose.yml`. |

Transport **wajib HTTPS** — Android dilarang `usesCleartextTraffic=true`. Realtime memakai WSS; push background memakai FCM.

Ada **tiga tingkat akses**:
- **Publik** — `POST /api/v1/auth/anonymous` (bootstrap identitas, tanpa token).
- **Auth opsional** — `GET /api/v1/events`: boleh tanpa token; bila header `Authorization` dikirim, token **wajib valid** (rusak/kedaluwarsa → `401`, tidak dianggap anonim).
- **Auth wajib** — `PUT /api/v1/users/location`, `PUT /api/v1/users/fcm-token`, `GET /api/v1/sensors`, `POST /api/v1/users/pseudonym/reroll`, `POST /api/v1/nodes/provision` (yang 2 terakhir bukan untuk Android).

---

## 2. Auth Flow (Anonymous, tanpa refresh)

Alur bootstrap identitas berjalan **sekali saat first-launch**:

```
[1] POST /api/v1/auth/anonymous          → 201 { token, user_id, pseudonym, expires_at, ... }
[2] Simpan token + user_id + pseudonym   → DataStore (bukan SharedPreferences)
[3] Semua request berikutnya             → Authorization: Bearer <token>
```

Detail:

- **Klaim JWT (HS256):** `sub` = `user_id` (UUID), `iat` + `exp` (detik epoch UTC). Tidak ada klaim lain — jangan mengekstrak identitas selain `sub`.
- **TTL:** default 30 hari (`expires_at` di respons = RFC3339 UTC). **Tidak ada alur refresh** — identitas anonim dibuat ulang. Saat token kedaluwarsa: panggil ulang `POST /api/v1/auth/anonymous` → ganti token lama (dan update `user_id`/`pseudonym` lokal).
- **Kehilangan token:** tidak bisa dipulihkan (server tak pernah mengembalikan token lama). Bila `user_id` lokal berubah, aplikasi harus menerima identitas baru (profile/settings keyed by user_id).
- **Idempotensi:** setiap pemanggilan membuat profil baru (bukan login). Jangan memanggil di setiap launch — cek validitas token lokal dulu (decode `exp`).

Respons `201` (`AnonymousAuthResponse`):

```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIzYTc4...",
  "token_type": "Bearer",
  "expires_at": "2026-09-18T11:02:00Z",
  "user_id": "3a78f1c2-9d4e-4b0a-8f77-2c1b5e9a6d30",
  "pseudonym": "Quakezen-7B9A",
  "created_at": "2026-08-19T11:02:00Z"
}
```

---

## 3. Header & Aturan Request

```http
Authorization: Bearer <jwt>
Content-Type: application/json
```

- **Header JWT:** `Authorization: Bearer <token>` (tipe skema `http` + `bearer` di OpenAPI). Gunakan persis — jangan kirim sebagai `X-API-Key` atau tanpa prefix `Bearer`.
- **Body:** JSON. Server memakai `DisallowUnknownFields` — field tak dikenal → `400 INVALID_ARGUMENT`.
- **Semantik PUT:** `PUT /api/v1/users/location` bersifat **replace** — mengirim body tanpa `location_name` akan **mengosongkan** label yang tersimpan.
- **Paginasi events:** `limit` default 20 (maks 100), `offset` default 0.

---

## 4. Endpoint Client-Facing (4 Endpoint REST)

> Seluruh 7 endpoint terdaftar di `router.go`; **4 berikut adalah permukaan integrasi Android** (ditandai `x-client: true` di OpenAPI).

### 4.1 `POST /api/v1/auth/anonymous` — bootstrap identitas
- **Publik**, tanpa token. Lihat §2.

### 4.2 `PUT /api/v1/users/location` — perbarui lokasi user
Request:

```json
{
  "latitude": -6.8721,
  "longitude": 107.5422,
  "location_name": "Cimahi, West Java, ID"
}
```

- `latitude`/`longitude` **wajib** (rentang -90..90 / -180..180).
- `location_name` opsional (≤150 char, hasil reverse-geocode klien). PUT = replace.
- `200` → `{ user_id, latitude, longitude, location_name|null, updated_at }`.
- **Kapan dipanggil:** saat onboarding berbagi lokasi, dan saat posisi berubah signifikan (mis. > 1 km). Lokasi tersimpan dipakai server untuk filter radius `GET /api/v1/events` tanpa koordinat eksplisit.

### 4.3 `PUT /api/v1/users/fcm-token` — daftarkan FCM registration token
Request:

```json
{ "fcm_token": "fMEP0vJq...:APA91bH-Xy9" }
```

- `fcm_token` wajib, ≤255 char. `200` → `{ updated_at }`.
- **Kapan dipanggil:** setiap kali Firebase `onNewToken` dipicu dan saat app mulai (pastikan token terbaru ada di server untuk delivery background).

### 4.4 `GET /api/v1/events` — riwayat gempa terkonfirmasi
- **Auth opsional.** Tanpa token data tetap dilayani (publik).
- Parameter: `limit` (1..100), `offset` (≥0), dan filter spasial `range_km` + `latitude` + `longitude`.
- **Filter spasial:** ketiga parameter (`range_km`, `latitude`, `longitude`) wajib dikirim bersamaan; sebagian → `400`. Tanpa koordinat eksplisit, `range_km` memakai **lokasi tersimpan user** (perlu auth + lokasi sudah di-set). `range_km` 1..2000 km.
- Respons `200`:

```json
{
  "limit": 20,
  "offset": 0,
  "count": 1,
  "range_km": 300,
  "events": [
    {
      "event_id": "a3f1c9de-51b2-4c77-9a0e-8d6f3b2c1a04",
      "status": "RESOLVED",
      "pga": 413.13,
      "mmi": "V",
      "intensity_label": "Strong",
      "latitude": -6.8721,
      "longitude": 107.5422,
      "depth_km": null,
      "location_name": "Cimahi, West Java, ID",
      "triggered_nodes_count": 4,
      "created_at": "2026-08-19T11:02:00Z",
      "resolved_at": "2026-08-19T11:04:30Z"
    }
  ]
}
```

- `count` = jumlah event **pada halaman ini** (bukan total). `range_km` = `null` bila filter spasial tidak aktif.
- **`depth_km` SELALU `null`** (jaringan MEMS permukaan mengestimasi centroid 2D, bukan hiposenter) — jangan ditampilkan sebagai kedalaman 0 km; pertahankan field agar model stabil.

---

## 5. MQTT Topic & QoS

### 5.1 Topic yang dipakai

| Arah | Topic | QoS | Penerbit | Skema |
|---|---|---|---|---|
| Sensor → Server | `sensor/<station_id>/trigger` | 1 | Firmware ESP32 | `contracts/mqtt/trigger.schema.json` |
| Sensor → Server | `sensor/<station_id>/heartbeat` | 1 | Firmware ESP32 | `contracts/mqtt/heartbeat.schema.json` |
| Server → Client | `alerts/earthquake` | 1 | Server (forward-looking) | `contracts/mqtt/alert.schema.json` |

- **QoS 1** dipilih untuk jalur life-safety agar pesan tidak hilang (QoS 0 fire-and-forget berisiko).
- **Transport:** produksi **wajib MQTTS/TLS** (ADR-0003); plaintext 1883 hanya untuk dev lokal.

### 5.2 `alerts/earthquake` — payload alert (forward-looking)

> **Status saat ini:** server belum mempublish topic ini — distribusi realtime dilakukan via **WebSocket** (foreground, `GET /ws`, auth JWT wajib) dan **FCM** (background). Skema MQTT ini adalah kontrak yang disamakan bentuknya dengan `AlertMessage` WS (`server/internal/dispatch/ws.go`) dan data kontrak FCM agar **satu model parse** dipakai untuk semua kanal. Kanal MQTT server→client diaktifkan pada integrasi berikutnya.

Payload inti (contoh):

```json
{
  "type": "EARTHQUAKE_ALERT",
  "event_id": "8f804561-1558-45ad-8982-1ab9193be589",
  "mmi": "V",
  "intensity_label": "Strong",
  "pga_gal": 413.13,
  "centroid_lat": -6.9175,
  "centroid_lon": 107.6191,
  "location_name": "Bandung, West Java, ID",
  "timestamp": 1723891234120,
  "node_count": 4
}
```

- `type`: `EARTHQUAKE_ALERT` (≥3 node, life-safety) | `EARTHQUAKE_ADVISORY` (1–2 node, banner kuning) | `EVENT_RESOLVED` (all-clear).
- `event_id` kosong (`""`) pada ADVISORY (tidak dipersistensikan). Gunakan `event_id` untuk **deduplikasi**.
- `pga_gal` satuan **gal**; `timestamp` **ms epoch UTC**; koordinat = **centroid** (BUKAN episenter — jangan tampilkan sebagai lokasi presisi).
- Kanal WebSocket/FCM: WebSocket memakai payload JSON yang sama (tipe `AlertMessage`); FCM mengirim data-only (semua nilai string) sesuai `contracts/fcm/alert_payload.json`, `android.priority=HIGH`.

### 5.3 Aturan life-safety untuk alert masuk (Android)

1. **Jangan percayai lokasi server saja** — jalankan **Haversine gating** lokal sebelum alarm: hitung `d(centroid, lokasi user)` dan hanya aktifkan alarm jika `d ≤ coverage_radius_km` (R = 6371.0 km).
2. **FCM data-only** (`FirebaseMessagingService`) + `EmergencyMessagingService` agar dipanggil saat app di background/killed.
3. Prioritaskan kanal yang tersedia: FCM (background) ↔ WS (foreground); dedup via `event_id`.

---

## 6. Error Code Mapping

Semua error memakai bentuk seragam `{ "code": string, "message": string }`.

| HTTP | `code` | Penyebab khas | Aksi klien |
|---|---|---|---|
| 400 | `INVALID_ARGUMENT` | Body/query invalid, field tak dikenal, koordinat di luar rentang, filter spasial tidak lengkap | Perbaiki request; jangan retry mentah-mentah. |
| 401 | `UNAUTHENTICATED` | Token tidak ada, rusak, kedaluwarsa, atau profil tak ditemukan | Re-auth via `POST /api/v1/auth/anonymous`, simpan token baru, retry sekali. |
| 429 | `RATE_LIMITED` | Reroll pseudonym > 1x/60s | Tampilkan cooldown; jangan retry sebelum jeda. |
| 500 | `INTERNAL` | Kesalahan internal server | Retry dengan backoff eksponensial; laporkan bila berulang. |

Catatan: `GET /api/v1/events` tanpa token mengabaikan 401 (auth opsional) — 401 hanya muncul bila token dikirim tapi invalid.

---

## 7. Format Payload & Satuan Kanonik

| Besaran | Satuan kanonik | Catatan |
|---|---|---|
| PGA | **gal** (cm/s²) | Konversi ke `g` hanya saat render (1 g ≈ 980.665 gal). |
| Timestamp | **ms epoch UTC** (int64) | Jangan pakai detik; konversi zona waktu hanya di UI. |
| Jarak / radius | **km** | Haversine dengan R = 6371.0 km. |
| RSSI | **dBm** | Integer negatif (telemetri sensor, bukan untuk Android). |
| Koordinat | WGS84 (lat -90..90, lon -180..180) | `latitude`/`longitude` = **centroid**, bukan episenter. |
| `depth_km` | `null` selalu | Jangan tampilkan sebagai 0 km. |

## 8. Checklist Integrasi Android (Ringkas)

- [ ] REST: repository dengan Retrofit/Ktor + OkHttp interceptor `Authorization: Bearer <token>`.
- [ ] First-launch: `POST /auth/anonymous` → simpan `token`/`user_id`/`pseudonym`/`expires_at` di DataStore.
- [ ] Onboarding lokasi: `PUT /users/location` (dengan `location_name` hasil reverse-geocode).
- [ ] Push: `PUT /users/fcm-token` di `onNewToken` + saat start; FCM data-only handler + `EmergencyMessagingService`.
- [ ] Realtime: WS `GET /ws` (auth JWT); dedup alert via `event_id`.
- [ ] Riwayat: `GET /events` dengan paginasi; filter spasial via lokasi tersimpan atau koordinat eksplisit.
- [ ] Life-safety: Haversine gating lokal sebelum alarm; `USE_FULL_SCREEN_INTENT` (API 34+ cek `Settings.canUseFullScreenIntent()`); audio `STREAM_ALARM`; minta pengecualian battery-optimization.
- [ ] Tolak `depth_km` sebagai 0; tampilkan lokasi sebagai "centroid terdeteksi" bukan episenter.
