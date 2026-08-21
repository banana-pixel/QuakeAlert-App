package id.web.quakealert.ui.common

import id.web.quakealert.data.UnitSystem
import id.web.quakealert.data.network.QuakeApiClient
import id.web.quakealert.domain.SafetyPolicy
import java.time.Duration
import java.time.Instant

/**
 * The two filter modes shown in the shared [QuakeFilterRow] across the History
 * (Figma node 1:711) and Sensors (Figma node 1:1105) screens. Both designs use
 * an identical "All" / "Near" pill pair, so a single enum is shared to avoid
 * divergent per-screen duplicates.
 */
enum class QuakeFilter { ALL, NEAR }

/**
 * Shaking-intensity buckets offered by the filter sheet (Figma node 1:709).
 *
 * Labelled in MMI because that is what the app shows everywhere else, but sent to
 * the server as a PGA floor in gal: `earthquake_events.mmi_scale` is a Roman
 * numeral string, so `max_pga` is the only intensity column that can be compared
 * numerically. There is deliberately no magnitude option — the QuakeAlert network
 * is a surface MEMS array that measures ground acceleration, not seismic moment.
 *
 * @property label sheet copy naming both the plain meaning and the MMI floor.
 * @property roman the MMI floor as a numeral, for prose such as "at MMI VI+".
 * @property description one line on what that shaking feels like, so the choice
 *   can be made without knowing the MMI table.
 * @property minMmi MMI floor, or 0 for "everything the network recorded".
 */
enum class QuakeIntensity(
    val label: String,
    val description: String,
    val minMmi: Int,
    val roman: String
) {
    ALL("All Intensities", "Everything the sensor network recorded.", 0, ""),
    FELT("Felt (MMI IV+)", "Light shaking, noticed by most people indoors.", 4, "IV"),
    MODERATE("Moderate (MMI VI+)", "Enough to crack plaster and swing hanging objects.", 6, "VI"),
    SEVERE("Severe (MMI VII+)", "Damaging shaking; matches the alert override threshold.", 7, "VII");

    /**
     * PGA floor in gal, or null when nothing should be filtered. Derived from
     * [SafetyPolicy.minPgaForMmi] — the inverse of the server's own PGA→MMI
     * regression — so a bucket returns exactly the events whose printed numeral is
     * at or above [minMmi], rather than a hand-picked threshold that would make the
     * list disagree with its own intensity chips.
     */
    val minPgaGal: Double?
        get() = if (minMmi <= 1) null else SafetyPolicy.minPgaForMmi(minMmi)
}

/**
 * Browse radius offered by the filter sheet. This is a *viewing* radius and has
 * nothing to do with [SafetyPolicy.ALERT_RADIUS_KM], which stays fixed at 200 km
 * because life-safety alerting is not a user preference.
 *
 * 1000 km exists because History is served by `/events`, which accepts up to
 * 2000 km. `/sensors` caps at [QuakeApiClient.MAX_SENSOR_RANGE_KM] (500), so the
 * Sensors tab clamps — see [QuakeFilterState.sensorsRadiusKm].
 */
enum class QuakeSearchRadius(val km: Int) {
    KM_100(100),
    KM_250(250),
    KM_500(500),
    KM_1000(1000);

    /** e.g. "250 km" / "155 mi". */
    fun label(unitSystem: UnitSystem): String = unitSystem.formatDistance(km)
}

/**
 * Time window offered by the filter sheet, as a lookback from "now".
 *
 * @property days lookback in days, or null for the whole archive.
 */
enum class QuakeTimeWindow(val label: String, val days: Int?) {
    ALL("Any time", null),
    DAY("Past 24 hours", 1),
    WEEK("Past 7 days", 7),
    MONTH("Past 30 days", 30);

    /** Lower bound to send as `since`, or null when the archive is unbounded. */
    fun since(now: Instant = Instant.now()): Instant? =
        days?.let { now.minus(Duration.ofDays(it.toLong())) }
}

