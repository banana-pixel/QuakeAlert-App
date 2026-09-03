-- ============================================================================
-- QuakeAlert — Migration 000009 (UP): Durabilitas catatan near-confirmation
-- (Fase 4, P4-M2′; diotorisasi D-012)
--
-- SATU tabel baru (event_near_confirmed) dan tidak satu pun perubahan pada tabel
-- yang sudah ada. Seluruhnya ADITIF: CREATE TABLE IF NOT EXISTS, tanpa ALTER,
-- tanpa DROP, tanpa perubahan tipe, tanpa penulisan ulang baris lama. Biner
-- Fase 2 maupun Fase 3 pra-000009 tetap berjalan di atas skema ini tanpa
-- menyadarinya — itulah yang membuat rollout ini dapat dibalik.
--
-- event_state_log TIDAK disentuh. Tabel itu berarti satu baris per TRANSISI
-- state, dan sebuah persilangan ambang independensi bukan transisi: ia dapat
-- terjadi tanpa perubahan state sama sekali (UNCONFIRMED -> UNCONFIRMED ilegal,
-- §5.2, sehingga tidak ada revisi, tidak ada baris riwayat, tidak ada frame).
-- Memasukkan baris bukan-transisi ke sana akan merusak satu-satunya arti yang
-- dimiliki tabel itu.
--
-- Yang dijawab tabel baru, dan mengapa tabel yang sudah ada tidak cukup:
--
--   Pertanyaan operasionalnya — "berapa event yang macet di >= 2 kontributor
--   independen, berapa lama, berapa yang akhirnya CONFIRMED, berapa yang mati
--   tanpa konfirmasi" — hari ini hanya dapat dijawab dari sebuah map di memori
--   yang lenyap saat proses mati. P4-M2′ menuntut jawabannya SELAMAT dari
--   restart, dan pada fleet satu-node jawaban yang benar adalah KOSONG: sebuah
--   log kosong karena map baru dibangun ulang tidak dapat dibedakan dari log
--   kosong karena memang tidak ada persilangan, dan justru perbedaan itu yang
--   diminta kriteria.
--
--   earthquake_events tidak dapat menjawabnya: ia mutable by design (eskalasi
--   menimpanya) dan tidak pernah memegang PUNCAK independensi — hanya nilai
--   terakhir. Sebuah event yang mencapai 3 independen lalu turun ke 1 karena
--   kontributornya dicabut akan terbaca sebagai "tidak pernah mendekati
--   konfirmasi".
--
--   event_state_log tidak dapat menjawabnya: persilangan yang TIDAK mengubah
--   state tidak menghasilkan baris apa pun di sana. Persilangan sunyi itulah
--   kasus yang paling umum pada fleet kecil, dan ia tepat kasus yang hilang.
--
-- Kolom, satu per satu:
--
--   event_id — PRIMARY KEY, dan bukan BIGSERIAL + UNIQUE: satu baris per EVENT,
--       bukan satu baris per penulisan. Bentuk itu cermin langsung
--       map[event_id]*NearConfirmedEntry di memori, yang tetap menjadi otoritas
--       (§9.5); tabel ini pengikutnya, jadi dua baris untuk satu event akan
--       berarti dua kebenaran untuk satu event.
--   first_two_independent_at — ms epoch, jam server, saat event ini PERTAMA KALI
--       memenuhi ambang independensi. Sejajar dengan seluruh timestamp jalur
--       peringatan (origin_ts, decided_at).
--   independent_count_at_peak — nilai TERTINGGI yang pernah dicapai, bukan nilai
--       sekarang. Independensi boleh turun setelah invalidasi kontributor; yang
--       ditanyakan pasca-kejadian adalah seberapa dekat event ini pernah sampai.
--   node_count_at_peak — total kontributor pada saat puncak itu. Dibawa bersama
--       puncaknya, bukan diambil belakangan: tiga node di satu atap bukan tiga
--       bukti, jadi kedua angka hanya bermakna berpasangan.
--   min_independent_cells — ambang yang BERLAKU saat persilangan, direkam apa
--       adanya. Tanpa kolom ini "mencapai 2" tidak dapat ditafsirkan oleh
--       pembaca yang MIN_INDEPENDENT_CELLS-nya sudah berbeda, dan menghitungnya
--       ulang dari konfigurasi sekarang berarti menilai keputusan lampau dengan
--       parameter yang tidak menghasilkannya (U-007 TIDAK dibuka di sini).
--   confirmed_at — NULL berarti TIDAK PERNAH CONFIRMED, bukan nol. Perbedaan itu
--       harus utuh sampai ke Go; kolom NOT NULL DEFAULT 0 akan menghapusnya.
--   terminal_state / terminal_at — NULL berarti masih terbuka. Keduanya bergerak
--       BERSAMA, dan aturan ON CONFLICT di UpsertNearConfirmed menjaga itu:
--       sebuah state terminal tanpa waktunya tidak dapat ditafsirkan.
--   algo_ver — versi algoritma keputusan PER BARIS, memuat ic=<km>. Alasannya
--       persis alasan kolom senama di earthquake_events dan event_state_log:
--       sebuah hitungan independensi hanya dapat ditafsirkan bersama jarak
--       pemisahan yang menghasilkannya. First-wins pada konflik (V3/V6, D-006):
--       baris lampau TIDAK ditulis ulang agar cocok dengan aturan baru.
--
-- TIDAK ada FOREIGN KEY ke earthquake_events(event_id), dan itu keputusan yang
-- disengaja, bukan kelalaian. Baris di sini diantre lewat antrean ledger yang
-- SENGAJA boleh membuang yang tertua (D17/D-002/S1), TERPISAH dari satuan
-- persistensi induknya — sebuah persilangan sunyi tidak punya satuan induk sama
-- sekali, karena tidak ada transisi yang menghasilkannya. Dengan FK, satuan induk
-- yang dibuang akan mengubah pencatatan yang sah menjadi KEGAGALAN TULIS, yaitu
-- galat kedua pada jalur yang tidak punya siapa pun untuk melapor. Tanpa FK,
-- baris yatim mungkin ada; itu kerugian yang benar dan terbatas pada model baca,
-- dan ia dihitung (event_near_confirmed_persist_dropped_total) alih-alih sunyi.
--
-- TIDAK ada indeks terpisah. Kardinalitas tabel ini adalah "event yang PERNAH
-- melampaui ambang independensi" — nol pada fleet satu-node, dan pada fleet mana
-- pun jauh lebih kecil daripada ledger observasi. Satu-satunya pembacaan adalah
-- pemindaian penuh sekali saat boot (ListNearConfirmed). Indeks kedua hanya akan
-- menambah amplifikasi penulisan pada jalur pencatatan.
--
-- Aditif & idempoten: aman dijalankan pada basis data berisi data, dan aman
-- dijalankan dua kali.
-- ============================================================================

CREATE TABLE IF NOT EXISTS event_near_confirmed (
    event_id                  UUID        PRIMARY KEY,
    first_two_independent_at  BIGINT      NOT NULL,  -- ms epoch, jam server
    independent_count_at_peak SMALLINT    NOT NULL,  -- puncak, bukan nilai sekarang
    node_count_at_peak        SMALLINT    NOT NULL,  -- kontributor saat puncak itu
    min_independent_cells     SMALLINT    NOT NULL,  -- ambang yang BERLAKU saat itu
    confirmed_at              BIGINT,                -- NULL = tidak pernah CONFIRMED
    terminal_state            VARCHAR(20),           -- NULL = masih terbuka
    terminal_at               BIGINT,                -- NULL = masih terbuka
    algo_ver                  VARCHAR(30) NOT NULL
);
