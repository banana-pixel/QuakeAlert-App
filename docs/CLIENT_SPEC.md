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
- **Auth wajib** — `PUT /api/v1/users/location`, `PUT /api/v1/users/fcm-token`, `GET /api/v1/sensors`, `GET /api/v1/chat/channels`, `GET /api/v1/chat/messages`, `POST /api/v1/chat/messages`, `POST /api/v1/users/pseudonym/reroll`, `POST /api/v1/nodes/provision` (yang 2 terakhir bukan untuk Android).

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

## 4. Endpoint Client-Facing (8 Endpoint REST)

> Seluruh endpoint terdaftar di `router.go`; **8 berikut adalah permukaan integrasi Android** (ditandai `x-client: true` di OpenAPI). `POST /api/v1/admin/broadcasts` sengaja TIDAK ada di daftar ini: kuncinya tidak pernah ikut dalam APK (§4.8).

### 4.1 `POST /api/v1/auth/anonymous` — bootstrap identitas
- **Publik**, tanpa token. Lihat §2.

### 4.2 `PUT /api/v1/users/location` — perbarui lokasi user
Request:

```json
{
  "latitude": -6.8721,
  "longitude": 107.5422,
  "location_name": "Cimahi, West Java, ID",
  "country_iso": "ID",
  "admin_area": "Jawa Barat"
}
```

- `latitude`/`longitude` **wajib** (rentang -90..90 / -180..180).
- `location_name` opsional (≤150 char, hasil reverse-geocode klien). PUT = replace.
- `country_iso` + `admin_area` opsional, dari reverse-geocode yang sama (`Address.countryCode` dan `Address.adminArea`). Server menormalisasinya menjadi kunci kanal chat regional — kirim apa adanya, jangan menyusun kuncinya sendiri.
- **Keduanya absen berarti "jangan diubah", bukan "kosongkan" — sampai posisinya sendiri membuktikan sebaliknya.** Reverse-geocode yang gagal di ponsel adalah kondisi sesaat, bukan bukti user pindah provinsi, jadi region yang tersimpan dibiarkan dan respons mengembalikan kanal yang sudah ada. **Kecuali** bila posisi baru lebih dari **25 km** dari posisi sebelumnya: jarak itu jauh di luar drift GPS dan masih nyaman di dalam satu provinsi, jadi server mengosongkan `region_code` dan menjawab `null`. Tanpa aturan ini `region_code` bersifat write-once — satu geocode yang berhasil akan mengunci user di ruang chat provinsi itu selamanya, termasuk setelah ia pindah. Geocode berikutnya yang berhasil menurunkannya ulang. Wilayah yang **dikirim** tetapi tidak dapat dinormalisasi adalah kasus lain lagi: region dikosongkan tanpa memandang jarak.
- **Klien tidak boleh mengirim ulang `location_name` lama untuk posisi baru.** Nama hanya boleh dipakai ulang bila **titik tempat nama itu diturunkan** dekat dengan fix sekarang (< 1 km) — bukan sekadar karena fix sebelumnya dekat. Bila tidak ada nama segar maupun nama yang masih berlaku, kirim string koordinat (`-6.91750, 107.61910`) alih-alih nama basi atau `null`: koordinat tidak pernah salah tempat, menimpa nama basi yang tersimpan di server, dan tetap menyisakan sesuatu yang benar untuk ditampilkan. Mengirim `null` akan membuat UI berbunyi "lokasi belum diatur" padahal posisinya ada.
- `200` → `{ user_id, latitude, longitude, location_name|null, region_code|null, updated_at }`.
- `region_code` adalah kanal chat regional yang berlaku setelah pembaruan ini, `null` bila wilayahnya tidak dapat dinormalisasi (mis. `admin_area` non-ASCII) atau tidak dapat dibaca — user itu hanya punya kanal global.
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

### 4.5 `GET /api/v1/chat/channels` — kanal yang boleh diakses

- `200` → `{ "channels": [ { "channel_id", "kind", "display_name" } ] }`, kanal `global` selalu di indeks pertama.
- **Server yang menjawab, klien tidak menebak.** Kunci regional (`ID-jawa-barat`) diturunkan dari `region_code` hasil §4.2; menyusunnya di klien akan meminta ruang yang tidak ada begitu normalisasi server berubah.
- Tanpa fix lokasi hanya ada `global` — itu keadaan yang sah, dan UI sebaiknya menyebut alasannya alih-alih menampilkan ruang kosong.
- **Kapan dipanggil:** saat tab Chat dibuka, dan setelah setiap `PUT /users/location` yang mengubah `region_code`.

### 4.6 `GET /api/v1/chat/messages` — riwayat satu kanal

