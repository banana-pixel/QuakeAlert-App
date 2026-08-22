package id.web.quakealert.domain

/**
 * What the app can honestly claim about alert delivery right now, reduced from the
 * flags the user can change and the grants the OS controls.
 *
 * Pure and Android-free so it can be asserted directly in tests: the notification it
 * feeds is the one surface that speaks for the app while nothing is happening, and a
 * status line that overstates readiness is worse than no line at all. Every field is
 * a fact the app already holds; nothing here is inferred or probed.
 *
 * @param alertsEnabled the user's own alert switch in Settings.
 * @param notificationsPermitted the OS `POST_NOTIFICATIONS` grant. False means every
 *   alert is dropped by the system no matter what the app does.
 * @param autoSyncEnabled whether the position refreshes itself, so alert targeting
 *   keeps up with the user.
 * @param batteryUnrestricted whether the app is exempt from battery optimisation.
 *   Under Doze a push can be held to the next maintenance window, which for an
 *   earthquake is indistinguishable from never arriving.
 * @param lastSyncLabel human-readable age of the stored position ("2 minutes ago"),
 *   or null when no position has ever been synced.
 */
data class ProtectionStatus(
    val alertsEnabled: Boolean,
    val notificationsPermitted: Boolean,
    val autoSyncEnabled: Boolean,
    val batteryUnrestricted: Boolean,
    val lastSyncLabel: String?
) {

    /** Whether an alert would reach the screen at all: the user's switch and the grant. */
    val deliverable: Boolean get() = alertsEnabled && notificationsPermitted

    /**
     * The single line shown collapsed in the shade, worst problem first.
     *
     * Ordered by what it costs the user: a revoked grant silences everything, their
     * own switch is the next most total, then an absent position (alerts arrive but
     * cannot be aimed), then a delivery that may merely be late.
     */
    val headline: String
        get() = when {
            !notificationsPermitted -> "Alerts blocked by system settings"
            !alertsEnabled -> "Alerts are turned off"
            lastSyncLabel == null -> "Watching, but your location is not set"
            !batteryUnrestricted -> "Watching, but alerts may arrive late"
            else -> "Watching for earthquakes near you"
        }

    /**
     * The expanded body: one line per fact, always all four, in a fixed order.
     *
     * Fixed rather than only-the-problems so the shade reads the same every time and
     * the absence of a line never has to be interpreted.
     */
    val lines: List<String>
        get() = listOf(
            if (alertsEnabled) "Alerts: on" else "Alerts: off in QuakeAlert",
            if (notificationsPermitted) {
                "Notifications: allowed"
            } else {
                "Notifications: blocked in system settings"
            },
            when {
                lastSyncLabel == null && !autoSyncEnabled ->
                    "Location: not synced, and auto sync is off"
                lastSyncLabel == null -> "Location: not synced yet"
                !autoSyncEnabled -> "Location: synced $lastSyncLabel, auto sync off"
                else -> "Location: synced $lastSyncLabel"
            },
            if (batteryUnrestricted) {
                "Background delivery: unrestricted"
            } else {
                "Background delivery: may be delayed by battery optimisation"
            }
        )
}
