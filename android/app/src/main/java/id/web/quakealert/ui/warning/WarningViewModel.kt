package id.web.quakealert.ui.warning

import androidx.lifecycle.ViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

/**
 * Hosts the [WarningUiState] for the Warning screen and exposes it as a
 * [StateFlow] following unidirectional data flow. Seeded with mock data
 * mirroring the Figma design (node 1:1024) so the UI can be verified visually
 * before a real alert transport is wired in.
 */
class WarningViewModel : ViewModel() {

    private val _uiState = MutableStateFlow(WarningUiState())
    val uiState: StateFlow<WarningUiState> = _uiState.asStateFlow()

    /** Placeholder hook for tapping the banner's "SEE DETAILS" action. */
    fun onSeeDetailsClicked() {
        // Intentionally empty until the alert-detail flow is implemented.
    }

    /** Placeholder hook for tapping the bottom "Emergency" call-to-action. */
    fun onEmergencyClicked() {
        // Intentionally empty until the emergency-contact flow is implemented.
    }
}
