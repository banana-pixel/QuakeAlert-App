package id.web.quakealert.ui.addsensor

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import id.web.quakealert.data.network.QuakeNetwork
import id.web.quakealert.data.network.model.NodePortalConfigDto
import id.web.quakealert.device.NodeLink
import id.web.quakealert.ui.sensors.SensorStatus
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/**
 * Drives the add-a-sensor wizard. All decisions are pure functions on
 * [AddSensorState]; this class is only the side effects in order:
 *
 *  1. **Provision** — `POST /nodes/provision` over the internet, before any local
 *     Wi-Fi business (the minted secret must reach the node later through the
 *     portal, and the phone still needs data to make this call).
 *  2. **Link** — bind the phone to the node's SoftAP ([NodeLink]), scan, then
 *     `POST /config` with everything provisioning returned.
 *  3. **Confirm** — poll `/sensors` until the node's echoed identity appears.
 *
     One subtlety the confirm loop handles: `/sensors` answers relative to the
 * position the *server* holds for this user. A user who never synced a position
 * would poll an endpoint that can only answer empty, so when no stored position
 * exists the wizard pushes its own pin first — it doubles as "this is roughly
 * where I install sensors", which is exactly what the field is for.
 */
class AddSensorViewModel(application: Application) : AndroidViewModel(application) {

    private val network = QuakeNetwork.from(application)
    private val apiClient = network.apiClient
    private val nodeLink = NodeLink(application)

    private val _state = MutableStateFlow(AddSensorState())
    val state: StateFlow<AddSensorState> = _state.asStateFlow()

    /** True once a confirm loop is running; guards against double-starts. */
    private var confirming = false

    init {
        viewModelScope.launch {
            // Seed the pin from the current fix; zero coordinates mean "no fix"
            // (the server treats them the same way), not the Gulf of Guinea.
            val fix = apiClient.currentUserLocation() ?: return@launch
            if (fix.latitude != 0.0 || fix.longitude != 0.0) {
                _state.update { it.copy(latitude = fix.latitude, longitude = fix.longitude) }
            }
        }
    }

    // --- DETAILS ---

    fun onNameChanged(raw: String) {
        _state.update { it.copy(locationName = raw, errorMessage = null) }
    }

    fun onModelChanged(model: String) {
        _state.update { it.copy(sensorModel = model) }
    }

    fun onUseCurrentPosition(latitude: Double, longitude: Double) {
        _state.update { it.copy(latitude = latitude, longitude = longitude) }
    }

    fun onManualLatitudeChanged(raw: String) {
        val value = raw.toDoubleOrNull() ?: return
        if (value in -90.0..90.0) {
            _state.update { it.copy(latitude = value) }
        }
    }

    fun onManualLongitudeChanged(raw: String) {
        val value = raw.toDoubleOrNull() ?: return
        if (value in -180.0..180.0) {
            _state.update { it.copy(longitude = value) }
        }
    }

    /** DETAILS gate: valid → provision immediately while the wizard shows progress. */
    fun onDetailsContinue() {
        val next = _state.value.advanceIfDetailsValid()
        _state.value = next
        if (next.step != WizardStep.CREDENTIALS || next.provisioned != null) return

        viewModelScope.launch {
            _state.update { it.copy(isBusy = true, errorMessage = null) }
            val details = _state.value
            apiClient.provisionNode(
                sensorModel = details.sensorModel,
                locationName = SensorNameRules.normalize(details.locationName),
                latitude = requireNotNull(details.latitude),
                longitude = requireNotNull(details.longitude)
            ).onSuccess { node ->
                _state.update { it.onProvisioned(node).copy(isBusy = false) }
            }.onFailure { throwable ->
                _state.update {
                    it.copy(
                        isBusy = false,
                        errorMessage = throwable.message ?: PROVISION_FAILED_MESSAGE
                    )
                }
            }
        }
    }

    fun retryFromDetails() {
        _state.update {
            it.copy(step = WizardStep.DETAILS, errorMessage = null)
        }
    }