- Parameter: `channel_id` (default `global`), `limit` (1..100, default 30), `before` (RFC3339).
- Urutan **menurun** (terbaru lebih dulu). Untuk halaman berikutnya kirim `created_at` pesan tertua yang sudah dipegang sebagai `before`.
- **Kursor waktu, bukan offset:** ruang yang aktif menggeser offset di antara dua permintaan, jadi paginasi offset akan melewatkan atau menggandakan baris tepat saat percakapan ramai.
- `count < limit` berarti tidak ada halaman lebih lama lagi. Retensi 7 hari — riwayat memang berujung.
- `403` bila `channel_id` bukan keanggotaan user.

### 4.7 `POST /api/v1/chat/messages` — kirim pesan

```json
{
  "channel_id": "ID-jawa-barat",
  "message": "Aman di sini, hanya lampu bergoyang.",
  "client_message_id": "5c2d1e90-3b7a-4f61-8e0d-9a4b7c6d5e1f"
}
```

- `message` wajib, 1..500 **karakter** (rune, bukan byte); spasi tepi dipangkas server.
- `client_message_id` opsional tapi **sangat disarankan**: kunci idempotensi. Kirim ulang dengan nilai yang sama mengembalikan pesan yang **sama**, bukan duplikat — klien yang timeout tidak tahu apakah percobaan pertamanya sampai, dan pesan ganda di ruang publik tidak bisa ditarik kembali.
- `201` → objek `ChatMessage`. `403` kanal asing, `429` melewati batas laju (1 pesan / 2 detik per user).
- **Kirim lewat REST, bukan WebSocket.** REST durable dan bisa diulang; socket hanya memfanout apa yang sudah tersimpan.

### 4.8 `GET /api/v1/broadcasts` — pengumuman operator

- `200` → `{ "broadcasts": [ { "broadcast_id", "title", "body", "region_code", "created_at" } ] }`, terbaru dulu. `region_code: null` = nasional.
- Parameter: `limit` (1..50, default 20). Retensi 90 hari.
- **Wilayah diambil dari profil, bukan dari query.** Klien tidak dapat (dan tidak perlu) meminta pengumuman wilayah lain; yang ia terima adalah nasional + wilayahnya sendiri hasil §4.2.
- **Kapan dipanggil:** saat daftar Pembaruan dibuka, dan setelah menerima frame/notifikasi `ADMIN_BROADCAST` (§5.7).
- Penerbitannya adalah `POST /api/v1/admin/broadcasts` dengan header `X-Admin-Key` — **bukan** JWT, dan bukan permukaan klien. Pengirimnya `deploy/scripts/broadcast.sh` di host terpercaya. Bila `ADMIN_API_KEY` tidak di-set, rute itu tidak didaftarkan sama sekali dan menjawab `404`: instalasi yang lupa mengisinya kehilangan fiturnya, bukan pagarnya.

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

1. **Jangan percayai lokasi server saja** — jalankan **Haversine gating** lokal sebelum alarm: hitung `d(centroid, lokasi user)` dan hanya aktifkan alarm jika `d ≤ 200 km` (`SafetyPolicy.ALERT_RADIUS_KM`, R = 6371.0 km). Radiusnya **tetap**, bukan pilihan pengguna, dan nilainya sama dengan `dispatch.AlertRadiusKm` di server.
   - **Override intensitas:** gempa dengan **MMI ≥ VII atau PGA ≥ 250 gal** membunyikan alarm **tanpa memeriksa jarak**. Server pun menyiarkannya ke topic tanpa filter jarak, jadi kedua sisi sepakat tanpa flag di payload.
   - Posisi tidak diketahui = **fail open** (alarm tetap berbunyi): satu notifikasi berlebih lebih baik daripada satu perangkat yang dibungkam.
2. **FCM data-only** (`FirebaseMessagingService`) + `EmergencyMessagingService` agar dipanggil saat app di background/killed.
3. Prioritaskan kanal yang tersedia: FCM (background) ↔ WS (foreground); dedup via `event_id`.

### 5.4 Frame chat di WebSocket yang sama

Socket `GET /ws` juga memfanout pesan chat, hanya untuk kanal yang menjadi keanggotaan koneksi (dibaca **sekali** saat handshake — klien yang wilayahnya berubah akan menyambung ulang):

```json
{
  "type": "CHAT_MESSAGE",
  "message_id": "8f14e45f-ceea-467a-9c3f-2b0c4d5e6f70",
  "channel_id": "ID-jawa-barat",
  "sender_id": "3a78f1c2-9d4e-4b0a-8f77-2c1b5e9a6d30",
  "sender_pseudonym": "AnonimTenang",
  "sender_location_tag": "Bandung",
  "message": "Aman di sini, hanya lampu bergoyang.",
  "is_admin": false,
  "timestamp": 1723891234120
}
```

