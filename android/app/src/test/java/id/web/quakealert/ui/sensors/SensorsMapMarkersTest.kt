package id.web.quakealert.ui.sensors

import id.web.quakealert.ui.common.MapFocus
import id.web.quakealert.ui.common.MapMarkerKind
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Guards what the Sensors map is allowed to claim.
 *
 * Two failures are worth a test each: a station with no fix drawn at 0,0 — a dot the
 * user would read as a real placement — and a camera pointed somewhere other than the
 * row that was tapped. Both are answered by pure functions here rather than in the
 * composable, which is why they can be tested at all.
 */
class SensorsMapMarkersTest {

    private fun station(
        id: String,
        status: SensorStatus = SensorStatus.ONLINE,
        latitude: Double? = -6.9,
        longitude: Double? = 107.6
    ) = SensorStationItem(
        id = id,
        stationId = id,
        location = "Bandung, West Java, ID",
        chipLabel = "MPU 6050",
        status = status,
        telemetry = SensorTelemetry(lastPing = "-", rssi = "-", latency = "-"),
        latitude = latitude,
        longitude = longitude
    )

    private val positioned = SensorsUiState(
        overview = SensorMapOverview(
            locationLabel = "Bandung, West Java, ID",
            rangeKm = 250,
            sensorCount = 2,
            latitude = -6.9175,
            longitude = 107.6191
        ),
        sensors = listOf(
            station("NODE-1"),
            station("NODE-2", status = SensorStatus.OFFLINE, latitude = -7.0, longitude = 107.9),
            station("NODE-3", latitude = null, longitude = null)
        )
    )

    @Test
    fun `each station is coloured by status and the device gets its own dot`() {
        val markers = positioned.mapMarkers()

        assertEquals(
            listOf(MapMarkerKind.STATION_ONLINE, MapMarkerKind.STATION_OFFLINE, MapMarkerKind.USER),
            markers.map { it.kind }
        )
        // Marker order follows the roll; the *paint* order is the enum's, so a
        // reordering there is a deliberate change and not a side effect of this list.
        assertTrue(MapMarkerKind.STATION_OFFLINE.ordinal < MapMarkerKind.STATION_ONLINE.ordinal)
        assertTrue(MapMarkerKind.SELECTED.ordinal < MapMarkerKind.USER.ordinal)
        // The station with no coordinates contributes nothing at all.
        assertTrue(markers.none { it.id == "NODE-3" })
    }

    @Test
    fun `with no synced position there is no user dot`() {
        val markers = positioned
            .copy(overview = positioned.overview.copy(latitude = null, longitude = null))
            .mapMarkers()

        assertTrue(markers.none { it.kind == MapMarkerKind.USER })
        assertEquals(2, markers.size)
    }

    @Test
    fun `the selected station is drawn as selected and framed by the camera`() {
        val selected = positioned.copy(selectedStationId = "NODE-2")

        assertEquals(
            MapMarkerKind.SELECTED,
            selected.mapMarkers().first { it.id == "NODE-2" }.kind
        )
        val focus = selected.mapFocus()!!
        assertEquals(-7.0, focus.latitude, 1e-6)
        assertEquals(107.9, focus.longitude, 1e-6)
        assertEquals(MapFocus.ZOOM_EVENT, focus.zoom, 1e-6)
    }

    @Test
    fun `no selection frames the device position, and no position frames nothing`() {
        val focus = positioned.mapFocus()!!
        assertEquals(-6.9175, focus.latitude, 1e-6)
        assertEquals(MapFocus.ZOOM_COVERAGE, focus.zoom, 1e-6)

        assertNull(
            positioned
                .copy(overview = positioned.overview.copy(latitude = null, longitude = null))
                .mapFocus()
        )
    }

    @Test
    fun `a selection the roll no longer carries falls back to the device position`() {
        // The ViewModel clears a stale id, but the derivation must not depend on it
        // having done so: a reload racing a tap would otherwise blank the camera.
        val stale = positioned.copy(selectedStationId = "NODE-GONE")

        assertEquals(-6.9175, stale.mapFocus()!!.latitude, 1e-6)
        assertTrue(stale.mapMarkers().none { it.kind == MapMarkerKind.SELECTED })
    }
    @Test
    fun `the pill names the framed station and falls back to the device place`() {
        // The pill and the camera answer the same question, so they are tested against
        // the same states: a selection names its station, no selection names the place.
        assertEquals("Station NODE-2", positioned.copy(selectedStationId = "NODE-2").mapPillLabel())
        assertEquals("Bandung, West Java, ID", positioned.mapPillLabel())
        // A selection the camera cannot move to must not be named either, or the pill
        // would claim a station the map is not showing.
        assertEquals(
            "Bandung, West Java, ID",
            positioned.copy(selectedStationId = "NODE-3").mapPillLabel()
        )
        assertEquals(
            "Bandung, West Java, ID",
            positioned.copy(selectedStationId = "NODE-GONE").mapPillLabel()
        )
    }
}
