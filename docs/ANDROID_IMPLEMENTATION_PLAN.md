# ANDROID IMPLEMENTATION PLAN — Fase 1 (Wiring Backend)

Dokumen perencanaan implementasi Android client (`android/`) terhadap backend QuakeAlert. Disusun dari analisis menyeluruh direktori `android/` (2026-08-19) dan keputusan fitur yang dikonfirmasi user. Referensi kontrak: `docs/CLIENT_SPEC.md`, `contracts/openapi/openapi.yaml`, `.clinerules/20-android-kotlin.md`.

---

## 1. Inventarisasi Halaman yang Sudah Ada

### 1.1 Screens & Routing

| File | Peran | Status | Catatan |
|---|---|---|---|
| `MainActivity.kt` | Entry Activity | ✅ Lengkap | `enableEdgeToEdge` + `AppRoot`; tipis, tidak perlu diubah |
| `ui/app/AppRoot.kt` | Root gate (onboarding ↔ main) | ✅ Lengkap | `AnimatedContent` fade; perlu penambahan gate auth (Fase 1) |
| `ui/app/AppViewModel.kt` | VM app-level | 🟡 Refactor | Hanya kelola `onboardingCompleted`; membuat repository sendiri (tanpa DI); belum tahu session/token |
| `ui/main/MainScreen.kt` | Scaffold 5 tab | 🟡 Refactor | Nav manual `remember { mutableStateOf }` — hilang saat config change/process death; tanpa back stack; komen usang baris 86–88; `QuakeBottomNavigation` custom bagus (dipertahankan) |
| `data/AppSettingsRepository.kt` | Persistensi lokal | 🟡 Refactor | **SharedPreferences** (bukan DataStore; API-nya sudah berbentuk `Flow` reaktif sehingga migrasi drop-in); hanya persist 1 flag: `onboarding_completed` |

### 1.2 Per-Layar (pola: Route → Screen stateless → UiState + ViewModel)

| Layar | File | Status UI | Status Data | Hook no-op / masalah |
|---|---|---|---|---|
| **Onboarding** | `OnboardingScreen.kt`, `OnboardingPage.kt`, `PermissionCard.kt`, `TestAlertControls.kt`, `TestAlertNotifier.kt`, `ReadyText.kt` | ✅ Lengkap (7 halaman pager) | 🟡 Sebagian nyata | Permission (NOTIF/BATTERY/LOCATION) & test alert **nyata**; state `remember` lokal (hilang saat config change); `keepAlerting` tidak disinkron ke Settings; **belum ada registrasi anonymous REST** (Fase 1) |
| **Warning** | `WarningScreen.kt`, `WarningComponents.kt`, `WarningUiState.kt`, `WarningViewModel.kt` | ✅ Lengkap (banner + tips + CTA) | 🔴 **Mock penuh** | Banner hardcoded `"Earthquake Detected / MMI VII"`; `onSeeDetails` & `onEmergency` **no-op**; **TIDAK ada audio, vibrasi, full-screen, notifikasi produksi** (grep audio/vibrate = 0) |
| **Sensors** | `SensorsScreen.kt`, `SensorItemCard.kt`, `SensorMapCard.kt`, `SensorsUiState.kt`, `SensorsViewModel.kt` | ✅ Lengkap | 🔴 **Mock** (7 stasiun hardcoded) | Filter NEAR tidak menyaring; `onCalendar`/`onSensorClicked` no-op; peta = placeholder `drawBehind` |
| **History** | `HistoryScreen.kt`, `QuakeHistoryCard.kt`, `HistoryUiState.kt`, `HistoryViewModel.kt` | ✅ Lengkap | 🔴 **Mock** (7 gempa hardcoded) | `onSeeMore` (detail) & `onShare` no-op; tanggal string pre-formatted (`"20 Jun 2026"`, `"07:19:18 WIB"`) |
| **Chat** | `ChatScreen.kt`, `ChatBubble.kt`, `ChatChannelCard.kt`, `ChatInputBar.kt`, `ChatUiState.kt`, `ChatViewModel.kt` | ✅ Lengkap | 🔴 **Mock** (local-only) | `onSend` berfungsi lokal; `onSwitchChannel` no-op; **tidak ada transport mesh di server** → out-of-scope (dipertahankan apa adanya) |
| **Settings** | `SettingsScreen.kt`, `SettingsComponents.kt`, `AboutModal.kt`, `SettingsUiState.kt`, `SettingsViewModel.kt` | ✅ Lengkap (5 section; AboutModal berfungsi) | 🔴 **Mock + in-memory** | `onSyncLocationNow` & `onTestAlertSound` no-op; `lightMode`/`language` placeholder; semua setting hilang saat restart; `CoverageRange` = 125/250/500 km (akan diganti slider 50–300) |
| **Common** | `QuakeAppBar.kt`, `QuakeCard.kt`, `QuakeFilter.kt`, `QuakeFilterRow.kt`, `QuakePill.kt`, `QuakeSwitch.kt`, `FadingEdges.kt` | ✅ Lengkap | — | Stateless & rapi; `onCalendarClicked` di semua layar no-op |
| **Theme** | `Color.kt`, `Dimens.kt`, `Type.kt`, `Theme.kt` | ✅ Lengkap | — | Dark-only; tidak ada light variant (sesuai keputusan: Light Mode tetap placeholder) |

