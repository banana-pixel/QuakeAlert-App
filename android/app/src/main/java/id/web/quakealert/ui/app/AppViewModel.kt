package id.web.quakealert.ui.app

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import id.web.quakealert.data.AppSettingsRepository
import id.web.quakealert.data.network.QuakeNetwork
import id.web.quakealert.data.network.mapper.QuakeFormat
import id.web.quakealert.device.canPostNotifications
import id.web.quakealert.device.isBatteryUnrestricted
import id.web.quakealert.domain.ProtectionStatus
import id.web.quakealert.service.StatusNotifier
import id.web.quakealert.service.WarningNotifier
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch
import java.time.Instant

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

    /**
     * The two OS-owned facts behind the status notification, refreshed on every
     * resume. Held as state rather than read at post time so a revocation the user
     * performs in system Settings updates the shade as soon as they come back.
     */
    private val systemState = MutableStateFlow(
        SystemState(
            notificationsPermitted = application.canPostNotifications(),
            batteryUnrestricted = application.isBatteryUnrestricted()
        )
    )

    init {
        // Push registration also runs here rather than only in `onNewToken`: a token
        // can rotate while the app is not running, and the server keeps only the last
        // one it was told about. No-ops when Firebase is not configured.
        network.pushRegistrar.register()

        // Registered up front so the channel exists (and its settings are the user's)
        // before the first alert needs it — creating a channel while posting to it
        // works, but leaves no chance to see it in system settings beforehand.
        WarningNotifier.ensureChannel(application)

        observeStatusNotification()
    }

    val uiState: StateFlow<AppUiState> = repository.isOnboardingCompleted
        .map<Boolean, AppUiState> { AppUiState.Ready(onboardingCompleted = it) }
        .stateIn(
            scope = viewModelScope,
            started = SharingStarted.WhileSubscribed(5_000),
            initialValue = AppUiState.Loading
        )

    /**
     * Re-checks position staleness whenever the app comes to the foreground.
     *
     * Called from [AppRoot] on every `ON_START`, not once at construction, because a
     * process that is never killed never re-checked: leave the app backgrounded across
     * a flight and the position it targets alerts with stays where you took off from
     * until Android happens to reclaim the process.
     *
     * Cheap enough to call on every resume without a guard of its own, because
     * [id.web.quakealert.data.users.UserLocationRepository.syncIfStale] already has
     * three: auto-sync must be on, the stored fix must be older than six hours, and a
     * device that has moved under a kilometre records the check without uploading. The
     * repository also serialises concurrent syncs, so a fast background/foreground flap
     * cannot produce two uploads of one fix. A failure leaves the previous position in
     * place.
     *
     * Runs on the process scope, so a rotation cannot cancel a request mid-flight.
     */
    fun onAppForegrounded() {
        // Both grants and the battery exemption can be changed in system Settings while
        // the app sits in the background, and none of them is observable — re-reading
        // them on every resume is what keeps the status notification from claiming a
        // permission the user has since revoked.
        systemState.value = SystemState(
            notificationsPermitted = getApplication<Application>().canPostNotifications(),
            batteryUnrestricted = getApplication<Application>().isBatteryUnrestricted()
        )

        network.applicationScope.launch {
            network.userLocationRepository.syncIfStale()
        }
    }

    /**
     * Keeps the opt-in status notification in step with the state it reports.
     *
     * Driven from here rather than from a service, because there is no service: the
     * facts it prints all live in [AppSettingsRepository] and in two system checks, and
     * a notification is posted, not hosted. See
     * [id.web.quakealert.service.StatusNotifier] for why a foreground service would be
     * the wrong shape for this.
     *
     * The notification outlives this collector, which is the intended behaviour — a
     * posted notification survives the UI going away, and the facts it carries do not
     * change while nothing is running. The toggle going off is the only thing that
     * clears it.
     */
    private fun observeStatusNotification() {
        val context = getApplication<Application>()
        // Folded in two stages: the persisted preference facts first, then those against
        // the system grants and the last-alert record. `combine` is typed only to five
        // sources, and grouping is clearer here than an untyped vararg array anyway.
        val preferences = combine(
            repository.statusNotification,
            repository.notificationsEnabled,
            repository.autoSyncLocation,
            repository.lastSyncAtMs
        ) { enabled, alertsEnabled, autoSync, lastSyncAtMs ->
            StatusPreferences(enabled, alertsEnabled, autoSync, lastSyncAtMs)
        }
        viewModelScope.launch {
            combine(preferences, systemState, repository.lastAlert) { prefs, system, lastAlert ->
                if (!prefs.enabled) {
                    null
                } else {
                    ProtectionStatus(
                        alertsEnabled = prefs.alertsEnabled,
                        notificationsPermitted = system.notificationsPermitted,
                        autoSyncEnabled = prefs.autoSyncEnabled,
                        batteryUnrestricted = system.batteryUnrestricted,
                        lastSyncLabel = prefs.lastSyncAtMs?.let(::relativeLabel),
                        lastAlertLabel = lastAlert?.let {
                            "${it.summary}, ${relativeLabel(it.atMs)}"
                        }
                    )
                }
            }
                .distinctUntilChanged()
                .collect { status ->
                    if (status == null) {
                        StatusNotifier.clear(context)
                    } else {
                        StatusNotifier.notify(context, status)
                    }
                }
        }
    }

    private fun relativeLabel(epochMs: Long): String =
        QuakeFormat.relativeTime(Instant.ofEpochMilli(epochMs), Instant.now())

    /** Persists onboarding completion, flipping the entry point to MainScreen. */
    fun completeOnboarding() {
        repository.completeOnboarding()
    }
}

/** The persisted half of [ProtectionStatus], collected as one value. */
private data class StatusPreferences(
    val enabled: Boolean,
    val alertsEnabled: Boolean,
    val autoSyncEnabled: Boolean,
    val lastSyncAtMs: Long?
)

/** The OS-owned half of [ProtectionStatus], polled rather than observed. */
private data class SystemState(
    val notificationsPermitted: Boolean,
    val batteryUnrestricted: Boolean
)
