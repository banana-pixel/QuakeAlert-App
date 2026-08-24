package id.web.quakealert.data.network

import id.web.quakealert.domain.ServerConnectionState
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

/**
 * The one verdict the app bar badge shows on every tab.
 *
 * Four states, because three questions must stay distinguishable: is it *us*
 * ([OFFLINE] with no network), *the server* ([OFFLINE] with a working link),
 * or merely *part of the backend* ([LIMITED])? A boolean cannot answer that, and
 * per-screen derivations answering it differently are exactly how the badge used
 * to lie — green on one tab, absent on the next, same server.
 *
 * [LIMITED] is the internal name; the UI never prints "Degraded" — the word means
 * nothing to a person checking whether alerts can arrive. See [label].
 */
enum class ServerHealth {
    /** No verdict yet: first probe still in flight, nothing contradicts anything. */
    CHECKING,

    /** Reachable, and nothing observed suggests otherwise. */
    HEALTHY,

    /**
     * The alert path works (or at least the server answers), but something
     * behind it does not — a failed probe while the WebSocket is alive, an
     * unhealthy dependency, or a reachable server whose whole known sensor
     * fleet has gone silent.
     */
    LIMITED,

    /** Alerts cannot be arriving right now. */
    OFFLINE
}

/**
 * What one `GET /healthz` round-trip concluded.
 *
 * Deliberately narrower than HTTP: any 2xx body that answers at all proves the
 * endpoint is up, whatever it says — a server that replies `"ok"` in plain text
 * and one that replies with full JSON are equally reachable.
 */
enum class ProbeOutcome {
    /** No probe has completed since the monitor started. */
    UNKNOWN,
    OK,

    /** 2xx whose body names a dependency that is down (JSON `/healthz`). */
    DEPENDENCIES_DEGRADED,

    /** Non-2xx, connection refused, timeout, or any other hard failure. */
    FAILED
}

/**
 * What the most recent successful `GET /sensors` said about the fleet the user
 * can see. Kept separate from [ProbeOutcome] because they degrade for different
 * reasons: the server can be perfectly healthy above a silent node fleet, and
 * that distinction belongs in the word "Limited", not in a red "Offline".
 */
enum class SensorNetworkStatus {
    /** No successful sensors call yet this process. */
    UNKNOWN,

    /** At least one trusted station within range is reporting. */
    REPORTING,

    /** The roll loaded fine, but every station in it has gone quiet. */
    ALL_SILENT,

    /**
     * The server answered `200` with an empty roll because the user has never
     * shared a location. This is explicitly **not** a degradation: an empty list
     * caused by a privacy choice is the server working exactly as designed.
     */
    NO_STORED_LOCATION
}

/** Wire shape of the JSON `/healthz` body (`router.go`'s handler). */
@Serializable
data class HealthDto(
    @SerialName("status") val status: String? = null,
    @SerialName("database") val database: String? = null,
    @SerialName("mqtt") val mqtt: String? = null
)

/**
 * The whole decision, as one pure function.
 *
 * Rules apply in order; the first match wins:
 *  1. No validated network — the device's own connectivity outranks everything,
 *     because no amount of server health helps a phone with no route out.
 *  2–3. A failed probe normally means [ServerHealth.OFFLINE], **unless** the alert
 *     socket is still open — one dropped poll must not outweigh a live push channel,
 *     so that case lands on [ServerHealth.LIMITED] instead.
 *  4. No verdict yet (nothing has probed successfully) and no live socket to
 *     contradict it: [ServerHealth.CHECKING].
 *  5. An unhealthy named dependency degrades without disconnecting: [ServerHealth.LIMITED].
 *  6. A reachable server above a wholly silent fleet: [ServerHealth.LIMITED].
 *  7. Everything else — including "reachable, sensors unknown" and the crucial
 *     "reachable, user shares no location" case: [ServerHealth.HEALTHY]. That last
 *     one is what the old badge got wrong, and why [SensorNetworkStatus.NO_STORED_LOCATION]
 *     deliberately reaches healthy here.
 */
internal fun evaluateServerHealth(
    deviceOnline: Boolean,
    probe: ProbeOutcome,
    socket: ServerConnectionState,
    sensors: SensorNetworkStatus
): ServerHealth = when {
    !deviceOnline -> ServerHealth.OFFLINE
    probe == ProbeOutcome.FAILED && socket == ServerConnectionState.CONNECTED -> ServerHealth.LIMITED
    probe == ProbeOutcome.FAILED -> ServerHealth.OFFLINE
    probe == ProbeOutcome.UNKNOWN && socket != ServerConnectionState.CONNECTED -> ServerHealth.CHECKING
    probe == ProbeOutcome.DEPENDENCIES_DEGRADED -> ServerHealth.LIMITED
    sensors == SensorNetworkStatus.ALL_SILENT -> ServerHealth.LIMITED
    else -> ServerHealth.HEALTHY
}

/**
 * One HTTP response → one [ProbeOutcome].
 *
 * - Non-2xx is [ProbeOutcome.FAILED]: a load balancer answering 503 on the server's
 *   behalf is exactly as useful as a refused connection.
 * - 2xx with an unparseable body is [ProbeOutcome.OK]: today's plain-text `ok` must
 *   keep working, and an endpoint that answers is up whatever its body says.
 * - 2xx parsing as [HealthDto] with a dependency reporting anything but `"ok"` is
 *   [ProbeOutcome.DEPENDENCIES_DEGRADED].
 */
internal fun healthOutcomeOf(httpCode: Int, body: String?): ProbeOutcome {
    if (httpCode !in 200..299) return ProbeOutcome.FAILED
    val dto = body?.let(::parseHealthBody) ?: return ProbeOutcome.OK
    val dependencyDown = listOf(dto.database, dto.mqtt).any { it != null && !it.equals("ok", true) }
    return if (dependencyDown) ProbeOutcome.DEPENDENCIES_DEGRADED else ProbeOutcome.OK
}

private fun parseHealthBody(body: String): HealthDto? =
    try {
        healthJson.decodeFromString(HealthDto.serializer(), body)
    } catch (_: Exception) {
        null
    }

private val healthJson = kotlinx.serialization.json.Json {
    ignoreUnknownKeys = true
    isLenient = true
}

/**
 * Pure classification of one successful `/sensors` envelope.
 *
 * [SensorNetworkStatus.NO_STORED_LOCATION] wins even at zero reporting stations:
 * the exact bug this module exists for was an empty roll (no stored location)
 * being read as "something is wrong".
 */
internal fun sensorNetworkStatusOf(hasStoredLocation: Boolean, activeSensorsCount: Int): SensorNetworkStatus = when {
    !hasStoredLocation -> SensorNetworkStatus.NO_STORED_LOCATION
    activeSensorsCount > 0 -> SensorNetworkStatus.REPORTING
    else -> SensorNetworkStatus.ALL_SILENT
}
