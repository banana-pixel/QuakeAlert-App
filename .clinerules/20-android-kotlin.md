# 20 — Android (Kotlin) Rules

Aplikasi native Kotlin + Jetpack Compose di `android/` (dipindahkan dari root). Package `id.web.quakealert`. Lihat ADR-0003 (transport) & SYSTEM_SPEC Bab 6.B.

## Wajib

1. **Life-safety background delivery:** `FirebaseMessagingService` untuk payload **data-only** (bukan `notification`) agar `EmergencyMessagingService` selalu dipanggil meski app di background/killed. Jangan berasumsi Activity berjalan saat gempa.
2. **Local distance gating:** Kalkulasi Haversine WAJIB dijalankan sebelum `startActivity()` / `MediaPlayer.start()`. Alarm hanya jika `d ≤ coverage_radius_km`. `R = 6371.0 km`.
3. **Audio darurat:** `AudioAttributes.USAGE_ALARM` + `CONTENT_TYPE_SONIFICATION` + `STREAM_ALARM`. Siren loop dengan timeout 90s auto-expire; tombol Mute menghentikan audio tapi UI merah tetap.
4. **Lock-screen bypass:** WarningActivity pakai `setShowWhenLocked(true)`, `setTurnScreenOn(true)`, `FLAG_KEEP_SCREEN_ON`. Deklarasikan `USE_FULL_SCREEN_INTENT`; di **API 34+** cek `Settings.canUseFullScreenIntent()` dengan fallback heads-up notification.
5. **Doze/battery:** Minta `REQUEST_IGNORE_BATTERY_OPTIMIZATIONS` agar FCM high-priority tidak tertunda.
6. **Transport:** REST via HTTPS, realtime via WSS. Tanpa cleartext (set `usesCleartextTraffic=false`).
7. **Persistensi:** History lokal pakai **Room** (bukan SQLite mentah); preferensi pakai **DataStore** (bukan SharedPreferences).
8. **Satuan:** Simpan/proses PGA dalam gal; konversi ke `g` hanya saat render. Timestamp ms epoch UTC; konversi zona waktu di UI.
9. **Arsitektur:** MVVM + StateFlow (sudah dipakai). Ganti mock data dengan repository nyata (REST/WS) secara bertahap; jaga UI state tetap testable.

## Catatan

- UI Compose sudah ada (~60%). Yang belum: FCM service, WarningActivity full-screen, WS client, Haversine gating, wiring REST nyata.
