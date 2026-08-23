package id.web.quakealert.ui.warning

import id.web.quakealert.data.UnitSystem
import id.web.quakealert.ui.history.MmiSeverity
import id.web.quakealert.ui.history.QuakeHistoryItem
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Covers the Warning screen's state contract: the read the composables make without
 * re-deriving it ([WarningUiState.ActiveAlert.proximityLabel]) and the unit fold that
 * keeps a new state variant from silently ignoring the user's Settings choice.
 *
 * Server health is no longer part of this contract — it lives in
 * [id.web.quakealert.domain.ServerConnectionState], covered by its own test.
 */
class WarningUiStateTest {

    private val alert = WarningUiState.ActiveAlert(
        eventId = "evt_1",
        intensityValue = "IV (moderate)",
        distanceKm = 3,
        locationName = "Bandung, West Java, ID"
    )

    // --- proximityLabel ------------------------------------------------------

    @Test
    fun `proximity label pairs distance with the geocoded name`() {
        assertEquals("3 km away (Bandung, West Java, ID)", alert.proximityLabel)
    }

    @Test
    fun `proximity label follows the selected unit system`() {
        assertEquals(
            "2 mi away (Bandung, West Java, ID)",
            alert.withUnitSystem(UnitSystem.IMPERIAL).proximityLabel
        )
    }

    @Test
    fun `unknown distance says so rather than reading as the epicentre`() {
        // "0 km away" on a full-screen alert is the most alarming possible rendering
        // of missing data. The name still renders: the two halves degrade
        // independently, because either can be missing on a real payload.
        assertEquals(
            "Distance unknown (Bandung, West Java, ID)",
            alert.copy(distanceKm = null).proximityLabel
        )
    }

    @Test
    fun `unnamed centroid drops the parenthetical instead of rendering empty braces`() {
        assertEquals("3 km away", alert.copy(locationName = "").proximityLabel)
    }

    @Test
    fun `both halves can degrade at once`() {
        assertEquals(
            "Distance unknown",
            alert.copy(distanceKm = null, locationName = "").proximityLabel
        )
    }

    // --- withUnitSystem ------------------------------------------------------

    @Test
    fun `with unit system preserves the idle payload`() {
        val idle = WarningUiState.Idle(
            banner = ActiveQuakeBanner(
                title = "Recent Earthquake Alert",
                intensityLabel = "Intensity : IV (moderate)",
                timeAgo = "20 minutes ago"
            ),
            sectionTitle = "Stay alert for aftershocks",
            tips = activeQuakeTips(),
            selectedEventDetails = details
        )

        val folded = idle.withUnitSystem(UnitSystem.IMPERIAL)

        assertEquals(UnitSystem.IMPERIAL, folded.unitSystem)
        assertEquals(idle.copy(unitSystem = UnitSystem.IMPERIAL), folded)
    }

    @Test
    fun `with unit system preserves the alert's hardware flags`() {
        // The unit fold runs on every emission, including while the siren is muted
        // and the torch is strobing — neither may be reset by it.
        val engaged = alert.copy(isMuted = true, isSosLightOn = true)

        val folded = engaged.withUnitSystem(UnitSystem.IMPERIAL)

        assertTrue(folded.isMuted)
        assertTrue(folded.isSosLightOn)
        assertEquals(UnitSystem.IMPERIAL, folded.unitSystem)
    }

    @Test
    fun `with unit system does not change which state is current`() {
        // Folding must never promote or demote a variant: the emergency screen is
        // raised by an alert, never by a Settings change. Held through the sealed
        // supertype, which is how the flow in WarningViewModel calls it.
        val idle: WarningUiState = WarningUiState.Idle()
        val active: WarningUiState = alert

        assertTrue(idle.withUnitSystem(UnitSystem.IMPERIAL) is WarningUiState.Idle)
        assertTrue(active.withUnitSystem(UnitSystem.METRIC) is WarningUiState.ActiveAlert)
    }

    // --- suggestedActions ----------------------------------------------------

    @Test
    fun `suggested actions keep the official Drop-Cover-Hold On order`() {
        // The order is the instruction, so it is asserted rather than assumed.
        assertEquals(
            listOf("Drop!", "Cover!", "Hold on!"),
            suggestedActions().map { it.label }
        )
    }

    @Test
    fun `suggested action ids are unique so the row can be keyed`() {
        val ids = suggestedActions().map { it.id }
        assertEquals(ids.size, ids.distinct().size)
    }

    // --- overlays ------------------------------------------------------------

    @Test
    fun `idle opens with no overlay raised`() {
        val idle = WarningUiState.Idle()
        assertNull(idle.selectedEventDetails)
        assertNull(idle.selectedActivity)
    }

