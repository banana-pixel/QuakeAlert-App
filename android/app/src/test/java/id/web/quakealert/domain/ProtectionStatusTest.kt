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
        lastSyncLabel: String? = "2 minutes ago",
        lastAlertLabel: String? = null
    ) = ProtectionStatus(
        alertsEnabled = alertsEnabled,
        notificationsPermitted = notificationsPermitted,
        autoSyncEnabled = autoSyncEnabled,
        batteryUnrestricted = batteryUnrestricted,
        lastSyncLabel = lastSyncLabel,
        lastAlertLabel = lastAlertLabel
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
        assertTrue(off.lines.contains("Alerts are switched off in QuakeAlert."))
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
                .contains("Auto sync is off. Your location is from 2 minutes ago.")
        )
        assertTrue(
            status(autoSyncEnabled = false, lastSyncLabel = null).lines
                .contains("Your location has not synced and auto sync is off.")
        )
    }

    @Test
    fun `all clear is one line, and problems replace it one for one`() {
        // The point of the redesign: a healthy app is a single calm row plus the last-alert
        // line, not a four-row audit, and every row beyond that is a thing the user can act
        // on. So one problem is the same height as none — it takes the all-clear's place —
        // and four problems are four rows.
        val clear = status().lines
        assertEquals(2, clear.size)
        assertEquals("Alerts can reach you. Location synced 2 minutes ago.", clear.first())

        assertEquals(2, status(batteryUnrestricted = false).lines.size)
        assertEquals(2, status(alertsEnabled = false).lines.size)
        assertEquals(2, status(lastSyncLabel = null).lines.size)
        assertEquals(3, status(alertsEnabled = false, batteryUnrestricted = false).lines.size)
        assertEquals(
            5,
            status(
                alertsEnabled = false,
                notificationsPermitted = false,
                autoSyncEnabled = false,
                batteryUnrestricted = false,
                lastSyncLabel = null
            ).lines.size
        )
    }

    @Test
    fun `the worst problem leads the body, matching the collapsed headline`() {
        // StatusNotifier shows lines.first() collapsed, so the two must agree about which
        // problem matters most.
        val blocked = status(notificationsPermitted = false, batteryUnrestricted = false)
        assertEquals(
            "Notifications are blocked in system settings, so alerts cannot arrive.",
            blocked.lines.first()
        )
    }

    @Test
    fun `the last alert is always answered, including when there has been none`() {
        // "Nothing yet" is the reassuring answer to the only question a user asks during a
        // quiet month, so the line is present in both states rather than appearing once an
        // alert has fired.
        assertEquals("No alerts since you installed QuakeAlert.", status().lines.last())
        assertEquals(
            "Last alert: Intensity IV near Cianjur, 20 minutes ago",
            status(lastAlertLabel = "Intensity IV near Cianjur, 20 minutes ago").lines.last()
        )
    }
}
