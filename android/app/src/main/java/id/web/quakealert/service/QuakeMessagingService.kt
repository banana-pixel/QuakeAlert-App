package id.web.quakealert.service

import android.util.Log
import com.google.firebase.messaging.FirebaseMessagingService
import com.google.firebase.messaging.RemoteMessage
import id.web.quakealert.data.AppSettingsRepository
import id.web.quakealert.data.network.QuakeNetwork
import id.web.quakealert.data.network.mapper.toWsAlertMessageOrNull
import id.web.quakealert.domain.AlertGate
import id.web.quakealert.domain.AlertType
import id.web.quakealert.domain.WsAlertMessage
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch

/**
 * Background delivery path for earthquake alerts.
 *
 * This is the part that makes the app an early-warning system rather than a
 * dashboard: the WebSocket only carries an alert while the app is open, and an
 * earthquake does not wait for the user to open an app. The payload is data-only by
 * contract (contracts/fcm/alert_payload.json) precisely so this service is invoked
 * even when the process was killed — a `notification` block would be handled by the
 * system tray instead and never reach this code.
 *
 * The sequence is fixed and every step matters:
 *  1. parse the all-string data map into the same [WsAlertMessage] the socket
 *     produces ([toWsAlertMessageOrNull]);
 *  2. drop it if the shared [id.web.quakealert.domain.AlertDedup] has already acted
 *     on it, so a socket frame and its push copy raise one alert, not two;
 *  3. **apply [AlertGate]** — mandatory, per .clinerules/20 rule 2. The server's FCM
 *     target is a nationwide topic, so without this every device in the country
 *     sounds a siren for every tremor;
 *  4. post the full-screen notification.
 *
 * Registered in the manifest but only ever instantiated when Firebase initialised,
 * which requires an `app/google-services.json`. Without one this class is dead code
 * and the app runs WebSocket-only (docs/FIREBASE_SETUP.md).
 */
class QuakeMessagingService : FirebaseMessagingService() {

    private val network by lazy { QuakeNetwork.from(applicationContext) }

    private val settings by lazy { AppSettingsRepository(applicationContext) }

    /**
     * A rotated token, delivered whenever Firebase issues a new one — including
     * while the app is not running, which is why registration cannot live only in
     * app start.
     */
    override fun onNewToken(token: String) {
        network.applicationScope.launch {
            network.pushRegistrar.uploadToken(token)
        }
    }

    override fun onMessageReceived(remoteMessage: RemoteMessage) {
        val message = remoteMessage.data.toWsAlertMessageOrNull()
        if (message == null) {
            Log.w(TAG, "push payload had no recognisable type; dropped")
            return
        }

        // An all-clear takes the notification down and needs no gate: the user is
        // being told something ended, and being told that too far away is harmless.
        if (message.type == AlertType.EVENT_RESOLVED) {
            network.alertDedup.markIfNew(message)
            WarningNotifier.clear(applicationContext)
            return
        }

        // Advisories are 1–2 unconfirmed nodes. They never wake the device — the app
        // shows them as a banner when it is open. Escalating them here would train
        // users to dismiss the real thing.
        if (message.type == AlertType.EARTHQUAKE_ADVISORY) return

        if (!message.isRecent()) {
            Log.i(TAG, "push alert older than the recent window; not raising")
            return
        }

        if (!network.alertDedup.markIfNew(message)) {
            Log.i(TAG, "push alert ${message.eventId} already handled; dropped")
            return
        }

        network.applicationScope.launch {
            if (!settings.notificationsEnabledOrDefault()) {
                Log.i(TAG, "user disabled alert notifications; not raising")
                return@launch
            }

            val decision = AlertGate.decide(
                userLocation = network.sessionStore.readUserLocation(),
                centroidLat = message.centroidLat,
                centroidLon = message.centroidLon,
                mmi = message.mmi,
                pgaGal = message.pgaGal
            )

            if (!decision.shouldAlarm) {
                Log.i(
                    TAG,
                    "alert ${message.eventId} is ${decision.distanceKm?.toInt()}km away; outside coverage"
                )
                return@launch
            }

            WarningNotifier.notify(applicationContext, message, decision)
        }
    }

    private suspend fun AppSettingsRepository.notificationsEnabledOrDefault(): Boolean =
        runCatching { notificationsEnabled.first() }.getOrDefault(true)

    private companion object {
        const val TAG = "QuakeMessaging"
    }
}
