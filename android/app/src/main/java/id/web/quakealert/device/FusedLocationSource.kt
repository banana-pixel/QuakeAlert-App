package id.web.quakealert.device

import android.annotation.SuppressLint
import android.content.Context
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
     * Asks for a fresh fix, falling back to the provider's cached one.
     *
     * `getCurrentLocation` returns null when it cannot produce a fix inside its own
     * deadline (all providers disabled, or indoors with no cached signal), so the
     * `lastLocation` fallback is what turns a slow cold start into a usable —
     * possibly stale — answer instead of no answer.
     */
    @SuppressLint("MissingPermission") // Guarded by hasLocationPermission below.
    override suspend fun currentFix(): Coordinates? {
        if (!appContext.hasLocationPermission()) return null

        val cancellation = CancellationTokenSource()
        return try {
            withTimeout(FIX_TIMEOUT_MS) {
                val fresh = client
                    .getCurrentLocation(Priority.PRIORITY_BALANCED_POWER_ACCURACY, cancellation.token)
                    .await()
                (fresh ?: client.lastLocation.await())?.let {
                    Coordinates(latitude = it.latitude, longitude = it.longitude)
                }
            }
        } catch (timeout: TimeoutCancellationException) {
            // Our own deadline, not the caller's cancellation: report no fix rather
            // than cancelling the coroutine that asked for one.
            cancellation.cancel()
            Log.w(TAG, "fused location timed out after ${FIX_TIMEOUT_MS}ms", timeout)
            null
        } catch (error: Exception) {
            cancellation.cancel()
            Log.w(TAG, "fused location unavailable", error)
            null
        }
    }

    private companion object {
        const val TAG = "FusedLocationSource"

        /**
         * Upper bound on how long a position sync may block. The fused provider has
         * no timeout of its own for a request without a duration, and every caller
         * here sits behind a user-visible action ("Sync Now", finishing onboarding).
         */
        const val FIX_TIMEOUT_MS = 15_000L
    }
}
