package id.web.quakealert.data.users

import android.content.Context
import android.util.Log
import id.web.quakealert.data.AppSettingsRepository
import id.web.quakealert.data.local.SessionStore
import id.web.quakealert.data.network.QuakeApiClient
import id.web.quakealert.device.Coordinates
import id.web.quakealert.device.LocationSource
import id.web.quakealert.device.PlaceNamer
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
    private val now: () -> Long = System::currentTimeMillis,
    /**
     * How a position is read. A factory rather than an instance because the choice
     * between Play Services and AOSP has to be re-made per call — Play Services can
     * be updated or disabled while the process lives — and a parameter rather than a
     * direct call so the sync rules below (the 1 km threshold, the label fallback,
     * the forced round trip) can be tested without a device.
     */
    private val locationSources: (Context) -> LocationSource = ::locationSource,
    placeNamer: PlaceNamer? = null
) {

    private val appContext: Context = context.applicationContext

    private val geocoder: PlaceNamer = placeNamer ?: ReverseGeocoder(appContext)

    /**
     * Reads the position and uploads it when it has meaningfully changed.
     *
     * @param force uploads even when the device has barely moved. Set by "Sync Now",
     *   where the user asked for a round trip and silence would look broken; left
     *   false by the automatic paths, which must not spend a request per launch.
     */
    suspend fun sync(force: Boolean = false): LocationSyncResult =
        syncOnce(force).also { Log.i(TAG, "sync(force=$force) -> ${it.logLabel()}") }

    /**
     * The sync itself. Split from [sync] only so every one of the five outcomes is
     * logged from one place: on an emulator or a de-Googled device the difference
     * between "no permission", "no fix" and "the upload was rejected" is the whole
     * diagnosis, and four of the five paths are otherwise silent.
     */
    private suspend fun syncOnce(force: Boolean): LocationSyncResult {
        if (!appContext.hasLocationPermission()) return LocationSyncResult.PermissionDenied

        // Resolved per call, not cached: Play Services can be updated or disabled
        // while the process lives.
        val source = locationSources(appContext)
        Log.i(TAG, "acquiring a fix via ${source.javaClass.simpleName} (force=$force)")
        // `force` doubles as "the user is watching": only then may a source spend a
        // satellite lock on this fix. The automatic app-start path must not pay a
        // 10 s GPS scan every launch in a place where the cheap providers fail.
        val fix = source.currentFix(allowHighAccuracy = force)
            ?: return LocationSyncResult.NoFix

        val stored = sessionStore.readUserLocation()
        val radiusKm = settings.readCoverageRadiusKm()
        val plan = planSync(
            stored = stored,
            fix = fix,
            force = force,
            radiusChanged = radiusKm != settings.readSyncedCoverageRadiusKm()
        )

        if (stored != null && !plan.upload) {
            // The position on the server is still correct, so the sync did succeed —
            // record the timestamp even though no request went out.
            settings.setLastSyncAtMs(now())
            return LocationSyncResult.Unchanged(stored)
        }

        // `PUT /users/location` replaces: omitting the label clears whatever the
        // server held. So a failed lookup falls back to the label already stored for
        // this same spot rather than sending null and wiping it.
        val label = geocoder.label(fix) ?: stored?.locationName?.takeIf { plan.reuseStoredLabel }

        return upload(
            latitude = fix.latitude,
            longitude = fix.longitude,
            label = label,
            radiusKm = radiusKm
        )
    }

    /**
     * Pushes the coverage radius on its own, reusing the position the server
     * already holds.
     *
     * Separate from [sync] because a slider is not a move: acquiring a fix to
     * report a preference change would spend a GPS scan (and, on the forced path, a
     * satellite lock) for a value that has nothing to do with where the device is.
     *
     * @return null when there is nothing to do — the server already has this
     *   radius, or it has no position yet, in which case the first real [sync]
     *   carries the radius with it.
     */
    suspend fun syncCoverageRadius(): LocationSyncResult? {
        val radiusKm = settings.readCoverageRadiusKm()
        if (radiusKm == settings.readSyncedCoverageRadiusKm()) return null
        val stored = sessionStore.readUserLocation() ?: return null

        Log.i(TAG, "pushing coverage radius change ($radiusKm km)")
        return upload(
            latitude = stored.latitude,
            longitude = stored.longitude,
            label = stored.locationName,
            radiusKm = radiusKm
        ).also { Log.i(TAG, "syncCoverageRadius -> ${it.logLabel()}") }
    }

    /**
     * The one `PUT /users/location` in this class, shared by the positional sync and
     * the radius-only push so both record the same three pieces of local state on
     * success: the accepted position (cached by [QuakeApiClient] itself), the sync
     * timestamp, and the radius the server confirmed.
     *
     * The radius recorded is the one the *server* reports as being in effect, not
     * the one that was sent. Those differ if the server ever clamps or rejects a
     * value, and recording the request would then leave the client believing a
     * radius is synced when it is not — silently, and for as long as the user leaves
     * the slider alone.
     */
    private suspend fun upload(
        latitude: Double,
        longitude: Double,
        label: String?,
        radiusKm: Int
    ): LocationSyncResult = apiClient.updateLocation(
        latitude = latitude,
        longitude = longitude,
        locationName = label,
        coverageRadiusKm = radiusKm
    ).fold(
        onSuccess = { effectiveRadiusKm ->
            settings.setLastSyncAtMs(now())
            settings.setSyncedCoverageRadiusKm(
                effectiveRadiusKm.takeIf { it > 0 } ?: radiusKm
            )
            LocationSyncResult.Updated(UserLocation(latitude, longitude, label))
        },
        onFailure = { error ->
            Log.w(TAG, "location sync failed", error)
            LocationSyncResult.Failed(error.message ?: "Could not update your location")
        }
    )

    /**
     * Syncs only when the stored position is missing or older than [STALE_AFTER_MS],
     * and only when the user left auto-sync on. The app-start path.
     */
    suspend fun syncIfStale(): LocationSyncResult? {
        if (!settings.readAutoSyncLocation()) return null
        val lastSync = settings.readLastSyncAtMs()
        val stale = lastSync == null || now() - lastSync >= STALE_AFTER_MS
        if (!stale && sessionStore.readUserLocation() != null) {
            // Not stale, but a radius change may still be waiting: the push from
            // Settings can be lost to a dropped connection or a process death, and
            // an unsynced radius means the server is aiming this device's alerts by
            // the wrong distance until something reports in.
            return syncCoverageRadius()
        }
        return sync()
    }

    /** The last position the server accepted, or null before the first sync. */
    suspend fun storedLocation(): UserLocation? = sessionStore.readUserLocation()

    /**
     * The outcome, with the coordinates removed.
     *
     * A position is the most sensitive thing this app holds, and `Log.i` survives
     * into release builds and bug reports — so the diagnosis stays (which of the
     * five outcomes, and any server-supplied failure text) while the fix itself
     * does not. Whether a fix was obtained is already evident from the outcome.
     */
    private fun LocationSyncResult.logLabel(): String = when (this) {
        is LocationSyncResult.Updated -> "Updated(position redacted)"
        is LocationSyncResult.Unchanged -> "Unchanged(position redacted)"
        LocationSyncResult.PermissionDenied -> "PermissionDenied"
        LocationSyncResult.NoFix -> "NoFix"
        is LocationSyncResult.Failed -> "Failed($message)"
    }

    private companion object {
        const val TAG = "UserLocationRepo"

        /** Six hours — a launch after a flight resyncs, a launch after lunch does not. */
        const val STALE_AFTER_MS = 6L * 60 * 60 * 1000
    }
}

