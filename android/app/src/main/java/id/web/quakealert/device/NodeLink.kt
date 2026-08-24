package id.web.quakealert.device

import android.annotation.SuppressLint
import android.content.Context
import android.content.Intent
import android.net.ConnectivityManager
import android.net.Network
import android.net.NetworkCapabilities
import android.net.NetworkRequest
import android.net.LinkProperties
import android.os.Build
import android.provider.Settings
import android.util.Log
import id.web.quakealert.data.network.model.NodePortalConfigDto
import id.web.quakealert.data.network.model.NodePortalConfigResponseDto
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.Json
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import java.util.concurrent.TimeUnit
import kotlin.coroutines.resume

/**
 * The local link to an unconfigured sensor node.
 *
 * A node in provisioning mode broadcasts a SoftAP (SSID `QuakeSetup`) serving a
 * tiny HTTP portal on [NODE_ADDRESS]. Talking to it means leaving the internet:
 * the phone's normal Wi-Fi cannot route to a captive AP, so on Android 10+ this
 * class asks the system for a *bound* secondary network via
 * [WifiNetworkSpecifier][android.net.NetSpecifier] — the user confirms a join in a
 * system dialog and the app keeps its cellular data meanwhile. That is why
 * provisioning happens **before** linking: the provision call needs the internet,
 * the config call must not use it.
 *
 * Everything decision-shaped about the portal's answers lives in the pure parsers
 * ([parseScanResponse], [parseConfigOutcome]); this class owns only transport and
 * permission-shaped plumbing that unit tests cannot reach without Robolectric.
 */
class NodeLink(context: Context) {

    private val appContext = context.applicationContext
    private val connectivity =
        appContext.getSystemService(ConnectivityManager::class.java)

    /** Bound per link session; null while not linked. */
    private var boundNetwork: Network? = null
    private var boundCallback: ConnectivityManager.NetworkCallback? = null

    private val json = Json { ignoreUnknownKeys = true }
    private val jsonMediaType = "application/json; charset=utf-8".toMediaType()

    /**
     * Asks the system to bring up the node network. Resumes with the bound
     * [Network] **once it has an IPv4 address** — [onAvailable][ConnectivityManager.NetworkCallback.onAvailable]
     * alone fires before DHCP finishes, and a portal call made in that window
     * fails with EHOSTUNREACH, which reads downstream as "no networks found".
     * Null when the user dismissed the dialog or it timed out.
     *
     * Pre-Android-10 there is no specifier API; returns null immediately after
     * firing [openWifiSettings] so the caller can explain the manual step.
     */
    @SuppressLint("MissingPermission")
    suspend fun bindToNode(
        apSsid: String = NODE_AP_SSID,
        openSettingsFallback: () -> Unit = ::openWifiSettings
    ): Network? {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.Q) {
            openSettingsFallback()
            return null
        }

        val manager = connectivity ?: return null
        releaseNode()
        Log.i(TAG, "binding to SoftAP \"$apSsid\"")

