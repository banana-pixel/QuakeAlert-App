-- ============================================================================
-- QuakeAlert — Migration 000007 (UP): Observation Provenance (Fase 2)
--
-- Enam kolom baru pada dua tabel ledger, nol tabel baru, nol perubahan pada
-- tabel di luar ledger. Kolom-kolom ini SENGAJA tidak dibuat pada migrasi 000006:
-- protokol v1 tidak dapat mengisinya sama sekali, dan kolom yang dijamin NULL di
-- setiap baris hanya menciptakan penampilan data (§5.4).
--
--   sensor_observations.attempt_no   — indeks percobaan kirim (1 = publikasi
--       pertama). Tanpa ini publish_ts tidak dapat ditafsirkan: laporan yang
--       diulang tidak dapat dikeluarkan dari analisis timing, dan kehilangan
--       paket tidak dapat dibedakan dari backlog broker.
--   sensor_observations.detrigger_ts — instan event ditutup menurut jam sensor.
--       Bersama onset_ts membuat dur_ms dapat diperiksa sendiri, dan memisahkan
--       "event berakhir" dari "laporan terkirim".
--
--   alert_emissions.{ws_client_count, fcm_attempted, fcm_succeeded, delivery_at}
--       — hasil PENGIRIMAN, terpisah dari KEPUTUSAN yang sudah dicatat 000006.
--       delivery_at NULL berarti hasil pengiriman tidak pernah dilaporkan; itu
--       sendiri sebuah temuan, bukan data yang hilang.
--
-- Aditif & idempoten (IF NOT EXISTS di setiap kolom): aman dijalankan pada
-- database yang sudah berisi data, dan aman dijalankan dua kali.
--
-- TIDAK ADA constraint NOT NULL dan TIDAK ADA nilai bawaan: node v1 tidak
-- mengirim attempt_no maupun detrigger_ts, observasi PRELIM tidak punya
-- detrigger_ts sama sekali (event-nya belum berakhir), dan sebuah default akan
-- mengubah "tidak diketahui" menjadi angka yang terlihat terukur.
-- ============================================================================

ALTER TABLE sensor_observations
    ADD COLUMN IF NOT EXISTS attempt_no    SMALLINT,   -- NULL untuk v1 (§5.2)
    ADD COLUMN IF NOT EXISTS detrigger_ts  BIGINT;     -- NULL untuk v1 dan PRELIM

ALTER TABLE alert_emissions
    ADD COLUMN IF NOT EXISTS ws_client_count INTEGER,
    ADD COLUMN IF NOT EXISTS fcm_attempted   INTEGER,
    ADD COLUMN IF NOT EXISTS fcm_succeeded   INTEGER,
    ADD COLUMN IF NOT EXISTS delivery_at     BIGINT;   -- kapan hasil kirim diisi
