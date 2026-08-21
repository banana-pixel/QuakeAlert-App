package id.web.quakealert.ui.history

import android.app.Application
import android.util.Log
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import id.web.quakealert.data.AppSettingsRepository
import id.web.quakealert.data.UnitSystem
import id.web.quakealert.data.network.QuakeApiClient
import id.web.quakealert.data.network.QuakeNetwork
import id.web.quakealert.data.network.mapper.toHistoryItems
import id.web.quakealert.ui.common.QuakeFilter
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.drop
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/**
 * Hosts the [HistoryUiState] for the History screen and exposes it as a
 * [StateFlow] following unidirectional data flow. Events come from
 * `GET /api/v1/events` via [QuakeApiClient], already sorted newest-first by the
 * server.
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

    private val apiClient = QuakeNetwork.from(application).apiClient

    private val _uiState = MutableStateFlow(HistoryUiState(isLoading = true))

    val uiState: StateFlow<HistoryUiState> = combine(
        repository.unitSystem,
        repository.coverageRadiusKm,
        _uiState
    ) { unit, radiusKm, state ->
        state.copy(unitSystem = unit, nearRadiusKm = radiusKm)
    }.stateIn(
        scope = viewModelScope,
        started = SharingStarted.WhileSubscribed(5_000),
        initialValue = HistoryUiState(isLoading = true)
    )

    init {
        load()
        observeRadius()
    }

    /**
     * Re-runs the feed when the coverage radius changes in Settings — but only while
     * the "Near" filter is active, since that is the only mode the radius alters.
     *
     * `drop(1)` skips the value the flow replays on collection: [load] in `init` has
     * already fetched with it.
     */
    private fun observeRadius() {
        viewModelScope.launch {
            repository.coverageRadiusKm.drop(1).collect {
                if (_uiState.value.selectedFilter == QuakeFilter.NEAR) load()
            }
        }
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
     * both the initial load and [onRetry].
     */
    private fun load() {
        viewModelScope.launch {
            _uiState.update {
                it.copy(isLoading = true, isError = false, errorMessage = null)
            }
            try {
                val items = fetchPage(offset = 0)
                _uiState.update {
                    it.copy(items = items, isLoading = false, hasMore = items.size >= PAGE_SIZE)
                }
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
     * Appends the next page, from the list scrolling near its end.
     *
     * Guarded on three conditions rather than one: a page already in flight, a
     * previous page that came back short, and the initial load still running. The
     * scroll listener fires on every frame near the bottom, so an unguarded call
     * would issue a dozen identical requests.
     */
    fun onLoadMore() {
        val state = _uiState.value
        if (state.isLoading || state.isLoadingMore || !state.hasMore) return

        _uiState.update { it.copy(isLoadingMore = true) }
        viewModelScope.launch {
            try {
                val page = fetchPage(offset = _uiState.value.items.size)
                _uiState.update { current ->
                    current.copy(
                        // De-duplicated by id: a new confirmed event shifts the
                        // server's window while the user scrolls, which would
                        // otherwise re-append the row that moved onto this page.
                        items = (current.items + page).distinctBy { item -> item.id },
                        isLoadingMore = false,
                        hasMore = page.size >= PAGE_SIZE
                    )
                }
            } catch (cancellation: CancellationException) {
                throw cancellation
            } catch (throwable: Throwable) {
                // A failed *append* keeps the list and stops paging rather than
                // replacing content the user is reading with an error screen.
                Log.w(TAG, "could not load more history", throwable)
                _uiState.update { it.copy(isLoadingMore = false, hasMore = false) }
            }
        }
    }

    /**
     * Fetches one page of confirmed events and maps it to the display model.
     *
     * `getOrThrow()` re-raises the client's [Result] failure so the callers' single
     * `try`/`catch` stays the only place that turns a failure into UI state — an
     * [id.web.quakealert.data.network.ApiException] carries the server's own copy,
     * which is what the error screen shows.
     *
     * The "Near" filter is applied server-side by sending the `range_km` /
     * `latitude` / `longitude` trio, so it narrows the *query* rather than hiding
     * rows from a page of distant events. With no stored position there is nothing
     * to measure from, and the unfiltered feed is returned instead — the same
     * fail-open reasoning as [id.web.quakealert.domain.AlertGate].
     */
    private suspend fun fetchPage(offset: Int): List<QuakeHistoryItem> {
        val userLocation = apiClient.currentUserLocation()
        val near = _uiState.value.selectedFilter == QuakeFilter.NEAR
        return apiClient.fetchEvents(
            limit = PAGE_SIZE,
            offset = offset,
            rangeKm = if (near) repository.readCoverageRadiusKm() else null,
            center = userLocation.takeIf { near }
        ).getOrThrow().toHistoryItems(userLocation)
    }

    /**
     * Switches between the "All" and "Near" filter pills, re-querying from page 0.
     *
     * A reload rather than a local filter: "Near" is a server-side `ST_DWithin`
     * query, so the pages already held were selected without it.
     */
    fun onFilterSelected(filter: QuakeFilter) {
        if (_uiState.value.selectedFilter == filter) return
        _uiState.update { it.copy(selectedFilter = filter, items = emptyList(), hasMore = false) }
        load()
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
        const val TAG = "HistoryViewModel"

        /** Fallback copy when a load failure carries no message of its own. */
        const val LOAD_ERROR_MESSAGE =
            "Could not load earthquake history. Check your connection and try again."

        /**
         * Page size for the feed. The contract caps `limit` at 100; 20 is the
         * server's own default and comfortably more than one screenful.
         */
        const val PAGE_SIZE = 20
    }
}
