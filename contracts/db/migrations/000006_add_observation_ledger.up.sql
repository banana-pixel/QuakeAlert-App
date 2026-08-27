-- ============================================================================
-- QuakeAlert — Migration 000006 (UP): Observation Ledger
--
-- Dua tabel baru, nol perubahan pada tabel yang sudah ada. Sampai migrasi ini
-- sistem tidak menyimpan satu pun rekaman MASUKAN: hanya event hasil konsensus
-- yang pernah dipersistensi (earthquake_events), sehingga tidak ada cara
-- menjawab "apa yang dilaporkan sensor" maupun "apa yang kami kirim ke
-- pengguna" selain membaca log yang sudah dirotasi.
--
--   1. sensor_observations — satu baris per trigger yang SAMPAI ke server, baik
--      lolos verifikasi maupun ditolak. Inilah bahan mentah untuk mengukur
--      latency publish, laju false trigger, dan drift jam per node.
--   2. alert_emissions    — satu baris per KEPUTUSAN dispatch (bukan per
--      pengiriman). Hasil delivery (jumlah klien WS, sukses/gagal FCM) BUKAN
--      bagian dari fase ini: FCM dikirim dari goroutine terpisah, jadi angkanya
--      belum ada saat keputusan dibuat, dan menunggunya berarti menaruh latensi
--      jaringan di depan jalur peringatan.
--
-- Keduanya ditulis ASINKRON (internal/ledger): tidak ada operasi basis data di
-- depan atau sebaris dengan jalur peringatan. Kehilangan baris ledger tidak
-- boleh pernah menjadi kehilangan peringatan.
--
-- Aditif & idempoten: aman dijalankan pada database yang sudah berisi data.
-- ============================================================================

