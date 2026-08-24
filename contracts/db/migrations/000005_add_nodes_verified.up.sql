-- ============================================================================
-- QuakeAlert — Migration 000005 (UP): Verifikasi Node
--
-- Dua tingkat kepercayaan node: verified = false saat provision (default),
-- true setelah operator mengonfirmasi (POST /api/v1/admin/nodes/{id}/verify).
--
-- Node yang belum terverifikasi TETAP terlihat di /sensors (heartbeat diterima),
-- tetapi trigger-nya ditolak di internal/ingest/verifier.go sehingga tidak pernah
-- ikut voting menuju ambang 3-node CONFIRMED. Tanpa ini, siapa pun yang menemukan
-- endpoint provisioning terbisa bisa mencetak node tak berujung dan menggerakkan
-- konsensus — pada aplikasi peringatan dini, itu adalah jalur kepercayaan yang
-- harus tertutup sebelum wizard "Add a Sensor" dibuka ke pengguna.
--
-- Node yang SUDAH ada saat migrasi dijalankan menjadi verified = false: mereka
-- harus dikonfirmasi operator sekali lewat endpoint admin. Pada instalasi dev
-- dengan tiga node uji, itu memang perilaku yang diinginkan.
--
-- Aditif & idempoten: aman dijalankan pada database yang sudah berisi data.
-- ============================================================================

ALTER TABLE iot_nodes ADD COLUMN IF NOT EXISTS verified BOOLEAN NOT NULL DEFAULT FALSE;

-- Daftar pending admin selalu pendek (node baru saja), jadi indeks parsial ini
-- cukup dan nyaris tidak pernah tumbuh.
CREATE INDEX IF NOT EXISTS idx_nodes_unverified
    ON iot_nodes(created_at)
    WHERE NOT verified;
