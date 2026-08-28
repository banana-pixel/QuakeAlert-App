> # ⚠️ HISTORICAL SNAPSHOT — SUPERSEDED
>
> This is a point-in-time gap audit taken against commit **`b7cd31a`**, audit
> date **2026-08-21**. Four phases of work have landed since. Its "current
> state" columns describe the **Phase 2** server, which has been superseded.
>
> **`docs/CURRENT_STATE.md` is now authoritative for current implementation
> status** (baseline `1ad1777`).
>
> Retained for traceability. Do not use it to decide what exists.
>
> Note: this document's claim that the chat feature is out of scope conflicts
> with `docs/CHAT_DESIGN.md`. That conflict is unresolved — see **U-005** in
> `docs/DECISIONS.md`.

# GAP ANALYSIS — QuakeAlert Ecosystem

Dokumen ini melacak selisih antara **arsitektur target** ([`SYSTEM_SPEC.md`](./SYSTEM_SPEC.md)) dan **kondisi kode aktual** di repo. Diperbarui setiap milestone.

**Tanggal audit:** 2026-08-21
**Commit basis:** `b7cd31a` + pekerjaan Fase A–E berjalan (lokasi, jalur alert, Settings, History/Sensors, perbaikan server)

> Revisi penuh. Versi sebelumnya (audit `ac26212`, 2026-08-18) menyatakan `server/`, `contracts/`, `firmware/` dan `deploy/` "belum ada" — keempatnya sudah ada dan berjalan. Jangan pakai versi lama sebagai acuan onboarding.

---

## 1. Ringkasan Status per Komponen

| Komponen | Target | Status Aktual | Kematangan |
|---|---|---|---|
| Contracts | OpenAPI + JSON Schema MQTT/FCM + DDL migrasi | Lengkap & jadi sumber kebenaran. `offset` max, `429` pada bootstrap, `intensity_label` huruf kecil, `/healthz` + `/ws` kini terdokumentasi | 🟢 Selesai |
| Server (Go) | Monolith: MQTT ingest, konsensus, FCM, WS, REST | Berjalan penuh: verifikasi HMAC, konsensus 8s, cooldown 90s, `EVENT_RESOLVED`, WS hub, FCM HTTP v1, rekonsiliasi saat start | 🟢 Selesai |
| Server — penargetan FCM | Kirim ke perangkat dalam radius event | `FCMTokensWithin` (ST_DWithin pada `user_profiles`) → kirim per token; topic `geo_alert_all` kini murni fallback | 🟢 Selesai |
| Firmware (ESP32) | PlatformIO, HMAC, MQTTS, NVS provisioning | Ada (`firmware/`), belum divalidasi lapangan | 🟡 Perlu uji perangkat |
| Deploy | docker-compose, Mosquitto, Postgres+PostGIS, Redis | Ada + `run_e2e_test.sh` (seed event, provision node, simulasi trigger) | 🟢 Selesai |
| Android — jaringan | REST + WS nyata, JWT anonim | `QuakeApiClient` (OkHttp + Bearer interceptor), `QuakeWebSocketClient` reconnect, `SessionStore` (DataStore) | 🟢 Selesai |
| Android — lokasi | Fix perangkat → `PUT /users/location` | `FusedLocationSource` bila Play Services ada, fallback AOSP `PlatformLocationSource`; reverse-geocode opsional; sinkron saat onboarding, start, dan "Sync Now" | 🟢 Selesai |
| Android — jalur alert life-safety | Gate Haversine, sirene 90s, full-screen di lock screen, FCM data-only | `AlertGate` sebelum sirene/Activity, `AlertSiren` auto-expire, `WarningActivity` (`showWhenLocked`/`turnScreenOn`), `QuakeMessagingService` + `WarningNotifier` dengan fallback heads-up API 34 | 🟢 Selesai |
| Android — Settings | Persisten, menggerakkan radius alert | DataStore penuh; slider 50–300 km yang SAMA dipakai gate, `range_km` events & sensors; Account & Privacy (reroll, reset), baterai, izin notifikasi | 🟢 Selesai |
| Android — History & Sensors | Filter radius + paginasi | `range_km`/`latitude`/`longitude` trio, paginasi append, filter "Near" server-side | 🟢 Selesai |
| Android — Chat | Kanal diskusi | **Mock lokal, out of scope**: server tidak punya kanal chat (tak ada endpoint maupun tabel yang dipakai) | 🔴 Sengaja belum |
| Android — Light Mode & Language | Tema terang, i18n | Placeholder bertanda "Coming Soon" (tidak inert secara diam-diam) | 🔴 Sengaja belum |
| Docs & ADR | Spec + ADR + gap analysis | Spec & ADR lengkap; `FIREBASE_SETUP.md` baru; dokumen ini direvisi | 🟢 Selesai |

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

## 3. Temuan Audit 2026-08-21 (Sudah Ditutup)

