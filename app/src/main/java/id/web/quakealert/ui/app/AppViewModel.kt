package id.web.quakealert.ui.app

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import id.web.quakealert.data.AppSettingsRepository
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn

/**
 * Top-level UI state that gates the application entry point.
 *
 * [Loading] is emitted until the persisted onboarding flag has been read for the
 * first time so the root can hold a neutral surface and avoid a flash of the
 * wrong screen (flicker) before state resolves.
 */
sealed interface AppUiState {
    data object Loading : AppUiState
    data class Ready(val onboardingCompleted: Boolean) : AppUiState
}

/**
 * Root ViewModel that owns the application-entry decision. It reads the
 * onboarding flag from [AppSettingsRepository] and exposes it as a [StateFlow]
 * following unidirectional data flow, and forwards [completeOnboarding] events
 * back into the repository.
 */
class AppViewModel(application: Application) : AndroidViewModel(application) {

    private val repository = AppSettingsRepository(application)

    val uiState: StateFlow<AppUiState> = repository.isOnboardingCompleted
        .map<Boolean, AppUiState> { AppUiState.Ready(onboardingCompleted = it) }
        .stateIn(
            scope = viewModelScope,
            started = SharingStarted.WhileSubscribed(5_000),
            initialValue = AppUiState.Loading
        )

    /** Persists onboarding completion, flipping the entry point to MainScreen. */
    fun completeOnboarding() {
        repository.completeOnboarding()
    }
}
