package id.web.quakealert.ui.addsensor

import android.app.Application
import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.util.Log
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import id.web.quakealert.data.network.ApiException
import id.web.quakealert.data.network.QuakeNetwork
import id.web.quakealert.data.network.model.NodePortalConfigDto
import id.web.quakealert.device.Coordinates
import id.web.quakealert.device.NodeLink
import id.web.quakealert.device.ReverseGeocoder
import id.web.quakealert.device.locationSource
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/**
 * Drives the redesigned add-a-sensor wizard (Figma 155:985 ... 155:1572).
 *
 * Screens: WELCOME to LOCATION to CREDENTIALS to WLAN to FINISHING, with RATE_LIMIT
 * shown instead of advancing when `POST /nodes/provision` answers 429. All
 * decisions stay pure functions on [AddSensorState]; this class owns every side
 * effect in order:
 *
 *  1. **Provision** - `POST /nodes/provision` over the internet, before any local
 *     Wi-Fi business.
 *  2. **Link** - bind the phone to the node's SoftAP ([NodeLink]), scan, then
 *     `POST /config` with everything provisioning returned.
 *  3. **Confirm** - poll `/sensors` until the node's echoed identity appears,
 *     pushing the wizard's own pin as the user's position first when none is
 *     stored (the endpoint answers relative to it).
 *
 * Failures are recorded as [WizardFailure] values, never as text: the exception is
 * logged here for diagnosis and the user-facing wording lives in AddSensorCopy.kt.
 */
class AddSensorViewModel(application: Application) : AndroidViewModel(application) {

    private val network = QuakeNetwork.from(application)
    private val apiClient = network.apiClient
    private val nodeLink = NodeLink(application)
    private val reverseGeocoder = ReverseGeocoder(application)
    private val locationSource = locationSource(application)

    private val _state = MutableStateFlow(AddSensorState())
    val state: StateFlow<AddSensorState> = _state.asStateFlow()

    /**
     * The running confirm loop, held so [reset] can cancel it. A plain boolean guard
     * could stop a second loop from starting but could never stop the first one, and
     * a discarded session that keeps polling `/sensors` every thirty seconds would
     * eventually write its answer into a fresh wizard.
     */
    private var confirmJob: Job? = null

    /**
     * Monotonic session generation. Captured when a provisioning request starts
     * and compared when its response lands: [reset] increments it, so a response
     * that arrives after the user has left the wizard belongs to a dead session
     * and must not resurrect wizard state ([onProvisioned] would otherwise drop a
     * fresh session onto CREDENTIALS with a secret nobody saw). The late-minted
     * row is revoked instead — this is race #1 of the approved lifecycle design.
     */
    private var sessionGeneration = 0L

    init {
        seedPinFromStoredFix()
    }

    /**
     * Seeds the pin from the position the account already stores, so a returning user
     * opens the map where they live instead of on an empty world. Zero coordinates
     * mean "no fix" (the server treats them the same way), not the Gulf of Guinea.
     */
    private fun seedPinFromStoredFix() {
        viewModelScope.launch {
            val fix = apiClient.currentUserLocation() ?: return@launch
            if (fix.latitude != 0.0 || fix.longitude != 0.0) {
                _state.update { it.copy(latitude = fix.latitude, longitude = fix.longitude) }
            }
        }
    }

    // --- WELCOME ---

    /**
     * Start pressed on Welcome. The wizard can skip straight to provisioning only
     * when both halves of the request already exist (a seeded pin AND a usable
     * place name); anything less walks the user to LOCATION first, which is also
     * where the rate-limit check will surface, because provisioning itself is the
     * thing the server throttles.
     */
    fun onStartClicked() {
        val current = _state.value
        if (current.isBusy) return
        val resolvedName = resolvedPlaceName()
        if (!current.locationStepValid || resolvedName == null) {
            _state.update { it.copy(currentStep = AddSensorWizardStep.LOCATION, failure = null) }
            return
        }
        provisionNode(resolvedName)
    }

    // --- LOCATION ---

    /** The place label follows the pin until the user edits it by hand. */
    fun onMapPinMoved(latitude: Double, longitude: Double) {
        _state.update {
            it.copy(
                latitude = latitude,
                longitude = longitude,
                detailsError = null,
                failure = null
            )
        }
        viewModelScope.launch { reverseGeocodeInto(latitude, longitude) }
    }

