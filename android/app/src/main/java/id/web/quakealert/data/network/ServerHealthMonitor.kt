package id.web.quakealert.data.network

import id.web.quakealert.domain.ServerConnectionState
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.flatMapLatest
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.flow.stateIn

/**
 * The process-wide answer to "can alerts reach this device right now?", published
 * as [health] for the app bar badge on every tab.
 *
 * Combines four independent signals — device connectivity ([NetworkMonitor.isOnline]),
 * a polled `GET /healthz` ([HealthProbe]), the alert socket's state, and what the
 * latest successful sensors call said about the node fleet — through the pure
 * [evaluateServerHealth] truth table. All judgement lives there; this class only
 * owns cadence and wiring.
 *
 * Cadence rules that keep the badge honest without burning battery:
 *
 *  - **Polling is gated on connectivity.** `flatMapLatest` restarts the probe loop
 *    when the device comes online and cancels it outright when it goes offline — no
 *    radio wakeups asking a network that does not exist whether the server is up,
 *    and an immediate probe on regaining connectivity instead of waiting out a delay.
 *  - **Flap guard.** A single failed probe holds the previous verdict and retries
 *    quickly; only the second consecutive failure flips to OFFLINE. One dropped
 *    packet must not strobe every tab red.
 *  - **Subscribed-only.** `WhileSubscribed(5_000)` means no screen showing the badge
 *    means no probes at all; the cost while visible is ~2 requests/min of ~150-byte
 *    responses, below what the idle WebSocket ping traffic already spends.
 */
class ServerHealthMonitor(
    private val networkMonitor: NetworkMonitor,
    private val webSocketClient: QuakeWebSocketClient,
    private val probe: HealthProbe,
    scope: CoroutineScope
) {

    /**
     * The sensor-fleet signal, written from the data layer (see
     * [sensorNetworkStatusOf]) rather than from whichever ViewModel happens to be
     * alive: health must not depend on which screen is open.
     */
    private val sensorNetwork = MutableStateFlow(SensorNetworkStatus.UNKNOWN)

    /** Reports the outcome of one successful `GET /sensors`; failures change nothing. */
    fun reportSensorNetwork(status: SensorNetworkStatus) {
        sensorNetwork.value = status
    }

    /**
     * The poll loop: emit UNKNOWN (no verdict), wait, repeat. Restarted from scratch
     * by [flatMapLatest] whenever connectivity changes.
     */
    private fun pollLoop() = flow {
        emit(ProbeOutcome.UNKNOWN)
        while (true) {
            var outcome = probe.probe()
            if (outcome == ProbeOutcome.FAILED) {
                // Flap guard: retry fast once before believing the failure.
                delay(FAST_RETRY_MS)
                outcome = probe.probe()
            }
            emit(outcome)
            delay(POLL_INTERVAL_MS)
        }
    }

    @OptIn(ExperimentalCoroutinesApi::class)
    private val combined =
        combine(
            networkMonitor.isOnline,
            networkMonitor.isOnline.flatMapLatest { online ->
                if (online) pollLoop() else flow { emit(ProbeOutcome.FAILED) }
            },
            webSocketClient.connectionState,
            sensorNetwork
        ) { online, probeOutcome, socket, sensors ->
            evaluateServerHealth(online, probeOutcome, socket, sensors)
        }

    val health: StateFlow<ServerHealth> = combined
        .distinctUntilChanged()
        .stateIn(
            scope = scope,
            started = SharingStarted.WhileSubscribed(STOP_TIMEOUT_MS),
            initialValue = ServerHealth.CHECKING
        )

    companion object {
        /** Steady-state poll period; ~2 requests/min of ~150 bytes. */
        const val POLL_INTERVAL_MS = 30_000L

        /** Quick second attempt after a first failure, before believing it. */
        const val FAST_RETRY_MS = 5_000L

        /** Holds the monitor across a tab switch, like every other shared flow. */
        const val STOP_TIMEOUT_MS = 5_000L
    }
}
