package id.web.quakealert.ui.addsensor

import id.web.quakealert.domain.ProvisionedNode

/**
 * The screens of the wizard (Figma 155:985 ... 155:1572), in the order the user
 * walks them. [WELCOME] and [RATE_LIMIT] are presentation states rather than work:
 * one introduces the flow, the other replaces it.
 *
 * One enum, not two: the flow used to carry a second parallel `WizardStep` for the
 * pure gates, and keeping two step fields in agreement was a bug waiting to happen.
 */
enum class AddSensorWizardStep {
    /** Welcome screen with sensor icons and Start button. */
    WELCOME,

    /** Location selection: interactive map, GPS sync, editable place name. */
    LOCATION,

    /** Station ID and provisioning secret display. */
    CREDENTIALS,

    /** Wi-Fi network selection and password entry. */
    WLAN,

    /** Processing / finishing with Check Now action. */
    FINISHING,

    /** Provisioning rate limit hit, shown right after Welcome. */
    RATE_LIMIT
}

/**
 * What the FINISHING step is currently seeing.
 */
enum class ConfirmState {
    /** Polling `/sensors`; the node has not appeared yet. */
    WAITING,

    /**
     * The station showed up with the grey Pending chip: configured correctly,
     * awaiting operator verification (migration 000005). Not an error.
     */
    PENDING,

    /** The station reports Online, end to end, including trust. */
    ONLINE
}

/** Why the LOCATION step refuses to continue. Mirrors the server's own rules. */
enum class DetailsError {
    NAME_REQUIRED,
    NAME_TOO_LONG,
    NAME_HAS_NODE_ID,
    NAME_NOT_PLACE_LIKE,
    POSITION_MISSING
}

/** Why the WLAN step refuses to configure. */
enum class LinkError {
    SSID_REQUIRED,
    PASSWORD_TOO_SHORT
}

/**
 * Every way the wizard can fail, as a closed set of situations.
 *
 * An enum rather than a message string, and that is the point: the state cannot
 * carry text at all, so no server sentence, socket state or exception name can
 * reach the screen even by accident. The wording lives in one place
 * ([failureCopy] in AddSensorCopy.kt) and the real cause stays in the log.
 */
enum class WizardFailure {
    /** No usable link to the alert network while registering the sensor. */
    OFFLINE,

    /** The alert network could not register the sensor right now. */
    REGISTER_REJECTED,

    /** The phone never joined the sensor's own network (dialog dismissed, timeout). */
    SENSOR_NOT_JOINED,

    /** Joined, but the sensor did not answer when asked what it can see. */
    SENSOR_NOT_ANSWERING,

    /** The sensor refused or dropped the settings we handed it. */
    SETTINGS_NOT_ACCEPTED,

    /**
     * The sensor could not join the Wi-Fi network with the credentials given —
     * typically a wrong password. Distinct from [SETTINGS_NOT_ACCEPTED] because
     * the fix is different: re-enter credentials, not retry the same ones.
     */
    WIFI_CREDENTIALS_REJECTED,

    /** No position could be obtained from the device. */
    LOCATION_UNAVAILABLE,

    /** The pin has no place name and none could be detected. */
    PLACE_NAME_MISSING,

    /** The sensor never checked in within the confirm budget. */
    SENSOR_NEVER_CHECKED_IN
}

/** Server-side limits the client mirrors so feedback is instant, not a 400. */
object SensorNameRules {
    const val MAX_LENGTH = 150
    const val MIN_CHAR_RUN = 5

    /**
     * The firmware speaks exactly one dialect: the MPU 6050. Kept as a list so a
     * future multi-sensor firmware flips this back into a picker without touching
     * call sites.
     */
    val MODEL_CHOICES = listOf("MPU 6050")

    /**
     * Normalises exactly like `nodeLocationName` (server/internal/api/api.go):
     * trim ends, collapse every whitespace run to one space. Applied before
     * validation and again on submit, so what the user previews is what is sent.
     */
    fun normalize(raw: String): String =
        raw.trim().split(Regex("\\s+")).joinToString(" ")

    private fun hasCharRun(normalized: String): Boolean {
        var run = 1
        for (i in 1 until normalized.length) {
            run = if (normalized[i] == normalized[i - 1]) run + 1 else 1
            if (run >= MIN_CHAR_RUN) return true
        }
        return false
    }

