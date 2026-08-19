package id.web.quakealert.ui.onboarding

import android.app.NotificationChannel
import android.app.NotificationManager
import android.content.Context
import android.os.Build
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import androidx.core.content.ContextCompat
import android.Manifest
import android.content.pm.PackageManager
import id.web.quakealert.R

/**
 * Small helper that owns the "test alert" notification channel and fires a
 * local notification so the user can confirm alerts reach them during
 * onboarding (Figma node 1:426). The test is a plain auto-cancelling alert;
 * real emergency alerts decide their own ongoing/insistent behaviour.
 */
object TestAlertNotifier {

    const val CHANNEL_ID = "quakealert_test_alerts"
    private const val NOTIFICATION_ID = 4201

    /** Registers the alert channel. Safe to call repeatedly. */
    fun ensureChannel(context: Context) {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return
        val manager = context.getSystemService(NotificationManager::class.java)
        if (manager.getNotificationChannel(CHANNEL_ID) != null) return

        val channel = NotificationChannel(
            CHANNEL_ID,
            "Earthquake Test Alerts",
            NotificationManager.IMPORTANCE_HIGH
        ).apply {
            description = "Test notifications used to verify the alert service."
            enableVibration(true)
            enableLights(true)
        }
        manager.createNotificationChannel(channel)
    }

    /**
     * Displays the test notification. Returns false when the runtime
     * POST_NOTIFICATIONS permission has not been granted (API 33+), letting the
     * caller prompt the user instead of silently failing.
     */
    fun showTestAlert(context: Context): Boolean {
        ensureChannel(context)

        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU &&
            ContextCompat.checkSelfPermission(
                context,
                Manifest.permission.POST_NOTIFICATIONS
            ) != PackageManager.PERMISSION_GRANTED
        ) {
            return false
        }

        val notification = NotificationCompat.Builder(context, CHANNEL_ID)
            .setSmallIcon(R.drawable.ic_alert_test)
            .setContentTitle("QuakeAlert Test")
            .setContentText("This is a test alert. The notification service is working!")
            .setPriority(NotificationCompat.PRIORITY_HIGH)
            .setCategory(NotificationCompat.CATEGORY_ALARM)
            .setAutoCancel(true)
            .build()

        NotificationManagerCompat.from(context).notify(NOTIFICATION_ID, notification)
        return true
    }
}