| # | Temuan | Dampak | Perbaikan |
|---|---|---|---|
| 1 | Aplikasi tidak pernah mengirim posisi — `updateLocation` tak punya pemanggil | `/sensors` selalu kosong, filter "Near" tak punya acuan, semua jarak tampil 0 | `UserLocationRepository` + sumber lokasi GMS/AOSP; sinkron di onboarding, app start, dan Settings |
| 2 | Tidak ada gate sebelum alarm | Sirene berbunyi untuk gempa yang jauh sekalipun (`.clinerules/20` rule 2 dilanggar) | `AlertGate` (Haversine) dipanggil sebelum `siren.start()` dan sebelum `startActivity()`; lokasi tak diketahui = fail **open** dengan banner "jarak tidak diketahui" |
| 3 | Sirene tidak pernah berhenti sendiri | Perangkat berbunyi tanpa batas | Auto-expire 90s pada `AlertSiren`; UI merah tetap sampai `EVENT_RESOLVED` |
| 4 | Tidak ada jalur background | Gempa saat app tertutup = tidak ada peringatan | `QuakeMessagingService` (data-only) + `WarningNotifier` + `WarningActivity` full-screen; guard tanpa `google-services.json` (lihat `FIREBASE_SETUP.md`) |
| 5 | Settings hanya `MutableStateFlow` | Semua pilihan hilang saat restart; radius hanya dekoratif | DataStore penuh; radius 50–300 km menggerakkan gate, `/events`, `/sensors`, dan lingkaran coverage di peta |
| 6 | `/ws` menolak semua klien native | Handshake OkHttp 403 selama `WS_ALLOWED_ORIGINS` kosong → jalur realtime mati pada konfigurasi default | `wsOriginChecker`: Origin absen (klien native) diizinkan, Origin browser tetap diperiksa allowlist |
| 7 | Token FCM tersimpan tapi tak pernah dipakai | Broadcast nasional: semua perangkat bangun untuk setiap event, seluruh penyaringan geografis di klien | `FCMTokensWithin` + kirim per token dalam `dispatchRadiusKm`; topic hanya bila tidak ada token dalam radius |
| 8 | Kontrak menyimpang dari implementasi | Klien mengira `intensity_label` kapital, `offset` tanpa batas, `429` bootstrap tak terduga | OpenAPI diperbarui: enum huruf kecil, `offset` max 50000, `429` per-IP 30s, `/healthz` + `/ws` didokumentasikan |
| 9 | `SaveEvent` salah jumlah placeholder — 8 kolom vs 7 ekspresi (`INSERT has more target columns than expressions`) | Event CONFIRMED **tidak pernah tersimpan**: History selalu kosong, frame `EARTHQUAKE_ALERT` membawa `event_id` kosong sehingga dedup lintas-kanal dan `EVENT_RESOLVED` kehilangan kunci | `triggered_nodes_count` diberi `$8`, `started_at` menjadi `$9`; diverifikasi lewat trigger MQTT nyata → event tersimpan dengan `event_id` UUID lalu `RESOLVED` setelah 90s |
| 10 | `scripts/simulate_trigger.go` menurunkan kunci HMAC dengan membuang prefix `sec_` lalu hex-decode | Setiap trigger simulasi ditolak `HMAC invalid`, sehingga `run_e2e_test.sh` tidak pernah bisa lulus | Kunci = byte ASCII `provisioning_secret` apa adanya, sama seperti server (`internal/api`) dan firmware (`firmware/src/mqtt.cpp`) |
| 11 | `scripts/setup_nodes.go` mengimpor `internal/cryptof` (typo) dan berbagi direktori dengan `main` kedua | `go build ./...` dan `go test ./...` gagal di root modul | Import diperbaiki ke `internal/crypto`; kedua helper manual diberi tag `//go:build ignore` |

---

## 4. Risiko Terbuka

- **Konsensus window 8s belum tervalidasi lapangan** — perlu kalibrasi dengan data sensor nyata.
- **Weighted centroid bukan episenter** — disclaimer sudah ada di UI; jangan hilang saat redesign.
- **Radius peringatan tetap 200 km di kedua sisi** — `dispatch.AlertRadiusKm` (server) dan `SafetyPolicy.ALERT_RADIUS_KM` (klien) harus selalu sama; berbeda satu sisi berarti server dan perangkat tidak sepakat tentang siapa yang dibangunkan. Gempa MMI ≥ VII / PGA ≥ 250 gal mengabaikan jarak sepenuhnya.
- **`coverage_radius_km` di `user_profiles` sudah tidak dipakai sama sekali** — kolomnya masih ada di migrasi 000001 (tidak dihapus agar migrasi lama tetap utuh), tetapi tidak ditulis maupun dibaca kode apa pun sejak radius menjadi tetap.
- **Sirene 90s, lock-screen bypass, dan jalur FCM belum dijalankan di perangkat/emulator** — `AlertSiren` bergantung `MediaPlayer`/`Handler` dan test source set tidak punya Robolectric, jadi keduanya hanya bisa dibuktikan manual; walkthrough 6 langkah di rencana implementasi masih **belum dieksekusi**. Logika murni (gate Haversine, parsing payload FCM, penyusunan query) sudah ditutup unit test JVM.
- **Migrasi Settings SharedPreferences → DataStore belum diuji otomatis** — `SharedPreferencesMigration` butuh `Context`, jadi verifikasinya instrumented/manual (force-stop lalu buka lagi), bukan unit test JVM.
- **Firmware belum diuji di perangkat nyata** — jalur HMAC/MQTTS baru dibuktikan lewat `run_e2e_test.sh` (simulasi trigger), bukan ESP32 fisik.
- **Chat tetap mock** — jangan tampilkan sebagai fitur berfungsi sampai kanal server ada.
