package id.web.quakealert.ui.history

import androidx.compose.runtime.Immutable
import id.web.quakealert.data.AppSettingsRepository
import id.web.quakealert.data.UnitSystem
import id.web.quakealert.domain.SafetyPolicy
import id.web.quakealert.ui.common.QuakeFilter

/**

 * Modified Mercalli Intensity severity buckets used to colour the MMI badge and
 * intensity label on a history card. The Figma design shows two treatments:
 * a moderate (orange) and a severe (red) variant.
 */
enum class MmiSeverity { MODERATE, SEVERE }

/**
 * Human-readable name for the bucket, shown as the "Intensity" metric in the
 * Earthquake Details overlay (Figma node 124:1130). Derived from [MmiSeverity]
 * rather than stored on [QuakeHistoryItem] so the overlay's intensity label can
 * never drift out of step with the badge colour it sits next to.
 */
val MmiSeverity.label: String
    get() = when (this) {
        MmiSeverity.MODERATE -> "Moderate"
        MmiSeverity.SEVERE -> "Severe"
    }

/**
 * A single earthquake history entry. Rendered compactly by [QuakeHistoryCard] in
 * the list and in full by [id.web.quakealert.ui.common.QuakeEventDetailModal] when the row is tapped, so it holds
 * both the list-row fields and the detail-only ones.
 *
 * Every field is already display-formatted: this is a UI-state DTO, and keeping
 * formatting out of the composables means the overlay and the card cannot render
 * the same value two different ways.
 *
 * @param id stable identity for list keys.
 * @param intensity Roman-numeral MMI label (e.g. "VII").
 * @param severity drives the badge/accent colour and the overlay's intensity
 *   metric via [MmiSeverity.label].
 * @param location human-readable epicentre (e.g. "Bandung, West Java, ID").
 * @param date formatted date (e.g. "20 Jun 2026").
 * @param time formatted time incl. zone (e.g. "07:19:18 WIB").
 * @param distanceKm distance from the user in kilometres (canonical data; the
 *   displayed "20 km Away" / "12 mi Away" pill is formatted per
 *   [UnitSystem] at the display boundary).
 * @param relativeTime coarse age of the event (e.g. "2 months ago"), detail only.
 * @param pgaLabel peak ground acceleration incl. unit (e.g. "61.5 gal"), detail only.
 * @param durationLabel shaking duration incl. unit (e.g. "7 sec"), detail only.
 * @param coordinates centroid latitude/longitude (e.g. "41.40338, 2.17403"),
 *   detail only.
 */
@Immutable
data class QuakeHistoryItem(

    val id: String,
    val intensity: String,
    val severity: MmiSeverity,
    val location: String,
    val date: String,
    val time: String,
    val distanceKm: Int,
    val relativeTime: String,
    val pgaLabel: String,
    val durationLabel: String,
    val coordinates: String
)

/**
 * Combined date + time line shown in the detail overlay's banner (Figma node
 * 124:1133), which joins the two fields the list card stacks on separate lines.
 */
val QuakeHistoryItem.timestampLabel: String
    get() = "$date  •  $time"

/**
 * Plain-text summary shared by `Intent.ACTION_SEND` from both the list card's
 * share button and the detail overlay's "Share" action. Lives beside the model so
 * both entry points emit byte-identical text.
 */
fun QuakeHistoryItem.toShareText(unitSystem: UnitSystem): String = buildString {
    appendLine("Earthquake — $location")
    appendLine("MMI $intensity (${severity.label})")
    appendLine(timestampLabel)
    appendLine("PGA (Max): $pgaLabel")
    appendLine("Duration: $durationLabel")
    appendLine("Distance from me: ${unitSystem.formatDistance(distanceKm)} Away")
    append("Centroid: $coordinates")
}

/**
 * Immutable UI state for the History screen. Hoisted into [HistoryViewModel] and
 * consumed by the stateless [HistoryScreen]. The filter uses the shared
 * [QuakeFilter] enum common to History and Sensors.
 *
 * [isLoading], [isError] and [errorMessage] form the screen's state machine: the
 * body renders exactly one of loading / error / empty / content. The header's
 * network badge is not derived here — it reads the global
 * [id.web.quakealert.domain.ServerConnectionState] so every tab agrees about it.
 *
 * @param isLoading true while the history feed is in flight.
 * @param isError true when the last load failed; pairs with [errorMessage].
 * @param errorMessage failure copy shown by
 *   [id.web.quakealert.ui.common.QuakeErrorState], or null when there is no error.
 * @param unitSystem distance unit system (Metric / Imperial), persisted via
 *   [id.web.quakealert.data.AppSettingsRepository] and shared with the Sensors
 *   and Settings screens.
 * @param selectedFilter which of the two feed modes is active. "Near" is a
 *   *server-side* filter — it re-queries `/events` with the `range_km` trio rather
 *   than hiding rows locally, so a nearby event on page 3 is not lost behind twenty
 *   distant ones.
 * @param nearRadiusKm the radius that filter uses. Fixed at
 *   [SafetyPolicy.HISTORY_NEAR_RADIUS_KM] — wider than the 200 km alert radius on
 *   purpose: looking back at what happened is not a life-safety path, and the
 *   regional picture is what someone browsing history is after.
 * @param isLoadingMore true while the next page is in flight; distinct from
 *   [isLoading] so appending does not blank the list already on screen.
 * @param hasMore whether another page might exist. The response carries no total,
 *   so this is inferred: a short page means the end.
 * @param selectedEvent the event whose [id.web.quakealert.ui.common.QuakeEventDetailModalDialog] overlay is open,
 *   or null when no overlay is showing. Holding the item itself rather than an id
 *   keeps the overlay a pure function of the state it is handed.
 */
@Immutable
data class HistoryUiState(

    val isLoading: Boolean = false,
    val isError: Boolean = false,
    val errorMessage: String? = null,
    val selectedFilter: QuakeFilter = QuakeFilter.ALL,
    val nearRadiusKm: Int = SafetyPolicy.HISTORY_NEAR_RADIUS_KM,
    val unitSystem: UnitSystem = UnitSystem.METRIC,
    val items: List<QuakeHistoryItem> = emptyList(),
    val isLoadingMore: Boolean = false,
    val hasMore: Boolean = false,
    val selectedEvent: QuakeHistoryItem? = null
)
