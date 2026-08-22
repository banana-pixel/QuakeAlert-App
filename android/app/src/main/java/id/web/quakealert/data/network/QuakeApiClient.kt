package id.web.quakealert.data.network

import id.web.quakealert.data.auth.AuthRepository
import id.web.quakealert.data.local.SessionStore
import id.web.quakealert.data.network.mapper.toDomain
import id.web.quakealert.data.network.model.EventsResponseDto
import id.web.quakealert.data.network.model.RerollPseudonymResponseDto
import id.web.quakealert.data.network.model.SensorsResponseDto
import id.web.quakealert.data.network.model.UpdateFcmTokenRequestDto
import id.web.quakealert.data.network.model.UpdateLocationRequestDto
import id.web.quakealert.domain.EarthquakeEvent
import id.web.quakealert.domain.SensorNode
import id.web.quakealert.domain.UserLocation
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.Flow
import kotlinx.serialization.decodeFromString
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json
import okhttp3.HttpUrl
import okhttp3.HttpUrl.Companion.toHttpUrl
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import java.time.Instant
import java.time.format.DateTimeFormatter

/**
 * The REST half of the backend contract: confirmed events, sensor health, and the
 * user's own position.
 *
 * Every method returns a [Result] rather than throwing, because the three screens
 * that call them already own a loading/error state machine and treat a failure as
 * one more UI state. [CancellationException] is the single exception that still
 * propagates — swallowing it into `Result.failure` would show an error banner for a
 * screen the user simply navigated away from.
 *
 * Authentication is handled in two halves. This class awaits
 * [AuthRepository.ensureToken] before issuing a request (a network call, so it
 * cannot live in an interceptor), and [AuthInterceptor] stamps the header on the
 * way out. That split is also what makes the 401 retry cheap: the request object
 * carries no token, so re-issuing it after a re-bootstrap automatically picks up
 * the new one.
 */
