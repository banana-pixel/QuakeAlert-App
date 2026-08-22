package id.web.quakealert.data.users

import android.content.Context
import android.util.Log
import id.web.quakealert.data.AppSettingsRepository
import id.web.quakealert.data.local.SessionStore
import id.web.quakealert.data.network.QuakeApiClient
import id.web.quakealert.device.Coordinates
import id.web.quakealert.device.LocationSource
import id.web.quakealert.device.PlaceNamer
import id.web.quakealert.device.ResolvedPlace
import id.web.quakealert.device.ReverseGeocoder
import id.web.quakealert.device.hasLocationPermission
import id.web.quakealert.device.locationSource
import id.web.quakealert.domain.UserLocation
import id.web.quakealert.domain.haversineKm
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

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

    /**
     * The upload itself failed, carrying the raw [cause] rather than a sentence.
     *
     * Copy is not this layer's job, and the string it used to carry was the server's
     * own operator text: `error.message` reaches the user in Indonesian, or as a
     * socket state. The UI layer classifies it instead, via
     * [id.web.quakealert.ui.common.errorCopy].
     */
    data class Failed(val cause: Throwable) : LocationSyncResult
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
        // Serialised, because the triggers are independent and can overlap: app start,
        // a return to the foreground, onboarding's grant and Settings' "Sync Now" all
        // call in. Two concurrent runs would each read the *pre-sync* stored position,
        // so both would decide the device had moved and both would upload the same fix.
        syncMutex.withLock {
            syncOnce(force).also { Log.i(TAG, "sync(force=$force) -> ${it.logLabel()}") }
        }

    /** One sync at a time, process-wide — this repository is a singleton. */
    private val syncMutex = Mutex()

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
        val plan = planSync(stored = stored, fix = fix, force = force)

        if (stored != null && !plan.upload) {
            // The position on the server is still correct, so the sync did succeed —
            // record the timestamp even though no request went out.
            settings.setLastSyncAtMs(now())
            return LocationSyncResult.Unchanged(stored)
        }

        // `PUT /users/location` replaces: omitting the label clears whatever the
        // server held. So a failed lookup falls back to the label already stored for
        // this same spot rather than sending null and wiping it.
        val place = geocoder.resolve(fix)
        val label = place?.label ?: stored?.locationName?.takeIf { plan.reuseStoredLabel }

        // The country/admin pair has no such fallback: it is sent only when this
        // lookup produced it. Absent, the server keeps the region it already holds
        // (docs/CLIENT_SPEC.md §4.2) — a stale label is a cosmetic wrong, but a
        // guessed region would move the user into someone else's chat channel.
        return upload(
            latitude = fix.latitude,
            longitude = fix.longitude,
            label = label,
            place = place
        )
    }

    /**
     * The one `PUT /users/location` in this class.
     *
     * The position and the place it resolved to are all that travels. The alert
     * radius is fixed by [id.web.quakealert.domain.SafetyPolicy] and identical on
     * the server, so there is no preference left to reconcile — which is why the only thing recorded
     * locally on success is the sync timestamp (the accepted position is cached by
     * [QuakeApiClient] itself).
     */
    private suspend fun upload(
        latitude: Double,
        longitude: Double,
        label: String?,
        place: ResolvedPlace?
    ): LocationSyncResult = apiClient.updateLocation(
        latitude = latitude,
        longitude = longitude,
        locationName = label,
        countryIso = place?.countryCode,
        adminArea = place?.adminArea
    ).fold(
        onSuccess = { regionCode ->
            settings.setLastSyncAtMs(now())
            // The server's answer to "which regional chat room does this position put
            // you in", recorded so Chat can notice that a room it could not offer a
            // moment ago now exists. Never derived on the client: the normalisation
            // that produces the key lives on the server (docs/CHAT_DESIGN.md §3).
            settings.setRegionCode(regionCode)
            LocationSyncResult.Updated(UserLocation(latitude, longitude, label))
        },
        onFailure = { error ->
            Log.w(TAG, "location sync failed", error)
            LocationSyncResult.Failed(error)
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
        if (!stale && sessionStore.readUserLocation() != null) return null
        return sync()
    }

    /** The last position the server accepted, or null before the first sync. */
    suspend fun storedLocation(): UserLocation? = sessionStore.readUserLocation()

    /**
     * The outcome, with the coordinates removed.
     *
     * A position is the most sensitive thing this app holds, and `Log.i` survives
     * into release builds and bug reports — so the diagnosis stays (which of the
     * five outcomes, and the failure's own type) while the fix itself
     * does not. Whether a fix was obtained is already evident from the outcome.
     */
    private fun LocationSyncResult.logLabel(): String = when (this) {
        is LocationSyncResult.Updated -> "Updated(position redacted)"
        is LocationSyncResult.Unchanged -> "Unchanged(position redacted)"
        LocationSyncResult.PermissionDenied -> "PermissionDenied"
        LocationSyncResult.NoFix -> "NoFix"
        is LocationSyncResult.Failed -> "Failed(${cause.javaClass.simpleName})"
    }

    private companion object {
        const val TAG = "UserLocationRepo"

        /** Six hours — a launch after a flight resyncs, a launch after lunch does not. */
        const val STALE_AFTER_MS = 6L * 60 * 60 * 1000
    }
}

/**
 * Below this the upload is skipped (docs/CLIENT_SPEC.md §4.2): the alert radius is
 * 200 km wide, so a sub-kilometre move cannot change which alerts reach this user.
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
    minMoveKm: Double = MIN_MOVE_KM
): SyncPlan {
    val movedKm = stored?.let {
        haversineKm(it.latitude, it.longitude, fix.latitude, fix.longitude)
    }
    val nearby = movedKm != null && movedKm < minMoveKm
    // A forced sync uploads regardless: the user pressed a button and expects a
    // round trip. An unforced one uploads unless the server already holds this spot.
    return SyncPlan(upload = force || stored == null || !nearby, reuseStoredLabel = nearby)
}
