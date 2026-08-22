-- ============================================================================
-- QuakeAlert — Migration 000003 (UP): Community Chat
-- Melengkapi tabel chat_messages yang sudah ada sejak 000001 dengan hal-hal yang
-- dibutuhkan kanal server-backed (docs/CHAT_DESIGN.md):
--   1. user_profiles.region_code — kunci kanal regional, diturunkan dari
--      reverse-geocode yang sudah mengisi location_name (000002).
--   2. chat_channels — nama tampilan kanal, agar semua anggota melihat satu
--      judul yang sama alih-alih ejaan masing-masing ponsel.
--   3. chat_messages.client_message_id — idempotensi pengiriman: retry setelah
--      jaringan putus harus mengembalikan pesan yang sama, bukan duplikat.
-- Aditif & idempoten: aman dijalankan pada database yang sudah berisi data.
-- ============================================================================

-- 1. Kunci kanal regional. VARCHAR(64) menampung '<ISO2>-<admin1-slug>'
--    (mis. 'ID-jawa-barat'). Nullable: tanpa fix lokasi user hanya punya Global.
ALTER TABLE user_profiles
    ADD COLUMN IF NOT EXISTS region_code VARCHAR(64);

-- Kanal regional dicari lewat kolom ini setiap kali seseorang membuka Chat.
CREATE INDEX IF NOT EXISTS idx_users_region ON user_profiles(region_code);

-- 2. Katalog kanal. kind membedakan satu-satunya kanal GLOBAL dari kanal
--    REGIONAL yang dibuat saat pertama dipakai.
CREATE TABLE IF NOT EXISTS chat_channels (
    channel_id   VARCHAR(50) PRIMARY KEY,
    kind         VARCHAR(16) NOT NULL,          -- 'GLOBAL' | 'REGIONAL'
    display_name VARCHAR(80) NOT NULL,
    created_at   TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT chat_channels_kind_check CHECK (kind IN ('GLOBAL', 'REGIONAL'))
);

-- Kanal global selalu ada: satu-satunya kanal yang berfungsi sebelum ada fix
-- lokasi, jadi ia tidak boleh bergantung pada penulis pertama.
INSERT INTO chat_channels (channel_id, kind, display_name)
VALUES ('global', 'GLOBAL', 'Global')
ON CONFLICT (channel_id) DO NOTHING;

-- 3. Idempotensi pengiriman. Nullable karena baris lama (dan pesan admin yang
--    mungkin ditulis server-side) tidak punya id dari klien.
ALTER TABLE chat_messages
    ADD COLUMN IF NOT EXISTS client_message_id UUID;

-- Partial unique: hanya baris yang benar-benar membawa client_message_id ikut
-- dibatasi, sehingga NULL berganda tetap sah.
CREATE UNIQUE INDEX IF NOT EXISTS idx_chat_client_msg
    ON chat_messages(sender_id, client_message_id)
    WHERE client_message_id IS NOT NULL;