- Satu socket, satu envelope ber-`type` — bukan koneksi kedua, sehingga alert dan chat berbagi satu auth dan satu jalur reconnect. Klien memilah berdasarkan `type` dan **mengabaikan tipe yang tidak dikenal**.
- **Alert selalu diprioritaskan:** bila buffer per-klien menipis, frame chat dilewati dan koneksi TIDAK ditutup. Chat yang hilang muncul kembali lewat §4.6; peringatan yang hilang tidak punya jalan kembali. Karena itu socket adalah jalur *cepat*, bukan sumber kebenaran.
- `sender_id` memungkinkan klien mengenali pesannya sendiri: cocokkan dengan `user_id` sendiri agar gelembung optimistis **diganti**, bukan digandakan.
- Frame yang **dikirim** klien tetap diabaikan server. Pengiriman lewat `POST /api/v1/chat/messages`.

### 5.5 Notifikasi status: tanpa foreground service (keputusan)

Klien menampilkan notifikasi **status** yang tenang dan berkelanjutan (`StatusNotifier`, kanal `quakealert_status`, `IMPORTANCE_MIN`, **opt-in dan mati secara default**). Notifikasi itu hanya mencetak fakta yang sudah dipegang app — sakelar alert user, izin `POST_NOTIFICATIONS`, umur posisi terakhir, dan status pembebasan battery optimization — dan **tidak pernah** mengklaim perlindungan yang tidak bisa diberikan.

Isinya **hanya masalah**, bukan audit empat baris: bila semua beres, badan notifikasi menyusut jadi satu baris menenangkan, dan setiap masalah menambah tepat satu baris dengan urutan prioritas yang sama seperti `headline`. Baris terakhir selalu menjawab satu pertanyaan yang benar-benar ditanyakan user di bulan yang sepi — apakah app ini pernah berbunyi: `"Last alert: …"`, atau `"No alerts since you installed QuakeAlert."` bila belum pernah. Catatan itu ditulis di `WarningNotifier.notify` **pada saat peringatan benar-benar sampai ke shade**, bukan saat pesan tiba di socket: alert yang disaring gate jarak atau dijatuhkan OS tidak pernah memperingatkan user ini, jadi tidak boleh diklaim pernah.

Yang **tidak** dilakukan, beserta alasannya, supaya tidak dibuka ulang tanpa sengaja:

1. **Bukan foreground service, dan socket tidak ditahan terbuka.** Jalur pengiriman background sudah ada dan direstui platform: FCM prioritas tinggi kebal Doze dan sampai walau proses mati (§5.3 poin 2–3). Socket yang ditahan di service hanya menang di latensi, tidak di keterjangkauan.
2. **`specialUse` tidak dapat dibenarkan.** Pada `targetSdk 36`, satu-satunya `foregroundServiceType` yang cocok untuk "menahan socket" adalah `specialUse`, yang ditinjau Google per kasus dan ditolak bila ada alternatif yang didukung. Menambah tinjauan Play, boot receiver, dan penanganan pembebasan Doze untuk menduplikasi jalur yang sudah bekerja adalah trade yang salah. Karena itu manifest **tidak** meminta `FOREGROUND_SERVICE`.
3. **Floating window (`SYSTEM_ALERT_WINDOW`) tidak dipakai.** Platform sudah memberi alat yang benar untuk ini: alert dikirim dengan **full-screen intent** ke `WarningActivity` pada kanal `IMPORTANCE_HIGH` `quakealert_emergency_alerts`, yang mengambil alih layar **termasuk di atas lockscreen** — hal yang justru tidak bisa dilakukan overlay. Sementara `SYSTEM_ALERT_WINDOW` butuh grant per-user lewat layar system settings, termasuk permission sensitif di kebijakan Play, dan sejak Android 12 overlay disembunyikan otomatis oleh dialog sistem. Jadi overlay menambah friksi izin dan risiko review untuk kemampuan yang lebih kecil dari yang sudah dipakai.
4. **Kesehatan socket tidak ikut dicetak.** `connectionState` dibagikan `WhileSubscribed` dan menutup ketika UI hilang, jadi baris "offline" akan muncul untuk app yang di-background padahal alert-nya memang datang lewat push. Berlangganan dari notifikasi agar barisnya jujur berarti menahan koneksi 24 jam demi satu baris di shade.

### 5.6 Nama episenter berasal dari label node (keputusan)

`location_name` pada sebuah alert **bukan** hasil geocoding: `internal/consensus/engine.go` mengambilnya dari `iot_nodes.location_name` milik node terdekat ke centroid, dan field itu diisi saat `POST /api/v1/nodes/provision`. Artinya nama tempat yang dibaca pengguna di dalam alert adalah teks yang **diketik operator** saat memasang sensor.