### 1.3 Temuan lintas-layar (prioritas refactor)

1. **Belum ada lapisan jaringan sama sekali** — tidak ada Retrofit/Ktor; dependency hanya Compose + Lifecycle (`gradle/libs.versions.toml`).
2. **Tidak ada DI (Hilt/Koin), Navigation Compose, DataStore, Room, Firebase.**
3. **Tidak ada mekanisme alert life-safety** (full-screen intent, siren, `FirebaseMessagingService`, `EmergencyMessagingService`) — inti produk masih kosong.
4. `collectAsState()` non-lifecycle di 5 layar (kecuali `AppRoot` yang sudah `collectAsStateWithLifecycle`) → ganti semua.
5. Semua ViewModel fitur tanpa repository → state hilang saat process death.
6. Data mentah berupa string pre-formatted di UiState (tanggal, telemetri, waktu) → tipe terstruktur + formatter.

---

## 2. Kebutuhan Halaman Baru (Screen Gap Analysis)

| # | Halaman | Status | Fungsi utama | Komponen Compose yang dibutuhkan |
|---|---|---|---|---|
| 1 | **Onboarding + Anonymous Registration** | 🔄 Perluas yang ada | Pager 7 halaman tetap; tambah `POST /api/v1/auth/anonymous` saat first-launch → simpan `token`+`user_id`+`pseudonym` di DataStore; tampilkan pseudonym di halaman READY | `HorizontalPager` (ada), `PermissionCard` (ada), **+ `AuthRepository`/`SessionManager`** |
| 2 | **Dashboard Monitor Siaga** (tab Warning → dashboard hidup) | 🔄 Refactor (Fase 6) | Banner dari alert WS/FCM nyata (dedup `event_id`); status healthy; tips siaga; haversine gating sebelum alarm | `AlertBanner` (ada), `QuakeAppBar` (ada), **+ `RealtimeClient` (WS), `HaversineUtils`, state berbasis event** |
| 3 | **History / Log Gempa** | 🔄 Wire ke API (Fase 2) | `GET /api/v1/events` (limit/offset, filter `range_km` dari lokasi user); pull-refresh; replace mock dengan DTO `EarthquakeEvent` | `QuakeHistoryCard` (ada), `QuakeFilterRow` (ada), **+ `EventsRepository`, paginasi, refresh** |
| 4 | **Detail Event** | 🆕 **Baru** (Fase 3) | Statistik & lokasi (MMI, PGA gal→g, `triggered_nodes_count`, centroid, `location_name`), tombol Share, mini-map centroid | `QuakeAppBar`, `SensorMapCard` (mini-map), kartu statistik, **route full-screen di atas bottom nav** |
| 5 | **Full-Screen Emergency Alert** | 🆕 **Baru** (Fase 5) | Layar merah penuh saat gempa (lock-screen bypass): siren loop 90s, mute, dedup `event_id`; dibuka dari FCM data-only **dan** WS; **Activity terpisah, bukan route** | `WarningActivity` (baru: `setShowWhenLocked`, `setTurnScreenOn`, `FLAG_KEEP_SCREEN_ON`), `MediaPlayer` `STREAM_ALARM`/`USAGE_ALARM`, `USE_FULL_SCREEN_INTENT` + fallback `Settings.canUseFullScreenIntent()` (API 34+), haversine gating lokal |
| 6 | **Settings** | 🆕 **Perluas** (Fase 4) | 4 kelompok opsi sesuai kapabilitas sistem (lihat §3) | Lihat §3 |
| 7 | Chat (mesh) | ⏸️ **Tunda** | Transport mesh belum ada di server | UI dipertahankan apa adanya; tandai di `GAP_ANALYSIS.md` |

