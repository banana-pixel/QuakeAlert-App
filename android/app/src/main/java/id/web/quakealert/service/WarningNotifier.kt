package id.web.quakealert.service

import android.Manifest
import android.annotation.SuppressLint
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.pm.PackageManager
import android.os.Build
import android.util.Log
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import androidx.core.content.ContextCompat
import id.web.quakealert.R
import id.web.quakealert.data.AppSettingsRepository
import id.web.quakealert.data.network.mapper.intensityValueLabel
import id.web.quakealert.domain.AlertDecision
import id.web.quakealert.domain.WsAlertMessage
import id.web.quakealert.ui.warning.WarningActivity
import kotlin.math.roundToInt

/**
 * Posts the emergency notification that wakes the device, and hands it the
 * full-screen intent to [WarningActivity].
 *
 * Separate from [id.web.quakealert.ui.onboarding.TestAlertNotifier] and on its own
 * channel: the test alert is a dismissible demonstration, this one is insistent,
 * and a user who mutes the channel carrying "your test worked" must not thereby
 * mute real earthquake warnings.
 *
 * The caller applies [id.web.quakealert.domain.AlertGate] before reaching here —
 * this class only reports the [AlertDecision] it was given.
 */
object WarningNotifier {

    const val CHANNEL_ID = "quakealert_emergency_alerts"
    private const val NOTIFICATION_ID = 4301
    private const val TAG = "WarningNotifier"

    /**
     * The event_id of the notification currently posted, or blank when nothing is shown.
     *
     * Used by [clear] to guard against a stand-down for Event B clearing the notification
     * that was posted for Event A. Volatile so writes from any thread are immediately
     * visible to callers on other threads (same guarantee as AlertDedup's revision field).
     *
     * Blank is the correct sentinel: the server guarantees event_id is a non-blank UUID
     * for every Phase-3 frame, and pre-Phase-3 frames that carry no id arrive as blank
     * strings — the legacy behaviour (unconditional clear) is preserved when either side
     * is blank.
     */
    @Volatile
    private var activeEventId: String = ""

    /**
     * Returns the event_id of the notification currently posted by this object.
     * Blank when no notification is active. Used by tests and by [WarningActivity]'s
     * stand-down observer to compare without exposing the mutable field.
     */
    fun activeNotificationEventId(): String = activeEventId

    /** Registers the emergency channel. Safe to call repeatedly. */
    fun ensureChannel(context: Context) {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return
        val manager = context.getSystemService(NotificationManager::class.java) ?: return
        if (manager.getNotificationChannel(CHANNEL_ID) != null) return

        val channel = NotificationChannel(
            CHANNEL_ID,
            "Earthquake Emergency Alerts",
            // IMPORTANCE_HIGH is the minimum a full-screen intent is honoured at.
            NotificationManager.IMPORTANCE_HIGH
        ).apply {
            description = "Life-safety warnings for earthquakes near you."
            enableVibration(true)
            enableLights(true)
            // The siren plays through AlertSiren on the alarm stream instead, so the
            // channel stays silent — two tones at once is noise, not urgency.
            setSound(null, null)
            lockscreenVisibility = NotificationCompat.VISIBILITY_PUBLIC
        }
        manager.createNotificationChannel(channel)
    }

