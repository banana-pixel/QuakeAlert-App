# ADR-0001: Monorepo Layout & Resource Budget (1 vCPU / 1 GB VPS)

- **Status:** Accepted
- **Tanggal:** 2026-08-18
- **Konteks keputusan:** Fase 1 restrukturisasi repo.

## Konteks

Ekosistem QuakeAlert terdiri dari 4 komponen yang saling terkait lewat kontrak yang sama (payload MQTT, REST, FCM, DDL): aplikasi Android, backend Go, firmware ESP32, dan artefak kontrak. Target deploy adalah **satu VPS 1 vCPU / 1 GB RAM** yang menjalankan Postgres+PostGIS, Redis, Mosquitto, dan binary Go secara bersamaan.

## Keputusan

1. **Monorepo** dengan direktori top-level:
   - `android/` — aplikasi Kotlin/Compose (dipindahkan dari root).
   - `server/` — monolith Go, module `github.com/banana-pixel/quakealert/server`.
   - `firmware/` — proyek PlatformIO ESP32.
   - `contracts/` — OpenAPI, JSON Schema MQTT/FCM, DDL + migrasi (sumber kebenaran).
   - `deploy/` — docker-compose, konfigurasi Mosquitto/Postgres/Redis.
   - `docs/` — spec, gap analysis, ADR.

2. **Anggaran memori** (indikatif, wajib divalidasi via `docker stats`):
   - PostgreSQL+PostGIS: ~256 MB (`shared_buffers=128MB`, `work_mem` kecil).
   - Redis: `maxmemory 128mb`, `maxmemory-policy allkeys-lru`.
   - Mosquitto: ~32 MB.
   - Go binary: target < 256 MB RSS.
   - Sisa untuk OS & buffer.

## Konsekuensi

- (+) Kontrak tunggal, perubahan lintas komponen atomik dalam satu commit/PR.
- (+) CI dapat memvalidasi drift kontrak vs implementasi.
- (−) Repo lebih besar; perlu path-based tooling (mis. build hanya folder yang berubah).
- Karena batasan RAM, ORM refleksi (GORM) dan library berat dilarang (lihat ADR-0002).

## Alternatif ditolak

- **Polyrepo:** sinkronisasi kontrak antar-repo rawan drift dan versi tak sinkron.
