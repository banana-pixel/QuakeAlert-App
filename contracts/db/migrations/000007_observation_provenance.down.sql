-- ============================================================================
-- QuakeAlert — Migration 000007 (DOWN): hapus kolom provenance Fase 2.
--
-- Rollback LENGKAP dengan alasan yang sama seperti 000006: keenam kolom hanya
-- dibaca dan ditulis oleh internal/ledger dan internal/store. Menjatuhkannya
-- mengembalikan skema ledger ke bentuk Fase 1 — kedua tabel tetap ada beserta
-- seluruh barisnya, hanya kolom provenance yang hilang.
--
-- HARUS dipasangkan dengan rollback KODE. Biner Fase 2 menyebut keenam kolom ini
-- secara eksplisit di INSERT/UPDATE-nya, jadi menjalankan down ini sementara
-- biner Fase 2 masih berjalan akan membuat setiap penulisan ledger gagal.
-- Kegagalannya terbatas pada ledger — penulisan ledger asinkron dan di luar
-- jalur peringatan (internal/ledger), sehingga peringatan tetap terkirim
-- sementara pencatatannya berhenti — tetapi urutan yang benar tetap: turunkan
-- biner ke Fase 1 lebih dulu, baru jalankan migrasi ini.
--
-- Node v2 yang masih memublikasikan payload proto_ver=2 tidak terpengaruh
-- migrasi ini sama sekali: verifikasi terjadi di memori, bukan di skema.
-- ============================================================================

ALTER TABLE sensor_observations
    DROP COLUMN IF EXISTS attempt_no,
    DROP COLUMN IF EXISTS detrigger_ts;

ALTER TABLE alert_emissions
    DROP COLUMN IF EXISTS ws_client_count,
    DROP COLUMN IF EXISTS fcm_attempted,
    DROP COLUMN IF EXISTS fcm_succeeded,
    DROP COLUMN IF EXISTS delivery_at;
