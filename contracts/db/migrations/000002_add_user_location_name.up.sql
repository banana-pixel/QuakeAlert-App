-- ============================================================================
-- QuakeAlert — Migration 000002 (UP): user_profiles.location_name
-- Menambah label lokasi hasil reverse-geocode yang dikirim klien Android pada
-- PUT /api/v1/users/location. Nullable karena geocoder bisa gagal / offline —
-- koordinat (last_location) tetap wajib, namanya opsional.
-- Aditif & idempoten: aman dijalankan pada database yang sudah berisi data.
-- ============================================================================

ALTER TABLE user_profiles
    ADD COLUMN IF NOT EXISTS location_name VARCHAR(150);
