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

    /** Placeholder hook for the "Test Alert Sound" action card. */
    fun onTestAlertSound() {
        // Intentionally empty until a real alert-sound source is wired in.
    }

    /** Toggles the "Light Mode (Beta)" switch (Appearance & Look section). */
    fun onLightModeToggled(enabled: Boolean) {
        _uiState.update { it.copy(lightMode = enabled) }
    }

    /** Selects the app language from the Language segmented control. */
    fun onLanguageSelected(language: AppLanguage) {
        _uiState.update { it.copy(language = language) }
    }

    /** Opens the About overlay from the "More About Us" call-to-action. */
    fun onMoreAboutUs() {
        _uiState.update { it.copy(showAboutModal = true) }
    }

    /**
     * Closes the About overlay. Called for every dismissal path — the close (X)
     * button, a system back press and a tap outside the card.
     */
    fun onAboutDismissed() {
        _uiState.update { it.copy(showAboutModal = false) }
    }
}

