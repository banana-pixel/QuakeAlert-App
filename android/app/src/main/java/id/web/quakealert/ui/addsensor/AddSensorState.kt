package id.web.quakealert.ui.addsensor

import id.web.quakealert.domain.ProvisionedNode

/**
 * The four steps of the wizard, in order. A step exists only while the user has
 * something to do or see on it — provisioning itself is a busy flag inside
 * [WizardStep.Details], not a fifth screen.
 */
enum class WizardStep {
    /** Name the sensor and drop its pin. */
    DETAILS,

    /** Show the minted identity + display-once secret; the gate before linking. */
    CREDENTIALS,

    /** Join the node's SoftAP and hand over credentials via its /config portal. */
    LINK,

    /** Wait for the node's first heartbeat to surface in `/sensors`. */
    CONFIRM
}

/**
 * What the CONFIRM step is currently seeing.
 */
enum class ConfirmState {
    /** Polling `/sensors`; the node has not appeared yet. */
    WAITING,

    /**
     * The station showed up with the grey Pending chip: configured correctly,
     * awaiting operator verification (migration 000005). Not an error.
     */
    PENDING,

    /** The station reports Online — end to end, including trust. */
    ONLINE
}

/** Why the DETAILS step refuses to continue. Mirrors the server's own rules. */
enum class DetailsError {
    NAME_REQUIRED,
    NAME_TOO_LONG,
    NAME_HAS_NODE_ID,
    NAME_NOT_PLACE_LIKE,
    POSITION_MISSING
}

/** Why the LINK step refuses to configure. */
enum class LinkError {
    SSID_REQUIRED,
    PASSWORD_TOO_SHORT
}

/** Server-side limits the client mirrors so feedback is instant, not a 400. */
object SensorNameRules {
    const val MAX_LENGTH = 150
    const val MIN_CHAR_RUN = 5
    val MODEL_CHOICES = listOf("MPU 6050", "MPU 9250", "ADXL355")

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
 * Everything decision-shaped lives in pure functions below ([DetailsError],
 * [WifiRules], [advanceIfDetailsValid], [onProvisioned], [onConfirmPoll]) — this
 * class only carries it. The ViewModel applies these functions to a
 * [MutableStateFlow] and owns every side effect: the provision call, the local
 * link, the poll loop.
 *
 * @param latitude/longitude the chosen placement. Nullable because they start as
 *   the current fix *if one exists*; a wizard opened without any fix shows an
 *   explicit "drop the pin" state rather than a map centred at Null Island.
 * @param attemptsLeft confirm polls remaining; surfaced so the failure message can
 *   say how long we actually waited instead of a vague "timed out".
 */
data class AddSensorState(
    val step: WizardStep = WizardStep.DETAILS,
    // --- DETAILS ---
    val locationName: String = "",
    val sensorModel: String = SensorNameRules.MODEL_CHOICES.first(),
    val latitude: Double? = null,
    val longitude: Double? = null,
    val detailsError: DetailsError? = null,
    // --- CREDENTIALS ---
    val provisioned: ProvisionedNode? = null,
    val secretRevealed: Boolean = false,
    // --- LINK ---
    val scannedSsids: List<String> = emptyList(),
    val selectedSsid: String = "",
    val wifiPassword: String = "",
    val linkError: LinkError? = null,
    val nodeConfigured: Boolean = false,
    val effectiveStationId: String? = null,
    // --- CONFIRM ---
    val confirmState: ConfirmState = ConfirmState.WAITING,
    val attemptsLeft: Int = MAX_CONFIRM_ATTEMPTS,
    // --- shared ---
    val isBusy: Boolean = false,
    val errorMessage: String? = null
) {

    val detailsValid: Boolean
        get() = latitude != null && longitude != null &&
            SensorNameRules.validate(locationName) == null

    val linkValid: Boolean
        get() = WifiRules.validate(selectedSsid, wifiPassword) == null

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
 * Gate for leaving DETAILS. Returns the state unchanged (with the error set) when
 * invalid — the caller never has to remember to validate separately.
 */
fun AddSensorState.advanceIfDetailsValid(): AddSensorState {
    val positionMissing = latitude == null || longitude == null
    val nameError = SensorNameRules.validate(locationName)
    val error = when {
        positionMissing -> DetailsError.POSITION_MISSING
        nameError != null -> nameError
        else -> null
    } ?: return copy(step = WizardStep.CREDENTIALS, detailsError = null)

    return copy(detailsError = error)
}

/** Stores the minted identity and moves to CREDENTIALS. */
fun AddSensorState.onProvisioned(node: ProvisionedNode): AddSensorState = copy(
    provisioned = node,
    secretRevealed = false,
    step = WizardStep.CREDENTIALS,
    errorMessage = null
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
 * Records the portal's answer. The echoed station id is authoritative — if the node
 * already carried a different NVS identity, that is what will heartbeat, and the
 * CONFIRM step must search for *this*, not for what the server minted.
 */
fun AddSensorState.onNodeConfigured(effectiveStationId: String?): AddSensorState = copy(
    nodeConfigured = true,
    effectiveStationId = effectiveStationId ?: provisioned?.stationId,
    step = WizardStep.CONFIRM
)

/**
 * One confirm poll. Matches by station id against the full roll (pending included).
 * An empty result changes nothing except the budget — absence is expected noise
 * while the node is still rebooting.
 */
fun AddSensorState.onConfirmPoll(
    stationsById: Map<String, String>
): AddSensorState {
    val target = effectiveStationId ?: provisioned?.stationId
    val status = target?.let { stationsById[it] }
    return when (status) {
        "Online" -> copy(confirmState = ConfirmState.ONLINE)
        "Pending" -> copy(confirmState = ConfirmState.PENDING)
        else -> {
            val left = attemptsLeft - 1
            if (left <= 0) copy(attemptsLeft = 0, errorMessage = EXHAUSTED_MESSAGE)
            else copy(attemptsLeft = left)
        }
    }
}

private const val EXHAUSTED_MESSAGE =
    "The sensor has not checked in yet. Make sure it stayed powered, is within " +
        "range of your Wi-Fi, and try confirming again from the Sensors tab."
