-- ============================================================================
-- QuakeAlert — Migration 000008 (UP): Event Lifecycle (Fase 3)
--
-- Enam kolom pada earthquake_events, dua pada alert_emissions, dan SATU tabel
-- baru (event_state_log). Seluruhnya ADITIF: ADD COLUMN IF NOT EXISTS,
-- CREATE TABLE IF NOT EXISTS, tanpa penulisan ulang baris lama, tanpa DROP
-- kolom, tanpa perubahan tipe. Biner Fase 2 tetap berjalan di atas skema ini —
-- itulah yang membuat rollout Fase 3 dapat dibalik.
--
-- Yang dijawab kolom-kolom baru, dan mengapa kolom yang sudah ada tidak cukup:
--
--   event_state — state machine lima nilai (DETECTED, UNCONFIRMED, CONFIRMED,
--       RESOLVED, CANCELLED). `status` hanya punya dua nilai dan karena itu
--       tidak dapat membedakan "belum terkonfirmasi" dari "terkonfirmasi", atau
--       "berakhir wajar" dari "bukti ditarik". NULLABLE dengan sengaja: baris
--       pra-Fase-3 harus tetap terbaca sebagai "state tak diketahui, status
--       diketahui".
--   revision — penghitung monoton per event. Baris earthquake_events bersifat
--       mutable (eskalasi menimpanya), jadi tanpa nomor revisi sebuah penulisan
--       yang datang terlambat dapat memundurkan keputusan. UpsertEvent memakai
--       kolom ini sebagai penjaga (WHERE revision < EXCLUDED.revision).
--   origin_ts / origin_ts_source — instan TANAH BERGERAK, dalam ms epoch agar
--       sejajar dengan sensor_observations.onset_ts dan seluruh timestamp jalur
--       peringatan. `started_at` tetap berarti "kapan server membuat baris ini";
--       mencampur keduanya adalah kekeliruan yang diperbaiki di sini.
--       origin_ts_source membedakan pengukuran ('SENSOR') dari batas atas
--       ('PUBLISH_BOUND', satu-satunya yang mungkin untuk v1).
--   independent_cell_count — jumlah sel spasial independen di antara
--       kontributor. Sebuah keputusan CONFIRMED tidak dapat ditafsirkan dari
--       triggered_nodes_count saja: tiga node di satu atap bukan tiga bukti.
--   algo_ver — versi algoritma keputusan PER BARIS, bukan hanya konstanta
--       compile-time, karena INDEPENDENCE_CELL_KM dapat dikonfigurasi. Ditulis
--       sebagai 'phase3-1.0/ic=<km>'.
--
--   alert_emissions.{event_state, event_revision} — state yang DIUMUMKAN sebuah
--       frame. Hari ini hal itu harus disimpulkan dari alert_type, dan untuk
--       frame resolusi tidak dapat disimpulkan sama sekali.
--
-- Aditif & idempoten: aman dijalankan pada basis data berisi data, dan aman
-- dijalankan dua kali.
-- ============================================================================

ALTER TABLE earthquake_events
    ADD COLUMN IF NOT EXISTS event_state            VARCHAR(20),  -- NULL = baris pra-Fase-3
    ADD COLUMN IF NOT EXISTS revision               INT     NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS origin_ts              BIGINT,       -- ms epoch, waktu tanah bergerak
    ADD COLUMN IF NOT EXISTS origin_ts_source       VARCHAR(20),  -- 'SENSOR' | 'PUBLISH_BOUND'
    ADD COLUMN IF NOT EXISTS independent_cell_count SMALLINT,
    ADD COLUMN IF NOT EXISTS algo_ver               VARCHAR(30);

-- Dua indeks, keduanya untuk pembacaan yang benar-benar ada: umpan REST publik
-- mengurutkan menurut waktu asal event, dan rekonsiliasi startup (§15.3) memuat
-- event yang masih terbuka menurut state-nya.
CREATE INDEX IF NOT EXISTS idx_events_origin_ts ON earthquake_events (origin_ts DESC);
CREATE INDEX IF NOT EXISTS idx_events_state     ON earthquake_events (event_state);

ALTER TABLE alert_emissions
    ADD COLUMN IF NOT EXISTS event_state    VARCHAR(20),
    ADD COLUMN IF NOT EXISTS event_revision INT;

