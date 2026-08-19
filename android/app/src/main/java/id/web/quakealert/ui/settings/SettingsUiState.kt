package id.web.quakealert.ui.settings

import androidx.compose.runtime.Immutable

/**
 * Selectable coverage radius options shown in the Coverage segmented control
 * (Figma node 1:872). Values mirror the "125 / 250 / 500 km" pills.
 *
 * [geofenceFraction] scales the reactive geofence circle drawn on the map preview
 * (0f..1f of the card's minimum dimension) so the visualised radius grows with
 * the selected coverage.
 */
enum class CoverageRange(val km: Int, val label: String, val geofenceFraction: Float) {
    KM_125(125, "125 km", 0.35f),
    KM_250(250, "250 km", 0.6f),
    KM_500(500, "500 km", 0.9f)
}

/**
 * Selectable app languages shown in the "Language" segmented control (Figma node
 * 1:912). Mirrors the "EN / ID" pills.
 */
enum class AppLanguage(val label: String) {
    EN("EN"),
    ID("ID")
}


/**
 * Immutable UI state for the Settings screen (Figma node 1:845, "Location &
 * Coverage" section). Hoisted into [SettingsViewModel] and consumed by the
 * stateless [SettingsScreen] following unidirectional data flow.
 *
 * @param isHealthy drives the top-bar green "Healthy" badge.
 * @param locationLabel current detected location (map location pill).
 * @param coverageRange selected coverage radius (Coverage segmented control);
 *   also drives the reactive geofence circle + range badge on the map preview.
 * @param sensorCount number of sensors within the selected range (map summary).
 * @param lastSyncLabel human-readable last-sync time (e.g. "2 min. ago").
 * @param autoSyncLocation "Auto Sync Location / Intelligent Location Sync" toggle.
 * @param keepAlerting "Keep Alerting" toggle (Alert & Notification section).
 * @param lightMode "Light Mode (Beta)" toggle (Appearance & Look section).
 * @param language selected app language (Language segmented control).
 * @param appCredit primary credit line shown on the About card
 *   ("QuakeAlert App by @banana-pixel").
 * @param appVersion secondary version line shown on the About card
 *   ("v 1.0.1 (Beta)").
 * @param showAboutModal whether the About overlay ([AboutModalDialog]) is open.
 *   Raised by "More About Us" and cleared by the overlay's close button, a back
 *   press or an outside tap.
 */
@Immutable
data class SettingsUiState(
    val isHealthy: Boolean = true,
    val locationLabel: String = "Bandung, West Java, ID",
    val coverageRange: CoverageRange = CoverageRange.KM_500,
    val sensorCount: Int = 2,
    val lastSyncLabel: String = "2 min. ago",
    val autoSyncLocation: Boolean = true,
    val keepAlerting: Boolean = true,
    val lightMode: Boolean = false,
    val language: AppLanguage = AppLanguage.EN,
    val appCredit: String = "QuakeAlert App by @banana-pixel",
    val appVersion: String = "v 1.0.1 (Beta)",
    val showAboutModal: Boolean = false
) {



    /** Pre-formatted "Range : {km} km, {n} sensors" summary badge text. */
    val rangeSummaryLabel: String
        get() = "Range : ${coverageRange.km} km, $sensorCount sensors"

    /** Pre-formatted "Last Sync : {time}" info-pill text. */
    val lastSyncPillLabel: String
        get() = "Last Sync : $lastSyncLabel"
}