---

## 3. Spesifikasi Halaman Pengaturan (Settings Screen)

Struktur: `LazyColumn` dipertahankan; section **"Appearance & Look" dipertahankan sebagai placeholder** (Light Mode & Language tanpa efek); sisanya diganti 4 kelompok berikut:

### A. Akun & Privasi (🆕)

| Opsi | Perilaku | Sumber data |
|---|---|---|
| Kartu identitas: **Pseudonym** + **User ID** | Tampil dari respons auth; tombol copy `user_id` ke klipboard | `AnonymousAuthResponse.pseudonym/user_id` (DataStore session) |
| **Reset / Re-auth Profil** | Konfirmasi dialog → panggil ulang `POST /api/v1/auth/anonymous` → ganti token+identitas lokal (CLIENT_SPEC §2: tidak ada refresh; re-auth = profil baru) | `AuthRepository` |
| *(opsional)* **Reroll Pseudonym** | `POST /api/v1/users/pseudonym/reroll` (rate-limit 1x/60s; tampilkan countdown saat 429) | `UsersRepository` |

### B. Lokasi & Radius

| Opsi | Perilaku |
|---|---|
| **Mode lokasi: GPS / Manual** | GPS → `ACCESS_FINE_LOCATION` (FusedLocationProvider) + label via Geocoder; Manual → input koordinat + `location_name` |
| **Radius filter alert (Slider 50–300 km, default 150)** | Ganti segmented 125/250/500 → `Slider`; simpan di DataStore; dipakai untuk `range_km` (`GET /events`), haversine gating alarm, dan geofence di peta |
| **Auto Sync Location** | Ada; wire ke lokasi tersimpan server (saat posisi berubah signifikan > 1 km) |
| **Sync Now** | Ada; jalankan `PUT /api/v1/users/location` sungguhan |

### C. Alarm & Sirine

| Opsi | Perilaku |
|---|---|
| **Test Sirene** | Mainkan siren 2–3 detik via `MediaPlayer` `USAGE_ALARM`/`STREAM_ALARM` (bukan sekadar notifikasi) |
| **Keep Alerting** | Ada; persist di DataStore + diteruskan ke notifikasi FCM/emergency (insistent/ongoing) |
| **Bypass Doze / Battery Optimization** | Status + tombol → `Settings.ACTION_REQUEST_IGNORE_BATTERY_OPTIMIZATIONS` (sinkronkan state onboarding ke sini + persist) |
| **Toggle Notifikasi** | Hidup/mati alert FCM + notifikasi lokal; validasi status `POST_NOTIFICATIONS` |

### D. Koneksi & Jaringan

| Opsi | Perilaku |
|---|---|
| **Status koneksi (WS)** | Indikator live: Terhubung / Terputus / Menghubung (dari `RealtimeClient` state) + health badge |
| **Mode Dev / Base URL** | Switch prod (`https://api.quakealert.id`) ↔ dev (`http://localhost:8080`); base URL dipakai semua repository; dev butuh cleartext khusus debug build |
| **Status last sync** | Ada (`lastSyncPillLabel`), wire ke timestamp nyata |

> Semua opsi persist via **DataStore** (migrasi dari SharedPreferences). Pemetaan kontrak: CLIENT_SPEC §4 (location), §5 (alert/WS), §6 (error 429 reroll).

---

## 4. Rekomendasi Navigasi & Arsitektur

### 4.1 Navigasi — hybrid

