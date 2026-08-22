package id.web.quakealert.data.network.mapper

import id.web.quakealert.data.network.model.EventDto
import id.web.quakealert.data.network.model.EventsResponseDto
import id.web.quakealert.data.network.model.SensorDto
import id.web.quakealert.data.network.model.WsAlertMessageDto
import id.web.quakealert.data.UnitSystem
import id.web.quakealert.domain.AlertType
import id.web.quakealert.domain.EventStatus
import id.web.quakealert.domain.UserLocation
import id.web.quakealert.ui.history.MmiSeverity
import id.web.quakealert.ui.history.distanceLabel
import id.web.quakealert.ui.sensors.SensorStatus
import kotlinx.serialization.json.Json
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Instant
import java.time.ZoneId

/**
 * Covers the wire → domain → UI mapping layer: the field-level contract with the
 * server, the coercions that keep one malformed row from blanking a feed, and the
 * display strings the cards render.
 */
class MappersTest {

    private val json = Json { ignoreUnknownKeys = true; explicitNulls = false }

    private val jakarta: ZoneId = ZoneId.of("Asia/Jakarta")

    // Bandung, the coordinates the design's fixtures use.
    private val user = UserLocation(latitude = -6.91750, longitude = 107.61910)

    private fun eventDto(
        status: String = "HAPPENING",
        pga: Double = 61.5,
        mmi: String = "V",
        intensityLabel: String = "moderate",
        createdAt: String = "2026-06-20T00:19:18Z"
    ) = EventDto(
        eventId = "evt-1",
        status = status,
        pga = pga,
        mmi = mmi,
        intensityLabel = intensityLabel,
        latitude = -6.91750,
        longitude = 107.61910,
        locationName = "Bandung, West Java, ID",
        triggeredNodesCount = 3,
        createdAt = createdAt
    )
    @Test
    fun `event dto maps to domain in canonical units`() {
        val event = eventDto().toDomain()

        assertEquals("evt-1", event.eventId)
        assertEquals(EventStatus.HAPPENING, event.status)
        assertEquals(61.5, event.pgaGal, 0.0)
        assertEquals(Instant.parse("2026-06-20T00:19:18Z"), event.createdAt)
        // depth_km is null by contract and must stay null, never 0.0.
        assertNull(event.depthKm)
        assertNull(event.resolvedAt)
    }

    @Test
    fun `unknown status is treated as still happening`() {
        assertEquals(EventStatus.HAPPENING, eventDto(status = "SOMETHING_NEW").toDomain().status)
        assertEquals(EventStatus.RESOLVED, eventDto(status = "resolved").toDomain().status)
    }

    @Test
    fun `unparseable timestamp falls back to the epoch instead of dropping the row`() {
        assertEquals(Instant.EPOCH, eventDto(createdAt = "not-a-date").toDomain().createdAt)
    }

    @Test
    fun `severity follows the label first and the pga threshold second`() {
        assertEquals(MmiSeverity.SEVERE, eventDto(intensityLabel = "strong").toDomain().severity())
        assertEquals(MmiSeverity.MODERATE, eventDto(intensityLabel = "light").toDomain().severity())
        // Blank label → the server's own 137.2 gal boundary decides.
        assertEquals(
            MmiSeverity.SEVERE,
            eventDto(intensityLabel = "", pga = 137.2).toDomain().severity()
        )
        assertEquals(
            MmiSeverity.MODERATE,
            eventDto(intensityLabel = "", pga = 137.1).toDomain().severity()
        )
    }

    @Test
    fun `history item is fully pre-formatted for the card`() {
        val item = eventDto().toDomain().toHistoryItem(
            userLocation = user,
            zone = jakarta,
            now = Instant.parse("2026-06-20T00:29:18Z")
        )

        assertEquals("20 Jun 2026", item.date)
        assertTrue(item.time.startsWith("07:19:18"))
        assertEquals("61.5 gal", item.pgaLabel)
        assertEquals("-6.91750, 107.61910", item.coordinates)
        assertEquals("10 minutes ago", item.relativeTime)
        // No duration in the REST contract; the card shows a dash, never a guess.
        assertEquals(QuakeFormat.UNAVAILABLE, item.durationLabel)
        assertEquals(0, item.distanceKm)
    }

    @Test
    fun `an unsynced position leaves the distance unknown rather than zero`() {
        val item = eventDto().toDomain().toHistoryItem(
            userLocation = null,
            zone = jakarta,
            now = Instant.parse("2026-06-20T00:29:18Z")
        )

        // Not 0: "0 km Away" reads as *at the epicentre*, which is the most
        // alarming thing the card could say about a fact we do not have.
        assertNull(item.distanceKm)
        assertEquals("Distance unknown", item.distanceLabel(UnitSystem.METRIC))
        assertEquals("Distance unknown", item.distanceLabel(UnitSystem.IMPERIAL))
    }

    @Test
    fun `a known distance is printed in the chosen unit`() {
        val item = eventDto().toDomain().toHistoryItem(
            userLocation = UserLocation(latitude = -6.20880, longitude = 106.84560),
            zone = jakarta,
            now = Instant.parse("2026-06-20T00:19:18Z")
        )

        assertTrue(item.distanceLabel(UnitSystem.METRIC).endsWith(" km Away"))
        assertTrue(item.distanceLabel(UnitSystem.IMPERIAL).endsWith(" mi Away"))
    }

    @Test
    fun `distance is measured from the stored user position`() {
        // Bandung → Jakarta is ~118 km great-circle.
        val item = eventDto().toDomain().toHistoryItem(
            userLocation = UserLocation(latitude = -6.20880, longitude = 106.84560),
            zone = jakarta,
            now = Instant.parse("2026-06-20T00:19:18Z")
        )
        assertTrue("expected ~118 km, was ${item.distanceKm}", item.distanceKm in 113..123)
    }