Konvensi wajib untuk field itu: **"Kecamatan, Kabupaten/Kota, Provinsi"** — tempat yang bisa dicari pembaca di peta (contoh: `"Lembang, Kab. Bandung Barat, Jawa Barat"`). Server menormalkan spasi, lalu menolak nilai kosong, lebih dari 150 karakter, yang memuat `NODE-` (station id bukan tempat), atau yang memuat 5+ karakter identik berurutan (`"Bandung AAAAAAAA"` — bentuk khas nilai uji). Penolakan datang sebagai `400 INVALID_ARGUMENT`.

Validasi itu hanya menyaring bentuk yang jelas bukan nama tempat; ia **tidak** bisa membuktikan sebuah string benar-benar menyebut lokasi node. Jawaban tahan lama adalah memberi nama event dari **gazetteer atau tabel batas administratif** berbasis centroid, dengan label node hanya sebagai fallback. Itu butuh sumber data batas wilayah dan diputuskan bersamaan dengan pengetatan kepercayaan `POST /nodes/provision` (endpoint itu masih terbuka untuk pemanggil anonim), jadi dicatat di sini sebagai keputusan, bukan pekerjaan yang sudah selesai.

### 5.7 Pengumuman operator: kanal sendiri, bukan kanal alert

Frame WebSocket dan payload FCM `type: "ADMIN_BROADCAST"`:

```json
{
  "type": "ADMIN_BROADCAST",
  "broadcast_id": "9c2f1a44-6e1b-4f0a-9a77-3d5b6c7e8f91",
  "title": "Gladi resik peringatan dini",
  "body": "Pukul 10.00 WIB akan ada uji sirene. Tidak perlu evakuasi.",
  "region_code": "ID-jawa-barat",
  "timestamp": 1781913558000
}
```

- **Kanal notifikasi terpisah** (`quakealert_updates`, importance rendah, tanpa suara, tanpa full-screen intent). Sebuah pengumuman yang berbunyi seperti sirene mengajari pengguna mengabaikan sirene, dan itu tidak dapat diperbaiki dengan pengumuman berikutnya.
- **Topic FCM terpisah** (`updates_all`, bukan `geo_alert_all`) dan prioritas `NORMAL`. Seseorang yang berhenti berlangganan kabar tidak boleh ikut kehilangan peringatan gempa.
- Siaran regional dikirim **hanya** ke token wilayah itu dan tidak pernah jatuh ke topic nasional; klien tidak perlu memfilter apa pun berdasarkan `region_code`, tetapi boleh menampilkannya.
- `broadcast_id` adalah kunci dedup: frame WS dan notifikasi FCM dapat tiba berdua.
- Payload push selalu `data`-only, seperti alert — kanal, ikon dan bunyinya ditentukan klien, bukan server.

---

## 6. Error Code Mapping

Semua error memakai bentuk seragam `{ "code": string, "message": string }`.

| HTTP | `code` | Penyebab khas | Aksi klien |
|---|---|---|---|
| 400 | `INVALID_ARGUMENT` | Body/query invalid, field tak dikenal, koordinat di luar rentang, filter spasial tidak lengkap | Perbaiki request; jangan retry mentah-mentah. |
| 401 | `UNAUTHENTICATED` | Token tidak ada, rusak, kedaluwarsa, atau profil tak ditemukan | Re-auth via `POST /api/v1/auth/anonymous`, simpan token baru, retry sekali. |
| 403 | `PERMISSION_DENIED` | Kanal chat bukan keanggotaan user | Muat ulang `GET /chat/channels`; jangan menyusun `channel_id` sendiri. |
| 429 | `RATE_LIMITED` | Reroll pseudonym > 1x/60s, atau kirim chat > 1x/2s | Tampilkan cooldown; jangan retry sebelum jeda. |
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
- [ ] Onboarding lokasi: `PUT /users/location` (dengan `location_name`, `country_iso` dan `admin_area` hasil reverse-geocode).
- [ ] Push: `PUT /users/fcm-token` di `onNewToken` + saat start; FCM data-only handler + `EmergencyMessagingService`.
- [ ] Realtime: WS `GET /ws` (auth JWT); dedup alert via `event_id`; pilah `type` dan abaikan tipe tak dikenal.
- [ ] Chat: `GET /chat/channels` saat tab dibuka; paginasi ke atas dengan kursor `before`; kirim dengan `client_message_id` dan ganti gelembung optimistis saat frame `CHAT_MESSAGE` sendiri kembali.
- [ ] Riwayat: `GET /events` dengan paginasi; filter spasial via lokasi tersimpan atau koordinat eksplisit.
- [ ] Life-safety: Haversine gating lokal sebelum alarm; `USE_FULL_SCREEN_INTENT` (API 34+ cek `Settings.canUseFullScreenIntent()`); audio `STREAM_ALARM`; minta pengecualian battery-optimization.
- [ ] Tolak `depth_km` sebagai 0; tampilkan lokasi sebagai "centroid terdeteksi" bukan episenter.
