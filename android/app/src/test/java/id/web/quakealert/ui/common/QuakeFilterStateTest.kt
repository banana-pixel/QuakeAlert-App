package id.web.quakealert.ui.common

import id.web.quakealert.data.UnitSystem
import id.web.quakealert.data.network.QuakeApiClient
import id.web.quakealert.domain.SafetyPolicy
import java.time.Duration
import java.time.Instant
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Guards the pure mapping from what the user chose to what the two endpoints are
 * asked. The interesting failures here are silent ones: a bucket that maps to the
 * wrong gal floor, or a radius that `/sensors` rejects, both produce a plausible
 * screen that answers a different question than the one on display.
 */
class QuakeFilterStateTest {

    @Test
    fun `intensity buckets map to the server's own PGA thresholds`() {
        // Derived from SafetyPolicy (which mirrors consensus.MMIFromPGA) rather than
        // hard-coded, so a change to the shared regression fails the server test that
        // owns it instead of silently disagreeing with this one.
        assertNull(QuakeIntensity.ALL.minPgaGal)
        assertEquals(SafetyPolicy.minPgaForMmi(4), QuakeIntensity.FELT.minPgaGal!!, 1e-9)
        assertEquals(SafetyPolicy.minPgaForMmi(6), QuakeIntensity.MODERATE.minPgaGal!!, 1e-9)
        assertEquals(SafetyPolicy.minPgaForMmi(7), QuakeIntensity.SEVERE.minPgaGal!!, 1e-9)

        // Round-trip at the *rounding* boundary, which is what the filter promises:
        // the floor is the lowest PGA whose printed numeral is still the bucket's, so
        // it reads back as `mmi - 0.5` and a hair above it rounds to `mmi`.
        QuakeIntensity.entries.filter { it.minMmi > 1 }.forEach { bucket ->
            val floor = bucket.minPgaGal!!
            assertEquals(bucket.name, bucket.minMmi - 0.5, SafetyPolicy.mmiFromPga(floor), 0.01)
            assertEquals(
                bucket.name,
                bucket.minMmi.toLong(),
                Math.round(SafetyPolicy.mmiFromPga(floor * 1.001))
            )
        }
    }

    @Test
    fun `radius only reaches the query when the Near pill is on`() {
        val all = QuakeFilterState(radius = QuakeSearchRadius.KM_1000)
        assertNull(all.eventsRadiusKm)
        assertNull(all.sensorsRadiusKm)
        assertFalse(all.isSensorsRadiusClamped)

        val near = all.copy(mode = QuakeFilter.NEAR)
        assertEquals(1000, near.eventsRadiusKm)
    }

    @Test
    fun `sensors radius is clamped visibly, never dropped`() {
        val near = QuakeFilterState(mode = QuakeFilter.NEAR, radius = QuakeSearchRadius.KM_1000)
        assertEquals(QuakeApiClient.MAX_SENSOR_RANGE_KM, near.sensorsRadiusKm)
        assertTrue(near.isSensorsRadiusClamped)

        val within = near.copy(radius = QuakeSearchRadius.KM_250)
        assertEquals(250, within.sensorsRadiusKm)
        assertFalse(within.isSensorsRadiusClamped)
    }

    @Test
    fun `time window becomes a since instant`() {
        val now = Instant.parse("2026-08-21T12:00:00Z")
        assertNull(QuakeFilterState().since(now))
        assertEquals(
            now.minus(Duration.ofDays(7)),
            QuakeFilterState(timeWindow = QuakeTimeWindow.WEEK).since(now)
        )
    }