/**
 * The complete state of the shared filter: the [QuakeFilterRow] pill plus the
 * three criteria chosen in the filter sheet.
 *
 * Held for the session only (see `QuakeFilterViewModel`) and never persisted:
 * someone who narrowed the list to severe quakes last week and opens the app
 * during an earthquake must see everything, not a stale slice.
 *
 * The radius applies only in [QuakeFilter.NEAR] — in [QuakeFilter.ALL] the query
 * carries no centre, so a radius would have nothing to be measured from.
 */
data class QuakeFilterState(
    val mode: QuakeFilter = QuakeFilter.ALL,
    val intensity: QuakeIntensity = QuakeIntensity.ALL,
    val radius: QuakeSearchRadius = DEFAULT_RADIUS,
    val timeWindow: QuakeTimeWindow = QuakeTimeWindow.ALL
) {
    /** PGA floor in gal for `/events?min_pga=`, or null when unfiltered. */
    val minPgaGal: Double?
        get() = intensity.minPgaGal

    /** Lower time bound for `/events?since=`, or null when unfiltered. */
    fun since(now: Instant = Instant.now()): Instant? = timeWindow.since(now)

    /** Radius for `/events`, or null in [QuakeFilter.ALL] (no centre to measure from). */
    val eventsRadiusKm: Int?
        get() = radius.km.takeIf { mode == QuakeFilter.NEAR }

    /**
     * Radius for `/sensors`, clamped to the endpoint's 500 km ceiling. The clamp is
     * surfaced in the sheet via [isSensorsRadiusClamped] rather than applied
     * silently, because a 1000 km request would otherwise be rejected by the server
     * with nothing on screen explaining why.
     */
    val sensorsRadiusKm: Int?
        get() = eventsRadiusKm?.coerceAtMost(QuakeApiClient.MAX_SENSOR_RANGE_KM)

    /** True when the chosen radius exceeds what the Sensors tab can request. */
    val isSensorsRadiusClamped: Boolean
        get() = mode == QuakeFilter.NEAR && radius.km > QuakeApiClient.MAX_SENSOR_RANGE_KM

    /**
     * True when any criterion is narrowing the query. Drives whether an empty
     * result offers a way out: with nothing narrowing it, "Reset Filters" would do
     * nothing, and the emptiness is simply the answer.
     */
    val isNarrowed: Boolean
        get() = mode == QuakeFilter.NEAR ||
            intensity != QuakeIntensity.ALL ||
            timeWindow != QuakeTimeWindow.ALL

    /** Number of sheet criteria active, for the filter button's badge. */
    val activeCriteriaCount: Int
        get() = listOf(
            intensity != QuakeIntensity.ALL,
            mode == QuakeFilter.NEAR,
            timeWindow != QuakeTimeWindow.ALL
        ).count { it }

    /**
     * Human sentence naming what is currently excluded, e.g. "at MMI VI+ within
     * 250 km in the past 7 days". Used by the no-data card so the user reads why
     * the list is empty instead of concluding there were no earthquakes.
     *
     * Returns null when nothing is narrowing the query.
     */
    fun summary(unitSystem: UnitSystem): String? {
        if (!isNarrowed) return null
        val parts = buildList {
            if (intensity != QuakeIntensity.ALL) {
                add("at MMI ${intensity.roman}+")
            }
            if (mode == QuakeFilter.NEAR) {
                add("within ${radius.label(unitSystem)}")
            }
            if (timeWindow != QuakeTimeWindow.ALL) {
                add("in the ${timeWindow.label.lowercase()}")
            }
        }
        return parts.joinToString(" ")
    }

    /** Clears the sheet criteria and the pill, back to the unfiltered feed. */
    fun reset(): QuakeFilterState = QuakeFilterState()

    /** Next radius up, for the no-coverage card's "Widen Search Radius" action. */
    fun widened(): QuakeFilterState {
        if (mode != QuakeFilter.NEAR) return this
        val wider = QuakeSearchRadius.entries.firstOrNull { it.km > radius.km }
            ?: return copy(mode = QuakeFilter.ALL)
        return copy(radius = wider)
    }

    companion object {
        /**
         * History's historical "Near" radius, kept as the sheet default so the
         * existing behaviour is unchanged until the user touches the sheet.
         */
        val DEFAULT_RADIUS = QuakeSearchRadius.entries
            .first { it.km == SafetyPolicy.HISTORY_NEAR_RADIUS_KM }
    }
}
