-- ---------------------------------------------------------------------------
-- QuakeAlert Fase 1 — kueri analisis observation ledger (§16.2)
--
-- Semua metrik Fase 1 diturunkan dengan KUERI, bukan sistem metrik: tidak ada
-- endpoint /metrics dan tidak ada /observations di Fase 1 (§21). Jalankan berkas
-- ini terhadap basis data setelah migrasi 000006:
--
--     psql "$DATABASE_URL" -f server/scripts/ledger_analysis.sql
--
-- Setiap kueri berdiri sendiri (tidak ada CTE bersama, tidak ada tabel
-- sementara) supaya satu kueri dapat disalin keluar tanpa menyeret yang lain.
--
-- Jendela default: 7 hari terakhir menurut received_ts. received_ts adalah jam
-- SERVER; publish_ts adalah jam NODE. Setiap metrik di bawah menyebut jam mana
-- yang dipakainya, karena mencampur keduanya menghasilkan angka yang terlihat
-- seperti latensi tetapi sebenarnya drift NTP.
--
-- DUA COUNTER TIDAK ADA DI SINI, dan tidak bisa ada: ledger_drops_total dan
-- ledger_unknown_node_rejections_total menghitung hal-hal yang secara definisi
-- TIDAK punya baris (observasi yang dibuang antrean berbatas, dan penolakan dari
-- node_id yang tidak dikenal). Keduanya hanya ada sebagai log proses. Bila
-- ledger_drops_total bukan nol, setiap angka di bawah ini TIDAK LENGKAP.
-- ---------------------------------------------------------------------------

\timing on

-- ===========================================================================
-- 1. Distribusi latensi transportasi per node: received_ts - publish_ts
--
-- Mencampur dua jam dengan sengaja — itulah satu-satunya cara mengukur latensi
-- dari data yang ada — sehingga nilai NEGATIF bukan mustahil dan berarti jam
-- node mendahului jam server, bukan pengiriman yang lebih cepat dari cahaya.
-- Verifier menerima -30 s..+5 menit, jadi rentang inilah batas yang mungkin.
-- ===========================================================================
SELECT
    node_id,
    count(*)                                                              AS n,
    percentile_disc(0.50) WITHIN GROUP (ORDER BY received_ts - publish_ts) AS p50_ms,
    percentile_disc(0.90) WITHIN GROUP (ORDER BY received_ts - publish_ts) AS p90_ms,
    percentile_disc(0.99) WITHIN GROUP (ORDER BY received_ts - publish_ts) AS p99_ms,
    min(received_ts - publish_ts)                                         AS min_ms,
    max(received_ts - publish_ts)                                         AS max_ms
FROM sensor_observations
WHERE verify_result = 'OK'
  AND received_ts >= (extract(epoch FROM now() - interval '7 days') * 1000)::bigint
GROUP BY node_id
ORDER BY p99_ms DESC NULLS LAST;

-- ===========================================================================
-- 2. Latensi keputusan: decided_at - received_ts
--
-- Kedua nilai berasal dari jam SERVER, jadi selisihnya adalah waktu server yang
-- sebenarnya. Emisi dipasangkan dengan observasi terakhir yang mendahuluinya di
-- dalam jendela konsensus (8 s bawaan), bukan lewat kunci asing: baris emisi
-- sengaja tidak menyimpan observasi mana yang memicunya (D12 —
-- correlation_key dihitung, tidak disimpan).
-- ===========================================================================
SELECT
    e.emission_id,
    e.status,
    e.node_count,
    e.audience,
    e.decided_at - (
        SELECT max(o.received_ts)
        FROM sensor_observations o
        WHERE o.verify_result = 'OK'
          AND o.received_ts <= e.decided_at
          AND o.received_ts >  e.decided_at - 8000
    ) AS decide_lag_ms
FROM alert_emissions e
WHERE e.decided_at >= (extract(epoch FROM now() - interval '7 days') * 1000)::bigint
ORDER BY e.decided_at DESC;