    /** Sync button: one fresh GPS fix, then the matching place name. */
    fun onSyncLocationClicked() {
        if (_state.value.isSyncingLocation) return
        _state.update { it.copy(isSyncingLocation = true, failure = null) }
        viewModelScope.launch {
            // High accuracy on purpose: the user is waiting on this exact result.
            val fix = locationSource.currentFix(allowHighAccuracy = true)
            if (fix == null) {
                _state.update {
                    it.copy(
                        isSyncingLocation = false,
                        failure = WizardFailure.LOCATION_UNAVAILABLE
                    )
                }
                return@launch
            }
            _state.update { it.copy(latitude = fix.latitude, longitude = fix.longitude) }
            reverseGeocodeInto(fix.latitude, fix.longitude)
            _state.update { it.copy(isSyncingLocation = false) }
        }
    }

    fun onLocationNameChanged(name: String) {
        _state.update { it.copy(locationName = name, detailsError = null, failure = null) }
    }

    /** LOCATION gate: a pin and a place name; provisioning fires while the card shows progress. */
    fun onLocationContinue() {
        val current = _state.value
        if (current.isBusy) return
        if (!current.locationStepValid) {
            _state.update { it.copy(detailsError = DetailsError.POSITION_MISSING) }
            return
        }
        val resolvedName = resolvedPlaceName()
        if (resolvedName == null) {
            // The place name feeds alerts and chat regions server-side; sending an
            // empty one would register an anonymous dot on the map.
            _state.update { it.copy(failure = WizardFailure.PLACE_NAME_MISSING) }
            return
        }
        _state.update { it.copy(failure = null, detailsError = null) }
        provisionNode(resolvedName)
    }

    // --- CREDENTIALS ---

    fun onSecretRevealed() {
        _state.update { it.copy(secretRevealed = true) }
    }

    /** Copies the display-once secret; feedback is the Copy button flipping state-side. */
    fun onCopySecret(): Boolean {
        val secret = _state.value.provisioned?.provisioningSecret ?: return false
        val context = getApplication<Application>()
        val clipboard = context.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
        clipboard.setPrimaryClip(ClipData.newPlainText("provisioning secret", secret))
        return true
    }

    /** CREDENTIALS gate to WLAN: bring up the bound node network and scan. */
    fun onCredentialsContinue() {
        if (_state.value.currentStep == AddSensorWizardStep.WLAN) return
        _state.update {
            it.copy(currentStep = AddSensorWizardStep.WLAN, isBusy = true, failure = null)
        }
        bindAndScan()
    }

    // --- WLAN ---

    /**
     * Re-attempts the link and scan. Deliberately *not* routed through
     * [onCredentialsContinue]'s step guard: the first attempt can fail for a
     * dozen mundane reasons (dialog dismissed, DHCP still settling, node booting),
     * and a Rescan button that silently does nothing is how a user gets stranded.
     */
    fun onRescanNetworks() {
        _state.update { it.copy(isBusy = true, failure = null) }
        bindAndScan()
    }

    private fun bindAndScan() {
        viewModelScope.launch {
            // Always rebind: a previous dialog dismissal or timeout leaves no
            // network to talk through, and the specifier join is what shows the
            // system dialog again.
            val bound = nodeLink.bindToNode()
            if (bound == null) {
                _state.update {
                    it.copy(isBusy = false, failure = WizardFailure.SENSOR_NOT_JOINED)
                }
                return@launch
            }

            nodeLink.scanNetworks().fold(
                onSuccess = { ssids ->
                    _state.update { current ->
                        current.copy(scannedSsids = ssids, isBusy = false, failure = null)
                    }
                },
                onFailure = { throwable ->
                    // Classified, not printed: the portal's transport errors name
                    // sockets and hosts, which is our diagnosis, not user copy.
                    Log.w(TAG, "scan failed", throwable)
                    _state.update { current ->
                        current.copy(
                            isBusy = false,
                            scannedSsids = emptyList(),
                            failure = WizardFailure.SENSOR_NOT_ANSWERING
                        )
                    }
                }
            )
        }
    }

    fun onNetworkSelected(ssid: String) {
        Log.i(TAG, "ssid selected")
        _state.update { it.onSsidSelected(ssid) }
    }

    fun onPasswordChanged(password: String) {
        _state.update { it.copy(wifiPassword = password, linkError = null) }
    }

    /** WLAN gate: configure the node, then start confirming. */
    fun onWlanContinue() {
        val gated = _state.value.advanceIfLinkValid()
        _state.value = gated
        if (gated.linkError != null) return
        val provisioned = gated.provisioned ?: return
        val lat = gated.latitude ?: return
        val lon = gated.longitude ?: return

        viewModelScope.launch {
            _state.update { it.copy(isBusy = true, failure = null) }
            Log.i(TAG, "sending /config to node")
            val payload = NodePortalConfigDto(
                ssid = gated.selectedSsid.trim(),
                password = gated.wifiPassword,
                latitude = lat,
                longitude = lon,
                hmacKey = provisioned.provisioningSecret,
                stationId = provisioned.stationId,
                mqttBroker = provisioned.mqttBroker,
                mqttPort = provisioned.mqttPort,
                mqttTls = provisioned.mqttTls
            )
            nodeLink.sendConfig(payload).fold(
                onSuccess = { echo ->
                    Log.i(TAG, "portal accepted config")
                    ensureServerKnowsAPosition()
                    _state.update { it.onNodeConfigured(echo).copy(isBusy = false) }
                    startConfirmLoop()
                    nodeLink.releaseNode()
                },
                onFailure = { throwable ->
                    Log.w(TAG, "portal rejected config", throwable)
                    _state.update {
                        it.copy(isBusy = false, failure = WizardFailure.SETTINGS_NOT_ACCEPTED)
                    }
                }
            )
        }
    }