    /** Returns the first rule the name breaks, or null when it would be accepted. */
    fun validate(raw: String): DetailsError? {
        val name = normalize(raw)
        return when {
            name.isEmpty() -> DetailsError.NAME_REQUIRED
            name.length > MAX_LENGTH -> DetailsError.NAME_TOO_LONG
            name.uppercase().contains("NODE-") -> DetailsError.NAME_HAS_NODE_ID
            hasCharRun(name) -> DetailsError.NAME_NOT_PLACE_LIKE
            else -> null
        }
    }
}

/** Wi-Fi rules for the node's home network (the portal's own constraints). */
object WifiRules {
    const val MAX_PASSWORD_LENGTH = 64

    fun validate(ssid: String, password: String): LinkError? = when {
        ssid.isBlank() -> LinkError.SSID_REQUIRED
        password.isNotEmpty() && password.length > MAX_PASSWORD_LENGTH -> LinkError.PASSWORD_TOO_SHORT
        else -> null
    }
}

/**
 * One wizard session, as a single value.
 *
 * Everything decision-shaped lives in the pure functions below ([advanceIfDetailsValid],
 * [onProvisioned], [previousStep], [onConfirmPoll]); this class only carries it. The
 * ViewModel applies those functions to a [kotlinx.coroutines.flow.MutableStateFlow]
 * and owns every side effect: the register call, the local link, the poll loop.
 *
 * @param latitude/longitude the chosen placement. Nullable because they start as the
 *   current fix *if one exists*; a wizard opened without any fix shows an explicit
 *   "drop the pin" state rather than a map centred at Null Island.
 * @param attemptsLeft confirm polls remaining; surfaced so the failure copy can say
 *   how long we actually waited instead of a vague "timed out".
 * @param failure the one situation the card is currently reporting, if any. Never a
 *   message: see [WizardFailure].
 */
data class AddSensorState(
    val currentStep: AddSensorWizardStep = AddSensorWizardStep.WELCOME,
    // --- LOCATION ---
    val locationName: String = "",
    val detectedLocationName: String? = null,
    val isSyncingLocation: Boolean = false,
    val sensorModel: String = SensorNameRules.MODEL_CHOICES.first(),
    val latitude: Double? = null,
    val longitude: Double? = null,
    val detailsError: DetailsError? = null,
    // --- CREDENTIALS ---
    val provisioned: ProvisionedNode? = null,
    val secretRevealed: Boolean = false,
    // --- WLAN ---
    val scannedSsids: List<String> = emptyList(),
    val selectedSsid: String = "",
    val wifiPassword: String = "",
    val linkError: LinkError? = null,
    val nodeConfigured: Boolean = false,
    val effectiveStationId: String? = null,
    // --- FINISHING ---
    val confirmState: ConfirmState = ConfirmState.WAITING,
    val attemptsLeft: Int = MAX_CONFIRM_ATTEMPTS,
    // --- shared ---
    val showingExitConfirm: Boolean = false,
    val isBusy: Boolean = false,
    val failure: WizardFailure? = null
) {

    val detailsValid: Boolean
        get() = latitude != null && longitude != null &&
            SensorNameRules.validate(locationName) == null

    /** The LOCATION screen only demands a pin; the name arrives auto-detected. */
    val locationStepValid: Boolean
        get() = latitude != null && longitude != null

    val linkValid: Boolean
        get() = WifiRules.validate(selectedSsid, wifiPassword) == null

    /**
     * Where the Back capsule leads, or null when there is nowhere to step back to
     * and Back therefore means "leave the wizard".
     *
     * Nothing has been created yet on LOCATION, so its Back is a plain step back to
     * WELCOME. Past that point the sensor identity exists on the server and cannot
     * be un-minted, so stepping back would either strand a half-configured sensor or
     * invite minting a second one; leaving (with the discard question) is the only
     * honest way back, which is what null asks the caller to do.
     */
    val previousStep: AddSensorWizardStep?
        get() = when (currentStep) {
            AddSensorWizardStep.LOCATION -> AddSensorWizardStep.WELCOME
            else -> null
        }

    /**
     * Whether leaving right now would throw work away. Welcome has nothing yet, the
     * rate-limit screen never started, and a finished flow is already saved; every
     * other screen holds a minted identity or a display-once secret.
     */
    val exitLosesProgress: Boolean
        get() = when (currentStep) {
            AddSensorWizardStep.WELCOME,
            AddSensorWizardStep.RATE_LIMIT -> false
            AddSensorWizardStep.FINISHING -> confirmState == ConfirmState.WAITING
            else -> true
        }

    /**
     * Whether exiting now would strand a minted-but-unconfigured node on the
     * server — the condition under which [AddSensorViewModel.reset] must fire a
     * revoke.
     *
     * The rule reads the state's own facts rather than tracking a separate flag:
     *
     *  - `provisioned != null` — an identity exists server-side at all. Before
     *    provisioning succeeds there is nothing to revoke and no capability to
     *    revoke it with; this single guard is what keeps cancel-before-provision
     *    from ever issuing a request.
     *  - `effectiveStationId == null` — the portal never accepted `/config`
     *    ([onNodeConfigured] sets it). A configured node has burned its identity
     *    into NVS: deleting its server row would brick it until a factory reset,
     *    so configuration — not verification — is the point of no return for the
     *    client. Verification stays server-authoritative in any case: even if
     *    this rule were wrong, the server refuses to delete `verified = TRUE`.
     */
    val shouldRevokeOnExit: Boolean
        get() = provisioned != null && effectiveStationId == null

    companion object {
        /**
         * Confirm polls `/sensors` every [CONFIRM_POLL_INTERVAL_MS] for up to ten
         * minutes: a node reboots (~10 s), joins Wi-Fi, NTP-syncs, then heartbeats.
         * Sixty seconds is routinely too tight on a slow router.
         */
        const val MAX_CONFIRM_ATTEMPTS = 20
        const val CONFIRM_POLL_INTERVAL_MS = 30_000L
    }
}