| Layer | Pendekatan | Alasan |
|---|---|---|
| **Root** | `AppRoot` gate (ada) + 2 level: `OnboardingNav` / `MainNav` | Pertahankan pola sekarang |
| **Main (5 tab)** | **Navigation Compose** `NavHost` di dalam `MainScreen`; bottom bar = `QuakeBottomNavigation` custom (dipertahankan); 5 destination | Back stack + `rememberSaveable` + state tab survive config change; transisi per-tab bisa tetap pakai `AnimatedContent` |
| **Detail Event** | Route **di luar** bottom nav (push full-screen, `popBackStack`) | Bukan tab; back gesture alami |
| **Emergency Alert** | **Activity terpisah** (`WarningActivity`, `singleTop` + full-screen flags) | Harus muncul saat app killed/background via FCM — tidak boleh di nav graph |

Trade-off yang sudah diputuskan: **pakai Navigation Compose** (dependency baru, idiomatik, back stack, deep link untuk FCM→detail).

### 4.2 Arsitektur (MVVM + StateFlow, sesuai .clinerules/20)

```
id.web.quakealert/
├── data/
│   ├── remote/ApiClient.kt            # Retrofit; base URL prod/dev (dari Settings §3D); interceptor Bearer JWT
│   ├── auth/AuthRepository.kt         # POST /auth/anonymous; session (token/user_id/pseudonym/expires_at) di DataStore
│   ├── events/EventsRepository.kt     # GET /events (limit/offset/range_km)
│   ├── users/UsersRepository.kt       # PUT /users/location; PUT /users/fcm-token; POST /users/pseudonym/reroll
│   ├── realtime/RealtimeClient.kt     # OkHttp WS /ws; state: Terhubung/Terputus/Menghubung
│   └── settings/AppSettingsRepository.kt  # migrasi → DataStore; semua opsi §3
├── domain/
│   ├── Haversine.kt                   # gating: d(centroid, user) ≤ radius → alarm; R = 6371.0 km
│   └── model/EarthquakeEvent.kt       # DTO = skema openapi (pga gal, ms epoch, depth_km null)
├── service/
│   ├── QuakeMessagingService.kt       # FCM data-only
│   └── QuakeEmergencyService.kt       # EmergencyMessagingService → full-screen Intent + siren
└── ui/
    ├── warning/WarningActivity.kt     # 🆕 full-screen emergency
    ├── history/EventDetailScreen.kt   # 🆕 detail event
    └── (layar existing dipertahankan: Route → Screen → UiState + VM)
```

**Prinsip:** UiState per layar dipertahankan; mock ViewModel diganti repository nyata satu per satu; hook no-op diisi; `collectAsState()` → `collectAsStateWithLifecycle()`.

---

## 5. Keputusan Fitur (hasil konfirmasi user)

| Topik | Keputusan |
|---|---|
| Chat mesh (tab 5) | **Skip/delay** — tab tetap ada, tidak diwiring ke backend |
| Light Mode & Language | **Placeholder** — toggle dipertahankan tanpa efek |
| Detail Event | Statistik & lokasi + **tombol Share** + **mini-map centroid** (tanpa timeline) |
| Kanal realtime | **WebSocket saja** (`/ws`); MQTT `alerts/earthquake` (forward-looking) ditunda |
| Radius filter | **Slider 50–300 km, default 150** — untuk `range_km`, haversine gating, geofence |
| Lokasi user | **GPS + Geocoder** (`play-services-location`, tanpa Maps SDK/API key) |
| Urutan implementasi | Auth → History → Settings → Alert (FCM+WarningActivity) → Sensors → WS dashboard |

---

## 6. Dependency Baru (`gradle/libs.versions.toml`)

- `androidx.navigation:navigation-compose`
- `androidx.datastore:datastore-preferences`
- `com.squareup.retrofit2:retrofit` (+ `converter-kotlinx-serialization`)
- `com.squareup.okhttp3:okhttp` (HTTP + WebSocket)
- `org.jetbrains.kotlinx:kotlinx-serialization-json`
- `com.google.firebase:firebase-messaging` (untuk `EmergencyMessagingService`)
- `com.google.android.gms:play-services-location`
- `org.jetbrains.kotlinx:kotlinx-coroutines-play-services`

