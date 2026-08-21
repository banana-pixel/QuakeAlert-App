package id.web.quakealert.device

import android.content.Context
import android.location.Address
import android.location.Geocoder
import android.os.Build
import android.util.Log
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.TimeoutCancellationException
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeout
import java.util.Locale
import kotlin.coroutines.resume

/**
 * Turns a fix into the human-readable `location_name` the backend stores alongside
 * it (`PUT /api/v1/users/location`, docs/CLIENT_SPEC.md §4.2).
 *
 * The label is cosmetic — it appears on the Settings location pill and in event
 * cards — so every failure path here returns null rather than propagating. A
 * position sync must never be lost because a geocoder backend was unreachable.
 */
/**
 * Turns a fix into a human-readable place name.
 *
 * An interface so callers can be tested without the platform `Geocoder`, which
 * needs a `Context` and a network round trip. [ReverseGeocoder] is the real one.
 */
interface PlaceNamer {

    /** The place name for [coordinates], or null when the lookup fails. */
    suspend fun label(coordinates: Coordinates): String?
}

class ReverseGeocoder(context: Context) : PlaceNamer {

    private val appContext: Context = context.applicationContext

    /**
     * A "City, Admin area, CC" label for [coordinates], or null when unavailable.
     *
     * Composed from parts rather than using the platform's own address line: the
     * server caps the field at 150 characters, and a full postal address (street,
     * number, postcode) is both longer and more precise than a coverage-radius
     * label should be.
     */
    override suspend fun label(coordinates: Coordinates): String? {
        if (!Geocoder.isPresent()) return null

        val geocoder = Geocoder(appContext, Locale.getDefault())
        val address = try {
            withTimeout(LOOKUP_TIMEOUT_MS) {
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
                    geocoder.firstAddressAsync(coordinates)
                } else {
                    geocoder.firstAddressBlocking(coordinates)
                }
            }
        } catch (timeout: TimeoutCancellationException) {
            Log.w(TAG, "reverse geocode timed out after ${LOOKUP_TIMEOUT_MS}ms", timeout)
            null
        } catch (error: Exception) {
            Log.w(TAG, "reverse geocode failed", error)
            null
        }

        return address?.toLabel()
    }

    /**
     * API 33+ callback form. The synchronous overload is deprecated there and can
     * be enforced as a strict-mode violation, so the newer path is not just
     * politeness.
     */
    @androidx.annotation.RequiresApi(Build.VERSION_CODES.TIRAMISU)
    private suspend fun Geocoder.firstAddressAsync(coordinates: Coordinates): Address? =
        suspendCancellableCoroutine { continuation ->
            getFromLocation(
                coordinates.latitude,
                coordinates.longitude,
                MAX_RESULTS,
                object : Geocoder.GeocodeListener {
                    override fun onGeocode(addresses: MutableList<Address>) {
                        continuation.resume(addresses.firstOrNull())
                    }

                    // Overridden so a backend error resolves the coroutine instead of
                    // leaving it suspended until the timeout.
                    override fun onError(errorMessage: String?) {
                        Log.w(TAG, "reverse geocode error: $errorMessage")
                        continuation.resume(null)
                    }
                }
            )
        }

    /** Pre-33 form: a blocking network call, so explicitly off the main thread. */
    @Suppress("DEPRECATION")
    private suspend fun Geocoder.firstAddressBlocking(coordinates: Coordinates): Address? =
        withContext(Dispatchers.IO) {
            getFromLocation(coordinates.latitude, coordinates.longitude, MAX_RESULTS)?.firstOrNull()
        }

    /**
     * "Cimahi, West Java, ID" — locality, admin area, country code, skipping parts
     * the geocoder did not fill in (ocean and desert fixes routinely have none).
     */
    private fun Address.toLabel(): String? = listOfNotNull(
        locality ?: subAdminArea,
        adminArea,
        countryCode
    ).filter { it.isNotBlank() }
        .distinct()
        .joinToString(", ")
        .take(MAX_LABEL_LENGTH)
        .ifBlank { null }

    private companion object {
        const val TAG = "ReverseGeocoder"
        const val MAX_RESULTS = 1
        const val LOOKUP_TIMEOUT_MS = 8_000L

        /** `location_name` is capped at 150 characters by the OpenAPI contract. */
        const val MAX_LABEL_LENGTH = 150
    }
}
