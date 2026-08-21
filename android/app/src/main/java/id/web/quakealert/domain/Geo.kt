package id.web.quakealert.domain

import kotlin.math.asin
import kotlin.math.cos
import kotlin.math.min
import kotlin.math.pow
import kotlin.math.sin
import kotlin.math.sqrt

/** Mean Earth radius in kilometres, fixed by .clinerules/20 rule 2. */
const val EARTH_RADIUS_KM: Double = 6371.0

/**
 * Great-circle distance in kilometres between two WGS84 coordinates.
 *
 * Uses the haversine formula with the project-mandated `R = 6371.0 km`
 * (.clinerules/20 rule 2, docs/CLIENT_SPEC.md §7) so client-side distance gating
 * agrees with the radius filters the server applies with PostGIS.
 *
 * The `asin(min(1.0, sqrt(a)))` clamp matters: for antipodal-ish inputs floating
 * point can push `a` marginally above 1, and an unclamped `asin` would return
 * NaN — which would silently disable a distance gate rather than fail loudly.
 */
fun haversineKm(
    lat1: Double,
    lon1: Double,
    lat2: Double,
    lon2: Double
): Double {
    val dLat = Math.toRadians(lat2 - lat1)
    val dLon = Math.toRadians(lon2 - lon1)
    val rLat1 = Math.toRadians(lat1)
    val rLat2 = Math.toRadians(lat2)

    val a = sin(dLat / 2).pow(2) + cos(rLat1) * cos(rLat2) * sin(dLon / 2).pow(2)
    return 2 * EARTH_RADIUS_KM * asin(min(1.0, sqrt(a)))
}

/**
 * The user's last known position, as pushed to `PUT /api/v1/users/location` and
 * cached locally so distance read-outs survive a restart without a fresh GPS fix.
 */
data class UserLocation(
    val latitude: Double,
    val longitude: Double,
    val locationName: String? = null
)

/**
 * Distance in kilometres from [this] location to the given coordinates, or null
 * when the user's position is unknown.
 *
 * Returning null rather than 0.0 is deliberate: "0 km away" reads as *at the
 * epicentre*, which is the most alarming possible value to show for missing data.
 */
fun UserLocation?.distanceKmTo(latitude: Double, longitude: Double): Double? =
    this?.let { haversineKm(it.latitude, it.longitude, latitude, longitude) }