-- ===========================================================================
-- 3. Kegagalan verifikasi per alasan, per node, per hari
--
-- verify_result mencatat OTENTIKASI saja. Kegagalan infrastruktur (basis data
-- tak terjangkau, dekripsi secret gagal) TIDAK pernah menjadi baris, jadi tidak
-- ada node yang tampak buruk di sini karena kesalahan server.
-- ===========================================================================
SELECT
    date_trunc('day', to_timestamp(received_ts / 1000.0)) AS day,
    node_id,
    verify_result,
    count(*) AS n
FROM sensor_observations
WHERE verify_result <> 'OK'
  AND received_ts >= (extract(epoch FROM now() - interval '7 days') * 1000)::bigint
GROUP BY day, node_id, verify_result
ORDER BY day DESC, n DESC;

-- ===========================================================================
-- 4. Observasi per node per hari (diterima vs ditolak)
--
-- Baseline volume. Node yang mendadak sepi sama informatifnya dengan node yang
-- mendadak ramai; keduanya tidak terlihat dari hitungan agregat saja.
-- ===========================================================================
SELECT
    date_trunc('day', to_timestamp(received_ts / 1000.0)) AS day,
    node_id,
    count(*)                                     AS total,
    count(*) FILTER (WHERE verify_result = 'OK') AS accepted,
    count(*) FILTER (WHERE verify_result <> 'OK') AS rejected,
    sum(suppressed_rejections)                   AS suppressed_carried,
    max(pga_gal)                                 AS max_pga_gal
FROM sensor_observations
WHERE received_ts >= (extract(epoch FROM now() - interval '7 days') * 1000)::bigint
GROUP BY day, node_id
ORDER BY day DESC, total DESC;

-- ===========================================================================
-- 5. Hitungan emisi: ADVISORY vs CONFIRMED, dan audiens yang benar-benar dipakai
--
-- audience = 'GEO_TOPIC_ALL' adalah baris yang paling perlu diawasi: itulah
-- jalur siar-luas. node_count = 1 bersama GEO_TOPIC_ALL berarti gerbang node
-- tunggal (D6) tidak aktif ketika keputusan itu dibuat.
-- ===========================================================================
SELECT
    status,
    audience,
    is_severe,
    count(*)         AS n,
    min(node_count)  AS min_nodes,
    max(node_count)  AS max_nodes,
    max(pga_gal)     AS max_pga_gal
FROM alert_emissions
WHERE decided_at >= (extract(epoch FROM now() - interval '7 days') * 1000)::bigint
GROUP BY status, audience, is_severe
ORDER BY n DESC;

-- Khusus: emisi siar-luas dari node tunggal. Idealnya nol baris.
SELECT emission_id, decided_at, status, node_count, pga_gal, mmi
FROM alert_emissions
WHERE audience = 'GEO_TOPIC_ALL'
  AND node_count <= 1
ORDER BY decided_at DESC;

-- ===========================================================================
-- 6. Observasi otentik yang tidak memengaruhi emisi apa pun
--
-- "Tidak memengaruhi" di sini berarti tidak ada baris emisi dalam satu jendela
-- konsensus setelah observasi itu tiba. Ini pendekatan berbasis waktu, bukan
-- bukti kausal: baris emisi tidak menyimpan observasi penyumbangnya (D12).
-- Sebuah observasi bisa muncul di sini karena PGA-nya di bawah MinPGAGal,
-- karena ia sendirian di luar radius kluster, atau karena selnya masih dalam
-- cooldown.
-- ===========================================================================
SELECT
    o.node_id,
    count(*) AS influenced_nothing
FROM sensor_observations o
WHERE o.verify_result = 'OK'
  AND o.received_ts >= (extract(epoch FROM now() - interval '7 days') * 1000)::bigint
  AND NOT EXISTS (
      SELECT 1
      FROM alert_emissions e
      WHERE e.decided_at >= o.received_ts
        AND e.decided_at <  o.received_ts + 8000
  )
