package id.web.quakealert.device

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The portal's answers, parsed. Transport is deliberately untested (needs
 * Robolectric + a real radio); everything the wizard decides from a response is
 * covered here.
 */
class NodeLinkParseTest {

    // --- /scan: root-level JSON array of SSIDs (firmware serializeJson of JsonArray) ---

    @Test
    fun `scan list parses as a plain string array`() {
        assertEquals(
            listOf("HomeWifi", "Neighbour", "CafeNet"),
            parseScanResponse("""["HomeWifi","Neighbour","CafeNet"]""")
        )
    }

    @Test
    fun `empty scan answer is an empty list`() {
        assertEquals(emptyList<String>(), parseScanResponse("[]"))
    }

    @Test
    fun `garbage scan body is empty rather than a failure`() {
        // A node that answered with anything at all is up; zero visible networks
        // is a fact about the room, not an error state.
        assertEquals(emptyList<String>(), parseScanResponse("<html>error</html>"))
        assertEquals(emptyList<String>(), parseScanResponse(null))
        assertEquals(emptyList<String>(), parseScanResponse(""))
    }

    // --- /config: {"status":"success","station_id":...} ---

    @Test
    fun `successful config yields the echoed station id`() {
        val outcome = parseConfigOutcome(
            """{"status":"success","station_id":"NODE-DEADBEEF"}""", 200
        )
        assertEquals("NODE-DEADBEEF", outcome.getOrNull())
    }

    @Test
    fun `success without an echo yields null id - older firmware stays workable`() {
        val outcome = parseConfigOutcome("""{"status":"success"}""", 200)
        assertNull(outcome.getOrNull())
    }

    @Test
    fun `a 400 rejection surfaces the portal's own message`() {
        val outcome = parseConfigOutcome(
            """{"status":"error","message":"invalid_station_id"}""", 400
        )
        val error = outcome.exceptionOrNull()
        assertTrue(error is PortalRejectedException)
        assertEquals("invalid_station_id", error?.message)
    }

    @Test
    fun `a non-2xx with an unparseable body still fails with something readable`() {
        val outcome = parseConfigOutcome("<html>gateway timeout</html>", 504)
        assertTrue(outcome.exceptionOrNull()?.message?.contains("504") == true)
    }

    @Test
    fun `2xx with status error is a refusal, not a success`() {
        val outcome = parseConfigOutcome("""{"status":"error"}""", 200)
        assertTrue(outcome.exceptionOrNull() is PortalRejectedException)
    }

    @Test
    fun `2xx with garbage fails instead of configuring half-blind`() {
        val outcome = parseConfigOutcome("not json at all", 200)
        assertTrue(outcome.exceptionOrNull() is PortalRejectedException)
    }
}
