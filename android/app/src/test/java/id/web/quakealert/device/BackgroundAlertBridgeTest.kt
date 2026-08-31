package id.web.quakealert.device

import id.web.quakealert.domain.AlertDedup
import id.web.quakealert.domain.AlertType
import id.web.quakealert.domain.WsAlertMessage
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Tests for the background-alert decision logic in [BackgroundAlertBridge].
 *
 * The bridge's observable contract is three-fold:
 *  1. foreground frames never notify (the ViewModel path owns them);
 *  2. background CONFIRMED frames mark dedup and proceed to notification;
 *  3. EVENT_RESOLVED always marks dedup and clears, in both states.
 *
 * Notification posting itself needs Android framework classes (NotificationManager),
 * so these tests drive the decision layer: dedup interaction + foreground gating.
 */
class BackgroundAlertBridgeTest {

    private fun alert(
        type: AlertType = AlertType.EARTHQUAKE_ALERT,
        eventId: String = "6384a245-test",
        ageMs: Long = 1_000
    ): WsAlertMessage = WsAlertMessage(
        type = type,
        eventId = eventId,
        mmi = "V",
        intensityLabel = "moderate",
        pgaGal = 60.0,
        centroidLat = -6.91,
        centroidLon = 107.61,
        locationName = "CANARY TEST",
        timestampMs = System.currentTimeMillis() - ageMs,
        nodeCount = 3,
        isTest = false
    )

    // --- foreground gating -------------------------------------------------

    @Test
    fun `foreground flag starts true`() {
        // A fresh process is foreground until lifecycle says otherwise — matches
        // the app opening directly onto the Warning screen.
        assertTrue(BackgroundAlertBridge.foreground)
    }

    @Test
    fun `onBackground flips state off and onForeground back on`() {
        BackgroundAlertBridge.onBackground()
        assertFalse("after onBackground", BackgroundAlertBridge.foreground)

        BackgroundAlertBridge.onForeground()
        assertTrue("after onForeground", BackgroundAlertBridge.foreground)
    }

    // --- dedup interaction: WS then FCM for one event ----------------------

    @Test
    fun `ws marks dedup so later fcm copy is suppressed - shared dedup instance`() {
        val dedup = AlertDedup()
        val msg = alert()

        // Simulate the bridge marking a background WS frame...
        assertTrue("first sight must count as new", dedup.markIfNew(msg))
        // ...and the FCM service using the SAME dedup instance seeing the copy:
        assertFalse("FCM duplicate must be suppressed", dedup.markIfNew(msg))
    }

    @Test
    fun `fcm arriving before ws also suppresses the ws copy`() {
        val dedup = AlertDedup()
        val msg = alert()

        assertTrue(dedup.markIfNew(msg)) // FCM first
        assertFalse(dedup.markIfNew(msg)) // WS second — still suppressed
    }

    @Test
    fun `advisory with blank event_id never blocks a subsequent real alert`() {
        val dedup = AlertDedup()
        val advisory = alert(type = AlertType.EARTHQUAKE_ADVISORY, eventId = "")
        val confirmed = alert(type = AlertType.EARTHQUAKE_ALERT, eventId = "REAL-0001")

        // Advisories have no usable key: they never poison dedup.
        assertTrue(dedup.markIfNew(advisory))
        assertTrue("confirmed alert after advisory must still raise", dedup.markIfNew(confirmed))
    }

    // --- resolution ---------------------------------------------------------

    @Test
    fun `resolved marks dedup so late fcm resolved copy is suppressed too`() {
        val dedup = AlertDedup()
        val resolved = alert(type = AlertType.EVENT_RESOLVED, eventId = "EVT-1")

        assertTrue(dedup.markIfNew(resolved))
        assertFalse(dedup.markIfNew(resolved)) // duplicate all-clear suppressed
    }

    // F-1: event_id-scoped stand-down (dedup layer)
    @Test
    fun `resolved for event B does not mark event A as seen in dedup`() {
        val dedup = AlertDedup()
        val alertA   = alert(type = AlertType.EARTHQUAKE_ALERT, eventId = "EVT-A")
        val resolvedB = alert(type = AlertType.EVENT_RESOLVED,  eventId = "EVT-B")
        val resolvedA = alert(type = AlertType.EVENT_RESOLVED,  eventId = "EVT-A")

        assertTrue("alert A is new",      dedup.markIfNew(alertA))
        assertTrue("resolved B is new",   dedup.markIfNew(resolvedB))
        // resolved B must NOT consume the resolved-A slot — A still needs clearing
        assertTrue("resolved A still new after B resolved", dedup.markIfNew(resolvedA))
    }

    @Test
    fun `resolved for same event is suppressed as duplicate`() {
        val dedup = AlertDedup()
        val resolved = alert(type = AlertType.EVENT_RESOLVED, eventId = "EVT-A")

        assertTrue("first resolved",     dedup.markIfNew(resolved))
        assertFalse("duplicate resolved", dedup.markIfNew(resolved))
    }

    // --- staleness guard ----------------------------------------------------

    @Test
    fun `stale background alert fails isRecent and would not raise`() {
        val stale = alert(ageMs = 16 * 60 * 1000) // 16 minutes old — past the 15-minute window
        assertFalse("stale alerts must not wake anyone", stale.isRecent())
    }

    @Test
    fun `fresh background alert passes isRecent`() {
        val fresh = alert(ageMs = 2_000)
        assertTrue(fresh.isRecent())
    }
}