    // --- FINISHING ---

    /** Manual re-check from the finishing screen. */
    fun onCheckNow() {
        viewModelScope.launch {
            if (_state.value.confirmState == ConfirmState.WAITING) {
                _state.update { it.onConfirmPoll(fetchStationStatuses()) }
            }
        }
    }

    // --- PROVISIONING ---

    /**
     * Mints the node identity. A 429 (`RATE_LIMITED`) is not a generic error:
     * the wizard swaps to its dedicated screen so the user reads exactly why and
     * when to come back, never a stack trace in disguise.
     */
    private fun provisionNode(resolvedName: String) {
        viewModelScope.launch {
            _state.update { it.copy(isBusy = true, failure = null, locationName = resolvedName) }
            val details = _state.value
            val generation = sessionGeneration
            apiClient.provisionNode(
                sensorModel = details.sensorModel,
                locationName = resolvedName,
                latitude = requireNotNull(details.latitude),
                longitude = requireNotNull(details.longitude)
            ).onSuccess { node ->
                if (generation != sessionGeneration) {
                    // The session was reset while this request was in flight: the
                    // user has left, so applying [onProvisioned] would reopen a
                    // wizard nobody is watching and strand a secret nobody read.
                    // Withdraw the just-minted row immediately; failure here is
                    // logged and left to the server sweep, same as any other
                    // fire-and-forget revoke.
                    Log.i(TAG, "provision answered after exit; revoking ${node.stationId}")
                    network.applicationScope.launch {
                        network.apiClient.revokeNode(node.stationId, node.provisioningSecret)
                            .onFailure { throwable ->
                                Log.w(TAG, "late-provision revoke failed for ${node.stationId}", throwable)
                            }
                    }
                    return@onSuccess
                }
                _state.update { it.onProvisioned(node).copy(isBusy = false) }
            }.onFailure { throwable ->
                Log.w(TAG, "provisioning failed", throwable)
                when {
                    throwable is ApiException &&
                        (throwable.code == CODE_RATE_LIMITED || throwable.httpCode == 429) ->
                        _state.update {
                            it.copy(
                                isBusy = false,
                                currentStep = AddSensorWizardStep.RATE_LIMIT
                            )
                        }

                    else -> _state.update {
                        it.copy(isBusy = false, failure = provisionFailure(throwable))
                    }
                }
            }
        }
    }

    /**
     * The place name the request will carry: the user's edit when they made one,
     * otherwise what the geocoder resolved for the pin. Null when neither exists,
     * because an empty name would register an anonymous dot server-side.
     */
    private fun resolvedPlaceName(): String? =
        SensorNameRules.normalize(
            _state.value.locationName.ifBlank { _state.value.detectedLocationName.orEmpty() }
        ).ifEmpty { null }

    /**
     * Provisioning failures reduce to exactly two situations the user can act on:
     * the network was unreachable, or the request itself did not land. The real
     * cause is in the log above either way.
     */
    private fun provisionFailure(throwable: Throwable): WizardFailure =
        if (throwable is java.io.IOException && throwable !is ApiException) {
            WizardFailure.OFFLINE
        } else {
            WizardFailure.REGISTER_REJECTED
        }

    // --- CONFIRM LOOP ---

    private fun startConfirmLoop() {
        if (confirmJob?.isActive == true) return
        confirmJob = viewModelScope.launch {
            while (_state.value.confirmState == ConfirmState.WAITING &&
                _state.value.attemptsLeft > 0
            ) {
                delay(AddSensorState.CONFIRM_POLL_INTERVAL_MS)
                _state.update { it.onConfirmPoll(fetchStationStatuses()) }
            }
        }
    }

    /** A failed poll is absence, not an error: the node may still be rebooting. */
    private suspend fun fetchStationStatuses(): Map<String, String> =
        apiClient.fetchSensors(rangeKm = MAX_CONFIRM_RANGE_KM)
            .getOrNull()
            ?.nodes
            ?.associate { node ->
                // Domain to wire vocabulary: trust is asked before health, exactly
                // as the /sensors DTO words it.
                node.stationId to when {
                    !node.verified -> "Pending"
                    node.online -> "Online"
                    else -> "Offline"
                }
            }
            ?: emptyMap()

