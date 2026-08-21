package id.web.quakealert.ui.settings

import android.Manifest
import android.app.Application
import android.content.Context
import android.content.pm.PackageManager
import android.os.PowerManager
import android.util.Log
import androidx.core.content.ContextCompat
import androidx.core.content.getSystemService
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import id.web.quakealert.data.AppSettingsRepository
import id.web.quakealert.data.UnitSystem
import id.web.quakealert.data.network.ApiException
import id.web.quakealert.data.network.QuakeNetwork
import id.web.quakealert.data.network.mapper.QuakeFormat
import id.web.quakealert.data.users.LocationSyncResult
import id.web.quakealert.device.AlertSiren
import id.web.quakealert.domain.SafetyPolicy
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.time.Instant

/**
 * Hosts the [SettingsUiState] and turns every control on the Settings screen into
 * a real effect.
 *
 * Two collaborators, no DI: [AppSettingsRepository] for the persisted preferences
 * and [QuakeNetwork] for the identity, the position sync and the sensor count.
 * Each persisted flow is collected into one [MutableStateFlow] rather than folded
 * through `combine`, because the screen also carries transient state (an in-flight
 * sync, the last status line) that no repository owns — one writable state holder
 * keeps both kinds in a single emission and avoids a seven-way combine.
 *
 * There is no radius control any more: the alert radius is fixed by
 * [id.web.quakealert.domain.SafetyPolicy] and identical on the server, so the only
 * thing this screen still syncs is the position — which is what makes the station
 * count and the targeting work at all.
 */
class SettingsViewModel(application: Application) : AndroidViewModel(application) {

    private val repository = AppSettingsRepository(application)
    private val network = QuakeNetwork.from(application)

    private val _uiState = MutableStateFlow(SettingsUiState())
    val uiState: StateFlow<SettingsUiState> = _uiState.asStateFlow()

    init {
        observePreferences()
        observeIdentity()
        refreshSystemState()
        // Explicit, because nothing else asks any more: the count used to ride along
        // with the radius flow, and that flow is gone.
        refreshSensorCount()
    }

    /** Mirrors the persisted preferences into state as they change. */
    private fun observePreferences() {
        viewModelScope.launch {
            repository.unitSystem.collect { unit -> _uiState.update { it.copy(unitSystem = unit) } }
        }
        viewModelScope.launch {
            repository.autoSyncLocation.collect { on -> _uiState.update { it.copy(autoSyncLocation = on) } }
        }
        viewModelScope.launch {
            repository.notificationsEnabled.collect { on ->
                _uiState.update { it.copy(notificationsEnabled = on) }
            }
        }
        viewModelScope.launch {
            // Formatted on arrival: the pill reads "2 minutes ago", and the epoch
            // value it comes from means nothing to the screen.
            repository.lastSyncAtMs.collect { at ->
                _uiState.update { it.copy(lastSyncLabel = at?.toRelativeLabel()) }
            }
        }
        viewModelScope.launch {
            repository.language.collect { tag ->
                _uiState.update { it.copy(language = AppLanguage.fromTag(tag)) }
            }
        }
    }

    /** Mirrors the anonymous identity and the last synced position into state. */
    private fun observeIdentity() {
        viewModelScope.launch {
            network.sessionStore.session.collect { session ->
                _uiState.update {
                    it.copy(pseudonym = session?.pseudonym, userId = session?.userId)
                }
            }
        }
        viewModelScope.launch {
            network.sessionStore.userLocation.collect { location ->
                _uiState.update { it.copy(locationLabel = location?.locationName) }
            }
        }
    }

    /**
     * Re-reads the two pieces of state the user can change outside the app — the
     * notification grant and the battery-optimisation exemption.
     *
     * Called from `init` and again whenever the screen resumes, since both can be
     * revoked in system Settings while the app sits in the background.
     */
    fun refreshSystemState() {
        val context = getApplication<Application>()
        _uiState.update {
            it.copy(
                notificationPermissionGranted = context.canPostNotifications(),
                batteryUnrestricted = context.isBatteryUnrestricted()
            )
        }
    }

