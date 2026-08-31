package id.web.quakealert.data.network.mapper

import id.web.quakealert.domain.AlertType
import id.web.quakealert.domain.EventState
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Every value in an FCM `data` map arrives as a string, so this mapper is where a
 * push either becomes a real alert or is dropped. The cases below mirror
 * contracts/fcm/alert_payload.json plus the ways a payload can arrive incomplete.
 */
class FcmAlertMapperTest {

    @Test
    fun `parses a confirmed alert payload with all-string values`() {
        val message = confirmedPayload().toWsAlertMessageOrNull(nowMs = NOW_MS)

        assertNotNull(message)
        requireNotNull(message)
        assertEquals(AlertType.EARTHQUAKE_ALERT, message.type)
        assertEquals("evt_01HZX", message.eventId)
        assertEquals("V", message.mmi)
        assertEquals("strong", message.intensityLabel)
        // Numbers cross the wire as text and must come back as numbers, in gal and
        // degrees, or the distance gate has nothing to measure.
        assertEquals(48.5, message.pgaGal, 1e-9)
        assertEquals(-6.9175, message.centroidLat, 1e-9)
        assertEquals(107.6191, message.centroidLon, 1e-9)
        assertEquals("Bandung, West Java", message.locationName)
        assertEquals(1_755_000_000_000L, message.timestampMs)
        assertEquals(4, message.nodeCount)
    }

    @Test
    fun `keeps the empty event id an advisory carries`() {
        val message = confirmedPayload()
            .plus("type" to "EARTHQUAKE_ADVISORY")
            .plus("event_id" to "")
            .toWsAlertMessageOrNull(nowMs = NOW_MS)

        requireNotNull(message)
        assertEquals(AlertType.EARTHQUAKE_ADVISORY, message.type)
        // Not defaulted to a placeholder: dedup has to be able to see "no key".
        assertEquals("", message.eventId)
    }

    @Test
    fun `parses a resolved payload`() {
        val message = mapOf("type" to "EVENT_RESOLVED", "event_id" to "evt_01HZX")
            .toWsAlertMessageOrNull(nowMs = NOW_MS)

        requireNotNull(message)
        assertEquals(AlertType.EVENT_RESOLVED, message.type)
        assertEquals("evt_01HZX", message.eventId)
    }

    @Test
    fun `accepts the lowercase type the server may emit`() {
        val message = confirmedPayload()
            .plus("type" to "earthquake_alert")
            .toWsAlertMessageOrNull(nowMs = NOW_MS)

        requireNotNull(message)
        assertEquals(AlertType.EARTHQUAKE_ALERT, message.type)
    }

    @Test
    fun `returns null when the type is missing blank or unknown`() {
        val base = confirmedPayload()

        assertNull((base - "type").toWsAlertMessageOrNull(nowMs = NOW_MS))
        assertNull(base.plus("type" to "   ").toWsAlertMessageOrNull(nowMs = NOW_MS))
        // An unrecognised type is not guessable — it decides between alarm, banner
        // and all-clear — so it is dropped rather than defaulted.
        assertNull(base.plus("type" to "VOLCANO_ALERT").toWsAlertMessageOrNull(nowMs = NOW_MS))
    }

    // --- F-6: is_test contract --------------------------------------------------

    @Test
    fun `is_test true string marks frame as drill`() {
        // Server sends exactly "true" (lowercase string) for drills.
        val message = confirmedPayload()
            .plus("is_test" to "true")
            .toWsAlertMessageOrNull(nowMs = NOW_MS, allowTestAlerts = true)

        requireNotNull(message)
        assertTrue("is_test='true' must be a drill", message.isTest)
    }

    @Test
    fun `is_test absent means real alert`() {
        // Server never sends is_test on a real event — absence = real.
        val message = confirmedPayload()
            .toWsAlertMessageOrNull(nowMs = NOW_MS, allowTestAlerts = false)

        requireNotNull(message)
        assertTrue("absent is_test must NOT be drill", !message.isTest)
    }

    @Test
    fun `is_test false string is treated as real alert not drill`() {
        // Server never sends "false"; if it somehow arrives, must not be a drill.
        // Failing safe: a real quake misread as a drill would be dropped on release.
        val message = confirmedPayload()
            .plus("is_test" to "false")
            .toWsAlertMessageOrNull(nowMs = NOW_MS, allowTestAlerts = false)

        requireNotNull(message)
        assertTrue("is_test='false' must NOT be drill", !message.isTest)
    }

    @Test
    fun `unexpected is_test values are treated as real alert`() {
        // Malformed / unexpected values must not trigger the drill fence.
        // Only the exact string "true" (after trimming) is a drill — any other
        // value is treated as real. Note: " true" and "true " both trim to "true"
        // and ARE treated as drills by the mapper, so they belong in the drill
        // test above, not here.
        for (bad in listOf("TRUE", "True", "1", "yes", "drill")) {
            val message = confirmedPayload()
                .plus("is_test" to bad)
                .toWsAlertMessageOrNull(nowMs = NOW_MS, allowTestAlerts = false)
            requireNotNull(message) { "null for is_test='$bad'" }
            assertTrue("is_test='$bad' must NOT be drill", !message.isTest)
        }
    }

