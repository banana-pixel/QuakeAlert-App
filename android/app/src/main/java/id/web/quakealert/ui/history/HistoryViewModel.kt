package id.web.quakealert.ui.history

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import id.web.quakealert.data.AppSettingsRepository
import id.web.quakealert.data.UnitSystem
import id.web.quakealert.ui.common.QuakeFilter
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/**
 * Hosts the [HistoryUiState] for the History screen and exposes it as a
 * [StateFlow] following unidirectional data flow. For now it is seeded with
 * mock data mirroring the Figma design (node 1:701) so the UI can be verified
 * visually before a real data source is wired in.
 *
 * The persisted [UnitSystem] from [AppSettingsRepository] is folded into every
 * emission so the distance pills, the "Near" filter pill and the share text all
 * render the same unit the user picked in Settings.
 *
 * Sharing is deliberately absent here: firing `Intent.ACTION_SEND` needs a
 * `Context`, not app state, so it lives in [HistoryRoute] alongside the other
 * composition-local work — the same split [id.web.quakealert.ui.settings.SettingsRoute]
 * uses for opening external links.
 */
class HistoryViewModel(application: Application) : AndroidViewModel(application) {

    private val repository = AppSettingsRepository(application)

    private val _uiState = MutableStateFlow(HistoryUiState(isLoading = true))

    val uiState: StateFlow<HistoryUiState> = combine(
        repository.unitSystem,
        _uiState
    ) { unit, state -> state.copy(unitSystem = unit) }.stateIn(
        scope = viewModelScope,
        started = SharingStarted.WhileSubscribed(5_000),
        initialValue = HistoryUiState(isLoading = true)
    )

    init {
        load()
    }

    /**
     * Re-runs the feed load after a failure, from
     * [id.web.quakealert.ui.common.QuakeErrorState]'s "Retry" action.
     */
    fun onRetry() {
        load()
    }

    /**
     * Single entry point into the loading → content / error state machine, used by
     * both the initial load and [onRetry]. [fetchHistory] is the only seam a real
     * REST/Room repository has to replace; the state transitions around it stay
     * exactly as they are.
     */
    private fun load() {
        viewModelScope.launch {
            _uiState.update {
                it.copy(isLoading = true, isError = false, errorMessage = null)
            }
            try {
                val items = fetchHistory()
                _uiState.update { it.copy(items = items, isLoading = false) }
            } catch (cancellation: CancellationException) {
                // Never treat scope cancellation as a load failure — rethrow so the
                // coroutine machinery sees it and the screen keeps its last state.
                throw cancellation
            } catch (throwable: Throwable) {
                _uiState.update {
                    it.copy(
                        isLoading = false,
                        isError = true,
                        errorMessage = throwable.message ?: LOAD_ERROR_MESSAGE
                    )
                }
            }
        }
    }

    /**
     * Fetches the event history. Currently returns the Figma-mirroring mock
     * fixture; `suspend` so swapping in the REST/Room source is a body change
     * rather than a signature change.
     */
    private suspend fun fetchHistory(): List<QuakeHistoryItem> = mockHistoryItems()

    /** Switches between the "All" and "Near" filter pills. */
    fun onFilterSelected(filter: QuakeFilter) {
        _uiState.update { it.copy(selectedFilter = filter) }
    }

    /** Placeholder hook for the calendar/date-range picker button. */
    fun onCalendarClicked() {
        // Intentionally empty until a date-range picker is implemented.
    }

    /**
     * Raises the [id.web.quakealert.ui.common.QuakeEventDetailModalDialog] overlay for the tapped card, from either
     * the card body or its trailing "see more" bar.
     */
    fun onSeeMoreClicked(item: QuakeHistoryItem) {
        _uiState.update { it.copy(selectedEvent = item) }
    }

    /**
     * Closes the Earthquake Details overlay. Called for every dismissal path — the
     * close (X) button, a back press and a tap outside the card.
     */
    fun onDetailDismissed() {
        _uiState.update { it.copy(selectedEvent = null) }
    }

    private companion object {
        /** Fallback copy when a load failure carries no message of its own. */
        const val LOAD_ERROR_MESSAGE =
            "Could not load earthquake history. Check your connection and try again."

        fun mockHistoryItems(): List<QuakeHistoryItem> = listOf(
            QuakeHistoryItem(
                id = "1",
                intensity = "VII",
                severity = MmiSeverity.MODERATE,
                location = "Bandung, West Java, ID",
                date = "20 Jun 2026",
                time = "07:19:18 WIB",
                distanceKm = 20,
                relativeTime = "2 months ago",
                pgaLabel = "61.5 gal",
                durationLabel = "7 sec",
                coordinates = "-6.91750, 107.61910"
            ),
            QuakeHistoryItem(
                id = "2",
                intensity = "IX",
                severity = MmiSeverity.SEVERE,
                location = "Lembang, West Java, ID",
                date = "16 Jun 2026",
                time = "04:43:19 WIB",
                distanceKm = 60,
                relativeTime = "2 months ago",
                pgaLabel = "142.0 gal",
                durationLabel = "23 sec",
                coordinates = "-6.81180, 107.61760"
            ),
            QuakeHistoryItem(
                id = "3",
                intensity = "VIII",
                severity = MmiSeverity.MODERATE,
                location = "Jakarta, Indonesia",
                date = "18 Jun 2026",
                time = "12:05:45 WIB",
                distanceKm = 130,
                relativeTime = "2 months ago",
                pgaLabel = "88.2 gal",
                durationLabel = "12 sec",
                coordinates = "-6.20880, 106.84560"
            ),
            QuakeHistoryItem(
                id = "4",
                intensity = "X",
                severity = MmiSeverity.SEVERE,
                location = "Surabaya, East Java, ID",
                date = "21 Jun 2026",
                time = "09:27:33 WIB",
                distanceKm = 350,
                relativeTime = "2 months ago",
                pgaLabel = "204.7 gal",
                durationLabel = "31 sec",
                coordinates = "-7.25750, 112.75210"
            ),
            QuakeHistoryItem(
                id = "5",
                intensity = "XI",
                severity = MmiSeverity.MODERATE,
                location = "Yogyakarta, Central Java, ID",
                date = "22 Jun 2026",
                time = "14:15:09 WIB",
                distanceKm = 290,
                relativeTime = "2 months ago",
                pgaLabel = "73.4 gal",
                durationLabel = "9 sec",
                coordinates = "-7.79560, 110.36950"
            ),
            QuakeHistoryItem(
                id = "6",
                intensity = "XII",
                severity = MmiSeverity.SEVERE,
                location = "Malang, East Java, ID",
                date = "23 Jun 2026",
                time = "06:48:52 WIB",
                distanceKm = 400,
                relativeTime = "2 months ago",
                pgaLabel = "318.9 gal",
                durationLabel = "44 sec",
                coordinates = "-7.96660, 112.63260"
            ),
            QuakeHistoryItem(
                id = "7",
                intensity = "XIII",
                severity = MmiSeverity.MODERATE,
                location = "Bali, Indonesia",
                date = "24 Jun 2026",
                time = "11:32:04 WITA",
                distanceKm = 900,
                relativeTime = "2 months ago",
                pgaLabel = "45.1 gal",
                durationLabel = "6 sec",
                coordinates = "-8.40950, 115.18890"
            )
        )
    }
}