    @Test
    fun `relative time is coarse and never negative`() {
        val base = Instant.parse("2026-06-20T00:00:00Z")
        assertEquals("just now", QuakeFormat.relativeTime(base, base.plusSeconds(59)))
        assertEquals("1 minute ago", QuakeFormat.relativeTime(base, base.plusSeconds(60)))
        assertEquals("2 hours ago", QuakeFormat.relativeTime(base, base.plusSeconds(7_200)))
        assertEquals("2 months ago", QuakeFormat.relativeTime(base, base.plusSeconds(5_200_000)))
        // Device clock behind the server must not render a negative age.
        assertEquals("just now", QuakeFormat.relativeTime(base, base.minusSeconds(600)))
    }

    @Test
    fun `events envelope decodes and tolerates unknown server fields`() {
        val payload = """
            {"limit":20,"offset":0,"count":1,"future_field":"ignored","events":[
              {"event_id":"evt-1","status":"HAPPENING","pga":61.5,"mmi":"V",
               "intensity_label":"moderate","latitude":-6.9175,"longitude":107.6191,
               "depth_km":null,"location_name":"Bandung, West Java, ID",
               "triggered_nodes_count":3,"created_at":"2026-06-20T00:19:18Z"}]}
        """.trimIndent()

        val response = json.decodeFromString<EventsResponseDto>(payload)

        assertEquals(1, response.events.size)
        assertEquals("evt-1", response.events.first().eventId)
        assertNull(response.events.first().depthKm)
    }

    @Test
    fun `websocket alert frame maps to a domain alert`() {
        val payload = """
            {"type":"EARTHQUAKE_ALERT","event_id":"evt-1","mmi":"IV",
             "intensity_label":"moderate","pga_gal":61.5,"centroid_lat":-6.9175,
             "centroid_lon":107.6191,"location_name":"Bandung, West Java, ID",
             "timestamp":1781913558000,"node_count":3}
        """.trimIndent()

        val alert = json.decodeFromString<WsAlertMessageDto>(payload).toDomainOrNull()

        assertEquals(AlertType.EARTHQUAKE_ALERT, alert?.type)
        assertEquals(1781913558000L, alert?.timestampMs)
        assertEquals("Intensity : IV (moderate)", alert?.intensityBannerLabel())
        assertEquals(MmiSeverity.MODERATE, alert?.severity())
    }

    @Test
    fun `unknown frame type is dropped rather than guessed into a bucket`() {
        val dto = wsDto(type = "EARTHQUAKE_SOMETHING")
        assertNull(dto.toDomainOrNull())
        // Case-insensitive so a server-side casing change is not an outage.
        assertEquals(
            AlertType.EVENT_RESOLVED,
            wsDto(type = "event_resolved").toDomainOrNull()?.type
        )
    }

    @Test
    fun `advisory without an event id still gets a stable list key`() {
        val advisory = wsDto(type = "EARTHQUAKE_ADVISORY", eventId = "").toDomainOrNull()
        val item = advisory!!.toHistoryItem(userLocation = user, zone = jakarta)
        assertEquals("advisory-1781913558000", item.id)
    }

    @Test
    fun `replayed alerts age out of the recent window`() {
        val alert = wsDto().toDomainOrNull()!!
        val fiveMinutesLater = alert.timestampMs + 5 * 60 * 1000L
        val hourLater = alert.timestampMs + 60 * 60 * 1000L

        assertTrue(alert.isRecent(nowMs = fiveMinutesLater))
        assertTrue(!alert.isRecent(nowMs = hourLater))
        // Device clock behind the server: a live alert is never discarded as stale.
        assertTrue(alert.isRecent(nowMs = alert.timestampMs - 30_000))
    }

    @Test
    fun `an offline station shows dashes instead of stale telemetry`() {
        val station = SensorDto(
            stationId = "NODE-163A149F",
            sensorModel = "MPU 6050",
            locationName = "Cimahi, West Java, ID",
            latitude = -6.87220,
            longitude = 107.54250,
            status = "Offline",
            lastPing = null,
            rssiDbm = null,
            latencyMs = null
        ).toDomain().toStationItem()

        assertEquals(SensorStatus.OFFLINE, station.status)
        assertEquals("NODE-163A149F", station.id)
        assertEquals("Last Ping : ${QuakeFormat.UNAVAILABLE}", station.telemetry.lastPing)
        assertEquals("RSSI : ${QuakeFormat.UNAVAILABLE} dBm", station.telemetry.rssi)
    }

    @Test
    fun `any status other than online counts as down`() {
        val degraded = SensorDto(
            stationId = "NODE-1",
            sensorModel = "MPU 6050",
            locationName = "Bandung, West Java, ID",
            latitude = -6.9175,
            longitude = 107.6191,
            status = "Degraded",
            lastPing = "33s ago",
            rssiDbm = -61,
            latencyMs = 2
        ).toDomain()

        assertTrue(!degraded.online)
        assertEquals(SensorStatus.ONLINE, degraded.copy(online = true).toStationItem().status)
    }

    private fun wsDto(
        type: String = "EARTHQUAKE_ALERT",
        eventId: String = "evt-1"
    ) = WsAlertMessageDto(
        type = type,
        eventId = eventId,
        mmi = "IV",
        intensityLabel = "moderate",
        pgaGal = 61.5,
        centroidLat = -6.91750,
        centroidLon = 107.61910,
        locationName = "Bandung, West Java, ID",
        timestamp = 1781913558000L,
        nodeCount = 3
    )
}