    // --- CREDENTIALS ---

    fun onSecretRevealed() {
        _state.update { it.copy(secretRevealed = true) }
    }

    /** CREDENTIALS gate → LINK: bring up the bound node network and scan. */
    fun onCredentialsContinue() {
        if (_state.value.step == WizardStep.LINK) return
        _state.update { it.copy(step = WizardStep.LINK, isBusy = true) }
        viewModelScope.launch {
            nodeLink.bindToNode()
            val scan = nodeLink.scanNetworks()
            _state.update { current ->
                scan.fold(
                    onSuccess = { ssids -> current.copy(scannedSsids = ssids, isBusy = false) },
                    onFailure = { current.copy(isBusy = false, scannedSsids = emptyList()) }
                )
            }
        }
    }

    // --- LINK ---

    fun onSsidSelected(ssid: String) {
        _state.update { it.onSsidSelected(ssid) }
    }

    fun onPasswordChanged(password: String) {
        _state.update { it.copy(wifiPassword = password) }
    }

    fun onRescanClicked() = onCredentialsContinue()

    /** LINK gate: configure the node, then start confirming. */
    fun onConfigureNode() {
        val gated = _state.value.advanceIfLinkValid()
        _state.value = gated
        val provisioned = gated.provisioned ?: return
        val lat = gated.latitude ?: return
        val lon = gated.longitude ?: return

        viewModelScope.launch {
            _state.update { it.copy(isBusy = true, errorMessage = null) }
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
                    ensureServerKnowsAPosition()
                    _state.update { it.onNodeConfigured(echo).copy(isBusy = false) }
                    startConfirmLoop()
                    nodeLink.releaseNode()
                },
                onFailure = { throwable ->
                    _state.update {
                        it.copy(isBusy = false, errorMessage = throwable.message ?: CONFIG_FAILED_MESSAGE)
                    }
                }
            )
        }
    }

    /**
     * Without a stored position `/sensors` can only answer empty, so a first-time
     * user gets the wizard's own pin pushed as their position. A user who already
     * has one is left alone — their phone location is theirs.
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

    // --- CONFIRM ---

    private fun startConfirmLoop() {
        if (confirming) return
        confirming = true
        viewModelScope.launch {
            try {
                while (_state.value.confirmState == ConfirmState.WAITING && _state.value.attemptsLeft > 0) {
                    delay(AddSensorState.CONFIRM_POLL_INTERVAL_MS)
                    _state.update { it.onConfirmPoll(fetchStationStatuses()) }
                }
            } finally {
                confirming = false
            }
        }
    }

    /** Manual re-check from the confirm screen (also used after Pending). */
    fun onRefreshConfirm() {
        viewModelScope.launch {
            if (_state.value.confirmState == ConfirmState.WAITING) {
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
                // Domain → wire vocabulary: trust is asked before health, exactly
                // as the /sensors DTO words it.
                node.stationId to when {
                    !node.verified -> "Pending"
                    node.online -> "Online"
                    else -> "Offline"
                }
            }
            ?: emptyMap()

    /**
     * Leaves the wizard: release the bound node network whatever step we were on.
     * Explicit rather than waiting for [onCleared] — dismissal keeps the ViewModel
     * alive for one more frame, and a lingering specifier network keeps the phone
     * off its normal Wi-Fi.
     */
    fun onDismissed() {
        nodeLink.releaseNode()
    }

    override fun onCleared() {
        nodeLink.releaseNode()
        super.onCleared()
    }

    companion object {
        /** Widest honest query: a sensor installed for a relative across town must still show up. */
        private const val MAX_CONFIRM_RANGE_KM = 500

        private const val PROVISION_FAILED_MESSAGE =
            "Could not register the new sensor. Check your connection and try again."

        private const val CONFIG_FAILED_MESSAGE =
            "The sensor could not be configured. Make sure you are still connected to QuakeSetup."
    }
}
