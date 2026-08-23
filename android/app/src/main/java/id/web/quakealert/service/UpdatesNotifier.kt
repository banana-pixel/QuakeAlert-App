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
import id.web.quakealert.domain.OperatorUpdate

/**
 * Posts an operator announcement — and nothing that resembles a warning.
 *
 * Its own channel is the whole point. [WarningNotifier.CHANNEL_ID] is
 * `IMPORTANCE_HIGH` with a full-screen intent and a siren; a notice about
 * maintenance or a drill arriving through that door would teach the user that a
 * QuakeAlert siren is sometimes nothing, which is the one lesson an early-warning
 * app must never teach. `quakealert_updates` is `IMPORTANCE_LOW` instead: it appears
 * in the shade, silently, and the user can mute it without touching alerts.
 *
 * `IMPORTANCE_LOW` rather than [StatusNotifier]'s `IMPORTANCE_MIN`: the status line
 * is a place to look up a fact the user already knows how to find, while an
 * announcement is news they have not seen — it should show in the shade and carry a
 * badge, just never make a sound. Not ongoing either, for the same reason: it is
 * read once and dismissed.
 *
 * There is no distance gate here and no [id.web.quakealert.domain.AlertGate] call.
 * The server decided who to tell, by region, before it sent anything
 * (server/internal/dispatch/broadcast.go), and a notice the user cannot act on is
 * clutter rather than danger — so being told once too often costs nothing that
 * silencing it would not cost more.
 */
object UpdatesNotifier {

    const val CHANNEL_ID = "quakealert_updates"

    /**
     * Base id, offset per announcement so two notices coexist in the shade instead of
     * replacing each other. Announcements are not a state being updated: two of them
     * are two things to read.
     */
    private const val NOTIFICATION_ID_BASE = 4500
    private const val TAG = "UpdatesNotifier"

    /** Registers the updates channel. Safe to call repeatedly. */
    fun ensureChannel(context: Context) {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return
        val manager = context.getSystemService(NotificationManager::class.java) ?: return
        if (manager.getNotificationChannel(CHANNEL_ID) != null) return

        val channel = NotificationChannel(
            CHANNEL_ID,
            "QuakeAlert Updates",
            NotificationManager.IMPORTANCE_LOW
        ).apply {
            description = "Announcements from the QuakeAlert operators. Never earthquake alerts."
            enableVibration(false)
            enableLights(false)
            setSound(null, null)
        }
        manager.createNotificationChannel(channel)
    }

    /**
     * Posts one announcement.
     *
     * @return false when the OS grant is missing, so the caller can log the one case
     *   where nothing was shown.
     */
    // canPost() is the checkSelfPermission call lint asks for; it cannot see through
    // the helper.
    @SuppressLint("MissingPermission")
    fun notify(context: Context, update: OperatorUpdate): Boolean {
        ensureChannel(context)
        if (!canPost(context)) {
            Log.i(TAG, "POST_NOTIFICATIONS not granted; announcement suppressed")
            return false
        }

        val notificationId = NOTIFICATION_ID_BASE + (update.id.hashCode() and 0xFFFF)
        val open = PendingIntent.getActivity(
            context,
            notificationId,
            Intent(context, MainActivity::class.java)
                .setFlags(Intent.FLAG_ACTIVITY_CLEAR_TOP or Intent.FLAG_ACTIVITY_SINGLE_TOP),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )

        val notification = NotificationCompat.Builder(context, CHANNEL_ID)
            // Deliberately not ic_alert_triangle: the shade icon is the first thing
            // read, and the triangle is the app's alert mark.
            .setSmallIcon(R.drawable.ic_info_circle)
            .setContentTitle(update.title.ifBlank { "QuakeAlert update" })
            .setContentText(update.body)
            .setStyle(NotificationCompat.BigTextStyle().bigText(update.body))
            .setPriority(NotificationCompat.PRIORITY_LOW)
            .setCategory(NotificationCompat.CATEGORY_RECOMMENDATION)
            .setVisibility(NotificationCompat.VISIBILITY_PUBLIC)
            .setWhen(update.publishedAt.toEpochMilli())
            .setShowWhen(true)
            .setSilent(true)
            .setAutoCancel(true)
            .setContentIntent(open)
            .setSubText("QuakeAlert")
            .build()

        NotificationManagerCompat.from(context).notify(notificationId, notification)
        return true
    }

    private fun canPost(context: Context): Boolean =
        Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU ||
            ContextCompat.checkSelfPermission(
                context,
                Manifest.permission.POST_NOTIFICATIONS
            ) == PackageManager.PERMISSION_GRANTED
}
