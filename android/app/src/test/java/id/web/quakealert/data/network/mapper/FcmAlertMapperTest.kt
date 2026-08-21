package id.web.quakealert.data.network.mapper

import id.web.quakealert.domain.AlertType
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
