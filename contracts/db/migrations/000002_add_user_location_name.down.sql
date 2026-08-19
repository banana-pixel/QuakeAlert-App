-- ============================================================================
-- QuakeAlert — Migration 000002 (DOWN): Rollback user_profiles.location_name
-- Hanya membuang label lokasi; koordinat (last_location) tidak tersentuh.
-- ============================================================================

ALTER TABLE user_profiles
    DROP COLUMN IF EXISTS location_name;
