package id.web.quakealert.data.network.mapper

import id.web.quakealert.data.network.model.EventDto
import id.web.quakealert.data.network.model.EventsResponseDto
import id.web.quakealert.data.network.model.ProvisionResponseDto
import id.web.quakealert.data.network.model.SensorDto
import id.web.quakealert.data.network.model.WsAlertMessageDto
import id.web.quakealert.data.UnitSystem
import id.web.quakealert.domain.AlertType
import id.web.quakealert.domain.EventState
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
        // The third metric cell reports how many stations triggered, because the
        // REST contract carries no shaking duration and a permanent dash taught
        // nobody anything.
        assertEquals("3 stations", item.reportingNodesLabel)
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
    fun `the station count is singular at one and absent at zero`() {
        assertEquals("1 station", QuakeFormat.reportingNodes(1))
        assertEquals("4 stations", QuakeFormat.reportingNodes(4))
        // An event exists because stations triggered, so zero is a missing field
        // rather than a real count, and must not print as "0 stations".
        assertEquals(QuakeFormat.UNAVAILABLE, QuakeFormat.reportingNodes(0))
        assertEquals(QuakeFormat.UNAVAILABLE, QuakeFormat.reportingNodes(-1))
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
    fun `phase 3 lifecycle fields are carried through, and their absence is not invented`() {
        val payload = """
            {"type":"EVENT_RESOLVED","event_id":"evt-9","mmi":"V",
             "intensity_label":"strong","pga_gal":180.0,"centroid_lat":-6.9175,
             "centroid_lon":107.6191,"location_name":"Bandung, West Java, ID",
             "timestamp":1781913558000,"node_count":4,"event_state":"CANCELLED",
             "event_revision":3,"origin_ts":1781913555000,
             "origin_ts_source":"SENSOR","independent_cell_count":2}
        """.trimIndent()

        val alert = json.decodeFromString<WsAlertMessageDto>(payload).toDomainOrNull()!!

        assertEquals(EventState.CANCELLED, alert.eventState)
        assertEquals(3, alert.eventRevision)
        assertEquals(1781913555000L, alert.originTsMs)
        assertEquals("SENSOR", alert.originTsSource)
        assertEquals(2, alert.independentCellCount)
        // origin_ts is the onset, timestamp is when the decision was taken: the two
        // must not collapse into one another.
        assertEquals(1781913558000L, alert.timestampMs)

        // A pre-Phase-3 frame carries none of them, and must read as "unknown".
        val legacy = wsDto().toDomainOrNull()!!
        assertNull(legacy.eventState)
        assertEquals(0, legacy.eventRevision)
        assertEquals(0L, legacy.originTsMs)
        assertEquals("", legacy.originTsSource)
        assertEquals(0, legacy.independentCellCount)
    }

    @Test
    fun `an unrecognised event state becomes absent instead of dropping the frame`() {
        // The opposite of the `type` rule, deliberately: a state this build has never
        // heard of must still clear an alarm, so the frame survives with no state.
        val alert = wsDto(type = "EVENT_RESOLVED")
            .copy(eventState = "SUPERSEDED")
            .toDomainOrNull()

        assertEquals(AlertType.EVENT_RESOLVED, alert?.type)
        assertNull("unknown state must read as unknown", alert?.eventState)

        // Casing is not an outage here either.
        assertEquals(
            EventState.UNCONFIRMED,
            wsDto(type = "EARTHQUAKE_ADVISORY").copy(eventState = "unconfirmed")
                .toDomainOrNull()?.eventState
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
            latencyMs = null,
            verified = true
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
            latencyMs = 2,
            verified = true
        ).toDomain()

        assertTrue(!degraded.online)
        assertEquals(SensorStatus.ONLINE, degraded.copy(online = true).toStationItem().status)
    }

    @Test
    fun `provision response decodes verbatim from the server shape`() {
        // Field-for-field from provisionResponse (server/internal/api/api.go):
        // every value is required, so a renamed key must fail loudly here rather
        // than hand the wizard a node it can never link.
        val body = """
            {
              "station_id": "NODE-163A149F",
              "provisioning_secret": "sec_9f2c1d7a8b4e5f60a1b2c3d4e5f6a7b8",
              "mqtt_broker": "broker.quakealert.id",
              "mqtt_port": 8883,
              "mqtt_tls": true
            }
        """.trimIndent()

        val node = json.decodeFromString<ProvisionResponseDto>(body).toDomain()

        assertEquals("NODE-163A149F", node.stationId)
        assertEquals("sec_9f2c1d7a8b4e5f60a1b2c3d4e5f6a7b8", node.provisioningSecret)
        assertEquals("broker.quakealert.id", node.mqttBroker)
        assertEquals(8883, node.mqttPort)
        assertTrue(node.mqttTls)
    }

    @Test
    fun `an unverified station is pending before it is online or offline`() {        // Migration 000005: trust is asked before health. A provisioned node that
        // heartbeats is still Pending — never Online — until an operator confirms it.
        val awaiting = SensorDto(
            stationId = "NODE-D",
            sensorModel = "MPU 6050",
            locationName = "Cimahi, West Java, ID",
            latitude = -6.87,
            longitude = 107.54,
            status = "Pending",
            lastPing = "3s ago",
            rssiDbm = -55,
            latencyMs = 1,
            verified = false
        ).toDomain().toStationItem()
        assertEquals(SensorStatus.PENDING, awaiting.status)

        // Even a payload still saying Online cannot promote an unverified node.
        val mislabeled = SensorDto(
            stationId = "NODE-E",
            sensorModel = "MPU 6050",
            locationName = "Cimahi, West Java, ID",
            status = "Online",
            lastPing = "3s ago",
            verified = false
        ).toDomain().toStationItem()
        assertEquals(SensorStatus.PENDING, mislabeled.status)
    }

    @Test
    fun `station coordinates survive the mapping and null island does not`() {
        val located = SensorDto(
            stationId = "NODE-1",
            sensorModel = "MPU 6050",
            locationName = "Bandung, West Java, ID",
            latitude = -6.9175,
            longitude = 107.6191,
            status = "Online",
            lastPing = "33s ago",
            rssiDbm = -61,
            latencyMs = 2,
            verified = true
        ).toDomain().toStationItem()

        assertEquals(-6.9175, located.latitude!!, 1e-6)
        assertEquals(107.6191, located.longitude!!, 1e-6)
        assertTrue(located.hasPosition)

        // The contract defaults both to 0.0 for a node provisioned without a fix,
        // which would otherwise put a station dot in the Gulf of Guinea.
        val unlocated = SensorDto(
            stationId = "NODE-2",
            sensorModel = "MPU 6050",
            locationName = "Unknown",
            status = "Online",
            lastPing = null,
            rssiDbm = null,
            latencyMs = null,
            verified = true
        ).toDomain().toStationItem()

        assertEquals(null, unlocated.latitude)
        assertEquals(null, unlocated.longitude)
        assertTrue(!unlocated.hasPosition)
    }

    // --- F-3: Phase 3 REST event fields ------------------------------------

    /** Full Phase 3 EventDto with all five new fields populated. */
    private fun phase3EventDto() = EventDto(
        eventId = "evt-phase3",
        status = "RESOLVED",
        pga = 182.4,
        mmi = "VI",
        intensityLabel = "strong",
        latitude = -6.900,
        longitude = 107.600,
        locationName = "Ngamprah, West Java, ID",
        triggeredNodesCount = 4,
        createdAt = "2026-08-28T11:58:16Z",
        resolvedAt = "2026-08-28T12:00:00Z",
        eventState = "CONFIRMED",
        eventRevision = 3,
        originTs = 1755601320000L,
        originTsSource = "SENSOR",
        independentCellCount = 2,
    )

    @Test
    fun `phase 3 event_state maps to EventState enum`() {
        val event = phase3EventDto().toDomain()
        assertEquals(EventState.CONFIRMED, event.eventState)
    }

    @Test
    fun `phase 3 event_revision passes through`() {
        assertEquals(3, phase3EventDto().toDomain().eventRevision)
    }

    @Test
    fun `phase 3 origin_ts passes through as ms`() {
        assertEquals(1755601320000L, phase3EventDto().toDomain().originTsMs)
    }

    @Test
    fun `phase 3 origin_ts_source passes through`() {
        assertEquals("SENSOR", phase3EventDto().toDomain().originTsSource)
    }

    @Test
    fun `phase 3 independent_cell_count passes through`() {
        assertEquals(2, phase3EventDto().toDomain().independentCellCount)
    }

    @Test
    fun `pre-phase3 response absent fields use safe defaults`() {
        // No Phase 3 fields set — the eventDto() helper is a pre-Phase-3 response.
        val event = eventDto().toDomain()
        assertNull("eventState must be null, not fabricated", event.eventState)
        assertEquals("eventRevision default 0", 0, event.eventRevision)
        assertEquals("originTsMs default 0", 0L, event.originTsMs)
        assertEquals("originTsSource default blank", "", event.originTsSource)
        assertEquals("independentCellCount default 0", 0, event.independentCellCount)
    }

    @Test
    fun `unknown event_state string maps to null without crashing`() {
        val dto = phase3EventDto().copy(eventState = "FUTURE_STATE_UNKNOWN")
        assertNull(dto.toDomain().eventState)
    }

    @Test
    fun `null event_state maps to null`() {
        val dto = phase3EventDto().copy(eventState = null)
        assertNull(dto.toDomain().eventState)
    }

    @Test
    fun `zero revision from wire is preserved as zero`() {
        // Zero is the correct default — must not be silently converted to something else.
        val dto = phase3EventDto().copy(eventRevision = 0)
        assertEquals(0, dto.toDomain().eventRevision)
    }

    @Test
    fun `event_state case-insensitive mapping`() {
        // Server sends uppercase; guard against case variation in future.
        assertEquals(EventState.RESOLVED,  phase3EventDto().copy(eventState = "RESOLVED").toDomain().eventState)
        assertEquals(EventState.CANCELLED, phase3EventDto().copy(eventState = "CANCELLED").toDomain().eventState)
        assertEquals(EventState.CONFIRMED, phase3EventDto().copy(eventState = "confirmed").toDomain().eventState)
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
