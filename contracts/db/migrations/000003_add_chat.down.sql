-- ============================================================================
-- QuakeAlert — Migration 000003 (DOWN): Community Chat
-- Mengembalikan skema ke keadaan 000002. chat_messages TIDAK di-drop: tabel itu
-- lahir di 000001, jadi bukan milik migrasi ini — hanya kolom yang ditambahkan
-- di sini yang dilepas.
-- ============================================================================

DROP INDEX IF EXISTS idx_chat_client_msg;
ALTER TABLE chat_messages DROP COLUMN IF EXISTS client_message_id;

DROP TABLE IF EXISTS chat_channels;

DROP INDEX IF EXISTS idx_users_region;
ALTER TABLE user_profiles DROP COLUMN IF EXISTS region_code;
