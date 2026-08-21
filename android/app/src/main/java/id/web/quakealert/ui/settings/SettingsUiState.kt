package id.web.quakealert.ui.settings

import androidx.compose.runtime.Immutable
import id.web.quakealert.data.AppSettingsRepository
import id.web.quakealert.data.UnitSystem

/**
 * Selectable app languages shown in the "Language" segmented control (Figma node
 * 1:912). Mirrors the "EN / ID" pills.
 *
 * @param tag the BCP-47 tag persisted by [AppSettingsRepository.setLanguage], kept
 *   separate from [label] so the stored value stays a language tag rather than a
 *   piece of UI text.
 */
enum class AppLanguage(val label: String, val tag: String) {
    EN("EN", "en"),
    ID("ID", "id");

    companion object {
        /** The entry for [tag], defaulting to [EN] for anything unrecognised. */
        fun fromTag(tag: String): AppLanguage =
            entries.firstOrNull { it.tag.equals(tag, ignoreCase = true) } ?: EN
    }
}

/**
 * Immutable UI state for the Settings screen (Figma node 1:845). Hoisted into
 * [SettingsViewModel] and consumed by the stateless [SettingsScreen] following
 * unidirectional data flow.
 *
 * Everything here except the transient flags is persisted: the radius, the
 * toggles, the unit system and the language come from
 * [AppSettingsRepository], and the identity fields from
 * [id.web.quakealert.data.local.SessionStore].
 *
 * @param locationLabel the reverse-geocoded name of the last synced position, or
 *   null when the device has never synced one.
 * @param coverageRadiusKm the alert radius in km, within
 *   [AppSettingsRepository.RADIUS_RANGE]. Not decorative: it gates the siren
 *   through [id.web.quakealert.domain.AlertGate] and becomes `range_km` on
 *   `GET /events` and `GET /sensors`.
 * @param sensorCount stations the server reports inside that radius.
 * @param lastSyncLabel human-readable last-sync time ("2 min. ago"), or null for
 *   "never".
 * @param autoSyncLocation whether the app refreshes the position on its own at
 *   start-up when the stored one has gone stale.
 * @param notificationsEnabled the user's own alert switch. Independent of the OS
 *   `POST_NOTIFICATIONS` grant, which [notificationPermissionGranted] carries — a
 *   revoked system permission has to be surfaced rather than silently flipping the
 *   user's preference.
 * @param notificationPermissionGranted whether the OS currently allows posting.
 * @param batteryUnrestricted whether the app is exempt from battery optimisation.
 *   Doze can delay a data-only push, so this is a delivery setting, not a nicety.
 * @param isSyncing a position sync is in flight (the refresh control spins).
 * @param statusMessage one-line result of the last action, shown as a pill and
 *   cleared by the next one.
 * @param pseudonym display name of the anonymous identity, null before bootstrap.
 * @param userId the server-side id of that identity, for support requests.
 * @param isRerolling a pseudonym reroll is in flight.
 * @param isResetting a profile reset (new identity) is in flight.
 * @param showResetDialog whether the destructive-reset confirmation is open.
 * @param lightMode "Light Mode (Beta)" toggle — inert and badged "Coming Soon"
 *   while the app stays dark-theme only.
 * @param language selected app language. Also inert: the strings ship in English
 *   only, so the choice is persisted but not yet applied.
 * @param unitSystem distance unit system, shared with History and Sensors.
 * @param appCredit primary credit line on the About card.
 * @param appVersion secondary version line on the About card.
 * @param showAboutModal whether the About overlay ([AboutModalDialog]) is open.
 */
@Immutable
data class SettingsUiState(
    val locationLabel: String? = null,
    val coverageRadiusKm: Int = AppSettingsRepository.DEFAULT_RADIUS_KM,
    val sensorCount: Int = 0,
    val lastSyncLabel: String? = null,
    val autoSyncLocation: Boolean = true,
    val notificationsEnabled: Boolean = true,
    val notificationPermissionGranted: Boolean = true,
    val batteryUnrestricted: Boolean = false,
    val isSyncing: Boolean = false,
    val statusMessage: String? = null,
    val pseudonym: String? = null,
    val userId: String? = null,
    val isRerolling: Boolean = false,
    val isResetting: Boolean = false,
    val showResetDialog: Boolean = false,
    val lightMode: Boolean = false,
    val language: AppLanguage = AppLanguage.EN,
    val unitSystem: UnitSystem = UnitSystem.METRIC,
    val appCredit: String = "QuakeAlert App by @banana-pixel",
    val appVersion: String = "v 1.0.1 (Beta)",
    val showAboutModal: Boolean = false
) {

    /** Map-card header text; falls back to a prompt when no position is stored. */
    val locationPillLabel: String
        get() = locationLabel?.takeIf { it.isNotBlank() } ?: "Location not set"

    /** Pre-formatted "Range : {km} km, {n} sensors" summary badge text. */
    val rangeSummaryLabel: String
        get() = "Range : ${unitSystem.formatDistance(coverageRadiusKm)}, $sensorCount sensors"

    /** Pre-formatted "Last Sync : {time}" info-pill text. */
    val lastSyncPillLabel: String
        get() = "Last Sync : ${lastSyncLabel ?: "never"}"

    /** The radius in the selected unit system, for the slider's value label. */
    val coverageRadiusLabel: String
        get() = unitSystem.formatDistance(coverageRadiusKm)

    /**
     * Radius of the map preview's geofence circle as a fraction of the card's
     * shorter side.
     *
     * Derived from the km value rather than stored per option: the slider is
     * continuous, so the circle has to grow continuously with it. The range is
     * deliberately narrow — a circle that shrank to a dot at 50 km would read as
     * "no coverage" rather than "less coverage".
     */
    val geofenceFraction: Float
        get() {
            val range = AppSettingsRepository.RADIUS_RANGE
            val span = (range.last - range.first).toFloat()
            val progress = (coverageRadiusKm - range.first).coerceAtLeast(0) / span
            return MIN_GEOFENCE_FRACTION +
                progress.coerceIn(0f, 1f) * (MAX_GEOFENCE_FRACTION - MIN_GEOFENCE_FRACTION)
        }

    /**
     * Whether alerts can actually reach the user right now: their own switch is on
     * *and* the OS still permits notifications.
     */
    val alertsDeliverable: Boolean
        get() = notificationsEnabled && notificationPermissionGranted

    private companion object {
        const val MIN_GEOFENCE_FRACTION = 0.35f
        const val MAX_GEOFENCE_FRACTION = 0.95f
    }
}
