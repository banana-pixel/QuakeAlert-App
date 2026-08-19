package id.web.quakealert.data

import kotlin.math.roundToInt

/**
 * The unit system used to display distances across the app (History distance
 * pills, filter radii, coverage ranges). Stored app-wide in
 * [AppSettingsRepository] so every screen renders the same choice.
 *
 * Data is always held in kilometres internally; [convertFromKm] and
 * [formatDistance] translate to the selected system at the display boundary.
 *
 * @property label segmented-control label shown in Settings ("Metric" /
 *   "Imperial").
 * @property distanceUnit abbreviated unit ("km" / "mi").
 */
enum class UnitSystem(val label: String, val distanceUnit: String) {
    METRIC("Metric", "km"),
    IMPERIAL("Imperial", "mi");

    /** Converts a distance expressed in kilometres into this system's unit. */
    fun convertFromKm(km: Int): Int = when (this) {
        METRIC -> km
        IMPERIAL -> (km * KM_TO_MILES).roundToInt()
    }

    /** Formats a kilometre distance in this system, e.g. "39 km" / "24 mi". */
    fun formatDistance(km: Int): String = "${convertFromKm(km)} $distanceUnit"

    private companion object {
        /** 1 km = 0.6213711922 mi. */
        const val KM_TO_MILES = 0.6213711922
    }
}