package id.web.quakealert.data.users

import android.content.Context
import android.util.Log
import id.web.quakealert.data.AppSettingsRepository
import id.web.quakealert.data.local.SessionStore
import id.web.quakealert.data.network.QuakeApiClient
import id.web.quakealert.device.ReverseGeocoder
import id.web.quakealert.device.hasLocationPermission
import id.web.quakealert.device.locationSource
import id.web.quakealert.domain.UserLocation
import id.web.quakealert.domain.haversineKm

/**
 * What a position sync did, for the caller that wants to say so on screen.
 *
 * A sealed result rather than `Result<Unit>` because the three non-failure
 * outcomes are all "nothing went wrong" but read differently to the user: a fresh
 * position, an unchanged one, or a device that cannot give one at all.
 */
sealed interface LocationSyncResult {

    /** The server accepted a new position. */
    data class Updated(val location: UserLocation) : LocationSyncResult

    /** The device has not moved far enough to be worth an upload. */
    data class Unchanged(val location: UserLocation) : LocationSyncResult

    /** Neither fine nor coarse location is granted. */
    data object PermissionDenied : LocationSyncResult

    /** Permission is granted but no provider produced a fix in time. */
    data object NoFix : LocationSyncResult

    /** The upload itself failed; [message] is already user-facing. */
    data class Failed(val message: String) : LocationSyncResult
}

/**
 * Acquires the device position and pushes it to `PUT /api/v1/users/location`.
 *
 * This is the piece the rest of the app waits on: without a server-stored
 * position `GET /api/v1/sensors` has no radius to work from and returns an empty
 * station list, the History "NEAR" filter has nothing to filter against, and the
 * Haversine gate in front of the siren has no origin.
 *
 * Called from onboarding (once permission is granted), from Settings → Sync Now,
 * and on app start when auto-sync is on.
 */
class UserLocationRepository(
    context: Context,
    private val apiClient: QuakeApiClient,
    private val sessionStore: SessionStore,
    private val settings: AppSettingsRepository,
    private val now: () -> Long = System::currentTimeMillis
) {

    private val appContext: Context = context.applicationContext

    private val geocoder = ReverseGeocoder(appContext)

    /**
     * Reads the position and uploads it when it has meaningfully changed.
     *
     * @param force uploads even when the device has barely moved. Set by "Sync Now",
     *   where the user asked for a round trip and silence would look broken; left
     *   false by the automatic paths, which must not spend a request per launch.
     */
    suspend fun sync(force: Boolean = false): LocationSyncResult {
        if (!appContext.hasLocationPermission()) return LocationSyncResult.PermissionDenied

        // Resolved per call, not cached: Play Services can be updated or disabled
        // while the process lives.
        val fix = locationSource(appContext).currentFix()
            ?: return LocationSyncResult.NoFix

        val stored = sessionStore.readUserLocation()
        val movedKm = stored?.let { haversineKm(it.latitude, it.longitude, fix.latitude, fix.longitude) }
        val nearby = movedKm != null && movedKm < MIN_MOVE_KM

        if (!force && stored != null && nearby) {
            // The position on the server is still correct, so the sync did succeed —
            // record the timestamp even though no request went out.
            settings.setLastSyncAtMs(now())
            return LocationSyncResult.Unchanged(stored)
        }

        // `PUT /users/location` replaces: omitting the label clears whatever the
        // server held. So a failed lookup falls back to the label already stored for
        // this same spot rather than sending null and wiping it.
        val label = geocoder.label(fix) ?: stored?.locationName?.takeIf { nearby }

        return apiClient.updateLocation(
            latitude = fix.latitude,
            longitude = fix.longitude,
            locationName = label
        ).fold(
            onSuccess = {
                // QuakeApiClient caches the accepted position in SessionStore itself.
                settings.setLastSyncAtMs(now())
                LocationSyncResult.Updated(
                    UserLocation(fix.latitude, fix.longitude, label)
                )
            },
            onFailure = { error ->
                Log.w(TAG, "location sync failed", error)
                LocationSyncResult.Failed(
                    error.message ?: "Could not update your location"
                )
            }
        )
    }

    /**
     * Syncs only when the stored position is missing or older than [STALE_AFTER_MS],
     * and only when the user left auto-sync on. The app-start path.
     */
    suspend fun syncIfStale(): LocationSyncResult? {
        if (!settings.readAutoSyncLocation()) return null
        val lastSync = settings.readLastSyncAtMs()
        val stale = lastSync == null || now() - lastSync >= STALE_AFTER_MS
        if (!stale && sessionStore.readUserLocation() != null) return null
        return sync()
    }

    /** The last position the server accepted, or null before the first sync. */
    suspend fun storedLocation(): UserLocation? = sessionStore.readUserLocation()

    private companion object {
        const val TAG = "UserLocationRepo"

        /**
         * Below this the upload is skipped (docs/CLIENT_SPEC.md §4.2): the coverage
         * radius is tens of kilometres wide, so a sub-kilometre move cannot change
         * which alerts reach this user.
         */
        const val MIN_MOVE_KM = 1.0

        /** Six hours — a launch after a flight resyncs, a launch after lunch does not. */
        const val STALE_AFTER_MS = 6L * 60 * 60 * 1000
    }
}