    @Test
    fun `summary names every active criterion and is null when nothing narrows`() {
        assertNull(QuakeFilterState().summary(UnitSystem.METRIC, FilterSection.HISTORY))

        val narrowed = QuakeFilterState(
            mode = QuakeFilter.NEAR,
            intensity = QuakeIntensity.MODERATE,
            radius = QuakeSearchRadius.KM_250,
            timeWindow = QuakeTimeWindow.WEEK
        )
        assertEquals(
            "at MMI VI+ within 250 km in the past 7 days",
            narrowed.summary(UnitSystem.METRIC, FilterSection.HISTORY)
        )
        assertEquals(3, narrowed.activeCriteriaCount(FilterSection.HISTORY))
        // The unit follows the user's choice, so the no-data card cannot contradict
        // the distances printed on the cards behind it.
        assertTrue(
            narrowed.summary(UnitSystem.IMPERIAL, FilterSection.HISTORY)!!.contains("mi")
        )
    }

    /**
     * The point of sharing one state across both tabs: a criterion a screen cannot
     * apply must not be counted, summarised, or offered as a reason its list is
     * empty. The state below is narrowed on all four criteria at once, so each
     * screen's answer is exactly the subset it acts on.
     */
    @Test
    fun `each screen counts and summarises only the criteria it applies`() {
        val everything = QuakeFilterState(
            mode = QuakeFilter.NEAR,
            intensity = QuakeIntensity.MODERATE,
            radius = QuakeSearchRadius.KM_250,
            timeWindow = QuakeTimeWindow.WEEK,
            stationStatus = QuakeStationStatus.ONLINE
        )

        assertEquals(3, everything.activeCriteriaCount(FilterSection.HISTORY))
        assertEquals(2, everything.activeCriteriaCount(FilterSection.SENSORS))
        assertEquals(
            "at MMI VI+ within 250 km in the past 7 days",
            everything.summary(UnitSystem.METRIC, FilterSection.HISTORY)
        )
        assertEquals(
            "within 250 km online only",
            everything.summary(UnitSystem.METRIC, FilterSection.SENSORS)
        )
    }

    @Test
    fun `a criterion the screen ignores never makes its list look narrowed`() {
        // An intensity floor set on History is still held while the user is on
        // Sensors, where it changes nothing: badging it there would offer a way out
        // of a filter that is not the reason the roll is empty.
        val intensityOnly = QuakeFilterState(intensity = QuakeIntensity.SEVERE)
        assertTrue(intensityOnly.isNarrowed(FilterSection.HISTORY))
        assertFalse(intensityOnly.isNarrowed(FilterSection.SENSORS))

        val statusOnly = QuakeFilterState(stationStatus = QuakeStationStatus.OFFLINE)
        assertTrue(statusOnly.isNarrowed(FilterSection.SENSORS))
        assertFalse(statusOnly.isNarrowed(FilterSection.HISTORY))
    }

    @Test
    fun `station status partitions the roll into disjoint halves`() {
        assertTrue(QuakeStationStatus.ALL.accepts(isOnline = true))
        assertTrue(QuakeStationStatus.ALL.accepts(isOnline = false))

        // ONLINE and OFFLINE must never both accept or both reject the same station,
        // or the two options would overlap and the roll would double-count.
        listOf(true, false).forEach { isOnline ->
            assertEquals(
                "isOnline=$isOnline",
                QuakeStationStatus.ONLINE.accepts(isOnline),
                !QuakeStationStatus.OFFLINE.accepts(isOnline)
            )
        }
    }

    @Test
    fun `widening steps up one radius then gives up the centre entirely`() {
        val near = QuakeFilterState(mode = QuakeFilter.NEAR, radius = QuakeSearchRadius.KM_100)
        assertEquals(QuakeSearchRadius.KM_250, near.widened().radius)

        // Past the widest radius the only wider thing is the whole feed.
        val widest = near.copy(radius = QuakeSearchRadius.KM_1000)
        assertEquals(QuakeFilter.ALL, widest.widened().mode)

        // Nothing to widen when no centre is in play.
        val unfiltered = QuakeFilterState()
        assertEquals(unfiltered, unfiltered.widened())
    }

    @Test
    fun `default radius equals the documented history near radius`() {
        assertEquals(
            SafetyPolicy.HISTORY_NEAR_RADIUS_KM,
            QuakeFilterState.DEFAULT_RADIUS.km
        )
    }
}
