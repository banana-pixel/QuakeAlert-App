package id.web.quakealert.domain

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Pins the one rule behind every tab's top-bar badge.
 *
 * Worth its own test despite being a one-liner: this predicate replaced five
 * per-screen `isHealthy` derivations that disagreed with each other, and the
 * regression it fixes was exactly a badge that appeared on one tab and not another.
 */
class ServerConnectionStateTest {

    @Test
    fun `connected is healthy`() {
        assertTrue(ServerConnectionState.CONNECTED.isHealthy)
    }

    @Test
    fun `connecting is not yet healthy`() {
        // An attempt in flight is not a reachable backend, and the design ships no
        // amber top-bar variant to say "almost".
        assertFalse(ServerConnectionState.CONNECTING.isHealthy)
    }

    @Test
    fun `disconnected is not healthy`() {
        assertFalse(ServerConnectionState.DISCONNECTED.isHealthy)
    }

    @Test
    fun `health depends on nothing but the connection`() {
        // The whole point of hoisting this out of the screens: no station roll, event
        // feed or mesh presence participates, so no two tabs can disagree.
        assertTrue(ServerConnectionState.entries.filter { it.isHealthy }.size == 1)
    }
}
