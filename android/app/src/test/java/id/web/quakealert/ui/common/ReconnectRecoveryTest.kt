package id.web.quakealert.ui.common

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The automatic-recovery rule shared by the History and Sensors tabs.
 *
 * Tested here rather than through the ViewModels because both build their
 * dependencies from [id.web.quakealert.data.network.QuakeNetwork] in their
 * constructors, leaving no seam to hand them a fake connectivity source.
 */
class ReconnectRecoveryTest {

    @Test
    fun `a screen showing an error reloads when the network returns`() {
        assertTrue(shouldReloadOnReconnect(isError = true, isBusy = false))
    }

    @Test
    fun `a screen showing content is left alone`() {
        // The row the user is reading must not scroll away for a reload nobody asked
        // for: nothing on screen is wrong.
        assertFalse(shouldReloadOnReconnect(isError = false, isBusy = false))
    }

    @Test
    fun `a request already in flight is not raced`() {
        assertFalse(shouldReloadOnReconnect(isError = true, isBusy = true))
        assertFalse(shouldReloadOnReconnect(isError = false, isBusy = true))
    }
}