    /** Toggles the "Auto Sync Location" switch. */
    fun onAutoSyncToggled(enabled: Boolean) {
        repository.setAutoSyncLocation(enabled)
        _uiState.update { it.copy(autoSyncLocation = enabled) }
    }

    /**
     * Toggles the user's own alert switch.
     *
     * Turning it on without the OS grant is reported rather than silently stored as
     * "on": the switch would then claim alerts are enabled while the system drops
     * every notification.
     */
    fun onNotificationsToggled(enabled: Boolean) {
        repository.setNotificationsEnabled(enabled)
        _uiState.update { it.copy(notificationsEnabled = enabled) }
        if (enabled && !getApplication<Application>().canPostNotifications()) {
            post("Allow notifications in system settings for alerts to arrive")
        }
    }

    /**
     * "Sync Location Now" — acquires a fix and pushes it, bypassing the
     * moved-less-than-1-km shortcut so an explicit tap always produces a result.
     */
    fun onSyncLocationNow() {
        if (_uiState.value.isSyncing) return
        _uiState.update { it.copy(isSyncing = true, statusMessage = null) }
        viewModelScope.launch {
            val result = network.userLocationRepository.sync(force = true)
            _uiState.update { it.copy(isSyncing = false, statusMessage = result.toMessage()) }
            if (result is LocationSyncResult.Updated) refreshSensorCount()
        }
    }

    /**
     * The user declined the location prompt raised by "Sync Now".
     *
     * Points at system Settings rather than re-asking: after a decline the OS stops
     * showing the dialog, so a second tap on the button would do nothing visible.
     */
    fun onLocationPermissionDenied() {
        post("Location permission denied — enable it in system Settings to sync")
    }

    /**
     * "Test Alert Sound" — plays the real siren on the alarm stream for a few
     * seconds so the user can check audibility before an actual quake.
     *
     * The same [AlertSiren] the warning path uses, deliberately: a test that played
     * a different tone at a different volume would prove nothing. Stopped explicitly
     * rather than left to the 90 s expiry.
     */
    fun onTestAlertSound() {
        viewModelScope.launch {
            val siren = AlertSiren(getApplication())
            siren.start()
            post("Playing test alert on the alarm volume")
            try {
                delay(TEST_SOUND_MS)
            } finally {
                siren.release()
            }
        }
    }

    /**
     * Toggles "Light Mode (Beta)".
     *
     * Rendered disabled and badged "Coming Soon" — the app ships dark-theme only —
     * so this never fires from the UI. Kept wired so enabling the control is a
     * one-flag change once a light palette exists.
     */
    fun onLightModeToggled(enabled: Boolean) {
        _uiState.update { it.copy(lightMode = enabled) }
    }

    /**
     * Selects the app language. Persisted so the choice survives a restart, but not
     * yet applied: the strings ship in English only, which is why the control is
     * badged in the UI.
     */
    fun onLanguageSelected(language: AppLanguage) {
        repository.setLanguage(language.tag)
        _uiState.update { it.copy(language = language) }
    }

    /** Selects the distance unit system, shared with History and Sensors. */
    fun onUnitSelected(unit: UnitSystem) {
        repository.setUnitSystem(unit)
        _uiState.update { it.copy(unitSystem = unit) }
    }

    /** Asks the server for a new pseudonym; a 429 is the once-per-60s cooldown. */
    fun onRerollPseudonym() {
        if (_uiState.value.isRerolling) return
        _uiState.update { it.copy(isRerolling = true, statusMessage = null) }
        viewModelScope.launch {
            val message = network.apiClient.rerollPseudonym().fold(
                onSuccess = { pseudonym -> "You are now $pseudonym" },
                onFailure = { error ->
                    if ((error as? ApiException)?.httpCode == HTTP_TOO_MANY_REQUESTS) {
                        "You can change your pseudonym once a minute — try again shortly"
                    } else {
                        error.message ?: "Could not change your pseudonym"
                    }
                }
            )
            _uiState.update { it.copy(isRerolling = false, statusMessage = message) }
        }
    }

    /** Opens the reset confirmation. Destructive: the old identity is unrecoverable. */
    fun onResetProfileRequested() {
        _uiState.update { it.copy(showResetDialog = true) }
    }