    // --- recent seismic activity ---------------------------------------------

    /**
     * The bug this wording exists to fix: the banner used to spend its whole line on
     * the count, so the screen said "No Recent Earthquake" directly above "3 events
     * nearby" and the newest event was reachable only by tapping through.
     */
    @Test
    fun `measured activity names the latest event before the count`() {
        val activity = measuredActivity(eventCount = 3)
        assertEquals(
            "Latest: IV (moderate), 2 days ago \u00b7 3 nearby in 30 days",
            activity.bannerLabel
        )
        assertEquals("No Active Earthquake", activity.bannerTitle)
        assertEquals("3 events", activity.countValue)
    }

    /** The only state in which "No Recent Earthquake" is a true sentence. */
    @Test
    fun `a measured quiet month is the only no-recent-earthquake headline`() {
        val quiet = measuredActivity(eventCount = 0)
        assertEquals("No Recent Earthquake", quiet.bannerTitle)
        assertEquals(
            "No quakes recorded near you in the past 30 days",
            quiet.bannerLabel
        )
    }

    /**
     * An unmeasured neighbourhood must not be reported as a quiet one, so neither
     * failure state may borrow the zero-event headline.
     */
    @Test
    fun `an unmeasured neighbourhood never claims to be quiet`() {
        assertEquals("No Active Earthquake", RecentSeismicActivity().bannerTitle)
        assertEquals(
            "No Active Earthquake",
            measuredActivity(eventCount = 3)
                .copy(availability = ActivityAvailability.UNAVAILABLE)
                .bannerTitle
        )
    }

    @Test
    fun `a single event is not pluralised`() {
        assertEquals("1 event", measuredActivity(eventCount = 1).countValue)
    }

    /** A full page is a floor, so the count has to admit there may be more behind it. */
    @Test
    fun `a capped count reads as a floor`() {
        val activity = measuredActivity(eventCount = 100, isCountCapped = true)
        assertEquals("100+ events", activity.countValue)
        assertEquals(
            "Latest: IV (moderate), 2 days ago \u00b7 100+ nearby in 30 days",
            activity.bannerLabel
        )
    }

    /**
     * The reading this state exists to prevent: a genuinely quiet area says so, and
     * only an area we actually measured is allowed to.
     */
    @Test
    fun `a measured quiet area reports no events`() {
        val activity = measuredActivity(eventCount = 0)
        assertEquals("No events", activity.countValue)
        assertEquals("None recorded", activity.mostRecentValue)
        assertEquals("None recorded", activity.strongestValue)
    }

    @Test
    fun `without a position every value asks for one instead of reporting zero`() {
        val activity = RecentSeismicActivity()
        assertEquals(ActivityAvailability.NO_POSITION, activity.availability)
        assertEquals("Sync your location to see nearby activity", activity.bannerLabel)
        assertEquals("Needs your location", activity.countValue)
        assertEquals("Needs your location", activity.mostRecentValue)
        assertEquals("Needs your location", activity.strongestValue)
    }

    /**
     * A failed query must never render as an absence of earthquakes — even when the
     * numbers it carries happen to be present from an earlier read.
     */
    @Test
    fun `a failed query reports unavailable rather than zero`() {
        val activity = measuredActivity(eventCount = 3)
            .copy(availability = ActivityAvailability.UNAVAILABLE)
        assertEquals("Recent activity unavailable", activity.bannerLabel)
        assertEquals("Unavailable offline", activity.countValue)
        assertEquals("Unavailable offline", activity.strongestValue)
    }

    private fun measuredActivity(
        eventCount: Int,
        isCountCapped: Boolean = false
    ) = RecentSeismicActivity(
        locationLabel = "-6.91750, 107.61910",
        availability = ActivityAvailability.MEASURED,
        eventCount = eventCount,
        isCountCapped = isCountCapped,
        mostRecent = "IV (moderate), 2 days ago".takeIf { eventCount > 0 },
        strongest = "V (strong), 61.5 gal".takeIf { eventCount > 0 },
        latitude = -6.91750,
        longitude = 107.61910
    )

    private val details = QuakeHistoryItem(
        id = "evt_1",
        intensity = "IV",
        severity = MmiSeverity.MODERATE,
        location = "Bandung, West Java, ID",
        date = "20 Jun 2026",
        time = "07:19:18 WIB",
        distanceKm = 3,
        relativeTime = "20 minutes ago",
        pgaLabel = "61.5 gal",
        reportingNodesLabel = "3 stations",
        coordinates = "-6.91750, 107.61910",
        latitude = -6.91750,
        longitude = 107.61910
    )
}
