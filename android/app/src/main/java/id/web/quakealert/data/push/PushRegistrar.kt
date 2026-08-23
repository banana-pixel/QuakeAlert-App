package id.web.quakealert.data.push

import android.content.Context
import android.util.Log
import com.google.firebase.FirebaseApp
import com.google.firebase.messaging.FirebaseMessaging
import id.web.quakealert.BuildConfig
import id.web.quakealert.data.network.QuakeApiClient
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.launch
import kotlinx.coroutines.tasks.await

/**
 * Everything the app has to tell Firebase and the server before a push can arrive.
 *
 * Two steps, and both are required — either one alone delivers nothing:
 *  - the **registration token** goes to `PUT /api/v1/users/fcm-token`, which is what
 *    lets the server target this device once per-user targeting lands
 *    (server/internal/dispatch/dispatcher.go);
 *  - the **`geo_alert_all` topic subscription**, because that topic is the only FCM
 *    target the server publishes to today, so an unsubscribed device gets nothing no
 *    matter how correctly its token is registered.
 *
 * ## The Firebase guard
 *
 * [isAvailable] is the single place that decides whether Firebase exists in this
 * build. Without an `app/google-services.json` the google-services plugin is never
 * applied, no `FirebaseApp` is initialised, and every call into `FirebaseMessaging`
 * would throw. So each entry point here checks first and returns quietly — the app
 * then runs exactly as it does today, receiving alerts over the WebSocket while it
 * is open. See docs/FIREBASE_SETUP.md.
 */
class PushRegistrar(
    context: Context,
    private val apiClient: QuakeApiClient,
    private val scope: CoroutineScope
) {

    private val appContext: Context = context.applicationContext

    /**
     * Whether a `FirebaseApp` was initialised for this process.
     *
     * Checked through `getApps` rather than a `try/catch` around `getInstance`:
     * absence is the expected state in a checkout without Firebase credentials, and
     * expected states should not be discovered by catching exceptions.
     */
    val isAvailable: Boolean
        get() = runCatching { FirebaseApp.getApps(appContext).isNotEmpty() }.getOrDefault(false)

    /**
     * Registers the current token and subscribes to the alert topic.
     *
     * Fire-and-forget from app start. Failures are logged and dropped: push is the
     * background delivery path, and losing it must not surface as an error on a
     * screen the user opened for something else.
     */
    fun register() {
        if (!isAvailable) {
            Log.i(TAG, "Firebase not configured; alerts will arrive over the WebSocket only")
            return
        }
        scope.launch {
            subscribeToAlertTopic()
            subscribeToUpdatesTopic()
            subscribeToTestAlertsTopic()
            val token = currentToken() ?: return@launch
            uploadToken(token)
        }
    }

    /** Pushes a token to the server. Also the `onNewToken` path. */
    suspend fun uploadToken(token: String) {
        if (token.isBlank()) return
        apiClient.updateFcmToken(token)
            .onFailure { Log.w(TAG, "could not register FCM token", it) }
    }

    private suspend fun currentToken(): String? = runCatching {
        FirebaseMessaging.getInstance().token.await()
    }.onFailure { Log.w(TAG, "could not read FCM token", it) }.getOrNull()

    /**
     * Subscribes to `geo_alert_all`.
     *
     * Kept even after the server gains per-user targeting: the topic remains the
     * fallback for users who have never synced a position, and an idempotent
     * subscription costs one local call.
     */
    private suspend fun subscribeToAlertTopic() {
        runCatching { FirebaseMessaging.getInstance().subscribeToTopic(GEO_TOPIC).await() }
            .onFailure { Log.w(TAG, "could not subscribe to $GEO_TOPIC", it) }
    }

    /**
     * Subscribes to `updates_all`, the operator-announcement topic.
     *
     * A **separate** topic from [GEO_TOPIC], and subscribed separately, so the two can
     * be unsubscribed one at a time: someone tired of announcements can leave this
     * topic without losing the siren, and no attempt to reduce noise can end in
     * earthquake alerts being switched off. Sharing one topic would make "mute the
     * news" and "mute the warnings" the same button
     * (server/internal/dispatch/broadcast.go).
     */
    private suspend fun subscribeToUpdatesTopic() {
        runCatching { FirebaseMessaging.getInstance().subscribeToTopic(UPDATES_TOPIC).await() }
            .onFailure { Log.w(TAG, "could not subscribe to $UPDATES_TOPIC", it) }
    }

    /**
     * Subscribes to `test_alerts` — **debug builds only**.
     *
     * This is the first of the two fences that keep a drill away from a real user. A
     * release install never subscribes, so a drill published by
     * `POST /api/v1/admin/test-alert` has no FCM route to it at all; the second fence
     * is the mapper, which drops an `is_test` frame that arrives by any other means
     * (id.web.quakealert.data.network.mapper.toDomainOrNull). Two independent fences
     * because a drill that reaches the public would train people to ignore the siren,
     * and one misconfiguration must not be enough to cause that.
     *
     * The `BuildConfig.DEBUG` check is here rather than at the call site so there is
     * exactly one place in the app that decides who may receive a drill.
     */
    private suspend fun subscribeToTestAlertsTopic() {
        if (!BuildConfig.DEBUG) return
        runCatching { FirebaseMessaging.getInstance().subscribeToTopic(TEST_ALERTS_TOPIC).await() }
            .onFailure { Log.w(TAG, "could not subscribe to $TEST_ALERTS_TOPIC", it) }
        Log.i(TAG, "debug build subscribed to $TEST_ALERTS_TOPIC")
    }

    private companion object {
        const val TAG = "PushRegistrar"

        /** `dispatch.GeoTopic` in server/internal/dispatch/dispatcher.go. */
        const val GEO_TOPIC = "geo_alert_all"

        /** `dispatch.UpdatesTopic` in server/internal/dispatch/broadcast.go. */
        const val UPDATES_TOPIC = "updates_all"

        /** `dispatch.TestAlertsTopic` in server/internal/dispatch/testalert.go. */
        const val TEST_ALERTS_TOPIC = "test_alerts"
    }
}