class QuakeApiClient(
    private val client: OkHttpClient,
    private val json: Json,
    private val authRepository: AuthRepository,
    private val sessionStore: SessionStore
) {

    /**
     * Last known device position, as accepted by `PUT /api/v1/users/location`.
     *
     * Exposed here because it is an input to the mappers (distance-from-user), and
     * every caller of this client already holds it.
     */
    val userLocation: Flow<UserLocation?> = sessionStore.userLocation

    /** One-shot read of [userLocation], for a mapper call on a background thread. */
    suspend fun currentUserLocation(): UserLocation? = sessionStore.readUserLocation()

    /**
     * `GET /api/v1/events` — confirmed events, newest first.
     *
     * Auth is optional on this endpoint, but a token is still attached: an
     * *invalid* one is a 401 rather than being downgraded to anonymous, so the
     * client must send either a good token or none at all.
     *
     * The spatial filter is **all-or-nothing**: the server activates `ST_DWithin`
     * only when `range_km`, `latitude` and `longitude` arrive together, and answers
     * 400 when one of the three is missing (contracts/openapi/openapi.yaml). The
     * trio is therefore assembled here rather than passed through piecemeal — a
     * caller that knows the radius but has no stored position gets an unfiltered
     * page instead of an error.
     *
     * @param limit page size, clamped to the contract's 1..100 so a UI-supplied
     *   value cannot turn into a 400.
     * @param offset pagination offset, clamped to the documented 0..50 000.
     * The intensity and time filters are evaluated in SQL by the server, not here:
     * narrowing a page after `limit` has been applied leaves a short page, which the
     * infinite scroll reads as "no more data" while the server still holds matches.
     *
     * @param rangeKm optional radius around [center], clamped to 1..2000. Ignored
     *   without a [center].
     * @param center the position to measure from; ignored without a [rangeKm].
     * @param minPgaGal optional intensity floor in gal. Sent as `min_pga` rather than
     *   an MMI numeral because `mmi_scale` is a Roman-numeral string server-side and
     *   only `max_pga` can be compared numerically; use
     *   [id.web.quakealert.domain.SafetyPolicy.minPgaForMmi] to turn the MMI a user
     *   picked into this threshold.
     * @param since optional inclusive lower bound on the event time.
     * @param until optional inclusive upper bound on the event time.
     */
    suspend fun fetchEvents(
        limit: Int = DEFAULT_LIMIT,
        offset: Int = 0,
        rangeKm: Int? = null,
        center: UserLocation? = null,
        minPgaGal: Double? = null,
        since: Instant? = null,
        until: Instant? = null
    ): Result<List<EarthquakeEvent>> =
        guarded {
            val url = eventsUrl(
                limit = limit,
                offset = offset,
                rangeKm = rangeKm,
                center = center,
                minPgaGal = minPgaGal,
                since = since,
                until = until
            )

            val body = perform(
                request = getRequest(url),
                fallback = "Could not load earthquake history"
            )
            json.decodeFromString<EventsResponseDto>(body).events.toDomain()
        }

    /**
     * `GET /api/v1/sensors` — the station list with link health.
     *
     * The radius is measured from the position the *server* holds, not one sent
     * here — which is why the station list comes back empty until
     * [updateLocation] has run at least once.
     *
     * @param rangeKm optional radius filter, clamped to this endpoint's 1..500
     *   (narrower than `/events`, which allows 2000). The server defaults to 50 km
     *   when omitted.
     */
    suspend fun fetchSensors(rangeKm: Int? = null): Result<List<SensorNode>> = guarded {
        val url = sensorsUrl(rangeKm)

        val body = perform(
            request = getRequest(url),
            fallback = "Could not load sensor status"
        )
        json.decodeFromString<SensorsResponseDto>(body).stations.toDomain()
    }

    /**
     * `PUT /api/v1/users/location` — replaces the stored position.
     *
     * Replace semantics are the reason [locationName] is a parameter and not an
     * afterthought: omitting it clears whatever label the server held. The accepted
     * position is cached locally on success so distance read-outs survive a cold
     * start without waiting for a fresh GPS fix.
     *
     * The position is the *only* thing synced. The alert radius is fixed by
     * [id.web.quakealert.domain.SafetyPolicy] and identical on the server, so there
     * is nothing about it left to agree on — which also means these coordinates are
     * the only reason the server knows to wake this device at all.
     */
    suspend fun updateLocation(
        latitude: Double,
        longitude: Double,
        locationName: String? = null
    ): Result<Unit> = guarded {
        val payload = UpdateLocationRequestDto(
            latitude = latitude,
            longitude = longitude,
            locationName = locationName?.takeIf { it.isNotBlank() }
        )
        val request = Request.Builder()
            .url(QuakeApiConfig.url(QuakeApiConfig.PATH_USER_LOCATION))
            .put(json.encodeToString(payload).toRequestBody(JSON_MEDIA_TYPE))
            .build()

        perform(request = request, fallback = "Could not update your location")
        sessionStore.saveUserLocation(
            UserLocation(latitude = latitude, longitude = longitude, locationName = locationName)
        )
    }

    /**
     * `PUT /api/v1/users/fcm-token` — registers this device for push delivery.
     *
     * Called from `onNewToken` and again on every app start: a token can be rotated
     * while the app is not running, and the server keeps only the last one it was
     * told about. Nothing is cached locally — the token's own store is Firebase, and
     * a second identical PUT is cheaper than a wrong one.
     */
    suspend fun updateFcmToken(token: String): Result<Unit> = guarded {
        val request = Request.Builder()
            .url(QuakeApiConfig.url(QuakeApiConfig.PATH_USER_FCM_TOKEN))
            .put(json.encodeToString(UpdateFcmTokenRequestDto(token)).toRequestBody(JSON_MEDIA_TYPE))
            .build()

        perform(request = request, fallback = "Could not register this device for alerts")
        Unit
    }

    /**
     * `POST /api/v1/users/pseudonym/reroll` — asks the server for a new display
     * name, returning the one it assigned.
     *
     * The server owns the name generator, so the new pseudonym can only be learned
     * from the response; it is written straight into [SessionStore] so every screen
     * showing the identity updates without a refetch. A `429` means the once-per-60s
     * cooldown has not elapsed and is surfaced as-is for the UI to explain.
     */
    suspend fun rerollPseudonym(): Result<String> = guarded {
        val request = Request.Builder()
            .url(QuakeApiConfig.url(QuakeApiConfig.PATH_USER_PSEUDONYM_REROLL))
            .post(EMPTY_BODY)
            .build()

        val body = perform(request = request, fallback = "Could not change your pseudonym")
        val pseudonym = json.decodeFromString<RerollPseudonymResponseDto>(body).pseudonym
        if (pseudonym.isNotBlank()) sessionStore.savePseudonym(pseudonym)
        pseudonym
    }

    private fun getRequest(url: HttpUrl): Request = Request.Builder().url(url).get().build()

    /**
     * Runs [block], converting a failure into [Result.failure] while letting
     * coroutine cancellation through untouched.
     */
    private suspend fun <T> guarded(block: suspend () -> T): Result<T> =
        try {
            Result.success(block())
        } catch (cancellation: CancellationException) {
            throw cancellation
        } catch (throwable: Throwable) {
            Result.failure(throwable)
        }

    /**
     * Issues [request] with a valid token, retrying exactly once through a fresh
     * anonymous identity if the server rejects the current one.
     *
     * One retry, not a loop: a second 401 means the token is not the problem, and
     * re-bootstrapping in a loop would mint a new `user_id` per attempt
     * (docs/CLIENT_SPEC.md §6).
     *
     * @return the response body as a string, which may be empty.
     */
    private suspend fun perform(request: Request, fallback: String): String {
        authRepository.ensureToken()
        return try {
            performOnce(request, fallback)
        } catch (unauthenticated: ApiException) {
            if (!unauthenticated.isUnauthenticated) throw unauthenticated
            authRepository.invalidate()
            authRepository.ensureToken()
            performOnce(request, fallback)
        }
    }

    private suspend fun performOnce(request: Request, fallback: String): String =
        client.newCall(request).awaitResponse { response ->
            val body = response.body?.string().orEmpty()
            if (!response.isSuccessful) {
                throw ApiException.from(
                    httpCode = response.code,
                    error = json.decodeApiError(body),
                    fallback = "$fallback (HTTP ${response.code})."
                )
            }
            body
        }

    /**
     * Public rather than private: the query bounds are the contract's, not this
     * class's, and callers that pick a `range_km` (the Sensors roll's "All" pill)
     * need the same ceiling the client clamps to instead of a second copy of 500.
     */
    companion object {
        const val DEFAULT_LIMIT = 20
        const val MIN_LIMIT = 1
        const val MAX_LIMIT = 100

        /** `offset` is capped by the contract; a larger value is a 400. */
        const val MAX_OFFSET = 50_000

        /** `range_km` bounds on `/events` (`/sensors` allows only up to 500). */
        const val MIN_RANGE_KM = 1
        const val MAX_RANGE_KM = 2_000

        /** `/sensors` caps `range_km` at 500 (server/internal/api/api.go). */
        const val MAX_SENSOR_RANGE_KM = 500

        /** `min_pga` ceiling on `/events`; above it the server answers 400. */
        const val MAX_MIN_PGA_GAL = 2_000.0

        /**
         * `since`/`until` are RFC3339 in the contract. [DateTimeFormatter.ISO_INSTANT]
         * always emits UTC with a `Z`, so no device time zone leaks into the query.
         */
        private val RFC3339: DateTimeFormatter = DateTimeFormatter.ISO_INSTANT

        /** The reroll endpoint takes no body, but OkHttp requires one for a POST. */
        private val EMPTY_BODY = ByteArray(0).toRequestBody(JSON_MEDIA_TYPE)

        /**
         * Assembles the `GET /api/v1/events` URL, clamping every value to the
         * contract's bounds.
         *
         * Split out of [fetchEvents] because the all-or-nothing spatial trio is the
         * part most likely to break silently — the server answers 400 when one of
         * `range_km`/`latitude`/`longitude` is missing — and a pure function can be
         * asserted directly, without a socket or a DataStore-backed session.
         */
        internal fun eventsUrl(
            limit: Int = DEFAULT_LIMIT,
            offset: Int = 0,
            rangeKm: Int? = null,
            center: UserLocation? = null,
            minPgaGal: Double? = null,
            since: Instant? = null,
            until: Instant? = null
        ): HttpUrl =
            QuakeApiConfig.url(QuakeApiConfig.PATH_EVENTS).toHttpUrl().newBuilder()
                .addQueryParameter("limit", limit.coerceIn(MIN_LIMIT, MAX_LIMIT).toString())
                .addQueryParameter("offset", offset.coerceIn(0, MAX_OFFSET).toString())
                .apply {
                    // Both or neither: a radius without a centre is a 400, so a caller
                    // that has the radius but no stored fix gets an unfiltered page.
                    if (rangeKm != null && center != null) {
                        addQueryParameter(
                            "range_km",
                            rangeKm.coerceIn(MIN_RANGE_KM, MAX_RANGE_KM).toString()
                        )
                        addQueryParameter("latitude", center.latitude.toString())
                        addQueryParameter("longitude", center.longitude.toString())
                    }

                    // Unset criteria are omitted rather than sent as a neutral value:
                    // `min_pga=0` would be a filter the server has to evaluate, and a
                    // zero-width time range would be a 400.
                    minPgaGal?.let {
                        addQueryParameter(
                            "min_pga",
                            it.coerceIn(0.0, MAX_MIN_PGA_GAL).toString()
                        )
                    }
                    // Swapped bounds are a 400 on the server; drop the pair here so a
                    // filter-sheet slip shows an unfiltered page rather than an error.
                    if (since == null || until == null || !since.isAfter(until)) {
                        since?.let { addQueryParameter("since", RFC3339.format(it)) }
                        until?.let { addQueryParameter("until", RFC3339.format(it)) }
                    }
                }
                .build()

        /**
         * Assembles the `GET /api/v1/sensors` URL. No centre is sent — this endpoint
         * measures from the position the server holds — so only `range_km` appears,
         * clamped to this endpoint's narrower 1..500 ceiling.
         */
        internal fun sensorsUrl(rangeKm: Int? = null): HttpUrl =
            QuakeApiConfig.url(QuakeApiConfig.PATH_SENSORS).toHttpUrl().newBuilder()
                .apply {
                    rangeKm?.let {
                        addQueryParameter(
                            "range_km",
                            it.coerceIn(MIN_RANGE_KM, MAX_SENSOR_RANGE_KM).toString()
                        )
                    }
                }
                .build()
    }
}
