package id.web.quakealert.device

import android.Manifest
import android.content.Context
import android.content.pm.PackageManager
import android.os.Build
import android.os.PowerManager
import androidx.core.content.ContextCompat
import androidx.core.content.getSystemService

/**
 * The two OS-owned conditions an alert has to pass through, beside
 * [hasLocationPermission].
 *
 * Shared here rather than kept private to a screen because three surfaces now read
 * them — the Settings delivery checklist, the status notification, and the alert
 * toggle's "blocked by system settings" pill — and a second copy of the check is how
 * they come to disagree.
 *
 * Neither is observable: both are changed in system Settings while the app is in the
 * background, with no callback, so every caller re-reads them on resume.
 */

/**
 * Whether the OS currently allows this app to post notifications.
 *
 * True below API 33, where the grant did not exist. Distinct from the user's own
 * alert switch: this says the app *may* post, that one says they *want* it.
 */
fun Context.canPostNotifications(): Boolean =
    Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU ||
        ContextCompat.checkSelfPermission(this, Manifest.permission.POST_NOTIFICATIONS) ==
        PackageManager.PERMISSION_GRANTED

/**
 * Whether the app is exempt from battery optimisation.
 *
 * Relevant to alert delivery, not battery life: under Doze a data-only FCM message
 * can be held back until the next maintenance window, which for an earthquake
 * warning is indistinguishable from never arriving.
 */
fun Context.isBatteryUnrestricted(): Boolean =
    getSystemService<PowerManager>()?.isIgnoringBatteryOptimizations(packageName) ?: false
