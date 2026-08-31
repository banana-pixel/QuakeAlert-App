package id.web.quakealert.service

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Tests the event_id-scoped stand-down guard in [WarningNotifier].
 *
 * [WarningNotifier] is an Android `object` — posting and cancelling
 * notifications requires the Android framework, which is not available in
 * JVM unit tests. What *can* be tested without it is the guard predicate
 * itself: given an active event_id and an incoming stand-down event_id, does
 * the notifier decide correctly whether to proceed?
 *
 * The predicate under test: proceed with clear iff
 *   standDownId.isBlank()  OR  activeId.isBlank()  OR  standDownId == activeId
 */
class WarningNotifierStandDownTest {

    /** Mirror of the guard condition in WarningNotifier.clear(). */
    private fun shouldClear(activeId: String, standDownId: String): Boolean {
        if (standDownId.isNotBlank() && activeId.isNotBlank() && standDownId != activeId) {
            return false
        }
        return true
    }

    // --- core event-scoped behaviour ---

    @Test
    fun `stand-down for active event clears`() {
        assertTrue(shouldClear(activeId = "evt-A", standDownId = "evt-A"))
    }

    @Test
    fun `stand-down for different event does NOT clear`() {
        assertFalse(shouldClear(activeId = "evt-A", standDownId = "evt-B"))
    }

    // --- legacy / blank-id paths ---

    @Test
    fun `blank standDownId clears unconditionally (pre-Phase-3 frame)`() {
        // Pre-Phase-3 servers send no event_id. The call site passes "" and the
        // notifier must still clear — this is the safe direction.
        assertTrue(shouldClear(activeId = "evt-A", standDownId = ""))
    }

    @Test
    fun `blank activeId clears unconditionally (nothing was tracked)`() {
        // Nothing posted yet, or posted by a pre-Phase-3 build that did not set
        // activeEventId. A stand-down should still cancel the notification.
        assertTrue(shouldClear(activeId = "", standDownId = "evt-B"))
    }

    @Test
    fun `both blank clears (no id context on either side)`() {
        assertTrue(shouldClear(activeId = "", standDownId = ""))
    }

    // --- duplicate stand-down ---

    @Test
    fun `duplicate stand-down for same event is safe (idempotent)`() {
        // First call clears and resets activeId to "". Second call arrives with the
        // same standDownId but activeId is now "". shouldClear must return true so
        // the cancel() call is repeated — cancel on an already-dismissed notification
        // is a no-op on all Android versions, so this is safe.
        assertTrue("first call",  shouldClear(activeId = "evt-A", standDownId = "evt-A"))
        assertTrue("second call", shouldClear(activeId = "",       standDownId = "evt-A"))
    }

    // --- ordering: notify(A) / clear(B) / notify(B) / clear(A) ---

    @Test
    fun `notify A then clear B then notify B then clear A`() {
        // Simulates two overlapping events. Server sends:
        //   CONFIRMED(A), CONFIRMED(B), RESOLVED(A), RESOLVED(B)
        // Client must: show A, replace with B, ignore RESOLVED(A), clear on RESOLVED(B).
        var activeId = ""

        // notify(A)
        activeId = "evt-A"
        // clear(B) must NOT clear while A is active
        assertFalse("clear(B) while A active", shouldClear(activeId, "evt-B"))
        // notify(B) — overwrites activeId
        activeId = "evt-B"
        // clear(A) must NOT clear while B is active
        assertFalse("clear(A) while B active", shouldClear(activeId, "evt-A"))
        // clear(B) MUST clear while B is active
        assertTrue("clear(B) while B active", shouldClear(activeId, "evt-B"))
        activeId = ""
        // clear(A) after already cleared — harmless
        assertTrue("clear(A) after already gone", shouldClear(activeId, "evt-A"))
    }
}
