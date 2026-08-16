package id.web.quakealert.ui.settings

import androidx.compose.runtime.Immutable

/**
 * Selectable coverage radius options shown in the Coverage segmented control
 * (Figma node 1:872). Values mirror the "125 / 250 / 500 km" pills.
 */
enum class CoverageRange(val km: Int, val label: String) {
    KM_125(125, "125 km"),
    KM_250(250, "250 km"),
    KM_500(500, "500 km")
}

/**
 * App display language (Figma node 1:912): English or Bahasa Indonesia.
 */
enum class AppLanguage(val label: String) {
    EN("EN"),
    ID("ID")
}

/**
 * Immutable UI state for the Settings screen (Figma node 1:845). Hoisted into
 * [SettingsViewModel] and consumed by the stateless [SettingsScreen] following
 * unidirectional data flow.
 *
 * @param isHealthy drives the top-bar green "Healthy" badge.
 * @param locationLabel current detected location (e.g. "Bandung, West Java, ID").
 * @param coverageRange selected coverage radius (Coverage segmented control).
 * @param sensorCount number of sensors within the selected range (map summary).
 * @param lastSyncLabel human-readable last-sync time (e.g. "2 min. ago").
 * @param autoSyncLocation "Auto Sync Location / Intelligent Location Sync" toggle.
 * @param keepAlerting "Keep Alerting" toggle.
 * @param lightMode "Light Mode (Beta)" appearance toggle.
 * @param language selected app language.
 * @param appVersion version footer text on the About card.
 */
@Immutable
data class SettingsUiState(
    val isHealthy: Boolean = true,
    val locationLabel: String = "Bandung, West Java, ID",
    val coverageRange: CoverageRange = CoverageRange.KM_500,
    val sensorCount: Int = 2,
    val lastSyncLabel: String = "2 min. ago",
    val autoSyncLocation: Boolean = true,
    val keepAlerting: Boolean = false,
    val lightMode: Boolean = false,
    val language: AppLanguage = AppLanguage.EN,
    val appVersion: String = "v 1.0.1 (Beta)"
) {
    /** Pre-formatted "Range : {km} km, {n} sensors" summary badge text. */
    val rangeSummaryLabel: String
        get() = "Range : ${coverageRange.km} km, $sensorCount sensors"

    /** Pre-formatted "Last Sync : {time}" info-pill text. */
    val lastSyncPillLabel: String
        get() = "Last Sync : $lastSyncLabel"
}
