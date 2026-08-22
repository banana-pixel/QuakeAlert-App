package id.web.quakealert.domain

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Pins the copy of the ongoing status notification, because that notification is the
 * app's standing claim about itself: it is read when nothing is happening, which is
 * exactly when the user cannot check it against anything.
 *
 * The rule under test is that a claim of protection is only made when every condition
 * holds, and that the worst problem is the one shown collapsed.
 */
class ProtectionStatusTest {

    private fun status(
        alertsEnabled: Boolean = true,
        notificationsPermitted: Boolean = true,
        autoSyncEnabled: Boolean = true,
        batteryUnrestricted: Boolean = true,
        lastSyncLabel: String? = "2 minutes ago"
    ) = ProtectionStatus(
        alertsEnabled = alertsEnabled,
        notificationsPermitted = notificationsPermitted,
        autoSyncEnabled = autoSyncEnabled,
        batteryUnrestricted = batteryUnrestricted,
        lastSyncLabel = lastSyncLabel
    )

    @Test
    fun `everything in place is the only all-clear`() {
        assertEquals("Watching for earthquakes near you", status().headline)
        assertTrue(status().deliverable)
    }

    @Test
    fun `a revoked grant outranks every other problem`() {
        // The one state where the app is silent no matter what it wants, so it must win
        // the headline even when the user's own switch is also off.
        val blocked = status(notificationsPermitted = false, alertsEnabled = false)
        assertEquals("Alerts blocked by system settings", blocked.headline)
        assertFalse(blocked.deliverable)
    }

    @Test
    fun `the user's own switch is reported as theirs, not as a fault`() {
        val off = status(alertsEnabled = false)
        assertEquals("Alerts are turned off", off.headline)
        assertTrue(off.lines.contains("Alerts: off in QuakeAlert"))
    }

    @Test
    fun `no position means no claim of protection`() {
        assertEquals(
            "Watching, but your location is not set",
            status(lastSyncLabel = null).headline
        )
    }

    @Test
    fun `battery optimisation is a late alert, not a blocked one`() {
        assertEquals(
            "Watching, but alerts may arrive late",
            status(batteryUnrestricted = false).headline
        )
    }

    @Test
    fun `auto sync off is named beside the position it affects`() {
        assertTrue(
            status(autoSyncEnabled = false).lines
                .contains("Location: synced 2 minutes ago, auto sync off")
        )
        assertTrue(
            status(autoSyncEnabled = false, lastSyncLabel = null).lines
                .contains("Location: not synced, and auto sync is off")
        )
    }

    @Test
    fun `the body always carries the same four facts in the same order`() {
        // Fixed shape so a missing line never has to be interpreted, and so the
        // collapsed line (lines.first()) is always the alert state.
        val lines = status(
            alertsEnabled = false,
            notificationsPermitted = false,
            autoSyncEnabled = false,
            batteryUnrestricted = false,
            lastSyncLabel = null
        ).lines
        assertEquals(4, lines.size)
        assertEquals(4, status().lines.size)
        assertTrue(lines.first().startsWith("Alerts:"))
        assertTrue(lines[1].startsWith("Notifications:"))
        assertTrue(lines[2].startsWith("Location:"))
        assertTrue(lines[3].startsWith("Background delivery:"))
    }
}