---

## 7. Fase Implementasi 1–6

### Fase 1 — DataStore + Auth
- Migrasi `AppSettingsRepository` → DataStore Preferences (API `Flow` dipertahankan agar call site tidak berubah).
- `ApiClient` (Retrofit + OkHttp): base URL dari settings (§3D), interceptor menambahkan `Authorization: Bearer <token>` saat session ada.
- `AuthRepository`: `POST /api/v1/auth/anonymous` saat first-launch; simpan `token`/`user_id`/`pseudonym`/`expires_at`; re-auth otomatis saat 401/expired (CLIENT_SPEC §2 — tanpa refresh, profil baru).
- `AppRoot`/`AppViewModel`: gate tambahan — menunggu session tersedia sebelum `MainScreen`; panggil auth saat onboarding selesai (atau pre-emptively saat `Ready` pertama).
- Ganti `collectAsState()` → `collectAsStateWithLifecycle()` di layar yang disentuh.

### Fase 2 — History (REST)
- `EventsRepository` + DTO `EarthquakeEvent` (peta field openapi: `pga` gal, `mmi`, `intensity_label`, `latitude/longitude` centroid, `depth_km` null, `location_name`, `triggered_nodes_count`, `created_at`/`resolved_at` RFC3339).
- `HistoryViewModel`: `GET /events?limit=20&offset=` (paginasi + refresh); filter All/Near — Near = `range_km` = radius settings (lokasi dari session/`GET /users/location`).
- Wire `onShare` (intent share text) & `onSeeMore` (→ Detail Event, Fase 3).
- Formatter satuan: gal→g hanya saat render; timestamp ms/RFC3339 → lokal.

### Fase 3 — Detail Event
- Route full-screen di luar bottom nav (Navigation Compose, `popBackStack`).
- Konten: header (MMI + `intensity_label`), kartu statistik (PGA gal→g, `triggered_nodes_count`, koordinat centroid, `location_name`, `created_at`), mini-map (`SensorMapCard`), tombol Share.
- Data: dari objek event yang sudah dimiliki History (tidak ada endpoint detail terpisah).

### Fase 4 — Settings nyata
- Wire semua kelompok §3 ke DataStore + REST.
- Lokasi: `FusedLocationProviderClient` + Geocoder (label); `PUT /api/v1/users/location` (perhatikan semantik PUT: `location_name` wajib dikirim bila ingin dipertahankan).
- Radius: `Slider` 50–300 default 150; simpan; perbarui geofence `SensorMapCard` & `range_km`.
- Akun: kartu pseudonym/user_id + copy; Reset/Re-auth (dialog konfirmasi); Reroll (handling 429 → countdown).
- Alarm: test sirene `MediaPlayer` `STREAM_ALARM`; bypass Doze (sinkron state onboarding); toggle notifikasi (cek `POST_NOTIFICATIONS`).
- Koneksi: status WS (dari `RealtimeClient`); mode dev/prod (base URL).
- Light Mode & Language: **tetap placeholder** (tanpa efek).

### Fase 5 — Alert life-safety
- `WarningActivity` (baru): full-screen; `setShowWhenLocked(true)`, `setTurnScreenOn(true)`, `FLAG_KEEP_SCREEN_ON`; siren loop 90s (`MediaPlayer` `USAGE_ALARM`/`CONTENT_TYPE_SONIFICATION`/`STREAM_ALARM`) + tombol Mute (audio berhenti, UI merah tetap); **haversine gating sebelum `startActivity()`/`MediaPlayer.start()`**.
- Manifest: `USE_FULL_SCREEN_INTENT`; API 34+ cek `Settings.canUseFullScreenIntent()` dengan fallback heads-up notification.
- `QuakeMessagingService` (FCM data-only) → `QuakeEmergencyService` (`EmergencyMessagingService`): parse payload sesuai `contracts/fcm/alert_payload.json` (semua string; `pga_gal` string→Double; `timestamp` string ms), dedup via `event_id`.
- `PUT /api/v1/users/fcm-token` di `onNewToken` + saat app start.
- Guard: bila `google-services.json`/FCM belum dikonfigurasi, service tidak crash (log + skip).

