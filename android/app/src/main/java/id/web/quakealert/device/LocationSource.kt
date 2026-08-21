package id.web.quakealert.device

/**
 * A single position read, in WGS84 degrees.
 *
 * Deliberately not [id.web.quakealert.domain.UserLocation]: that type carries the
 * reverse-geocoded `location_name` the server stores, which a raw fix does not
 * have yet. [id.web.quakealert.data.users.UserLocationRepository] joins the two.
 */
data class Coordinates(
    val latitude: Double,
    val longitude: Double
)

/**
 * One-shot access to the device's position.
 *
 * An interface with two implementations rather than one class with a branch,
 * because the two providers share no API surface: [FusedLocationSource] talks to
 * Play Services, [PlatformLocationSource] to the AOSP `LocationManager`. Choose
 * with [locationSource].
 */
interface LocationSource {

    /**
     * The current position, or null when it could not be obtained — permission not
     * granted, location services off, no provider available, or the request timed
     * out. Implementations never throw for those cases: a missing fix is an
     * expected outcome that callers already handle (the app falls back to the last
     * position it stored, or to no distance gating at all).
     *
     * @param allowHighAccuracy permits a satellite-grade attempt as a last resort.
     *   Off by default because the accuracy is useless here — the position feeds a
     *   coverage radius measured in tens of kilometres — and the cost is a GPS scan.
     *   Callers set it only when a user is waiting on the result, so the background
     *   paths cannot turn a weak-signal location into a per-launch battery drain.
     */
    suspend fun currentFix(allowHighAccuracy: Boolean = false): Coordinates?
}
