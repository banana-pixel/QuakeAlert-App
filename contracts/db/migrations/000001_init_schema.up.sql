-- ============================================================================
-- QuakeAlert — Migration 000001 (UP): Initial Schema
-- PostgreSQL 16 + PostGIS.
-- Satuan kanonik: PGA=gal (NUMERIC(8,4)), ts anti-replay=ms epoch UTC (BIGINT),
-- lokasi=GEOGRAPHY(Point,4326), timestamp DB=TIMESTAMPTZ (UTC).
-- Kontrak ini adalah sumber kebenaran (contract-first, ADR-0004).
-- ============================================================================

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS postgis;

-- ----------------------------------------------------------------------------
-- 1. Tabel Master Sensor Node
--    Secret HMAC disimpan TERENKRIPSI-REVERSIBEL (AES-256-GCM), BUKAN hash,
--    karena verifikasi HMAC butuh key mentah. secret_key_enc = ciphertext,
--    secret_key_nonce = nonce/IV 12 byte GCM. Lihat ADR-0003 & SYSTEM_SPEC Bab 4.
-- ----------------------------------------------------------------------------
CREATE TABLE iot_nodes (
    station_id        VARCHAR(32) PRIMARY KEY,
    sensor_model      VARCHAR(32) DEFAULT 'MPU 6050',
    location_name     VARCHAR(150) NOT NULL,
    location          GEOGRAPHY(Point, 4326) NOT NULL,
    secret_key_enc    BYTEA NOT NULL,
    secret_key_nonce  BYTEA NOT NULL,
    key_version       INT NOT NULL DEFAULT 1,
    is_active         BOOLEAN DEFAULT TRUE,
    last_rssi         INT DEFAULT 0,
    last_latency_ms   INT DEFAULT 0,
    last_seen_ts      BIGINT DEFAULT 0,          -- ms epoch UTC, anti-replay
    last_heartbeat    TIMESTAMPTZ DEFAULT NOW(),
    created_at        TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_nodes_spatial ON iot_nodes USING GIST(location);

-- ----------------------------------------------------------------------------
-- 2. Profil Pengguna (Anonymous JWT)
-- ----------------------------------------------------------------------------
CREATE TABLE user_profiles (
    user_id             UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    pseudonym           VARCHAR(32) NOT NULL,          -- e.g., 'Quakezen-7B9A'
    last_location       GEOGRAPHY(Point, 4326),
    coverage_radius_km  INT DEFAULT 50,                -- default konservatif
    is_admin            BOOLEAN DEFAULT FALSE,
    fcm_token           VARCHAR(255),
    created_at          TIMESTAMPTZ DEFAULT NOW(),
    last_active         TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_users_spatial ON user_profiles USING GIST(last_location);

-- ----------------------------------------------------------------------------
-- 3. Riwayat Kejadian Gempa
--    estimated_centroid = pusat massa stasiun pemicu (BUKAN episenter presisi).
--    max_pga NUMERIC(8,4) dalam gal (satuan kanonik).
-- ----------------------------------------------------------------------------
CREATE TABLE earthquake_events (
    event_id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    status                VARCHAR(20) DEFAULT 'HAPPENING',    -- 'HAPPENING' | 'RESOLVED'
    estimated_centroid    GEOGRAPHY(Point, 4326) NOT NULL,
    location_name         VARCHAR(150) NOT NULL,
    mmi_scale             VARCHAR(10) NOT NULL,
    intensity_label       VARCHAR(30) NOT NULL,
    max_pga               NUMERIC(8,4) NOT NULL,              -- gal
    triggered_nodes_count INT NOT NULL,
    started_at            TIMESTAMPTZ DEFAULT NOW(),
    resolved_at           TIMESTAMPTZ
);
CREATE INDEX idx_events_started ON earthquake_events(started_at DESC);
CREATE INDEX idx_events_centroid ON earthquake_events USING GIST(estimated_centroid);

-- ----------------------------------------------------------------------------
-- 4. Multi-Channel Community Chat (retensi 7 hari via job terjadwal)
-- ----------------------------------------------------------------------------
CREATE TABLE chat_messages (
    message_id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    channel_id          VARCHAR(50) NOT NULL,               -- 'global', 'region_west_java'
    sender_id           UUID REFERENCES user_profiles(user_id) ON DELETE SET NULL,
    sender_pseudonym    VARCHAR(32) NOT NULL,
    sender_location_tag VARCHAR(50),
    message             TEXT NOT NULL,
    is_admin            BOOLEAN DEFAULT FALSE,
    created_at          TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_chat_channel ON chat_messages(channel_id, created_at DESC);

-- Retensi 7 hari: Postgres tidak punya TTL native. Pilih salah satu strategi:
--   (a) pg_cron:
--       SELECT cron.schedule('purge_chat','0 * * * *',
--         $$DELETE FROM chat_messages WHERE created_at < NOW() - INTERVAL '7 days'$$);
--   (b) partisi harian (RANGE created_at) + DROP partisi tua; atau
--   (c) goroutine terjadwal di backend Go (fallback bila pg_cron tak tersedia).
