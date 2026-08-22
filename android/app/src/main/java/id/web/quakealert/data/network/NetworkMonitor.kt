package id.web.quakealert.data.network

import android.content.Context
import android.net.ConnectivityManager
import android.net.Network
import android.net.NetworkCapabilities
import android.net.NetworkRequest
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.FlowPreview
import kotlinx.coroutines.channels.awaitClose
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.callbackFlow
import kotlinx.coroutines.flow.conflate
import kotlinx.coroutines.flow.debounce
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.drop
import kotlinx.coroutines.flow.filter
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.shareIn

/**
 * Whether the device currently has an internet connection that actually works.
 *
 * Exists because recovery used to be entirely manual on the REST screens. The
 * WebSocket already reconnects itself with jittered exponential backoff, which is why
 * the header badge turns green without help — but a History or Sensors list that
 * failed while offline kept its error state until someone tapped Retry, and in the
 * worst case sat next to a green badge saying the server was fine.
 *
 * Two deliberate choices:
 *
 *  1. **Callback, not polling.** `registerNetworkCallback` is how the platform reports
 *     this; a timer would burn wakeups to learn the same thing later.
 *  2. **[NetworkCapabilities.NET_CAPABILITY_VALIDATED] is required, not just
 *     `NET_CAPABILITY_INTERNET`.** "Attached to Wi-Fi" is not "the internet answers":
 *     a captive portal satisfies the second and fails the first, and treating it as
 *     online is how a screen ends up retrying into a login page forever.
 */
class NetworkMonitor(context: Context, scope: CoroutineScope) {

    private val connectivity =
        context.applicationContext.getSystemService(ConnectivityManager::class.java)

    /**
     * True while at least one validated internet-capable network is available.
     *
     * Shared with [SharingStarted.WhileSubscribed] and a replay of 1: several
     * ViewModels observe it, and one registered callback is enough for all of them.
     * The callback is unregistered once the last of them stops collecting.
     */
    @OptIn(ExperimentalCoroutinesApi::class)
    val isOnline: Flow<Boolean> = callbackFlow {
        val manager = connectivity
        if (manager == null) {
            // No ConnectivityManager at all (which should not happen outside a test
            // double): report online rather than online-never, because a false
            // negative here would suppress every automatic recovery in the app.
            trySend(true)
            awaitClose { }
            return@callbackFlow
        }

        // Tracked as a set rather than a boolean: a Wi-Fi/cellular handover delivers
        // onAvailable for the new network before onLost for the old one, and a
        // boolean would report a drop that never happened.
        val available = mutableSetOf<Network>()

        val callback = object : ConnectivityManager.NetworkCallback() {
            override fun onAvailable(network: Network) {
                available += network
                trySend(available.isNotEmpty())
            }

            override fun onLost(network: Network) {
                available -= network
                trySend(available.isNotEmpty())
            }

            override fun onCapabilitiesChanged(
                network: Network,
                capabilities: NetworkCapabilities
            ) {
                // A network can be available before it is validated — exactly what a
                // captive portal looks like — so validation is re-checked here rather
                // than trusted from onAvailable alone.
                if (capabilities.hasCapability(NetworkCapabilities.NET_CAPABILITY_VALIDATED)) {
                    available += network
                } else {
                    available -= network
                }
                trySend(available.isNotEmpty())
            }
        }

        val request = NetworkRequest.Builder()
            .addCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)
            .addCapability(NetworkCapabilities.NET_CAPABILITY_VALIDATED)
            .build()
        manager.registerNetworkCallback(request, callback)
        // The current state, before any change arrives: a screen that subscribes while
        // already offline must not have to wait for a transition to learn it.
        trySend(manager.hasValidatedInternet())

        awaitClose { manager.unregisterNetworkCallback(callback) }
    }
        .conflate()
        .distinctUntilChanged()
        .shareIn(
            scope = scope,
            started = SharingStarted.WhileSubscribed(STOP_TIMEOUT_MS),
            replay = 1
        )

    /**
     * Emits once each time connectivity is *regained*, and never on the state a
     * collector starts with.
     *
     * The drop and the debounce are what make this usable as a retry trigger: without
     * the drop, every screen would reload the moment it subscribed while online, and
     * without the debounce a Wi-Fi/cellular handover that flaps would fire several
     * reloads of the same request.
     */
    @OptIn(FlowPreview::class)
    val onlineRegained: Flow<Unit> = isOnline
        .drop(1)
        .debounce(SETTLE_DELAY_MS)
        .filter { it }
        .map { }

    private fun ConnectivityManager.hasValidatedInternet(): Boolean {
        val capabilities = getNetworkCapabilities(activeNetwork) ?: return false
        return capabilities.hasCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET) &&
            capabilities.hasCapability(NetworkCapabilities.NET_CAPABILITY_VALIDATED)
    }

    private companion object {
        /** Keeps the callback registered briefly across a tab switch. */
        const val STOP_TIMEOUT_MS = 5_000L

        /** Long enough for a handover to settle, short enough to feel automatic. */
        const val SETTLE_DELAY_MS = 500L
    }
}