        val request = NetworkRequest.Builder()
            .addTransportType(NetworkCapabilities.TRANSPORT_WIFI)
            .removeCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)
            .setNetworkSpecifier(
                android.net.wifi.WifiNetworkSpecifier.Builder()
                    .setSsid(apSsid)
                    .build()
            )
            .build()

        val network: Network? = suspendCancellableCoroutine { continuation ->
            val callback = object : ConnectivityManager.NetworkCallback() {
                override fun onAvailable(network: Network) {
                    Log.i(TAG, "node network available (pre-DHCP): $network")
                }

                // The gate that matters: link properties carry the interface's
                // addresses, so their arrival means DHCP finished and 192.168.4.1
                // is actually reachable through this network.
                override fun onLinkPropertiesChanged(
                    network: Network,
                    linkProperties: LinkProperties
                ) {
                    val hasV4 = linkProperties.getLinkAddresses().any { addr -> addr.address is java.net.Inet4Address }
                    Log.i(TAG, "link properties changed, ipv4=$hasV4")
                    if (hasV4 && continuation.isActive) {
                        continuation.resume(network)
                    }
                }

                override fun onUnavailable() {
                    Log.w(TAG, "node network unavailable (dialog dismissed or timed out)")
                    continuation.resume(null)
                }
            }
            boundCallback = callback
            // requestNetwork (not registerNetworkCallback): brings the network UP,
            // showing the join dialog, instead of waiting for one to exist.
            manager.requestNetwork(request, callback)
            continuation.invokeOnCancellation { runCatching { manager.unregisterNetworkCallback(callback) } }
        }

        boundNetwork = network
        if (network == null) boundCallback = null else Log.i(TAG, "bound: $network")
        return network
    }

    /** Whether a node network is currently bound (portal calls need one). */
    val isLinked: Boolean get() = boundNetwork != null

    /** Releases the bound node network so the phone returns to its normal Wi-Fi. */
    fun releaseNode() {
        val manager = connectivity ?: return
        boundCallback?.let { runCatching { manager.unregisterNetworkCallback(it) } }
        boundCallback = null
        boundNetwork = null
    }

    /**
     * Fetches the visible-network list from the node's `/scan` endpoint.
     * Requires [bindToNode] to have succeeded.
     */
    suspend fun scanNetworks(): Result<List<String>> = withContext(Dispatchers.IO) {
        runPortalCall {
            val response = clientFor(boundNetwork).newCall(
                Request.Builder().url("http://$NODE_ADDRESS/scan").build()
            ).execute()
            parseScanResponse(response.body?.string())
        }
    }

    /**
     * Posts the wizard's handoff to `/config`. Returns the node's effective
     * station id from the echo, or throws through [Result] on refusal — a `400`
     * invalid_station_id is exactly as actionable as a refused connection.
     */
    suspend fun sendConfig(config: NodePortalConfigDto): Result<String?> = withContext(Dispatchers.IO) {
        runPortalCall {
            val request = Request.Builder()
                .url("http://$NODE_ADDRESS/config")
                .post(json.encodeToString(config).toRequestBody(jsonMediaType))
                .build()
            val response = clientFor(boundNetwork).newCall(request).execute()
            val outcome = parseConfigOutcome(response.body?.string(), response.code)
            outcome.getOrThrow()
        }
    }

    private inline fun <T> runPortalCall(block: () -> T): Result<T> =
        if (boundNetwork == null) {
            Result.failure(IllegalStateException(NOT_LINKED_MESSAGE))
        } else {
            try {
                Result.success(block())
            } catch (throwable: Exception) {
                Log.w(TAG, "portal call failed", throwable)
                Result.failure(throwable)
            }
        }

    private fun clientFor(network: Network?): OkHttpClient {
        val base = OkHttpClient.Builder()
            .connectTimeout(PORTAL_TIMEOUT_S, TimeUnit.SECONDS)
            .readTimeout(PORTAL_TIMEOUT_S, TimeUnit.SECONDS)
            .callTimeout(PORTAL_CALL_TIMEOUT_S, TimeUnit.SECONDS)
            .build()
        if (network == null) return base
        // The specifier network has no default route: without this binding, calls
        // to 192.168.4.1 go out the mobile interface and vanish.
        return base.newBuilder()
            .socketFactory(network.socketFactory)
            .dns(
                object : okhttp3.Dns {
                    override fun lookup(hostname: String): List<java.net.InetAddress> =
                        network.getAllByName(hostname).toList()
                }
            )
            .build()
    }

    private fun openWifiSettings() {
        val intent = Intent(Settings.ACTION_WIFI_SETTINGS).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        runCatching { appContext.startActivity(intent) }
    }

    companion object {
        private const val TAG = "NodeLink"

        /** The SoftAP name the firmware broadcasts while provisioning. */
        const val NODE_AP_SSID = "QuakeSetup"

        /** The node's address inside its own SoftAP network. */
        const val NODE_ADDRESS = "192.168.4.1"

        private const val PORTAL_TIMEOUT_S = 10L
        private const val PORTAL_CALL_TIMEOUT_S = 30L

        private const val NOT_LINKED_MESSAGE =
            "Not connected to the sensor's setup network yet."
    }
}

/**
 * `/scan` answers a root-level JSON array of SSID strings (`serializeJson` of a
 * `JsonArray`). Anything else — garbage body, HTML error page, empty string — is
 * an empty list, not a failure: a node whose radio found nothing still answered.
 */
internal fun parseScanResponse(body: String?): List<String> {
    if (body.isNullOrBlank()) return emptyList()
    return try {
        val parser = Json { ignoreUnknownKeys = true }
        parser.decodeFromString<List<String>>(body)
    } catch (_: Exception) {
        emptyList()
    }
}

/**
 * `/config` answers `{"status":"success","station_id":...}` or an error shape.
 * Success yields the echoed station id (may be absent on older firmware);
 * non-success or a non-2xx fails with the portal's own message when it has one.
 */
internal fun parseConfigOutcome(body: String?, httpCode: Int): Result<String?> {
    if (httpCode !in 200..299) {
        val message = body?.let {
            try {
                Json { ignoreUnknownKeys = true }.decodeFromString<NodePortalConfigResponseDto>(it).message
            } catch (_: Exception) {
                null
            }
        }
        return Result.failure(PortalRejectedException(message ?: "HTTP $httpCode"))
    }
    return try {
        val dto = Json { ignoreUnknownKeys = true }
            .decodeFromString<NodePortalConfigResponseDto>(requireNotNull(body))
        if (dto.status.equals("success", ignoreCase = true)) {
            Result.success(dto.stationId)
        } else {
            Result.failure(PortalRejectedException(dto.message ?: "The sensor refused the configuration"))
        }
    } catch (_: Exception) {
        Result.failure(PortalRejectedException("Unreadable answer from the sensor"))
    }
}

/** The portal refused or answered nonsense; [message] is user-displayable. */
class PortalRejectedException(message: String) : Exception(message)
