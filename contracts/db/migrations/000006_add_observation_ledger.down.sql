-- ============================================================================
-- QuakeAlert — Migration 000006 (DOWN): hapus observation ledger.
--
-- Rollback LENGKAP: migrasi 000006 tidak menambah kolom pada tabel mana pun yang
-- sudah ada, dan tidak ada kode di luar internal/ledger yang membaca kedua tabel
-- ini. Menjatuhkannya hanya menghilangkan data ledger — jalur peringatan,
-- earthquake_events, iot_nodes, user_profiles, dan chat_messages tidak
-- tersentuh. Itulah properti yang membuat Fase 1 dapat dibatalkan.
--
-- Indeks ikut terhapus bersama tabelnya (DROP TABLE menjatuhkan indeksnya).
-- ============================================================================

DROP TABLE IF EXISTS alert_emissions;
DROP TABLE IF EXISTS sensor_observations;