GROUP BY o.node_id
ORDER BY influenced_nothing DESC;

-- ===========================================================================
-- 7. A16 — observasi otentik yang HILANG karena pencarian lokasi gagal
--
-- Dikodekan sebagai verify_result = 'OK' DENGAN node_location NULL. Sengaja
-- BUKAN nilai verify_result tersendiri: observasinya memang sah: yang tidak ada
-- hanyalah koordinat yang dibutuhkan konsensus. Setiap baris di sini adalah satu
-- suara yang tidak pernah sampai ke ambang 3-node.
-- ===========================================================================
SELECT
    date_trunc('day', to_timestamp(received_ts / 1000.0)) AS day,
    node_id,
    count(*) AS lost_to_location_lookup
FROM sensor_observations
WHERE verify_result = 'OK'
  AND node_location IS NULL
  AND received_ts >= (extract(epoch FROM now() - interval '7 days') * 1000)::bigint
GROUP BY day, node_id
ORDER BY day DESC, lost_to_location_lookup DESC;

-- Total tunggal, untuk kriteria keluar Fase 1 (§18).
SELECT count(*) AS a16_total_lost
FROM sensor_observations
WHERE verify_result = 'OK'
  AND node_location IS NULL
  AND received_ts >= (extract(epoch FROM now() - interval '7 days') * 1000)::bigint;

-- ===========================================================================
-- 8. publish_ts - onset_ts_upper_bound per node
--
-- Ini SAMA DENGAN dur_ms menurut konstruksi (onset_ts_upper_bound =
-- publish_ts - dur_ms), dan itu memang intinya: satu-satunya pegangan Fase 1
-- atas publish delay adalah BATAS-nya, bukan nilainya. publish_delay >= 0 tidak
-- terbatas dan tidak terobservasi karena ts distempel ULANG pada setiap retry
-- dan payload v1 tidak membawa nomor percobaan (§5.1).
--
-- Boleh dipakai untuk mengurutkan dan mengelompokkan. TIDAK boleh dipakai untuk
-- mengkalibrasi jendela korelasi berbasis onset — itu pekerjaan Fase 2, setelah
-- kabelnya membawa attempt_no.
-- ===========================================================================
SELECT
    node_id,
    count(*) FILTER (WHERE onset_ts_upper_bound IS NOT NULL) AS n_bounded,
    percentile_disc(0.50) WITHIN GROUP (ORDER BY publish_ts - onset_ts_upper_bound) AS p50_ms,
    percentile_disc(0.90) WITHIN GROUP (ORDER BY publish_ts - onset_ts_upper_bound) AS p90_ms,
    max(publish_ts - onset_ts_upper_bound)                   AS max_ms
FROM sensor_observations
WHERE verify_result = 'OK'
  AND onset_ts_upper_bound IS NOT NULL
  AND received_ts >= (extract(epoch FROM now() - interval '7 days') * 1000)::bigint
GROUP BY node_id
ORDER BY max_ms DESC;

-- ===========================================================================
-- 9. Sanity ledger: apakah kesimpulan apa pun dari berkas ini boleh dipercaya?
--
-- Jawaban lengkapnya membutuhkan ledger_drops_total dari log proses, yang tidak
-- dapat dikueri. Yang bisa dilihat dari SQL hanyalah cakupan waktu dan celah
-- besar di antara baris — celah yang tidak dapat dijelaskan patut dicocokkan
-- dengan log restart server.
-- ===========================================================================
SELECT
    min(to_timestamp(received_ts / 1000.0)) AS earliest,
    max(to_timestamp(received_ts / 1000.0)) AS latest,
    count(*)                                AS rows_total,
    count(DISTINCT node_id)                 AS nodes_seen,
    sum(suppressed_rejections)              AS rejections_suppressed_by_limiter
FROM sensor_observations;
