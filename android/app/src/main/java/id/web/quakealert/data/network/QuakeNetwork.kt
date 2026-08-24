package id.web.quakealert.data.network

import android.content.Context
import id.web.quakealert.data.AppSettingsRepository
import id.web.quakealert.data.auth.AuthRepository
import id.web.quakealert.data.users.UserLocationRepository
import id.web.quakealert.data.local.SessionStore
import id.web.quakealert.data.push.PushRegistrar
import id.web.quakealert.domain.AlertDedup
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.serialization.json.Json
import okhttp3.OkHttpClient
import java.util.concurrent.TimeUnit

/**
 * Hand-rolled service locator for the network layer.
 *
 * The app has no DI framework and its ViewModels are built by Compose's default
 * `viewModel()` factory, which can only pass an `Application` — so a ViewModel
 * reaches its dependencies through [from] rather than through its constructor.
 * Everything below is a process-lifetime singleton on purpose: one [OkHttpClient]
 * (its connection pool and dispatcher threads are the expensive part), one
 * [SessionStore] (a second DataStore over the same file throws), one identity.
 */
class QuakeNetwork private constructor(context: Context) {

    private val appContext: Context = context.applicationContext

    /** Shared codec, configured once in [quakeJson]. */
    val json: Json = quakeJson()

    val sessionStore: SessionStore = SessionStore(appContext)

    val authRepository: AuthRepository by lazy {
        // The client is passed as a provider, not a value: its interceptor needs this
        // repository, so eager construction either way would be circular.
        AuthRepository(sessionStore = sessionStore, json = json) { httpClient }
    }

    /**
     * REST transport. Timeouts are short by design — a stale earthquake feed is
     * worth retrying, not waiting on.
     */
    private val httpClient: OkHttpClient by lazy {
        OkHttpClient.Builder()
            .connectTimeout(CONNECT_TIMEOUT_S, TimeUnit.SECONDS)
            .readTimeout(READ_TIMEOUT_S, TimeUnit.SECONDS)
            .writeTimeout(WRITE_TIMEOUT_S, TimeUnit.SECONDS)
            .retryOnConnectionFailure(true)
            .addInterceptor(AuthInterceptor(authRepository))
            .build()
    }

    /**
     * WebSocket transport: the same client with client-side pings enabled, so a
     * connection killed by a NAT or a sleeping radio is detected instead of looking
     * healthy forever. Kept below the server's 30 s pong deadline
     * (server/internal/dispatch/ws.go).
     */
    private val webSocketHttpClient: OkHttpClient by lazy {
        httpClient.newBuilder()
            .pingInterval(WS_PING_INTERVAL_S, TimeUnit.SECONDS)
            .build()
    }

    /**
     * Scope owning the shared WebSocket connection. It outlives every ViewModel
     * (the socket must survive navigation between screens) and is never cancelled —
     * the process is its lifetime.
     */
    private val networkScope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.IO)

    /**
     * The same scope, for fire-and-forget work started from a screen that may go
     * away before it finishes — a position sync kicked off by the onboarding
     * permission grant must not be cancelled by the navigation that follows it.
     */
    val applicationScope: CoroutineScope get() = networkScope

    val apiClient: QuakeApiClient by lazy {
        QuakeApiClient(
            client = httpClient,
            json = json,
            authRepository = authRepository,
            sessionStore = sessionStore
        )
    }

    /**
     * Position acquisition + `PUT /users/location`. Shared rather than per-ViewModel
     * so onboarding, Settings and app start cannot race each other into three
     * concurrent uploads of the same fix.
     */
    val userLocationRepository: UserLocationRepository by lazy {
        UserLocationRepository(
            context = appContext,
            apiClient = apiClient,
            sessionStore = sessionStore,
            settings = AppSettingsRepository(appContext)
        )
    }

    /**
     * Connectivity, shared: one registered `NetworkCallback` for the whole process
     * rather than one per screen that wants to recover from an outage.
     */
    val networkMonitor: NetworkMonitor by lazy {
        NetworkMonitor(context = appContext, scope = networkScope)
    }

    /**
     * Probe transport: the shared client's pool and DNS, with its own short
     * timeouts. A health answer that takes 20 s to arrive has already told the
     * user the wrong thing by waiting; 3 s connect/read and a 5 s call ceiling
     * keep one probe inside a badge tick.
     */
    private val healthHttpClient: OkHttpClient by lazy {
        httpClient.newBuilder()
            .connectTimeout(HEALTH_CONNECT_TIMEOUT_S, TimeUnit.SECONDS)
            .readTimeout(HEALTH_READ_TIMEOUT_S, TimeUnit.SECONDS)
            .callTimeout(HEALTH_CALL_TIMEOUT_S, TimeUnit.SECONDS)
            .build()
    }

    /**
     * The single verdict behind the top-bar badge on every tab. Lazy like its
     * inputs; constructing it opens no socket and starts no polling — both begin
     * when the first screen subscribes to [ServerHealthMonitor.health].
     */
    val serverHealthMonitor: ServerHealthMonitor by lazy {
        ServerHealthMonitor(
            networkMonitor = networkMonitor,
            webSocketClient = webSocketClient,
            probe = HealthProbe(healthHttpClient),
            scope = networkScope
        )
    }

    /**
     * Shared across both delivery channels, which is the entire point: the WebSocket
     * frame and its FCM copy describe one earthquake and must raise one alert.
     */
    val alertDedup: AlertDedup = AlertDedup()

    /** FCM token registration + topic subscription, a no-op without Firebase. */
    val pushRegistrar: PushRegistrar by lazy {
        PushRegistrar(context = appContext, apiClient = apiClient, scope = networkScope)
    }

    val webSocketClient: QuakeWebSocketClient by lazy {
        QuakeWebSocketClient(
            client = webSocketHttpClient,
            json = json,
            authRepository = authRepository,
            scope = networkScope
        )
    }

    companion object {
        @Volatile
        private var instance: QuakeNetwork? = null

        /** Returns the process-wide instance, creating it on first use. */
        fun from(context: Context): QuakeNetwork =
            instance ?: synchronized(this) {
                instance ?: QuakeNetwork(context).also { instance = it }
            }

    private const val CONNECT_TIMEOUT_S = 10L
    private const val READ_TIMEOUT_S = 20L
    private const val WRITE_TIMEOUT_S = 10L
    private const val WS_PING_INTERVAL_S = 20L

    private const val HEALTH_CONNECT_TIMEOUT_S = 3L
    private const val HEALTH_READ_TIMEOUT_S = 3L
    private const val HEALTH_CALL_TIMEOUT_S = 5L
    }
}
