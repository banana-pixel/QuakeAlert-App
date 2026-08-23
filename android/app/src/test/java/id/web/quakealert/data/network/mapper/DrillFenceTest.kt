package id.web.quakealert.data.network.mapper

import id.web.quakealert.data.network.model.WsAlertMessageDto
import id.web.quakealert.domain.AlertType
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The client half of the drill fence.
 *
 * A drill exists so an operator can rehearse the alert path; it becomes dangerous the
 * moment it reaches someone who cannot tell it from a real quake, because a person who
 * has learned to ignore the siren once will ignore the one that matters. Two
 * independent fences stop that: the server publishes a drill only to the `test_alerts`
 * FCM topic, which no release build subscribes to
 * (id.web.quakealert.data.push.PushRegistrar), and [toDomainOrNull] drops an `is_test`
 * frame that reaches a release build by any other route — a WebSocket replay, a
 * mis-targeted push.
 *
 * These tests exercise the second fence on both transports, and they exercise the
 * *release* branch specifically: `allowTestAlerts` is a parameter rather than a bare
 * `BuildConfig.DEBUG` read precisely so the branch whose failure would be a public
 * false alarm can be asserted from a debug-only test run.
 */
class DrillFenceTest {

    @Test
    fun `a release build drops a drill frame from the socket`() {
        assertNull(drillDto().toDomainOrNull(allowTestAlerts = false))
    }

    @Test
    fun `a debug build keeps the drill and carries the flag through`() {
        val alert = drillDto().toDomainOrNull(allowTestAlerts = true)

        requireNotNull(alert)
        assertEquals(AlertType.EARTHQUAKE_ALERT, alert.type)
        // The flag has to survive the mapping: it is what puts the "TEST" badge on the
        // emergency card and the word "drill" in the notification shade, which is what
        // stops the tester mistaking the rehearsal for the real thing.
        assertTrue(alert.isTest)
    }

    @Test
    fun `a real alert is untouched by the fence on either build`() {
        val onRelease = realDto().toDomainOrNull(allowTestAlerts = false)
        val onDebug = realDto().toDomainOrNull(allowTestAlerts = true)

        requireNotNull(onRelease)
        requireNotNull(onDebug)
        assertFalse(onRelease.isTest)
        assertFalse(onDebug.isTest)
    }

    @Test
    fun `an absent flag is a real alert, not a drill`() {
        // The server omits `is_test` from a real event's payload entirely, so the
        // default decides how a silent payload reads. It must read as real: a drill
        // shown to a tester is a wasted afternoon, a real quake suppressed as a drill
        // is the failure that cannot be undone.
        assertFalse(realDto().isTest)

        val silentPush = drillPayload()
            .minus("is_test")
            .toWsAlertMessageOrNull(allowTestAlerts = false)
        assertNotNull(silentPush)
        assertFalse(silentPush!!.isTest)
    }

    @Test
    fun `the push mapper reads a drill only from the exact string the server sends`() {
        val drill = drillPayload().toWsAlertMessageOrNull(allowTestAlerts = true)
        requireNotNull(drill)
        assertTrue(drill.isTest)

        // "1", "TRUE", "yes" and a typo all read as a real alert. Failing in this
        // direction means a malformed flag can at worst raise an alarm that need not
        // have been raised; it can never suppress one that had to be.
        for (value in listOf("1", "TRUE", "True", "yes", "ture", "")) {
            val message = drillPayload()
                .plus("is_test" to value)
                .toWsAlertMessageOrNull(allowTestAlerts = false)
            assertNotNull("is_test=$value should stay a real alert", message)
            assertFalse("is_test=$value should stay a real alert", message!!.isTest)
        }
    }

    @Test
    fun `a release build drops a drill that arrives by push`() {
        assertNull(drillPayload().toWsAlertMessageOrNull(allowTestAlerts = false))
    }

    @Test
    fun `surrounding whitespace does not hide a drill flag`() {
        val drill = drillPayload()
            .plus("is_test" to "  true  ")
            .toWsAlertMessageOrNull(allowTestAlerts = true)

        requireNotNull(drill)
        assertTrue(drill.isTest)
    }

    private fun drillDto() = realDto().copy(isTest = true)

    private fun realDto() = WsAlertMessageDto(
        type = "EARTHQUAKE_ALERT",
        eventId = "test-01HZX",
        mmi = "IV",
        intensityLabel = "moderate",
        pgaGal = 61.5,
        centroidLat = -6.91750,
        centroidLon = 107.61910,
        locationName = "Bandung, West Java, ID",
        timestamp = 1_781_913_558_000L,
        nodeCount = 3
    )

    private fun drillPayload(): Map<String, String> = mapOf(
        "type" to "EARTHQUAKE_ALERT",
        "event_id" to "test-01HZX",
        "mmi" to "IV",
        "intensity_label" to "moderate",
        "pga_gal" to "61.5",
        "centroid_lat" to "-6.9175",
        "centroid_lon" to "107.6191",
        "location_name" to "Bandung, West Java, ID",
        "timestamp" to "1781913558000",
        "is_test" to "true"
    )
}
