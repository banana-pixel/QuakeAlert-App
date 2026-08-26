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
 * @param lastAlertLabel the last alert this device showed, already formatted
 *   ("Intensity IV near Cianjur, 20 minutes ago"), or null when it has never shown one.
 * @param radiusLabel the fixed alert radius in the user's unit system ("200 km"),
 *   for the healthy-state body line. The domain stays unit-blind: callers format
 *   from [SafetyPolicy.ALERT_RADIUS_KM] with the same formatter History uses.
 */
data class ProtectionStatus(
    val alertsEnabled: Boolean,
    val notificationsPermitted: Boolean,
    val autoSyncEnabled: Boolean,
    val batteryUnrestricted: Boolean,
    val lastSyncLabel: String?,
    val lastAlertLabel: String? = null,
    val radiusLabel: String = ""
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
            !alertsEnabled -> "Earthquake protection disabled"
            lastSyncLabel == null -> "Watching, but your location is not set"
            !batteryUnrestricted -> "Watching, but alerts may arrive late"
            else -> "Earthquake protection active"
        }

    /**
     * The expanded body: what is wrong, and what the app has done.
     *
     * Only problems are listed. The four-facts-always version read as a settings audit —
     * "Background delivery: unrestricted" is not news, and four rows of it buried the one
     * row that was — so a healthy app now collapses to a single reassurance and each
     * problem costs exactly one line, in the same order [headline] ranks them.
     *
     * The last line is the one thing a user of an early-warning app actually wonders
     * during a quiet month: whether it has ever fired. Present in both states, because
     * "nothing yet" is the reassuring answer and an absent line would only look like a
     * missing feature.
     */
    val lines: List<String>
        get() = buildList {
            if (!notificationsPermitted) {
                add("Notifications are blocked in system settings, so alerts cannot arrive.")
            }
            if (!alertsEnabled) add("Earthquake warnings are turned off. Re-enable them in Settings.")
            if (lastSyncLabel == null) {
                add(
                    if (autoSyncEnabled) {
                        "Your location has not synced yet, so alerts cannot be aimed."
                    } else {
                        "Your location has not synced and auto sync is off."
                    }
                )
            } else if (!autoSyncEnabled) {
                add("Auto sync is off. Your location is from $lastSyncLabel.")
            }
            if (!batteryUnrestricted) {
                add("Battery optimisation is on, which can hold an alert back.")
            }
            // All clear: one calm row instead of the absence of rows, which would leave
            // the expanded notification looking empty rather than looking fine.
            if (deliverable && lastSyncLabel != null && batteryUnrestricted) {
                add("Watching within $radiusLabel of you.")
            }
            add(lastAlertLabel?.let { "Last alert: $it" } ?: "No alerts since you installed QuakeAlert.")
        }
}
