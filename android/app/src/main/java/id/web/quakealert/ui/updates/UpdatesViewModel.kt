package id.web.quakealert.ui.updates

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import id.web.quakealert.data.network.QuakeNetwork
import id.web.quakealert.data.network.mapper.toUpdateItems
import id.web.quakealert.domain.OperatorUpdate
import id.web.quakealert.ui.common.errorCopy
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.time.Instant

/**
 * Hosts the [UpdatesUiState] for the Updates overlay: operator announcements from
 * `GET /api/v1/broadcasts`, plus anything that arrives on the socket while the
 * overlay is open.
 *
 * Loads once on construction rather than on every recomposition — the overlay is
 * opened from Settings and the ViewModel is scoped to it, so construction *is* the
 * open. [refresh] exists for the retry action.
 *
 * The socket subscription is what makes the list live: a user reading it while a
 * notice is published sees the notice appear rather than having to reopen the
 * overlay. The same announcement can also arrive as a push copy, so the merge is
 * keyed on [OperatorUpdate.id] — one notice, one row, whichever path delivered it
 * first.
 */
class UpdatesViewModel(application: Application) : AndroidViewModel(application) {

    private val network = QuakeNetwork.from(application)

    private val _uiState = MutableStateFlow(UpdatesUiState(isLoading = true))

    val uiState: StateFlow<UpdatesUiState> = _uiState.asStateFlow()

    /**
     * The domain rows behind [uiState], kept so a socket frame can be merged into the
     * loaded page without re-requesting it. Re-formatted on every change, because the
     * relative ages are computed against *now*.
     */
    private var held: List<OperatorUpdate> = emptyList()

    init {
        load()
        observeSocket()
    }

    /** Retry hook for the error state's action. */
    fun refresh() = load()

    private fun load() {
        _uiState.update { it.copy(isLoading = true, error = null) }
        viewModelScope.launch {
            network.apiClient.fetchBroadcasts()
                .onSuccess { updates ->
                    held = updates.newestFirst()
                    _uiState.value = UpdatesUiState(
                        isLoading = false,
                        updates = held.toUpdateItems(Instant.now())
                    )
                }
                .onFailure { failure ->
                    if (failure is CancellationException) throw failure
                    _uiState.value = UpdatesUiState(
                        isLoading = false,
                        // The page already held is kept: a failed refresh must not
                        // blank a list the user was reading.
                        updates = held.toUpdateItems(Instant.now()),
                        error = if (held.isEmpty()) errorCopy(failure) else null
                    )
                }
        }
    }

    /**
     * Folds live announcements into the loaded page.
     *
     * Collected for the ViewModel's whole life rather than while the list is visible:
     * the overlay is short-lived, and a notice that arrived a second before it opened
     * is still the newest thing to show.
     */
    private fun observeSocket() {
        viewModelScope.launch {
            network.webSocketClient.operatorUpdates.collect { incoming ->
                held = held.mergedWith(incoming)
                _uiState.update {
                    it.copy(updates = held.toUpdateItems(Instant.now()), error = null)
                }
            }
        }
    }
}
