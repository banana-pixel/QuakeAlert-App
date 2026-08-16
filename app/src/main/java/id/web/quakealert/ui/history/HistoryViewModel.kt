package id.web.quakealert.ui.history

import androidx.lifecycle.ViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update

/**
 * Hosts the [HistoryUiState] for the History screen and exposes it as a
 * [StateFlow] following unidirectional data flow. For now it is seeded with
 * mock data mirroring the Figma design (node 1:701) so the UI can be verified
 * visually before a real data source is wired in.
 */
class HistoryViewModel : ViewModel() {

    private val _uiState = MutableStateFlow(HistoryUiState(items = mockHistoryItems()))
    val uiState: StateFlow<HistoryUiState> = _uiState.asStateFlow()

    /** Switches between the "All" and "Near" filter pills. */
    fun onFilterSelected(filter: HistoryFilter) {
        _uiState.update { it.copy(selectedFilter = filter) }
    }

    /** Placeholder hook for the calendar/date-range picker button. */
    fun onCalendarClicked() {
        // Intentionally empty until a date-range picker is implemented.
    }

    /** Placeholder hook for the per-card share action. */
    fun onShareClicked(item: QuakeHistoryItem) {
        // Intentionally empty until sharing is implemented.
    }

    /** Placeholder hook for the per-card "see more" action. */
    fun onSeeMoreClicked(item: QuakeHistoryItem) {
        // Intentionally empty until a detail screen is implemented.
    }

    private companion object {
        fun mockHistoryItems(): List<QuakeHistoryItem> = listOf(
            QuakeHistoryItem(
                id = "1",
                intensity = "VII",
                severity = MmiSeverity.MODERATE,
                location = "Bandung, West Java, ID",
                date = "20 Jun 2026",
                time = "07:19:18 WIB",
                distanceLabel = "20 km Away"
            ),
            QuakeHistoryItem(
                id = "2",
                intensity = "IX",
                severity = MmiSeverity.SEVERE,
                location = "Lembang, West Java, ID",
                date = "16 Jun 2026",
                time = "04:43:19 WIB",
                distanceLabel = "60 km Away"
            ),
            QuakeHistoryItem(
                id = "3",
                intensity = "VIII",
                severity = MmiSeverity.MODERATE,
                location = "Jakarta, Indonesia",
                date = "18 Jun 2026",
                time = "12:05:45 WIB",
                distanceLabel = "130 km Away"
            ),
            QuakeHistoryItem(
                id = "4",
                intensity = "X",
                severity = MmiSeverity.SEVERE,
                location = "Surabaya, East Java, ID",
                date = "21 Jun 2026",
                time = "09:27:33 WIB",
                distanceLabel = "350 km Away"
            ),
            QuakeHistoryItem(
                id = "5",
                intensity = "XI",
                severity = MmiSeverity.MODERATE,
                location = "Yogyakarta, Central Java, ID",
                date = "22 Jun 2026",
                time = "14:15:09 WIB",
                distanceLabel = "290 km Away"
            ),
            QuakeHistoryItem(
                id = "6",
                intensity = "XII",
                severity = MmiSeverity.SEVERE,
                location = "Malang, East Java, ID",
                date = "23 Jun 2026",
                time = "06:48:52 WIB",
                distanceLabel = "400 km Away"
            ),
            QuakeHistoryItem(
                id = "7",
                intensity = "XIII",
                severity = MmiSeverity.MODERATE,
                location = "Bali, Indonesia",
                date = "24 Jun 2026",
                time = "11:32:04 WITA",
                distanceLabel = "900 km Away"
            )
        )
    }
}
