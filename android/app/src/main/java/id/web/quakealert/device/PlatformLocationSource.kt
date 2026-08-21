package id.web.quakealert.device

import android.annotation.SuppressLint
import android.content.Context
import android.location.Location
import android.location.LocationManager
import android.util.Log
import androidx.core.content.getSystemService
import androidx.core.location.LocationManagerCompat
import kotlinx.coroutines.TimeoutCancellationException
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withTimeout
import java.util.concurrent.Executor
import kotlin.coroutines.resume

/**
 * [LocationSource] backed by the platform `LocationManager`, for devices without
 * Play Services.
 *
 * Not a lesser copy of [FusedLocationSource] but the reason the app runs at all on
 * a de-Googled or Chinese-market device — and an earthquake warning that only
 * reaches GMS devices is not one this project can ship.
 *
 * `LocationManagerCompat.getCurrentLocation` is used rather than the raw API 30+
 * method so `minSdk = 28` needs no version branch here; androidx implements the
 * older path (a single update request) internally.
 */
class PlatformLocationSource(context: Context) : LocationSource {

    private val appContext: Context = context.applicationContext

    private val manager: LocationManager? = appContext.getSystemService()

    /**
     * Requests one fix from the best enabled provider, falling back to the newest
     * cached fix across providers.
     *
     * Provider choice is explicit because there is no fused provider to arbitrate:
     * network (Wi-Fi/cell) first, since it is fast, indoor-capable and accurate
     * enough for a radius in tens of kilometres, then GPS.
     */
    @SuppressLint("MissingPermission") // Guarded by hasLocationPermission below.
    override suspend fun currentFix(allowHighAccuracy: Boolean): Coordinates? {
        val manager = manager ?: return null
        if (!appContext.hasLocationPermission()) return null
        if (!LocationManagerCompat.isLocationEnabled(manager)) return lastKnown(manager)

        // GPS is offered only when a user is waiting: a satellite lock is the one
        // request here that meaningfully costs battery, and the accuracy it buys is
        // far finer than a radius in tens of kilometres needs. Without it the
        // automatic path still answers from the network provider or the cache.
        val candidates = if (allowHighAccuracy) PROVIDERS else PROVIDERS - LocationManager.GPS_PROVIDER
        val provider = candidates.firstOrNull { manager.isProviderEnabled(it) }
            ?: return lastKnown(manager)

        return try {
            withTimeout(FIX_TIMEOUT_MS) {
                suspendCancellableCoroutine { continuation ->
                    val signal = androidx.core.os.CancellationSignal()
                    continuation.invokeOnCancellation { signal.cancel() }
                    LocationManagerCompat.getCurrentLocation(
                        manager,
                        provider,
                        signal,
                        Executor { it.run() }
                    ) { location -> continuation.resume(location) }
                }
            }?.toCoordinates() ?: lastKnown(manager)
        } catch (timeout: TimeoutCancellationException) {
            Log.w(TAG, "platform location timed out after ${FIX_TIMEOUT_MS}ms", timeout)
            lastKnown(manager)
        } catch (error: Exception) {
            Log.w(TAG, "platform location unavailable", error)
            lastKnown(manager)
        }
    }

    /** Newest cached fix across the providers, or null when none has one. */
    @SuppressLint("MissingPermission") // Only reached from currentFix, which checks.
    private fun lastKnown(manager: LocationManager): Coordinates? =
        PROVIDERS
            .mapNotNull { provider ->
                runCatching { manager.getLastKnownLocation(provider) }.getOrNull()
            }
            .maxByOrNull { it.time }
            ?.toCoordinates()

    private fun Location.toCoordinates() = Coordinates(latitude = latitude, longitude = longitude)

    private companion object {
        const val TAG = "PlatformLocation"
        const val FIX_TIMEOUT_MS = 15_000L

        /** Preference order: fast and indoor-capable first, then satellite. */
        val PROVIDERS = listOf(LocationManager.NETWORK_PROVIDER, LocationManager.GPS_PROVIDER)
    }
}
