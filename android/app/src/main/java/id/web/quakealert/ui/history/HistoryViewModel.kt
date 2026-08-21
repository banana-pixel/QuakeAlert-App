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
import id.web.quakealert.ui.common.QuakeFilterState
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
        _uiState
    ) { unit, state ->
        state.copy(unitSystem = unit)
    }.stateIn(
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
     * Re-queries page 0 from a pull-to-refresh gesture.
     *
     * Distinct from [load] in what it shows, not in what it fetches: the list the
     * user pulled stays on screen under the indicator instead of being replaced by a
     * skeleton, because a refresh that blanks the row someone was reading looks like
     * a failure. Ignored while any load is already in flight — the gesture is easy
     * to repeat by accident.
     */
    fun onRefresh() {
        if (_uiState.value.isLoading || _uiState.value.isRefreshing) return
        load(isRefresh = true)
    }

    /**
     * Single entry point into the loading → content / error state machine, used by
     * the initial load, [onRetry] and [onRefresh].
     *
     * @param isRefresh routes the in-flight flag to [HistoryUiState.isRefreshing]
     *   instead of [HistoryUiState.isLoading], and keeps the current rows while the
     *   request runs.
     */
    private fun load(isRefresh: Boolean = false) {
        viewModelScope.launch {
            _uiState.update {
                it.copy(
                    isLoading = !isRefresh,
                    isRefreshing = isRefresh,
                    isError = false,
                    errorMessage = null
                )
            }
            try {
                val items = fetchPage(offset = 0)
                _uiState.update {
                    it.copy(
                        items = items,
                        isLoading = false,
                        isRefreshing = false,
                        hasMore = items.size >= PAGE_SIZE
                    )
                }
            } catch (cancellation: CancellationException) {
                // Never treat scope cancellation as a load failure — rethrow so the
                // coroutine machinery sees it and the screen keeps its last state.
                throw cancellation
            } catch (throwable: Throwable) {
                // A failed *refresh* keeps the rows it could not replace: the error
                // screen is for having nothing to show, and after a pull there is
                // still a list. Only a refresh over an empty list surfaces it.
                val hadContent = isRefresh && _uiState.value.items.isNotEmpty()
                if (hadContent) Log.w(TAG, "could not refresh history", throwable)
                _uiState.update {
                    it.copy(
                        isLoading = false,
                        isRefreshing = false,
                        isError = !hadContent,
                        errorMessage = if (hadContent) {
                            null
                        } else {
                            throwable.message ?: LOAD_ERROR_MESSAGE
                        }
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
     * Every criterion is applied server-side — the `range_km` / `latitude` /
     * `longitude` trio for "Near", `min_pga` for the intensity bucket, `since` for
     * the time window — so the filter narrows the *query* rather than hiding rows
     * from a page that was already fetched. That is not only cheaper: filtering a
     * 20-item page down to two locally would read as "no more data" to the screen's
     * prefetch and stall pagination.
     *
     * With no stored position there is nothing to measure a radius from, so the
     * spatial part is dropped and the rest of the filter still applies — the same
     * fail-open reasoning as [id.web.quakealert.domain.AlertGate].
     */
    private suspend fun fetchPage(offset: Int): List<QuakeHistoryItem> {
        val userLocation = apiClient.currentUserLocation()
        val filter = _uiState.value.filter
        val near = filter.mode == QuakeFilter.NEAR
        return apiClient.fetchEvents(
            limit = PAGE_SIZE,
            offset = offset,
            rangeKm = filter.eventsRadiusKm,
            center = userLocation.takeIf { near },
            minPgaGal = filter.minPgaGal,
            since = filter.since()
        ).getOrThrow().toHistoryItems(userLocation)
    }

    /**
     * Adopts the shared filter and re-queries from page 0.
     *
     * Pushed in by [HistoryRoute] from
     * [id.web.quakealert.ui.common.QuakeFilterViewModel] rather than owned here, so
     * the Sensors tab answers the same question without either tab knowing about the
     * other. A reload rather than a local re-filter: the pages already held were
     * selected by the server under the *previous* criteria.
     *
     * The rows are dropped before the reload because they no longer answer the
     * question being asked; the screen shows its skeleton for the moment it takes to
     * fetch a page that does.
     */
    fun applyFilter(filter: QuakeFilterState) {
        if (_uiState.value.filter == filter) return
        _uiState.update { it.copy(filter = filter, items = emptyList(), hasMore = false) }
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
