package id.web.quakealert.device

import android.content.Context
import id.web.quakealert.data.network.QuakeNetwork
import id.web.quakealert.domain.AlertGate
import kotlinx.coroutines.flow.first
import id.web.quakealert.domain.AlertDecision
import id.web.quakealert.domain.AlertType
import id.web.quakealert.domain.WsAlertMessage
import id.web.quakealert.domain.standDownCopyFor
import id.web.quakealert.service.WarningNotifier
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.launch

/**
 * App-scoped collector for WebSocket alerts that raises **system notifications**
 * when no activity is visible to show the in-app alert.
 *
 * Why this exists: before this class, an EARTHQUAKE_ALERT arriving while the app was
 * backgrounded-but-alive had two dead ends. The paused [WarningActivity] collected it
 * into ViewModel state nobody could see (and marked it handled in the shared
 * [id.web.quakealert.domain.AlertDedup]), and the FCM copy that arrived seconds later
 * was then suppressed as a duplicate — so the user never heard about the quake at all.
 * MIUI-class devices made this worse by muting the siren of a backgrounded app.
 *
 * The fix keeps one dedup authority but adds a second *delivery* surface:
 *
 *  - When an activity is RESUMED (foreground), nothing changes: the existing
 *    ViewModel/Activity path shows the in-app alert, marks dedup, and this collector
 *    stays silent for that frame.
 *  - When NO activity is RESUMED (backgrounded, screen off, swiped away), the WS path
 *    hands the frame here instead: dedup is marked once (so the later FCM copy is
 *    still suppressed), and the emergency notification is posted — full-screen intent,
 *    HIGH-importance channel — exactly as the FCM path would have.
 *
 * EVENT_RESOLVED cancels the posted notification regardless of foreground state, so
 * an all-clear received while backgrounded clears what the background alert posted.
 *
 * Foreground detection uses [androidx.lifecycle.ProcessLifecycleOwner] rather than
 * activity callbacks: a paused MainActivity with no other resumed activity means the
 * user cannot see any in-app alert, which is precisely when a system notification is
 * required.
 */
object BackgroundAlertBridge {

    private const val TAG = "BackgroundAlertBridge"

    /**
     * Whether any activity is currently at least STARTED — i.e. something on screen
     * can render the in-app alert. Maintained by [attach] via lifecycle events.
     */
    @Volatile
    var foreground: Boolean = true
        private set

    fun onForeground() {
        foreground = true
    }

    fun onBackground() {
        foreground = false
    }

    /**
     * Subscribes the process-lifetime scope to socket alerts for the duration of the
     * app's life. Called once from Application.onCreate. Only frames received while
     * [foreground] is false produce notifications; foreground frames remain wholly
     * owned by the existing ViewModel path.
     */
    fun attach(context: Context, scope: CoroutineScope) {
        val appContext = context.applicationContext
        scope.launch {
            QuakeNetwork.from(appContext).webSocketClient.alerts.collect { message ->
                // Foreground frames are the existing path's job — never double-notify.
                if (foreground) return@collect

                when (message.type) {
                    AlertType.EVENT_RESOLVED -> {
                        // All-clear cancels whatever this bridge posted; harmless if none.
                        // Dedup is marked so the FCM copy stays suppressed too.
                        // Pass the event_id so WarningNotifier.clear() only removes the
                        // notification it actually posted — not one from a different event.
                        QuakeNetwork.from(appContext).alertDedup.markIfNew(message)
                        WarningNotifier.clear(appContext, message.eventId)
                        // A withdrawal and an all-clear arrive as the SAME wire type and
                        // differ only in event_state, so the distinction has to be drawn
                        // here or nowhere. Logged rather than posted: this branch runs
                        // while nothing is on screen, and the user-visible wording lives
                        // on the idle banner they return to (ui.warning.WarningViewModel).
                        val copy = standDownCopyFor(message.eventState)
                        android.util.Log.i(
                            TAG,
                            "background stand-down ${message.eventId}: ${copy.title}"
                        )
                    }

                    AlertType.EARTHQUAKE_ADVISORY -> {
                        // Advisories stay banner-only by design (never wake the device),
                        // matching the FCM path's behaviour. Since server Phase 3 an
                        // advisory does not reach this bridge by FCM at all — it is
                        // WS-only, so this branch only ever sees a socket frame while the
                        // app is backgrounded, and it must stay empty for the same reason
                        // it always did: escalating an unconfirmed 1-2 node tremor to a
                        // full-screen notification is how users learn to dismiss the real
                        // thing.
                    }

                    AlertType.EARTHQUAKE_ALERT -> handleConfirmedAlert(appContext, message)
                }
            }
        }
    }

    private suspend fun handleConfirmedAlert(context: Context, message: WsAlertMessage) {
        val network = QuakeNetwork.from(context)

        if (!message.isRecent()) {
            android.util.Log.i(TAG, "background ws alert older than recent window; not raising")
            return
        }

        // Mark dedup HERE, once, whether or not a notification can ultimately be shown:
        // the FCM copy arriving moments later must be suppressed either way, and posting
        // happens below only after the gate agrees.
        if (!network.alertDedup.markIfNew(message)) {
            android.util.Log.i(TAG, "background ws alert ${message.eventId} already handled")
            return
        }

        val settings = id.web.quakealert.data.AppSettingsRepository(context)
        val enabled = runCatching { settings.notificationsEnabled.first() }.getOrDefault(true)
        if (!enabled) {
            android.util.Log.i(TAG, "user disabled alert notifications; not raising")
            return
        }

        val decision = AlertGate.decide(
            userLocation = network.sessionStore.readUserLocation(),
            centroidLat = message.centroidLat,
            centroidLon = message.centroidLon,
            mmi = message.mmi,
            pgaGal = message.pgaGal
        )

        if (!decision.shouldAlarm) {
            android.util.Log.i(
                TAG,
                "background alert ${message.eventId} is ${decision.distanceKm?.toInt()}km away; outside coverage"
            )
            return
        }

        val posted = WarningNotifier.notify(context, message, decision)
        if (!posted) {
            android.util.Log.w(TAG, "background alert could not be posted (no permission)")
        }
    }

    /** Test seam: expose the gate decision without touching Android framework classes. */
    internal suspend fun decideForTest(
        network: QuakeNetwork,
        message: WsAlertMessage
    ): AlertDecision = AlertGate.decide(
        userLocation = network.sessionStore.readUserLocation(),
        centroidLat = message.centroidLat,
        centroidLon = message.centroidLon,
        mmi = message.mmi,
        pgaGal = message.pgaGal
    )
}