    /**
     * Without a stored position `/sensors` can only answer empty, so a first-time
     * user gets the wizard's own pin pushed as their position. A user who already
     * has one is left alone: their phone location is theirs.
     */
    private suspend fun ensureServerKnowsAPosition() {
        if (apiClient.currentUserLocation() == null) {
            val state = _state.value
            val lat = state.latitude ?: return
            val lon = state.longitude ?: return
            apiClient.updateLocation(
                latitude = lat,
                longitude = lon,
                locationName = SensorNameRules.normalize(state.locationName).takeIf { it.isNotEmpty() }
            )
        }
    }

    /**
     * Resolves a human-readable place for the pin. Best-effort by design: a
     * geocoder miss leaves the previous label alone and never blocks the flow.
     */
    private suspend fun reverseGeocodeInto(latitude: Double, longitude: Double) {
        val place = reverseGeocoder.resolve(Coordinates(latitude, longitude)) ?: return
        _state.update {
            it.copy(detectedLocationName = place.label)
        }
    }

    // --- NAVIGATION ---

    /**
     * Back capsule. Steps back when there is somewhere safe to step back to
     * ([AddSensorState.previousStep]) and otherwise asks the exit question, because
     * past LOCATION the sensor identity exists on the server and cannot be un-minted.
     */
    fun onBack(onDismiss: () -> Unit) {
        val previous = _state.value.previousStep
        if (previous == null) {
            requestExit(onDismiss)
            return
        }
        _state.update { it.copy(currentStep = previous, failure = null, detailsError = null) }
    }

    // --- EXIT ---

    /**
     * Back or close pressed mid-flow. Screens with nothing to lose close straight
     * away; anything else asks once inside the card, because exiting discards the
     * display-once secret forever.
     */
    fun requestExit(onDismiss: () -> Unit) {
        val current = _state.value
        if (current.isBusy) return // never strand a half-written node behind a mis-tap
        if (current.exitLosesProgress) {
            _state.update { it.copy(showingExitConfirm = true) }
        } else {
            onDismiss()
        }
    }

    fun onExitCancelled() {
        _state.update { it.copy(showingExitConfirm = false) }
    }

    /**
     * Discards the session: this ViewModel outlives the dialog (it is activity
     * scoped), so without this a reopened wizard would resume mid-flow on a secret
     * the user can no longer read and a node that was never configured.
     *
     * Cancelling the confirm loop is part of the discard, not an optimisation: a
     * surviving poll would write its answer into the fresh session.
     *
     * The revoke is part of the same discard when [AddSensorState.shouldRevokeOnExit]
     * held: the station id and secret are captured into locals BEFORE the state is
     * cleared (the state is the only place they live), then the request is fired
     * fire-and-forget on the process-wide network scope. Fire-and-forget on
     * purpose: exit must stay instant, a transient failure must not trap the user
     * in a wizard they asked to leave, and no failure here may be reported as
     * success — an unreachable server leaves the orphan row behind for the
     * server-side sweep to reap instead.
     */
    fun reset() {
        // Invalidate any in-flight provisioning response before anything else:
        // from here on, a late answer belongs to a dead session and is revoked
        // rather than applied (see [provisionNode]).
        sessionGeneration++

        confirmJob?.cancel()
        confirmJob = null
        nodeLink.releaseNode()

        val exiting = _state.value
        if (exiting.shouldRevokeOnExit) {
            val stationId = exiting.provisioned?.stationId ?: return
            val secret = exiting.provisioned?.provisioningSecret ?: return
            network.applicationScope.launch {
                network.apiClient.revokeNode(stationId, secret)
                    .onFailure { throwable ->
                        // Logged with the station id only — never the secret.
                        // Not surfaced as UI: the wizard is already gone, and a
                        // "revoke failed" toast after a successful exit would
                        // claim a state (the row survived) without offering any
                        // action. The 14-day server sweep owns this residue.
                        Log.w(TAG, "revoke failed for $stationId", throwable)
                    }
            }
        }

        _state.value = AddSensorState()
        seedPinFromStoredFix()
    }

    override fun onCleared() {
        confirmJob?.cancel()
        nodeLink.releaseNode()
        super.onCleared()
    }

    companion object {
        /** Widest honest query: a sensor installed for a relative across town must still show up. */
        private const val MAX_CONFIRM_RANGE_KM = 500

        /** Server machine code for throttled provisioning requests. */
        private const val CODE_RATE_LIMITED = "RATE_LIMITED"

        private const val TAG = "AddSensor"
    }
}
