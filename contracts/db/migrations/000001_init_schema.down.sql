-- ============================================================================
-- QuakeAlert — Migration 000001 (DOWN): Rollback Initial Schema
-- Urutan drop menghormati foreign key (chat_messages -> user_profiles).
-- ============================================================================

DROP TABLE IF EXISTS chat_messages;
DROP TABLE IF EXISTS earthquake_events;
DROP TABLE IF EXISTS user_profiles;
DROP TABLE IF EXISTS iot_nodes;

-- Ekstensi sengaja TIDAK di-drop (postgis/uuid-ossp bisa dipakai objek lain).
