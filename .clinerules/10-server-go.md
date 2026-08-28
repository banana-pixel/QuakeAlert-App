# 10 — Server (Go) Rules

Backend monolith Go 1.22+ di `server/`. Berjalan di box 1 vCPU / 1 GB. Lihat ADR-0001 & ADR-0002.

## Wajib

1. **DB:** `jackc/pgx/v5` + prepared statement. Pool: `MaxConns=8`, `MinConns=2`, `MaxConnIdleTime=5m`. Query spasial pakai PostGIS (`ST_DWithin`, `GEOGRAPHY`). **Dilarang GORM / ORM refleksi.**
2. **Logging:** `log/slog` stdlib (JSON handler di produksi). Tanpa zap/logrus.
3. **Context:** Semua IO (Redis, PostGIS, MQTT) membawa `context.Context` timeout ≤ 2s.
4. **Graceful shutdown:** Tangani `SIGTERM`/`SIGINT`, drain WS & MQTT, tutup pool pgx sebelum exit.
5. **Memory:** Batasi alokasi di hot loop subscriber MQTT; `sync.Pool` untuk buffer bila profiling menunjukkan tekanan GC. Target < ~256 MB RSS.
6. **Konsensus (Phase 3):** Window `CORRELATION_WINDOW_MS`. Gerbang CONFIRMED: PGA ≥ `MIN_PGA_GAL` (16.6 gal) AND ≥ `MIN_NODES_CONFIRMED` (3) kontributor unik AND ≥ `MIN_INDEPENDENT_CELLS` (2) kontributor saling independen dengan jarak geodesik ≥ `INDEPENDENCE_CELL_KM` (5 km). Di bawah gerbang: UNCONFIRMED (WebSocket saja). Mesin negara lima state: DETECTED → UNCONFIRMED → CONFIRMED → RESOLVED / CANCELLED. Weighted centroid global (antimeridian-safe) = estimasi lokasi (BUKAN episenter).
7. **Keamanan:** Verifikasi HMAC-SHA256 setiap trigger sebelum diproses. Tolak `ts` menyimpang >30s & `ts` ≤ `last_seen_ts`. Secret di-decrypt dari `secret_key_enc` (AES-GCM).
8. **Test:** Unit test untuk verifikasi HMAC, konsensus, centroid, konversi MMI.

## Struktur disarankan

```
server/
  cmd/quakealert/main.go
  internal/ingest/     # MQTT subscriber + HMAC verify
  internal/consensus/  # spatial engine + centroid + MMI
  internal/dispatch/   # FCM + WebSocket hub
  internal/api/        # REST handlers (chi)
  internal/store/      # pgx repositories
  go.mod
  Dockerfile
```