/**
 * Below this the upload is skipped (docs/CLIENT_SPEC.md §4.2): the coverage radius
 * is tens of kilometres wide, so a sub-kilometre move cannot change which alerts
 * reach this user.
 */
internal const val MIN_MOVE_KM = 1.0

/** What [UserLocationRepository.sync] should do with a fix it just obtained. */
internal data class SyncPlan(

    /** Whether the fix is worth a `PUT /users/location`. */
    val upload: Boolean,

    /**
     * Whether the stored `location_name` still describes this fix, and so may stand
     * in for a failed reverse-geocode. False once the device has actually moved:
     * `PUT /users/location` replaces, and labelling a new city with the old name is
     * worse than sending none.
     */
    val reuseStoredLabel: Boolean
)

/**
 * Decides whether a fix is worth uploading. Pure, and separated from [sync] for
 * that reason: the rule is the one piece of judgement in the sync path, and every
 * other part of that path needs a device to exercise.
 */
internal fun planSync(
    stored: UserLocation?,
    fix: Coordinates,
    force: Boolean,
    radiusChanged: Boolean = false,
    minMoveKm: Double = MIN_MOVE_KM
): SyncPlan {
    val movedKm = stored?.let {
        haversineKm(it.latitude, it.longitude, fix.latitude, fix.longitude)
    }
    val nearby = movedKm != null && movedKm < minMoveKm
    // A forced sync uploads regardless: the user pressed a button and expects a
    // round trip. An unforced one uploads unless the server already holds this spot
    // *and* already knows the radius — an unsynced radius is a reason to upload on
    // its own, since the position alone no longer carries everything the server
    // needs to aim this device's alerts.
    return SyncPlan(
        upload = force || stored == null || !nearby || radiusChanged,
        reuseStoredLabel = nearby
    )
}
