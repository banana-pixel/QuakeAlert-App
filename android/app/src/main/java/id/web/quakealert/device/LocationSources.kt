package id.web.quakealert.device

import android.Manifest
import android.content.Context
import android.content.pm.PackageManager
import androidx.core.content.ContextCompat
import com.google.android.gms.common.ConnectionResult
import com.google.android.gms.common.GoogleApiAvailability

/**
 * The permission pair the app requests together, and checks together.
 *
 * Onboarding asks for both in one prompt; the system may grant only coarse, which
 * is why every check here is an `any` rather than an `all`.
 */
val LOCATION_PERMISSIONS: Array<String> = arrayOf(
    Manifest.permission.ACCESS_FINE_LOCATION,
    Manifest.permission.ACCESS_COARSE_LOCATION
)

/**
 * Whether the app may read the device position at all.
 *
 * Coarse consent counts: the coverage radius is tens of kilometres wide, so a
 * city-block-accurate position serves every use here, and treating coarse-only as
 * "no permission" would silently disable alerts for a user who granted exactly
 * what the feature needs.
 */
fun Context.hasLocationPermission(): Boolean =
    LOCATION_PERMISSIONS.any {
        ContextCompat.checkSelfPermission(this, it) == PackageManager.PERMISSION_GRANTED
    }

/**
 * The best [LocationSource] this device can offer.
 *
 * Resolved per call rather than cached once: Play Services can be updated,
 * disabled or repaired while the process lives, and the availability check is a
 * cheap local lookup. Any throw from the check itself counts as "not available" —
 * a broken Play Services install must not take the position sync down with it,
 * since the AOSP provider is always there.
 */
fun locationSource(context: Context): LocationSource =
    if (hasPlayServices(context)) {
        FusedLocationSource(context)
    } else {
        PlatformLocationSource(context)
    }

private fun hasPlayServices(context: Context): Boolean = runCatching {
    GoogleApiAvailability.getInstance().isGooglePlayServicesAvailable(context) ==
        ConnectionResult.SUCCESS
}.getOrDefault(false)