    /** Dismisses the reset confirmation without touching the identity. */
    fun onResetProfileDismissed() {
        _uiState.update { it.copy(showResetDialog = false) }
    }

    /**
     * Discards the anonymous identity and bootstraps a fresh one.
     *
     * There is no refresh endpoint: `POST /auth/anonymous` mints a *new* `user_id`
     * and pseudonym, so the server knows nothing about this device afterwards. The
     * position and the FCM token are therefore re-pushed against the new identity —
     * skipping that would leave the server with no position to target and no push
     * registration, i.e. no alerts.
     */
    fun onResetProfileConfirmed() {
        if (_uiState.value.isResetting) return
        _uiState.update { it.copy(showResetDialog = false, isResetting = true, statusMessage = null) }
        viewModelScope.launch {
            val message = runCatching {
                network.authRepository.invalidate()
                network.authRepository.ensureToken()
                network.userLocationRepository.sync(force = true)
                network.pushRegistrar.register()
            }.fold(
                onSuccess = { "New anonymous profile created" },
                onFailure = { error ->
                    Log.w(TAG, "profile reset failed", error)
                    error.message ?: "Could not create a new profile"
                }
            )
            _uiState.update { it.copy(isResetting = false, statusMessage = message) }
            refreshSensorCount()
        }
    }

    /** Clears the status pill once the user has had a chance to read it. */
    fun onStatusMessageShown() {
        _uiState.update { it.copy(statusMessage = null) }
    }

    /** Opens the About overlay from the "More About Us" call-to-action. */
    fun onMoreAboutUs() {
        _uiState.update { it.copy(showAboutModal = true) }
    }

    /** Closes the About overlay — close button, back press or outside tap. */
    fun onAboutDismissed() {
        _uiState.update { it.copy(showAboutModal = false) }
    }

    /**
     * Counts the stations the server reports inside [SafetyPolicy.ALERT_RADIUS_KM].
     *
     * A failure leaves the previous count rather than showing zero: "0 sensors" and
     * "could not ask" mean very different things to someone reading this card. The
     * server needs a stored position for this to be non-empty, which is why a
     * successful sync re-runs it.
     */
    private fun refreshSensorCount() {
        viewModelScope.launch {
            network.apiClient.fetchSensors(rangeKm = SafetyPolicy.ALERT_RADIUS_KM)
                .onSuccess { sensors -> _uiState.update { it.copy(sensorCount = sensors.size) } }
                .onFailure { Log.d(TAG, "sensor count unavailable", it) }
        }
    }

    private fun post(message: String) {
        _uiState.update { it.copy(statusMessage = message) }
    }

    private fun Long.toRelativeLabel(): String =
        QuakeFormat.relativeTime(Instant.ofEpochMilli(this), Instant.now())

    private fun LocationSyncResult.toMessage(): String = when (this) {
        is LocationSyncResult.Updated -> "Location updated"
        is LocationSyncResult.Unchanged -> "Location unchanged — you have not moved"
        LocationSyncResult.PermissionDenied -> "Location permission is needed to sync"
        LocationSyncResult.NoFix -> "Could not get a location fix — try again outdoors"
        is LocationSyncResult.Failed -> message
    }

    private companion object {
        const val TAG = "SettingsViewModel"
        const val TEST_SOUND_MS = 3_000L
        const val HTTP_TOO_MANY_REQUESTS = 429
    }
}

/** Whether the OS currently allows this app to post notifications. */
private fun Context.canPostNotifications(): Boolean =
    if (android.os.Build.VERSION.SDK_INT < android.os.Build.VERSION_CODES.TIRAMISU) {
        true
    } else {
        ContextCompat.checkSelfPermission(this, Manifest.permission.POST_NOTIFICATIONS) ==
            PackageManager.PERMISSION_GRANTED
    }

/**
 * Whether the app is exempt from battery optimisation.
 *
 * Relevant to alert delivery, not battery life: under Doze a data-only FCM message
 * can be held back until the next maintenance window, which for an earthquake
 * warning is indistinguishable from never arriving.
 */
private fun Context.isBatteryUnrestricted(): Boolean =
    getSystemService<PowerManager>()?.isIgnoringBatteryOptimizations(packageName) ?: false
