package id.web.quakealert.ui.sensors

import id.web.quakealert.data.UnitSystem
import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * Guards the one place the app turns a radius into words. The failure this exists to
 * catch is a confident lie rather than a crash: printing the endpoint's own 500 km
 * ceiling as "Range : 500 km" claims the user chose a radius they never chose, and
 * reads as a limit on what they are being shown.
 */
class SensorMapOverviewTest {

    private val overview = SensorMapOverview(
        locationLabel = "Bandung, West Java, ID",
        rangeKm = 250,
        sensorCount = 4
    )

    @Test
    fun `an explicit radius prints the number in the chosen unit`() {
        assertEquals("Range : 250 km", overview.rangeLabel(UnitSystem.METRIC))
        assertEquals("Range : 155 mi", overview.rangeLabel(UnitSystem.IMPERIAL))
    }

    @Test
    fun `no radius says all areas instead of inventing one`() {
        val unfiltered = overview.copy(rangeKm = null)
        assertEquals("All areas", unfiltered.rangeLabel(UnitSystem.METRIC))
        // Unit-independent: there is no number to convert.
        assertEquals("All areas", unfiltered.rangeLabel(UnitSystem.IMPERIAL))
    }

    @Test
    fun `the count is its own token, pluralised`() {
        assertEquals("4 sensors", overview.countLabel)
        assertEquals("1 sensor", overview.copy(sensorCount = 1).countLabel)
        assertEquals("0 sensors", overview.copy(sensorCount = 0).countLabel)
    }

    @Test
    fun `the badge joins the two halves without conflating them`() {
        // The separator is a middot rather than a comma: the count is not scoped by
        // the radius in the reading a comma invites.
        assertEquals("Range : 250 km · 4 sensors", overview.summaryLabel(UnitSystem.METRIC))
        assertEquals(
            "All areas · 4 sensors",
            overview.copy(rangeKm = null).summaryLabel(UnitSystem.METRIC)
        )
    }

    @Test
    fun `the default state promises no coverage it has not measured`() {
        val initial = SensorsUiState().overview
        assertEquals("All areas", initial.rangeLabel(UnitSystem.METRIC))
        assertEquals(0, initial.sensorCount)
        assertEquals(null, initial.latitude)
        assertEquals(null, initial.longitude)
    }
}
