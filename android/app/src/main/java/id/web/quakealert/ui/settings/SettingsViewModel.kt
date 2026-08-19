package id.web.quakealert.ui.settings

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import id.web.quakealert.data.AppSettingsRepository
import id.web.quakealert.data.UnitSystem
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.flow.update

/**
 * Hosts the [SettingsUiState] for the Settings screen and exposes it as a
 * [StateFlow] following unidirectional data flow. Seeded with mock defaults
 * mirroring the Figma design ("Settings Page (Fix)", node 1:845) so the
 * "Location & Coverage" section can be verified visually before real
 * preferences persistence is wired in.
 *
 * The persisted [UnitSystem] from [AppSettingsRepository] is folded into every
 * emission, so the Coverage labels, range summaries and the Units control all
 * render the choice saved across restarts.
 */
class SettingsViewModel(application: Application) : AndroidViewModel(application) {

    private val repository = AppSettingsRepository(application)

    private val _uiState = MutableStateFlow(SettingsUiState())

    val uiState: StateFlow<SettingsUiState> = combine(
        repository.unitSystem,
        _uiState
    ) { unit, state -> state.copy(unitSystem = unit) }.stateIn(
        scope = viewModelScope,
        started = SharingStarted.WhileSubscribed(5_000),
        initialValue = SettingsUiState()
    )

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

    /**
     * Selects the distance unit system (Metric / Imperial). Persisted via
     * [AppSettingsRepository] so History and Sensors render the same choice, and
     * mirrored into state so the control updates immediately.
     */
    fun onUnitSelected(unit: UnitSystem) {
        repository.setUnitSystem(unit)
        _uiState.update { it.copy(unitSystem = unit) }
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

