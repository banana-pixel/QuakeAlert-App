package id.web.quakealert.ui.history

import androidx.compose.runtime.Immutable
import id.web.quakealert.ui.common.QuakeFilter

/**

 * Modified Mercalli Intensity severity buckets used to colour the MMI badge and
 * intensity label on a history card. The Figma design shows two treatments:
 * a moderate (orange) and a severe (red) variant.
 */
enum class MmiSeverity { MODERATE, SEVERE }

/**
 * A single earthquake history entry rendered by [QuakeHistoryCard].
 *
 * @param id stable identity for list keys.
 * @param intensity Roman-numeral MMI label (e.g. "VII").
 * @param severity drives the badge/accent colour.
 * @param location human-readable epicentre (e.g. "Bandung, West Java, ID").
 * @param date formatted date (e.g. "20 Jun 2026").
 * @param time formatted time incl. zone (e.g. "07:19:18 WIB").
 * @param distanceLabel distance-from-user pill text (e.g. "20 km Away").
 */
@Immutable
data class QuakeHistoryItem(

    val id: String,
    val intensity: String,
    val severity: MmiSeverity,
    val location: String,
    val date: String,
    val time: String,
    val distanceLabel: String
)

/**
 * Immutable UI state for the History screen. Hoisted into [HistoryViewModel] and
 * consumed by the stateless [HistoryScreen]. The filter uses the shared
 * [QuakeFilter] enum common to History and Sensors.
 */
@Immutable
data class HistoryUiState(

    val isHealthy: Boolean = true,
    val selectedFilter: QuakeFilter = QuakeFilter.ALL,
    val nearRadiusKm: Int = 39,
    val items: List<QuakeHistoryItem> = emptyList()
)
