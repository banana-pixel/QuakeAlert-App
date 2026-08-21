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
 * Which criteria a screen's filter sheet offers.
 *
 * The sheet is one component with two configurations rather than two sheets: the
 * location scope is genuinely shared by both tabs, while the rest is not. A station
 * has no intensity and no time of occurrence, and an earthquake has no uptime, so a
 * tab that showed all four would be offering controls that cannot change its own
 * answer — which reads as a broken filter rather than an inapplicable one.
 */
enum class FilterSection {
    INTENSITY,
    DISTANCE,
    TIME,
    STATION_STATUS;

    companion object {
        /** What History filters on: shaking, distance and when it happened. */
        val HISTORY: Set<FilterSection> = setOf(INTENSITY, DISTANCE, TIME)

        /** What Sensors filters on: where the station is and whether it reports. */
        val SENSORS: Set<FilterSection> = setOf(DISTANCE, STATION_STATUS)
    }
}

/**
 * Station connectivity criterion offered on the Sensors tab.
 *
 * Applied client-side over the `/sensors` response, which already carries each
 * station's status, so narrowing it costs no request. Distinct from
 * [id.web.quakealert.ui.sensors.SensorStatus], which is one station's actual state
 * rather than a question being asked about the roll.
 *
 * @property label sheet copy for the option pill.
 * @property emptyRollSubtitle what an empty result under this criterion actually
 *   means, for the Sensors empty card. Held here rather than composed at the call
 *   site because "no stations match" is ambiguous in a way the real reason is not:
 *   asking for reporting stations and getting none is an outage, not a gap in
 *   coverage, and the two need different words. Empty for [ALL], which cannot
 *   exclude anything.
 */
enum class QuakeStationStatus(val label: String, val emptyRollSubtitle: String) {
    ALL("All stations", ""),
    ONLINE("Online only", "Every station in this area is currently offline."),
    OFFLINE("Offline only", "Every station in this area is currently reporting.");

    /** Whether a station in the given state belongs in the filtered roll. */
    fun accepts(isOnline: Boolean): Boolean = when (this) {
        ALL -> true
        ONLINE -> isOnline
        OFFLINE -> !isOnline
    }
}

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
 * criteria chosen in the filter sheet.
 *
 * One state for both tabs, but each tab reads only the [FilterSection]s it can act
 * on, so History never applies a station-status choice and Sensors never applies an
 * intensity floor. Holding the unused half rather than clearing it is what lets the
 * user set a narrow History filter, look something up on Sensors and come back to
 * the filter they left.
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
    val timeWindow: QuakeTimeWindow = QuakeTimeWindow.ALL,
    val stationStatus: QuakeStationStatus = QuakeStationStatus.ALL
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
     * Whether this filter asks a question that cannot be answered without a device
     * position: "Near" is measured from one, and [QuakeFilter.ALL] is not.
     *
     * Lives here, next to [eventsRadiusKm], because it is the same fact read the
     * other way round: a radius the client cannot send is a query it must not issue.
     *
     * @param hasPosition whether a position has ever been synced.
     */
    fun needsPosition(hasPosition: Boolean): Boolean =
        mode == QuakeFilter.NEAR && !hasPosition

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
     * The criteria in [sections] that are currently narrowing the query, as the
     * booleans behind every count and summary below.
     *
     * Scoped by section on purpose: the state is shared across both tabs, so an
     * intensity floor set on History is still held while the user is on Sensors,
     * where it changes nothing. Counting it there would badge the Sensors filter
     * button over a criterion that tab does not apply.
     */
    private fun activeCriteria(sections: Set<FilterSection>): List<Boolean> = listOf(
        FilterSection.INTENSITY in sections && intensity != QuakeIntensity.ALL,
        FilterSection.DISTANCE in sections && mode == QuakeFilter.NEAR,
        FilterSection.TIME in sections && timeWindow != QuakeTimeWindow.ALL,
        FilterSection.STATION_STATUS in sections && stationStatus != QuakeStationStatus.ALL
    )

    /**
     * True when a criterion the given screen applies is narrowing the query. Drives
     * whether an empty result offers a way out: with nothing narrowing it, "Reset
     * Filters" would do nothing, and the emptiness is simply the answer.
     */
    fun isNarrowed(sections: Set<FilterSection>): Boolean =
        activeCriteria(sections).any { it }

    /** Number of criteria active on this screen, for the filter button's badge. */
    fun activeCriteriaCount(sections: Set<FilterSection>): Int =
        activeCriteria(sections).count { it }

    /** Whether a station in the given state survives the station-status criterion. */
    fun acceptsStation(isOnline: Boolean): Boolean = stationStatus.accepts(isOnline)

    /**
     * Human sentence naming what is currently excluded, e.g. "at MMI VI+ within
     * 250 km in the past 7 days". Used by the no-data card so the user reads why
     * the list is empty instead of concluding there were no earthquakes.
     *
     * Returns null when nothing the screen applies is narrowing the query.
     */
    fun summary(unitSystem: UnitSystem, sections: Set<FilterSection>): String? {
        if (!isNarrowed(sections)) return null
        val parts = buildList {
            if (FilterSection.INTENSITY in sections && intensity != QuakeIntensity.ALL) {
                add("at MMI ${intensity.roman}+")
            }
            if (FilterSection.DISTANCE in sections && mode == QuakeFilter.NEAR) {
                add("within ${radius.label(unitSystem)}")
            }
            if (FilterSection.TIME in sections && timeWindow != QuakeTimeWindow.ALL) {
                add("in the ${timeWindow.label.lowercase()}")
            }
            if (FilterSection.STATION_STATUS in sections &&
                stationStatus != QuakeStationStatus.ALL
            ) {
                add(stationStatus.label.lowercase())
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