    /**
     * Posts the alert.
     *
     * The full-screen intent is what turns a notification into a warning: it launches
     * [WarningActivity] over the lock screen without the user touching anything.
     * From API 34 that needs `canUseFullScreenIntent()` to be true — the permission is
     * granted by default only to calling and alarm apps — so the heads-up notification
     * is the documented fallback rather than an afterthought.
     *
     * Returns false when nothing could be posted (no `POST_NOTIFICATIONS` grant),
     * so the caller can log a delivery that the user will never see.
     */
    // canPost() below is exactly the checkSelfPermission call lint asks for; it cannot
    // see through the helper, and letting the post throw instead would drop an alert.
    @SuppressLint("MissingPermission")
    fun notify(context: Context, message: WsAlertMessage, decision: AlertDecision): Boolean {
        ensureChannel(context)
        if (!canPost(context)) {
            Log.w(TAG, "POST_NOTIFICATIONS not granted; alert cannot be shown")
            return false
        }

        val distanceKm = decision.distanceKm?.roundToInt()
        val fullScreen = PendingIntent.getActivity(
            context,
            NOTIFICATION_ID,
            WarningActivity.intent(
                context = context,
                eventId = message.eventId,
                intensityValue = message.intensityValueLabel(),
                locationName = message.locationName,
                distanceKm = distanceKm,
                isTest = message.isTest
            ),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )

        val builder = NotificationCompat.Builder(context, CHANNEL_ID)
            .setSmallIcon(R.drawable.ic_alert_triangle)
            // A drill says so in the shade as well as on the screen. Only ever
            // reachable on a debug build (the mapper drops an is_test frame
            // otherwise), so this branch cannot change what a real user is told.
            .setContentTitle(if (message.isTest) "TEST - earthquake drill" else "Earthquake detected")
            .setContentText(bodyText(message, distanceKm))
            .setPriority(NotificationCompat.PRIORITY_MAX)
            .setCategory(NotificationCompat.CATEGORY_ALARM)
            .setVisibility(NotificationCompat.VISIBILITY_PUBLIC)
            // Ongoing and non-cancelling: the user stands the alert down by acting on
            // it, not by swiping it away.
            .setOngoing(true)
            .setAutoCancel(false)
            .setContentIntent(fullScreen)

        if (canUseFullScreen(context)) {
            builder.setFullScreenIntent(fullScreen, true)
        } else {
            Log.w(TAG, "full-screen intents not permitted; falling back to heads-up")
        }

        NotificationManagerCompat.from(context).notify(NOTIFICATION_ID, builder.build())
        // Track which event_id is currently displayed so clear() can guard against
        // a stand-down for a different event removing this notification.
        activeEventId = message.eventId
        // Remember what was shown, for the status notification's "Last alert" line. Here
        // and not on arrival: the claim it feeds is that the app has alerted this user,
        // and an alert filtered out by the distance gate or dropped by the OS never did.
        AppSettingsRepository(context).setLastAlert(
            summary = summaryText(message, distanceKm),
            epochMs = System.currentTimeMillis()
        )
        return true
    }

    /**
     * Clears the emergency notification on `EVENT_RESOLVED`, but only when the
     * stand-down belongs to the event currently displayed.
     *
     * @param standDownEventId the event_id carried by the resolved/cancelled frame.
     *   Blank (pre-Phase-3 frames, or callers that do not have an id) always clears
     *   unconditionally, preserving the original behaviour.
     */
    fun clear(context: Context, standDownEventId: String = "") {
        // Guard: if both sides are non-blank and they disagree, the stand-down is for
        // a different event. Do nothing — the active notification stays.
        if (standDownEventId.isNotBlank() && activeEventId.isNotBlank()
            && standDownEventId != activeEventId
        ) {
            Log.d(
                TAG,
                "stand-down for $standDownEventId ignored; active notification is for $activeEventId"
            )
            return
        }
        NotificationManagerCompat.from(context).cancel(NOTIFICATION_ID)
        activeEventId = ""
    }

    /**
     * The one-line record kept for the status notification: what and where, no advice and
     * no distance-unknown caveat. It is read weeks later in a shade, not during shaking.
     */
    private fun summaryText(message: WsAlertMessage, distanceKm: Int?): String {
        val where = message.locationName.takeIf { it.isNotBlank() } ?: "your area"
        // Kept distinguishable weeks later: a "Last alert" line that cannot be told
        // apart from a real one would misrepresent what the app has warned about.
        if (message.isTest) return "Drill (test alert) near $where"
        val proximity = distanceKm?.let { ", $it km away" }.orEmpty()
        return "Intensity ${message.mmi} near $where$proximity"
    }

    private fun bodyText(message: WsAlertMessage, distanceKm: Int?): String {
        val where = message.locationName.takeIf { it.isNotBlank() } ?: "your area"
        // "Distance unknown" rather than a fabricated number — the gate fails open on
        // an unknown position, so this is a real case and not a defensive branch.
        val proximity = distanceKm?.let { "$it km away" } ?: "distance unknown"
        return "Intensity ${message.mmi} near $where ($proximity). Drop, cover, hold on."
    }

    private fun canPost(context: Context): Boolean =
        Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU ||
            ContextCompat.checkSelfPermission(
                context,
                Manifest.permission.POST_NOTIFICATIONS
            ) == PackageManager.PERMISSION_GRANTED

    private fun canUseFullScreen(context: Context): Boolean {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.UPSIDE_DOWN_CAKE) return true
        val manager = context.getSystemService(NotificationManager::class.java) ?: return false
        return runCatching { manager.canUseFullScreenIntent() }.getOrDefault(false)
    }
}
