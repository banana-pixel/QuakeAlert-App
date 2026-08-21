package id.web.quakealert.device

import android.annotation.SuppressLint
import android.content.Context
import android.location.Location
import android.util.Log
import com.google.android.gms.location.LocationServices
import com.google.android.gms.location.Priority
import com.google.android.gms.tasks.CancellationTokenSource
import kotlinx.coroutines.TimeoutCancellationException
import kotlinx.coroutines.tasks.await
import kotlinx.coroutines.withTimeout

/**
 * [LocationSource] backed by Play Services' fused provider.
 *
 * Preferred where it exists: it merges GPS, Wi-Fi and cell signals, and reuses a
 * fix another app on the device just paid for, which matters for a screen the user
 * opens rarely and a background sync that must not drain the battery.
 *
 * `PRIORITY_BALANCED_POWER_ACCURACY` (~100 m) rather than high accuracy — the
 * position feeds a coverage radius measured in tens of kilometres and a
 * city-level reverse-geocode, so a GPS-grade fix would cost power for precision
 * nothing here can use.
 */
class FusedLocationSource(context: Context) : LocationSource {

    private val appContext: Context = context.applicationContext

    private val client = LocationServices.getFusedLocationProviderClient(appContext)

    /**
     * Walks the balanced → cached → high-accuracy ladder and returns the first
     * position any rung produces.
     *
     * Balanced accuracy is tried first because it is the cheap answer that suits
     * this app; the cache turns a slow cold start into a usable, possibly stale
     * answer; and high accuracy is the last resort for a device whose only working
     * provider is GPS. Null means every rung came back empty.
     */
    @SuppressLint("MissingPermission") // Guarded by hasLocationPermission below.
    override suspend fun currentFix(allowHighAccuracy: Boolean): Coordinates? {
        if (!appContext.hasLocationPermission()) return null

        // A ladder rather than one request, because each rung fails for a different
        // reason and the next rung is what covers it: balanced accuracy needs a
        // network provider, the cache needs some app to have asked recently, and
        // high accuracy needs a satellite lock. An emulator fed by `adb emu geo fix`
        // has GPS only, so the first rung there hangs until its deadline and only
        // the third rung answers — the same shape as a real de-Googled handset with
        // no Wi-Fi scan available.
        return fresh(Priority.PRIORITY_BALANCED_POWER_ACCURACY, BALANCED_TIMEOUT_MS)
            ?: cached()
            ?: if (allowHighAccuracy) {
                fresh(Priority.PRIORITY_HIGH_ACCURACY, HIGH_ACCURACY_TIMEOUT_MS)
            } else {
                null
            }
    }

    /** One `getCurrentLocation` attempt at [priority], bounded by [timeoutMs]. */
    @SuppressLint("MissingPermission") // Only reached from currentFix, which checks.
    private suspend fun fresh(priority: Int, timeoutMs: Long): Coordinates? {
        val cancellation = CancellationTokenSource()
        return try {
            withTimeout(timeoutMs) {
                client.getCurrentLocation(priority, cancellation.token).await()
            }?.toCoordinates()
        } catch (timeout: TimeoutCancellationException) {
            // Our own deadline, not the caller's cancellation: report no fix rather
            // than cancelling the coroutine that asked for one. Cancel the request
            // too, so a lock arriving later does not keep the radio awake.
            cancellation.cancel()
            Log.w(TAG, "fused fix (priority=$priority) timed out after ${timeoutMs}ms")
            null
        } catch (error: Exception) {
            cancellation.cancel()
            Log.w(TAG, "fused fix (priority=$priority) unavailable", error)
            null
        }
    }

    /**
     * The provider's cached fix — whatever any app on the device last obtained.
     *
     * Kept on its own short deadline: it is a local lookup that should answer
     * immediately, so waiting the full fresh-fix budget for it would only delay the
     * high-accuracy attempt behind it.
     */
    @SuppressLint("MissingPermission") // Only reached from currentFix, which checks.
    private suspend fun cached(): Coordinates? = try {
        withTimeout(CACHE_TIMEOUT_MS) { client.lastLocation.await() }?.toCoordinates()
    } catch (error: Exception) {
        Log.w(TAG, "no cached fused fix", error)
        null
    }

    private fun Location.toCoordinates() = Coordinates(latitude = latitude, longitude = longitude)

    private companion object {
        const val TAG = "FusedLocationSource"

        /**
         * Per-rung deadlines. The fused provider has no timeout of its own for a
         * request without a duration, and every caller here sits behind a
         * user-visible action ("Sync Now", finishing onboarding) — so the worst case
         * is the sum, kept near 20 s in total rather than 15 s per attempt.
         */
        const val BALANCED_TIMEOUT_MS = 8_000L
        const val CACHE_TIMEOUT_MS = 2_000L
        const val HIGH_ACCURACY_TIMEOUT_MS = 10_000L
    }
}