/**
 * Gate for leaving LOCATION. Returns the state unchanged (with the error set) when
 * invalid, so the caller never has to remember to validate separately.
 */
fun AddSensorState.advanceIfDetailsValid(): AddSensorState {
    val positionMissing = latitude == null || longitude == null
    val nameError = SensorNameRules.validate(locationName)
    val error = when {
        positionMissing -> DetailsError.POSITION_MISSING
        nameError != null -> nameError
        else -> null
    } ?: return copy(currentStep = AddSensorWizardStep.CREDENTIALS, detailsError = null)

    return copy(detailsError = error)
}

/** Stores the minted identity and moves to CREDENTIALS. */
fun AddSensorState.onProvisioned(node: ProvisionedNode): AddSensorState = copy(
    provisioned = node,
    secretRevealed = false,
    currentStep = AddSensorWizardStep.CREDENTIALS,
    failure = null
)

/** Applies the Wi-Fi selection, clearing stale link errors when it changes. */
fun AddSensorState.onSsidSelected(ssid: String): AddSensorState = copy(
    selectedSsid = ssid,
    wifiPassword = "",
    linkError = null
)

/** Gate for configuring the node. Same contract as [advanceIfDetailsValid]. */
fun AddSensorState.advanceIfLinkValid(): AddSensorState {
    val error = WifiRules.validate(selectedSsid, wifiPassword)
        ?: return copy(linkError = null)
    return copy(linkError = error)
}

/**
 * Records the portal's answer. The echoed station id is authoritative: if the node
 * already carried a different NVS identity, that is what will heartbeat, and the
 * FINISHING step must search for *this*, not for what the server minted.
 */
fun AddSensorState.onNodeConfigured(effectiveStationId: String?): AddSensorState = copy(
    nodeConfigured = true,
    effectiveStationId = effectiveStationId ?: provisioned?.stationId,
    currentStep = AddSensorWizardStep.FINISHING
)

/**
 * One confirm poll. Matches by station id against the full roll (pending included).
 * An empty result changes nothing except the budget: absence is expected noise while
 * the node is still rebooting.
 */
fun AddSensorState.onConfirmPoll(
    stationsById: Map<String, String>
): AddSensorState {
    val target = effectiveStationId ?: provisioned?.stationId
    val status = target?.let { stationsById[it] }
    return when (status) {
        "Online" -> copy(confirmState = ConfirmState.ONLINE, failure = null)
        "Pending" -> copy(confirmState = ConfirmState.PENDING, failure = null)
        else -> {
            val left = attemptsLeft - 1
            if (left <= 0) copy(attemptsLeft = 0, failure = WizardFailure.SENSOR_NEVER_CHECKED_IN)
            else copy(attemptsLeft = left)
        }
    }
}
