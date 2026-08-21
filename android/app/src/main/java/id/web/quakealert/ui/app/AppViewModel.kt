package id.web.quakealert.ui.app

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import id.web.quakealert.data.AppSettingsRepository
import id.web.quakealert.data.network.QuakeNetwork
import id.web.quakealert.service.WarningNotifier
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch

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

    private val network = QuakeNetwork.from(application)

    init {
        // App start is the third position-sync trigger, alongside the onboarding grant
        // and Settings → Sync Now. Cheap and silent: it no-ops unless auto-sync is on
        // and the stored fix has gone stale, and a failure only leaves the previous
        // position in place. Run on the process scope so a rotation cannot cancel a
        // request mid-flight.
        network.applicationScope.launch {
            network.userLocationRepository.syncIfStale()
        }

        // Push registration also runs here rather than only in `onNewToken`: a token
        // can rotate while the app is not running, and the server keeps only the last
        // one it was told about. No-ops when Firebase is not configured.
        network.pushRegistrar.register()

        // Registered up front so the channel exists (and its settings are the user's)
        // before the first alert needs it — creating a channel while posting to it
        // works, but leaves no chance to see it in system settings beforehand.
        WarningNotifier.ensureChannel(application)
    }

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
