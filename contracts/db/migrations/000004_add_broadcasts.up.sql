-- ============================================================================
-- QuakeAlert — Migration 000004 (UP): Siaran Admin
-- Pengumuman operator yang dikirim sebagai push DAN tersimpan agar dapat dibaca
-- ulang di dalam aplikasi. Disimpan lebih dulu, baru difanout: siaran yang
-- terkirim tetapi tidak tersimpan tidak dapat dibaca lagi setelah notifikasinya
-- disapu dari shade, dan pengumuman yang tidak bisa dibaca ulang sama saja
-- dengan tidak pernah dikirim.
--
-- Aditif & idempoten: aman dijalankan pada database yang sudah berisi data.
-- ============================================================================

CREATE TABLE IF NOT EXISTS broadcasts (
    broadcast_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    title        VARCHAR(120) NOT NULL,
    body         VARCHAR(500) NOT NULL,
    -- NULL = nasional (seluruh pengguna). Bila terisi, nilainya adalah
    -- user_profiles.region_code — kunci yang sama yang dipakai kanal chat
    -- regional, jadi penargetan siaran dan keanggotaan ruang tidak pernah
    -- berbeda pendapat tentang apa itu "Jawa Barat".
    region_code  VARCHAR(64),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Daftar "Pembaruan" di aplikasi selalu dibaca terbaru-dulu.
CREATE INDEX IF NOT EXISTS idx_broadcasts_created ON broadcasts(created_at DESC);

-- Satu pengguna hanya melihat siaran nasional + siaran wilayahnya sendiri;
-- indeks ini melayani separuh regional dari kueri itu.
CREATE INDEX IF NOT EXISTS idx_broadcasts_region
    ON broadcasts(region_code, created_at DESC)
    WHERE region_code IS NOT NULL;