-- ----------------------------------------------------------------------------
-- 1. sensor_observations — rekaman masukan per-sensor
--
-- SENGAJA TANPA FOREIGN KEY ke iot_nodes: kasus forensik yang paling penting
-- justru observasi dari node yang sudah dihapus (DeleteUnverifiedNode,
-- PurgeAbandonedPendingNodes) atau tidak dikenal. FK akan membuat ledger
-- berbohong lewat penghilangan.
--
-- NUMERIC(8,4) untuk pga_gal menyamakan presisi dengan earthquake_events.max_pga
-- dan `multipleOf: 0.0001` pada kontrak, sehingga nilai yang tersimpan identik
-- byte-per-byte dengan nilai yang DITANDATANGANI.
--
-- Kolom yang sengaja NULL pada protokol v1 (proto_ver, obs_seq, onset_ts) sudah
-- ada sejak sekarang karena Fase 2 mengisinya tanpa perubahan skema; kolom yang
-- v1 tidak bisa mengisi SAMA SEKALI (attempt_no, detrigger_ts) TIDAK dibuat di
-- sini — lihat migrasi 000007 pada Fase 2.
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS sensor_observations (
    observation_id   BIGSERIAL PRIMARY KEY,   -- SEKALIGUS urutan kedatangan
    node_id          VARCHAR(32)  NOT NULL,
    source_class     VARCHAR(20)  NOT NULL DEFAULT 'FIXED_ESP32',
    phase            VARCHAR(10)  NOT NULL DEFAULT 'FINAL',
    proto_ver        SMALLINT,                 -- NULL untuk v1 (legacy)
    obs_seq          BIGINT,                   -- NULL sampai Fase 2
    pga_gal          NUMERIC(8,4) NOT NULL,
    dur_ms           INTEGER      NOT NULL,    -- DICATAT, bukan gerbang keputusan
    publish_ts       BIGINT       NOT NULL,    -- ts payload (ms epoch UTC)
    received_ts      BIGINT       NOT NULL,    -- jam server (ms epoch UTC)
    onset_ts         BIGINT,                   -- NULL sampai Fase 2 (sensor-true)
    onset_ts_upper_bound BIGINT,               -- publish_ts - dur_ms; BATAS ATAS
    -- VARCHAR(16), bukan (10) seperti tertulis di §13.1 rencana: nilai bawaannya
    -- sendiri, 'PUBLISH_BOUND', panjangnya 13 karakter. Ditemukan oleh test
    -- integrasi §20.4 terhadap Postgres nyata, bukan oleh pembacaan ulang DDL.
    onset_ts_source  VARCHAR(16)  NOT NULL DEFAULT 'PUBLISH_BOUND',
    node_location    GEOGRAPHY(Point,4326),    -- SNAPSHOT saat ingest, boleh NULL
    signature        CHAR(64),
    verify_result    VARCHAR(24)  NOT NULL,    -- 'OK' atau nama Err* verifier
    suppressed_rejections INTEGER NOT NULL DEFAULT 0,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- onset_ts_upper_bound BUKAN estimasi onset. ts distempel saat PUBLISH dan
-- distempel ULANG pada setiap retry (firmware: backoff 5 s, sampai 36 percobaan,
-- usia maksimum 5 menit), dan payload v1 tidak membawa nomor percobaan. Jadi:
--     onset_ts_upper_bound = publish_ts - dur_ms = onset + publish_delay
-- dengan publish_delay >= 0, tak terbatas, dan belum terobservasi. Nilai ini
-- boleh dipakai untuk pengurutan dan pengelompokan, TIDAK boleh dipakai untuk
-- mengkalibrasi jendela korelasi berbasis onset.

-- verify_result mencatat OTENTIKASI saja. Kegagalan pencarian lokasi node BUKAN
-- kegagalan verifikasi: kasus itu terekam sebagai verify_result = 'OK' dengan
-- node_location NULL — observasi yang sah lalu dibuang konsensus karena tidak
-- punya koordinat.

-- suppressed_rejections menutup lubang yang muncul dari pembatasan laju: baris
-- penolakan dibatasi satu per node per menit (kredensial broker berlaku untuk
-- seluruh fleet, jadi publish tak terotentikasi tidak boleh menjadi penulisan
-- durable tanpa batas). Penolakan yang ditekan dihitung dan angkanya dibawa oleh
-- baris berikutnya yang diterima, sehingga JUMLAHnya tidak pernah hilang meski
-- barisnya hilang.

CREATE INDEX IF NOT EXISTS idx_obs_received  ON sensor_observations(received_ts DESC);
CREATE INDEX IF NOT EXISTS idx_obs_node_time ON sensor_observations(node_id, received_ts DESC);
CREATE INDEX IF NOT EXISTS idx_obs_spatial   ON sensor_observations USING GIST(node_location);
CREATE INDEX IF NOT EXISTS idx_obs_failed    ON sensor_observations(received_ts DESC)
    WHERE verify_result <> 'OK';

-- ----------------------------------------------------------------------------
-- 2. alert_emissions — rekaman KEPUTUSAN dispatch
--
-- event_id boleh NULL: ADVISORY hari ini tidak punya identitas event sama sekali
-- (hanya CONFIRMED yang dipersistensi ke earthquake_events).
--
-- audience adalah satu dari tiga nilai, karena jalur severe dan jalur bertarget
-- saling eksklusif di dispatcher:
--   TOKENS_RADIUS_200KM — token perangkat dalam AlertRadiusKm dari centroid
--   GEO_TOPIC_ALL       — topic nasional (severe, atau fallback tanpa token)
--   NONE                — tidak ada FCM sama sekali (mis. kluster satu node
--                         tanpa token dalam radius; lihat guard D6)
--
-- TANPA kolom event_state: Fase 1 tidak punya state machine event, jadi kolomnya
-- akan NULL di setiap baris sambil TERLIHAT seperti state yang tercatat.
-- TANPA kolom hasil delivery: lihat catatan di kepala berkas.
--
-- decided_at adalah jam server saat keputusan dibuat — bukan waktu gempa. Tidak
-- ada kolom di sini yang boleh dibaca sebagai waktu terjadinya guncangan.
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS alert_emissions (
    emission_id     BIGSERIAL PRIMARY KEY,
    event_id        UUID,                    -- NULL untuk ADVISORY hari ini
    alert_type      VARCHAR(24) NOT NULL,    -- EARTHQUAKE_ALERT | ..._ADVISORY | EVENT_RESOLVED
    status          VARCHAR(20) NOT NULL,    -- CONFIRMED | ADVISORY (sebagaimana diemisikan)
    mmi             VARCHAR(10),
    pga_gal         NUMERIC(8,4),
    node_count      INTEGER     NOT NULL,
    centroid        GEOGRAPHY(Point,4326),
    is_severe       BOOLEAN     NOT NULL,
    audience        VARCHAR(24) NOT NULL,
    decided_at      BIGINT      NOT NULL,    -- ms epoch UTC, jam server
    algo_ver        VARCHAR(16) NOT NULL,    -- konstanta compile-time di Go
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_emis_decided ON alert_emissions(decided_at DESC);
CREATE INDEX IF NOT EXISTS idx_emis_severe  ON alert_emissions(decided_at DESC) WHERE is_severe;
