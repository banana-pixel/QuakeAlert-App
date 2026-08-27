package id.web.quakealert.domain

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Covers the three client-side consequences of the server's Phase 3 lifecycle
 * fields, none of which the compiler can check:
 *
 *  1. [AlertDedup] suppresses an OLDER revision of a known event but not a newer one
 *     — the two delivery channels are unordered relative to each other;
 *  2. a frame carrying no revision (every pre-Phase-3 frame) dedups exactly as it
 *     did before, first-wins;
 *  3. the copy for a WITHDRAWN report does not claim the shaking ended, and an
 *     unrecognised state falls back to all-clear wording rather than to silence.
 */
class AlertLifecycleTest {

    private fun frame(
        type: AlertType = AlertType.EARTHQUAKE_ALERT,
        eventId: String = "1f3c9a10-0000-4000-8000-000000000001",
        revision: Int = 0,
        state: EventState? = null,
        nodeCount: Int = 3
    ) = WsAlertMessage(
        type = type,
        eventId = eventId,
        mmi = "V",
        intensityLabel = "strong",
        pgaGal = 180.0,
        centroidLat = -6.91,
        centroidLon = 107.61,
        locationName = "Bandung, West Java, ID",
        timestampMs = System.currentTimeMillis(),
        nodeCount = nodeCount,
        eventState = state,
        eventRevision = revision
    )

    // --- revision-aware dedup ----------------------------------------------

    @Test
    fun `a newer revision of a known event is news, an older one is not`() {
        val dedup = AlertDedup()
        assertTrue("first frame", dedup.markIfNew(frame(revision = 2)))

        // Out-of-order re-delivery: the FCM copy of an earlier decision arriving after
        // the socket already delivered a later one. Acting on it would re-raise a
        // state the event has already left.
        assertFalse("older revision", dedup.markIfNew(frame(revision = 1)))
        assertFalse("same revision", dedup.markIfNew(frame(revision = 2)))
        assertTrue("newer revision", dedup.markIfNew(frame(revision = 3)))
    }

    @Test
    fun `hasSeen follows the same revision rule as markIfNew`() {
        val dedup = AlertDedup()
        dedup.markIfNew(frame(revision = 2))

        assertTrue("revision already acted on", dedup.hasSeen(frame(revision = 2)))
        assertTrue("older revision", dedup.hasSeen(frame(revision = 1)))
        assertFalse("newer revision is not seen yet", dedup.hasSeen(frame(revision = 3)))
    }

    @Test
    fun `a frame without a revision keeps the pre-Phase-3 first-wins behaviour`() {
        val dedup = AlertDedup()
        assertTrue(dedup.markIfNew(frame()))
        assertFalse("duplicate of a revision-less frame", dedup.markIfNew(frame()))
    }

    @Test
    fun `revision is compared within one event only`() {
        val dedup = AlertDedup()
        dedup.markIfNew(frame(eventId = "evt-a", revision = 5))

        // A different event's revision 1 is not "older" — revisions are per event, and
        // treating them as global would silently drop the start of every new quake
        // after a long-running one.
        assertTrue(dedup.markIfNew(frame(eventId = "evt-b", revision = 1)))
    }

    @Test
    fun `the all-clear is still keyed apart from the alert it clears`() {
        val dedup = AlertDedup()
        assertTrue(dedup.markIfNew(frame(revision = 2)))
        assertTrue(
            "EVENT_RESOLVED shares the event id but not the key",
            dedup.markIfNew(frame(type = AlertType.EVENT_RESOLVED, revision = 3))
        )
    }

    // --- stand-down copy ----------------------------------------------------

    @Test
    fun `a cancelled report is described as withdrawn, not as ended shaking`() {
        val copy = standDownCopyFor(EventState.CANCELLED)
        assertEquals("Report Withdrawn", copy.title)
        assertTrue("must say withdrawn: ${copy.detail}", copy.detail.contains("withdrawn"))
        val lower = copy.detail.lowercase()
        assertFalse("must not claim the shaking ended: ${copy.detail}", lower.contains("ended"))
        assertFalse("must not claim the shaking stopped: ${copy.detail}", lower.contains("stopped"))
    }

    @Test
    fun `an unknown or absent state falls back to all-clear wording`() {
        val fallback = standDownCopyFor(null)
        assertEquals("All Clear", fallback.title)
        // A pre-Phase-3 server sends no state at all; the wording it produced must not
        // change, because an all-clear is never the unsafe direction.
        assertEquals(fallback, standDownCopyFor(EventState.RESOLVED))
    }

    @Test
    fun `no stand-down copy mentions magnitude or epicentre`() {
        val forbidden = listOf("magnitude", "epicentre", "epicenter", "seconds")
        for (state in listOf(null) + EventState.entries) {
            val copy = standDownCopyFor(state)
            val text = "${copy.title} ${copy.detail}".lowercase()
            for (word in forbidden) {
                assertFalse("$state copy claims '$word': $text", text.contains(word))
            }
        }
    }

    // --- unconfirmed copy ---------------------------------------------------

    @Test
    fun `the unconfirmed read-out counts stations and says it is unconfirmed`() {
        assertEquals(
            "1 station is reporting shaking - not yet confirmed by separated stations",
            unconfirmedActivityLabel(1)
        )
        assertEquals(
            "2 stations are reporting shaking - not yet confirmed by separated stations",
            unconfirmedActivityLabel(2)
        )
    }

    @Test
    fun `an absent station count drops the number rather than printing zero`() {
        val label = unconfirmedActivityLabel(0)
        assertFalse("'0 stations' reads as no evidence at all: $label", label.contains("0"))
        assertTrue(label.startsWith("A station is"))
    }

    @Test
    fun `the unconfirmed read-out never reads as a confirmation`() {
        val label = unconfirmedActivityLabel(2).lowercase()
        assertTrue(label.contains("not yet confirmed"))
        assertFalse(label.contains("earthquake detected"))
        assertFalse(label.contains("magnitude"))
    }
}