-- Pelebaran algo_ver dari VARCHAR(16) ke VARCHAR(30), menyamakannya dengan kedua
-- kolom algo_ver baru di atas. §9.1 menetapkan format per-baris
-- 'phase3-1.0/ic=<km>'; pada VARCHAR(16) format itu hanya muat selama
-- INDEPENDENCE_CELL_KM berupa satu atau dua digit, dan sebuah nilai tiga digit
-- akan membuat SETIAP penulisan alert_emissions gagal. Karena kolomnya ditulis
-- oleh jalur pencatatan (bukan jalur peringatan) kegagalan itu tidak akan
-- menghentikan peringatan — ia hanya akan menghapus provenance-nya diam-diam,
-- yang justru satu-satunya alasan kolom ini ada.
--
-- Pelebaran VARCHAR di Postgres adalah perubahan katalog saja: tanpa penulisan
-- ulang tabel, tanpa lock panjang, dan idempoten (menjalankannya dua kali
-- menghasilkan tipe yang sama). Biner Fase 1/Fase 2 tidak terpengaruh — keduanya
-- menulis 'phase1-1.0' (10 karakter) dan membaca kolom sebagai string.
ALTER TABLE alert_emissions
    ALTER COLUMN algo_ver TYPE VARCHAR(30);

-- ----------------------------------------------------------------------------
-- event_state_log — satu-satunya tabel baru Fase 3.
--
-- Satu baris per TRANSISI state. Ia satu-satunya catatan durable bahwa sebuah
-- state pernah dipegang: earthquake_events mutable by design, jadi tanpa log
-- ini pertanyaan "apakah peringatan ini pernah sekadar advisory?", "kapan ia
-- naik menjadi CONFIRMED?", "mengapa ia dibatalkan?" tidak terjawab — dan
-- ketiganya adalah pertanyaan yang ditanyakan SETELAH insiden, bukan sebelum.
--
-- from_state NULL hanya pada baris penciptaan yang tidak punya state
-- sebelumnya; untuk event yang menjadi publik, from_state berisi 'DETECTED'
-- karena state itu memang pernah dipegang di memori meski tidak pernah menjadi
-- baris (persistensi DETECTED bersifat lazy, §9.5).
--
-- evidence_summary adalah SNAPSHOT JSONB, bukan join. Baris yang akan
-- dijoin (sensor_observations) ditulis melalui antrean yang SENGAJA boleh
-- membuang (D17/I9); sebuah join ke data yang droppable akan membuat jejak
-- audit sebuah keputusan bergantung pada kedalaman antrean saat keputusan itu
-- dibuat. Snapshot diambil di bawah lock Tracker dari memori yang sama yang
-- membuat keputusan, jadi ia eksak by construction.
--
-- TIDAK ada indeks terpisah. UNIQUE (event_id, revision) sudah didukung btree
-- atas persis (event_id, revision) dalam urutan kolom itu, yang melayani kedua
-- query yang dimiliki tabel ini — "revisi terakhir event X" dan "seluruh
-- transisi event X berurutan" — karena event_id adalah kolom terdepan. Indeks
-- kedua yang identik hanya menambah amplifikasi penulisan pada jalur audit.
--
-- FK ke earthquake_events(event_id) dipenuhi by construction: upsert induk dan
-- baris log ini diantre sebagai SATU satuan kerja, induk lebih dahulu, dan bila
-- upsert gagal maka INSERT log DILEWATI, tidak pernah dicoba (§9.5).
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS event_state_log (
    id                BIGSERIAL PRIMARY KEY,
    event_id          UUID        NOT NULL REFERENCES earthquake_events(event_id),
    revision          INT         NOT NULL,
    from_state        VARCHAR(20),                 -- NULL hanya saat penciptaan
    to_state          VARCHAR(20) NOT NULL,
    reason            VARCHAR(40) NOT NULL,        -- kosakata tertutup (§5.3)
    decided_at        BIGINT      NOT NULL,        -- ms epoch, jam server
    node_count        SMALLINT    NOT NULL,
    independent_cells SMALLINT    NOT NULL,
    peak_pga          NUMERIC(8,4),
    evidence_summary  JSONB       NOT NULL,        -- snapshot kontributor (§5.3)
    algo_ver          VARCHAR(30) NOT NULL,
    UNIQUE (event_id, revision)
);
