# ADR-0002: Go Backend — pgx Tanpa ORM, slog, Concurrency Terkendali

- **Status:** Accepted
- **Tanggal:** 2026-08-18

## Konteks

Backend berjalan di box 1 GB RAM bersama Postgres/Redis/Mosquitto. Hot path adalah subscriber MQTT yang mem-parsing trigger, memverifikasi HMAC, dan menjalankan konsensus spasial. Alokasi berlebih dan refleksi runtime akan menekan GC dan memori.

## Keputusan

1. **Akses DB pakai `jackc/pgx/v5`** (bukan `database/sql` generik, bukan GORM/ORM refleksi). Prepared statements + pool terbatas:
   - `MaxConns=8`, `MinConns=2`, `MaxConnIdleTime=5m`.
   - Query spasial memakai PostGIS langsung (`ST_DWithin`, `GEOGRAPHY`).
2. **Logging pakai `log/slog`** (stdlib), JSON handler di produksi. Tanpa zap/logrus.
3. **Concurrency:** setiap IO membawa `context.Context` dengan timeout ≤ 2s. Worker pool tetap untuk ingest MQTT; hindari goroutine tak terbatas.
4. **Alokasi:** `sync.Pool` untuk buffer JSON di hot loop bila profiling menunjukkan tekanan GC. Hindari `encoding/json` reflection berlebih pada struct besar; struct payload dijaga kecil dan sesuai `/contracts`.
5. **HTTP/WS:** stdlib `net/http` + router ringan (mis. `chi`) diperbolehkan; framework berat (Gin+deps besar) dihindari kecuali ada justifikasi.

## Konsekuensi

- (+) Footprint kecil, kontrol penuh atas query & alokasi.
- (+) Query PostGIS eksplisit lebih mudah dioptimasi (indeks GiST).
- (−) Lebih banyak boilerplate SQL manual dibanding ORM.
- Boilerplate ditekan dengan code-gen kontrak (lihat ADR-0004) bila diperlukan.

## Alternatif ditolak

- **GORM:** refleksi runtime, overhead memori & alokasi tinggi, query spasial PostGIS canggung.