### Fase 6 — Sensors + WS dashboard
- `SensorsRepository`: `GET /api/v1/sensors?range_km=` → ganti mock 7 stasiun; status Online/Offline, `rssi_dbm`, `latency_ms`, `last_ping`.
- `RealtimeClient`: OkHttp WS `GET /ws` (header Bearer JWT); terima `AlertMessage` (json sama dengan `contracts/mqtt/alert.schema.json`); ekspos `StateFlow<AlertMessage?>` + koneksi state; reconnect dengan backoff; nonaktif bila mode dev/prod belum di-set.
- Tab Warning → dashboard: banner dari alert WS/FCM terbaru; dedup `event_id`; tombol See Details → Detail Event.
- Status koneksi ditampilkan di Settings §3D.

---

## 8. Verifikasi

| Level | Cara |
|---|---|
| Compile | `./gradlew :app:compileDebugKotlin` per fase |
| Lint | `./gradlew :app:lintDebug` per fase |
| Unit test | `Haversine` (jarak & gating), formatter satuan (gal↔g, ms epoch↔lokal), DTO parsing (depth_km null, resolved_at absen) |
| E2E REST | Server dev via `server/scripts/test_e2e_smoke.sh` (stack sudah terverifikasi 5/5 PASS); UI diuji terhadap `http://localhost:8080` (mode dev) |
| WS | Verifikasi manual dengan `websocat`/debug log saat alert dipublish (simulasi trigger sensor) |
| FCM | Butuh `google-services.json` + Firebase project; tanpa itu, verifikasi fallback (heads-up lokal) |

---

## 9. Risiko & Catatan

- **Cleartext dev:** `http://localhost:8080` butuh `usesCleartextTraffic=true` **khusus debug build** (produksi tetap HTTPS-only, ADR-0003). Implementasi: `debug` manifest overlay.
- **FCM belum terkonfigurasi:** `google-services.json` belum ada di repo → service dibuat dengan guard agar app tetap berjalan; dokumentasikan langkah setup Firebase.
- **Chat mesh:** UI dipertahankan apa adanya (no-op); tandai sebagai out-of-scope di `docs/GAP_ANALYSIS.md` (server tidak punya kanal chat).
- **Semantik PUT location:** mengirim body tanpa `location_name` akan mengosongkan label tersimpan (CLIENT_SPEC §3) — klien wajib selalu mengirim label saat update.
- **Depth tidak tersedia:** `depth_km` selalu `null` — jangan tampilkan sebagai 0 km (CLIENT_SPEC §7).
- **Centroid ≠ episenter:** label UI wajib menyebut "centroid terdeteksi", bukan "episenter".
- **Process death:** semua state penting (session, settings, radius) harus di DataStore; state tab via Navigation Compose/`rememberSaveable`.

---

## 10. Referensi Kontrak (pemetaan)

| Fitur | Kontrak | Endpoint/Sumber |
|---|---|---|
| Auth anonymous | CLIENT_SPEC §2 | `POST /api/v1/auth/anonymous` |
| Header JWT | CLIENT_SPEC §3 | `Authorization: Bearer <jwt>` |
| Lokasi user | CLIENT_SPEC §4.2 | `PUT /api/v1/users/location` |
| FCM token | CLIENT_SPEC §4.3 | `PUT /api/v1/users/fcm-token` |
| Riwayat event | CLIENT_SPEC §4.4 | `GET /api/v1/events` (limit/offset/range_km) |
| Alert payload | CLIENT_SPEC §5 + `contracts/mqtt/alert.schema.json` | WS `/ws` (aktif), FCM data-only, MQTT `alerts/earthquake` (forward-looking) |
| Error mapping | CLIENT_SPEC §6 | `INVALID_ARGUMENT` 400, `UNAUTHENTICATED` 401, `RATE_LIMITED` 429, `INTERNAL` 500 |
| Satuan kanonik | CLIENT_SPEC §7 | gal, ms epoch UTC, km, WGS84, `depth_km` null |
| Sensor list | openapi `GET /api/v1/sensors` | `range_km`, status, RSSI, latency |
| Reroll pseudonym | openapi `POST /api/v1/users/pseudonym/reroll` | rate-limit 1x/60s |