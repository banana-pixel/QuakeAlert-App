package id.web.quakealert.ui.settings

import androidx.lifecycle.ViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update

/**
 * Hosts the [SettingsUiState] for the Settings screen and exposes it as a
 * [StateFlow] following unidirectional data flow. Seeded with mock defaults
 * mirroring the Figma design ("Settings Page (Fix)", node 1:845) so the
 * "Location & Coverage" section can be verified visually before real
 * preferences persistence is wired in.
 */
class SettingsViewModel : ViewModel() {

    private val _uiState = MutableStateFlow(SettingsUiState())
    val uiState: StateFlow<SettingsUiState> = _uiState.asStateFlow()

    /** Selects a coverage radius from the Coverage segmented control. */
    fun onCoverageSelected(range: CoverageRange) {
        _uiState.update { it.copy(coverageRange = range) }
    }

    /** Toggles the "Auto Sync Location / Intelligent Location Sync" switch. */
    fun onAutoSyncToggled(enabled: Boolean) {
        _uiState.update { it.copy(autoSyncLocation = enabled) }
    }

    /** Placeholder hook for the "Sync Location Now" refresh action. */
    fun onSyncLocationNow() {
        // Intentionally empty until a real location sync source is wired in.
    }
}
