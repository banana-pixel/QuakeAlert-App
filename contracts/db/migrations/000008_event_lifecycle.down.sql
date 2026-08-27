-- ============================================================================
-- QuakeAlert — Migration 000008 (DOWN): hapus lifecycle event Fase 3.
--
-- Rollback LENGKAP. event_state_log dijatuhkan LEBIH DAHULU: ia memegang FK ke
-- earthquake_events, dan urutan sebaliknya akan gagal.
--
-- Menjatuhkan kolom-kolom ini mengembalikan skema ke bentuk Fase 2 —
-- earthquake_events dan alert_emissions tetap ada beserta seluruh barisnya,
-- hanya kolom lifecycle yang hilang. `status` tidak tersentuh migrasi ini,
-- sehingga umpan REST publik tetap dapat dibaca biner Fase 2 tanpa perubahan.
--
-- HARUS dipasangkan dengan rollback KODE, dan dalam urutan itu: biner Fase 3
-- menyebut kolom-kolom ini secara eksplisit di UpsertEvent dan AppendStateLog,
-- jadi menjalankan down ini sementara biner Fase 3 masih hidup akan membuat
-- setiap penulisan event gagal. Kegagalannya terbatas pada PENCATATAN —
-- persistensi event asinkron dan berada SETELAH emisi (§9.5), sehingga
-- peringatan tetap terkirim sementara pencatatannya berhenti — tetapi urutan
-- yang benar tetap: turunkan biner ke Fase 2 lebih dulu (atau setel
-- EVENT_TRACKER_ENABLED=false), baru jalankan migrasi ini.
--
-- Yang hilang permanen: seluruh riwayat transisi state. Tidak ada tempat lain
-- yang menyimpannya. Itu konsekuensi yang dimaksud dari sebuah rollback, bukan
-- efek samping yang terlewat.
-- ============================================================================

DROP TABLE IF EXISTS event_state_log;

DROP INDEX IF EXISTS idx_events_origin_ts;
DROP INDEX IF EXISTS idx_events_state;

ALTER TABLE earthquake_events
    DROP COLUMN IF EXISTS event_state,
    DROP COLUMN IF EXISTS revision,
    DROP COLUMN IF EXISTS origin_ts,
    DROP COLUMN IF EXISTS origin_ts_source,
    DROP COLUMN IF EXISTS independent_cell_count,
    DROP COLUMN IF EXISTS algo_ver;

ALTER TABLE alert_emissions
    DROP COLUMN IF EXISTS event_state,
    DROP COLUMN IF EXISTS event_revision;

-- alert_emissions.algo_ver DIBIARKAN pada VARCHAR(30) dan TIDAK dipersempit
-- kembali ke VARCHAR(16). Dua alasan, keduanya mengarah ke sisi yang sama:
-- mempersempit akan GAGAL bila satu baris saja sudah menyimpan nilai lebih
-- panjang, sehingga rollback yang seharusnya mekanis menjadi rollback yang
-- macet di tengah; dan kolom yang lebih lebar tidak terlihat berbeda bagi biner
-- Fase 1/Fase 2, yang menulis 10 karakter dan membaca string. Kelonggaran tipe
-- adalah satu-satunya jejak Fase 3 yang sengaja ditinggalkan migrasi ini.
