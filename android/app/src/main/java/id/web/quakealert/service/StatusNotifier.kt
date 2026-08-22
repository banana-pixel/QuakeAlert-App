package id.web.quakealert.service

import android.Manifest
import android.annotation.SuppressLint
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.util.Log
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import androidx.core.content.ContextCompat
import id.web.quakealert.MainActivity
import id.web.quakealert.R
import id.web.quakealert.domain.ProtectionStatus

/**
 * Posts the quiet, ongoing notification that answers "is QuakeAlert actually
 * watching?" — the question a user of an early-warning app asks after a week in
 * which nothing has happened.
 *
 * Opt-in and off by default (`AppSettingsRepository.statusNotification`): an ongoing
 * notification nobody asked for is clutter.
 *
 * **Not a foreground service, deliberately.** Keeping the socket open in one would
 * duplicate a delivery path the platform already sanctions: high-priority FCM
 * messages are exempt from Doze and arrive when the process is dead, which a held
 * socket improves on only in latency. At `targetSdk 36` the only
 * `foregroundServiceType` that would cover "keep a socket open" is `specialUse`,
 * which Google reviews case by case and rejects where a supported alternative
 * exists. So this posts a notification and nothing more — no service, no
 * `FOREGROUND_SERVICE` permission, no boot receiver. See docs/CLIENT_SPEC.md.
 *
 * It therefore reports only facts the app already holds, through
 * [ProtectionStatus], and never claims a protection it cannot provide: a revoked
 * notification grant reads as blocked, not as "Protected".
 *
 * Socket health is deliberately absent from the copy. The socket is shared
 * `WhileSubscribed` and closes once the UI goes away
 * ([id.web.quakealert.data.network.QuakeWebSocketClient.connectionState]), so a line
 * bound to it would read "offline" for a backgrounded app whose alerts arrive by
 * push exactly as designed — and subscribing here to keep it truthful would hold the
 * connection open around the clock for a shade line.
 */
object StatusNotifier {

    const val CHANNEL_ID = "quakealert_status"
    private const val NOTIFICATION_ID = 4401
    private const val TAG = "StatusNotifier"

    /**
     * Registers the status channel. Safe to call repeatedly.
     *
     * `IMPORTANCE_MIN`, no sound and no vibration, so it collapses into the shade's
     * silent section and can never compete with [WarningNotifier.CHANNEL_ID]. Its own
     * channel also means muting the status line does not touch real warnings.
     */
    fun ensureChannel(context: Context) {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return
        val manager = context.getSystemService(NotificationManager::class.java) ?: return
        if (manager.getNotificationChannel(CHANNEL_ID) != null) return

        val channel = NotificationChannel(
            CHANNEL_ID,
            "QuakeAlert Status",
            NotificationManager.IMPORTANCE_MIN
        ).apply {
            description = "A quiet, ongoing summary of whether alerts can reach you."
            setShowBadge(false)
            enableVibration(false)
            enableLights(false)
            setSound(null, null)
        }
        manager.createNotificationChannel(channel)
    }

    /**
     * Posts (or updates in place) the status notification.
     *
     * Returns false when the OS grant is missing — the one case where the app cannot
     * say it is blocked, because saying so is itself a notification.
     */
    // canPost() is the checkSelfPermission call lint asks for; it cannot see through
    // the helper.
    @SuppressLint("MissingPermission")
    fun notify(context: Context, status: ProtectionStatus): Boolean {
        ensureChannel(context)
        if (!canPost(context)) {
            Log.i(TAG, "POST_NOTIFICATIONS not granted; status notification suppressed")
            return false
        }

        val open = PendingIntent.getActivity(
            context,
            NOTIFICATION_ID,
            Intent(context, MainActivity::class.java)
                .setFlags(Intent.FLAG_ACTIVITY_CLEAR_TOP or Intent.FLAG_ACTIVITY_SINGLE_TOP),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )

        val body = status.lines.joinToString(separator = "\n")
        val notification = NotificationCompat.Builder(context, CHANNEL_ID)
            .setSmallIcon(R.drawable.ic_status_wave)
            .setContentTitle(status.headline)
            .setContentText(status.lines.first())
            .setStyle(NotificationCompat.BigTextStyle().bigText(body))
            .setPriority(NotificationCompat.PRIORITY_MIN)
            .setCategory(NotificationCompat.CATEGORY_STATUS)
            .setVisibility(NotificationCompat.VISIBILITY_PUBLIC)
            // Ongoing and silent: it is a place to look, never an interruption. No
            // timestamp, because "since when" is not one of the facts it reports and a
            // stale-looking clock would suggest the status itself is stale.
            .setOngoing(true)
            .setShowWhen(false)
            .setSilent(true)
            .setOnlyAlertOnce(true)
            .setContentIntent(open)
            .build()

        NotificationManagerCompat.from(context).notify(NOTIFICATION_ID, notification)
        return true
    }

    /** Removes the status notification — the toggle going off, and nothing else. */
    fun clear(context: Context) {
        NotificationManagerCompat.from(context).cancel(NOTIFICATION_ID)
    }

    private fun canPost(context: Context): Boolean =
        Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU ||
            ContextCompat.checkSelfPermission(
                context,
                Manifest.permission.POST_NOTIFICATIONS
            ) == PackageManager.PERMISSION_GRANTED
}
