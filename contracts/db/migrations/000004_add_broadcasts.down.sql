-- ============================================================================
-- QuakeAlert — Migration 000004 (DOWN): Siaran Admin
-- Mengembalikan skema ke keadaan 000003. Isi tabel ikut hilang: siaran adalah
-- riwayat pengumuman, bukan data keselamatan, dan tidak ada tabel lain yang
-- merujuknya.
-- ============================================================================

DROP INDEX IF EXISTS idx_broadcasts_region;
DROP INDEX IF EXISTS idx_broadcasts_created;
DROP TABLE IF EXISTS broadcasts;
