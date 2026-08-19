package id.web.quakealert.ui.warning

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import id.web.quakealert.data.AppSettingsRepository
import id.web.quakealert.data.UnitSystem
import id.web.quakealert.ui.history.MmiSeverity
import id.web.quakealert.ui.history.QuakeHistoryItem
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.flow.update

/**
 * Hosts the [WarningUiState] for the Warning screen and exposes it as a
 * [StateFlow] following unidirectional data flow. Seeded with mock data
 * mirroring the Figma design (nodes 124:1297 / 124:1426 / 124:1605) so the UI
 * can be verified visually before a real alert transport is wired in.
 *
 * The persisted [UnitSystem] from [AppSettingsRepository] is folded into every
 * emission so the detail overlay's "Distance from you" row and the share text
 * render the same unit the user picked in Settings — the same split
 * [id.web.quakealert.ui.history.HistoryViewModel] makes.
 *
 * The seeded state is the resting variant ([PossibilityBanner] + preparedness
 * tips). When the alert feed lands, choosing the variant becomes a one-line
 * decision from the newest event — e.g. an event inside the recent window flips
 * the state to [ActiveQuakeBanner] + [activeQuakeTips], whose fixture lives in
 * [id.web.quakealert.ui.warning.WarningUiState.kt] next to the resting one.
 *
 * Sharing is deliberately absent here: firing `Intent.ACTION_SEND` needs a
 * `Context`, not app state, so it lives in [WarningRoute] alongside the other
 * composition-local work.
 */
class WarningViewModel(application: Application) : AndroidViewModel(application) {

    private val repository = AppSettingsRepository(application)

    private val _uiState = MutableStateFlow(WarningUiState())

    val uiState: StateFlow<WarningUiState> = combine(
        repository.unitSystem,
        _uiState
    ) { unit, state -> state.copy(unitSystem = unit) }.stateIn(
        scope = viewModelScope,
        started = SharingStarted.WhileSubscribed(5_000),
        initialValue = WarningUiState()
    )

    /**
     * Raises the overlay behind the banner's "SEE DETAILS" capsule, dispatched
     * by the current variant: the active banner opens the "Recent Earthquake"
     * event detail (Figma 124:1192), the resting banner opens the "Earthquake
     * Possibility" card (Figma 124:1605). Keeping the dispatch here means the
     * screen never needs to know which overlay a tap raises.
     */
    fun onSeeDetailsClicked() {
        when (_uiState.value.banner) {
            is ActiveQuakeBanner -> _uiState.update {
                it.copy(selectedEventDetails = activeAlertDetails)
            }
            is PossibilityBanner -> _uiState.update {
                it.copy(selectedPossibility = EarthquakePossibility())
            }
        }
    }

    /**
     * Closes the "Recent Earthquake" detail overlay. Called for every dismissal
     * path — the close (X) button, a back press and a tap outside the card.
     */
    fun onDetailDismissed() {
        _uiState.update { it.copy(selectedEventDetails = null) }
    }

    /**
     * Closes the "Earthquake Possibility" overlay. Called for every dismissal
     * path — the close (X) button, a back press and a tap outside the card.
     */
    fun onPossibilityDismissed() {
        _uiState.update { it.copy(selectedPossibility = null) }
    }

    /** Placeholder hook for tapping the bottom "Emergency" call-to-action. */
    fun onEmergencyClicked() {
        // Intentionally empty until the emergency-resource flow is defined.
    }

    private companion object {
        /**
         * Event detail shown by [onSeeDetailsClicked] for the active banner,
         * mirroring the values Figma annotates on node 124:1192 (MMI XI / severe,
         * "Lembang, West Java, ID").
         */
        val activeAlertDetails = QuakeHistoryItem(
            id = "active-alert",
            intensity = "XI",
            severity = MmiSeverity.SEVERE,
            location = "Lembang, West Java, ID",
            date = "20 Jun 2026",
            time = "07:19:18 WIB",
            distanceKm = 60,
            relativeTime = "2 months ago",
            pgaLabel = "61.5 gal",
            durationLabel = "7 sec",
            coordinates = "41.40338, 2.17403"
        )
    }
}