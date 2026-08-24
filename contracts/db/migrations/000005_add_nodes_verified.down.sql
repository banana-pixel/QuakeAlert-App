-- ============================================================================
-- QuakeAlert — Migration 000005 (DOWN): hapus verifikasi node.
-- Perhatian: menjalankan DOWN lalu UP lagi mengembalikan SEMUA node ke
-- verified = false — mereka harus diverifikasi ulang oleh operator.
-- ============================================================================

DROP INDEX IF EXISTS idx_nodes_unverified;
ALTER TABLE iot_nodes DROP COLUMN IF EXISTS verified;