    @Test
    fun `drill frame is dropped on release build (allowTestAlerts=false)`() {
        // The client-side drill fence: a drill that reaches a release build is null.
        val message = confirmedPayload()
            .plus("is_test" to "true")
            .toWsAlertMessageOrNull(nowMs = NOW_MS, allowTestAlerts = false)

        assertNull("drill on release build must be dropped", message)
    }

    @Test
    fun `defaults absent numeric fields to zero`() {
        val message = mapOf(
            "type" to "EARTHQUAKE_ALERT",
            "event_id" to "evt_partial"
        ).toWsAlertMessageOrNull(nowMs = NOW_MS)

        requireNotNull(message)
        assertEquals(0.0, message.pgaGal, 0.0)
        assertEquals(0.0, message.centroidLat, 0.0)
        assertEquals(0.0, message.centroidLon, 0.0)
        assertEquals(0, message.nodeCount)
        assertEquals("", message.mmi)
        assertEquals("", message.intensityLabel)
        assertEquals("", message.locationName)
    }

    @Test
    fun `falls back to now for a missing or malformed timestamp`() {
        val absent = (confirmedPayload() - "timestamp").toWsAlertMessageOrNull(nowMs = NOW_MS)
        requireNotNull(absent)
        assertEquals(NOW_MS, absent.timestampMs)

        // Epoch 0 would fail isRecent() and silently discard what may be a live
        // alert, so an unparseable value becomes *now* instead.
        val malformed = confirmedPayload()
            .plus("timestamp" to "not-a-number")
            .toWsAlertMessageOrNull(nowMs = NOW_MS)
        requireNotNull(malformed)
        assertEquals(NOW_MS, malformed.timestampMs)
        assertTrue(malformed.isRecent(nowMs = NOW_MS))
    }

    @Test
    fun `ignores surrounding whitespace and unparseable numbers`() {
        val message = confirmedPayload()
            .plus("mmi" to "  V  ")
            .plus("pga_gal" to " 12.25 ")
            .plus("centroid_lat" to "n/a")
            .toWsAlertMessageOrNull(nowMs = NOW_MS)

        requireNotNull(message)
        assertEquals("V", message.mmi)
        assertEquals(12.25, message.pgaGal, 1e-9)
        assertEquals(0.0, message.centroidLat, 0.0)
    }

    @Test
    fun `parses the phase 3 lifecycle keys, which arrive as strings too`() {
        val message = confirmedPayload()
            .plus("type" to "EVENT_RESOLVED")
            .plus("event_state" to "CANCELLED")
            .plus("event_revision" to "4")
            .plus("origin_ts" to "1754999998000")
            .plus("origin_ts_source" to "PUBLISH_BOUND")
            .plus("independent_cell_count" to "2")
            .toWsAlertMessageOrNull(nowMs = NOW_MS)

        requireNotNull(message)
        assertEquals(EventState.CANCELLED, message.eventState)
        assertEquals(4, message.eventRevision)
        assertEquals(1_754_999_998_000L, message.originTsMs)
        assertEquals("PUBLISH_BOUND", message.originTsSource)
        assertEquals(2, message.independentCellCount)
    }

    @Test
    fun `absent or malformed lifecycle keys read as unknown, not as zero-meaning-something`() {
        // The server omits these keys when it has nothing to say, and a pre-Phase-3
        // server never sends them at all — the same payload the older client parsed.
        val bare = confirmedPayload().toWsAlertMessageOrNull(nowMs = NOW_MS)
        requireNotNull(bare)
        assertNull(bare.eventState)
        assertEquals(0, bare.eventRevision)
        assertEquals(0L, bare.originTsMs)
        assertEquals("", bare.originTsSource)
        assertEquals(0, bare.independentCellCount)

        // Garbage must not drop the frame: the alert or all-clear still has to land.
        val broken = confirmedPayload()
            .plus("event_state" to "???")
            .plus("event_revision" to "two")
            .plus("origin_ts" to "")
            .plus("independent_cell_count" to "-")
            .toWsAlertMessageOrNull(nowMs = NOW_MS)

        requireNotNull(broken)
        assertEquals(AlertType.EARTHQUAKE_ALERT, broken.type)
        assertNull(broken.eventState)
        assertEquals(0, broken.eventRevision)
        assertEquals(0L, broken.originTsMs)
        assertEquals(0, broken.independentCellCount)
    }

    private fun confirmedPayload(): Map<String, String> = mapOf(
        "type" to "EARTHQUAKE_ALERT",
        "event_id" to "evt_01HZX",
        "mmi" to "V",
        "intensity_label" to "strong",
        "pga_gal" to "48.5",
        "centroid_lat" to "-6.9175",
        "centroid_lon" to "107.6191",
        "location_name" to "Bandung, West Java",
        "timestamp" to "1755000000000",
        "node_count" to "4"
    )

    private companion object {
        const val NOW_MS = 1_755_000_123_456L
    }
}
