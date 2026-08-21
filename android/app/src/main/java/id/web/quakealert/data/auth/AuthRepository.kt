package id.web.quakealert.data.auth

import android.util.Log
import id.web.quakealert.data.local.SessionStore
import id.web.quakealert.data.local.StoredSession
import id.web.quakealert.data.network.ApiException
import id.web.quakealert.data.network.QuakeApiConfig
import id.web.quakealert.data.network.await
import id.web.quakealert.data.network.decodeApiError
import id.web.quakealert.data.network.model.AuthResponseDto
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.serialization.decodeFromString
import kotlinx.serialization.json.Json
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import java.time.Instant
import java.util.Base64

/**
 * Owns the app's anonymous identity: the JWT, the `user_id` it encodes and the
 * pseudonym shown in the UI.
 *
 * The flow is the one docs/CLIENT_SPEC.md §2 mandates. On first use the cached
 * token is read from [SessionStore]; if it is missing or its `exp` has passed, the
 * repository bootstraps a fresh identity via the public
 * `POST /api/v1/auth/anonymous` and persists it. There is **no refresh endpoint** —
 * re-bootstrapping mints a *new* `user_id` and pseudonym, which is why the spec
 * insists the client check `exp` locally instead of calling auth on every launch.
 *
 * Concurrency matters here: History, Sensors and the WebSocket all come up at once
 * on a cold start. A [Mutex] serialises them so three simultaneous first requests
 * produce one identity rather than three abandoned ones.
 *
 * @param clientProvider supplies the shared [OkHttpClient] lazily. Passing the
 *   client itself would be circular — the client's
 *   [id.web.quakealert.data.network.AuthInterceptor] needs this repository.
 */
class AuthRepository(
    private val sessionStore: SessionStore,
    private val json: Json,
    private val clientProvider: () -> OkHttpClient
) {

    /**
     * Last known session, mirrored in memory so
     * [id.web.quakealert.data.network.AuthInterceptor] can attach the header
     * without blocking an OkHttp thread on a disk read.
     */
    @Volatile
    private var cachedSession: StoredSession? = null

    private val bootstrapMutex = Mutex()

    /** The current pseudonym for display, or null before the first bootstrap. */
    val pseudonym: Flow<String?> = sessionStore.session.map { it?.pseudonym }

    /** The current `user_id`, or null before the first bootstrap. */
    val userId: Flow<String?> = sessionStore.session.map { it?.userId?.takeIf(String::isNotBlank) }

    /**
     * The cached token without touching disk or network, for the request
     * interceptor. Null until [ensureToken] has run at least once.
     */
    fun peekToken(): String? = cachedSession?.token

    /**
     * Returns a usable bearer token, bootstrapping a new anonymous identity when
     * none is cached or the cached one has expired.
     *
     * @throws ApiException if the bootstrap call is rejected by the server.
     * @throws java.io.IOException if the bootstrap call cannot reach the server.
     */
    suspend fun ensureToken(nowMs: Long = System.currentTimeMillis()): String =
        bootstrapMutex.withLock {
            val existing = cachedSession ?: sessionStore.readSession()?.also { cachedSession = it }
            if (existing != null && !existing.isExpired(nowMs)) {
                return@withLock existing.token
            }
            bootstrap().token
        }

    /**
     * Forgets the identity after the server rejected it, so the next
     * [ensureToken] mints a new one. Called on a 401 from any endpoint.
     */
    suspend fun invalidate() {
        bootstrapMutex.withLock {
            cachedSession = null
            sessionStore.clearSession()
        }
    }

    /**
     * Issues and persists a new anonymous identity. Must be called with
     * [bootstrapMutex] held.
     */
    private suspend fun bootstrap(): StoredSession {
        val request = Request.Builder()
            .url(QuakeApiConfig.url(QuakeApiConfig.PATH_AUTH_ANONYMOUS))
            // The endpoint takes no body, but POST with a zero-length body is what
            // the server's route expects; omitting the body entirely makes OkHttp
            // reject the POST before it leaves the device.
            .post(ByteArray(0).toRequestBody(null, 0, 0))
            .build()

        val body: String
        clientProvider().newCall(request).await().use { response ->
            body = response.body?.string().orEmpty()
            if (!response.isSuccessful) {
                throw ApiException.from(
                    httpCode = response.code,
                    error = json.decodeApiError(body),
                    fallback = "Could not create an anonymous profile (HTTP ${response.code})."
                )
            }
        }

        val dto = json.decodeFromString<AuthResponseDto>(body)
        val session = StoredSession(
            token = dto.token,
            userId = dto.userId,
            pseudonym = dto.pseudonym,
            expiresAtMs = dto.expiresAt.toEpochMillisOrNull() ?: dto.token.jwtExpiryMillis()
        )
        sessionStore.saveSession(session)
        cachedSession = session
        Log.i(TAG, "anonymous identity bootstrapped for user ${dto.userId}")
        return session
    }

    private companion object {
        const val TAG = "AuthRepository"

        /**
         * Treat a token as expired this long before its real `exp`, so a request
         * cannot be issued with a token that dies in flight.
         */
        const val EXPIRY_SKEW_MS = 60_000L

        /**
         * True when the token's expiry is known and within [EXPIRY_SKEW_MS].
         *
         * An unknown expiry counts as valid: the server is the authority, and
         * discarding a possibly-good identity would pointlessly mint a new
         * `user_id` on every launch.
         */
        fun StoredSession.isExpired(nowMs: Long): Boolean {
            val expiry = expiresAtMs ?: return false
            return nowMs >= expiry - EXPIRY_SKEW_MS
        }

        /** Parses an RFC3339 UTC timestamp, tolerating a malformed value. */
        fun String?.toEpochMillisOrNull(): Long? =
            this?.takeIf { it.isNotBlank() }
                ?.let { runCatching { Instant.parse(it).toEpochMilli() }.getOrNull() }

        /**
         * Reads the `exp` claim straight from the JWT payload as a fallback for a
         * missing `expires_at`.
         *
         * The signature is deliberately **not** verified — the client has no
         * business validating a token it only has to carry, and the HMAC secret
         * lives server-side. This is a scheduling hint, nothing more.
         */
        fun String.jwtExpiryMillis(): Long? = runCatching {
            val payload = split('.').getOrNull(1) ?: return null
            val decoded = Base64.getUrlDecoder().decode(payload.padBase64()).decodeToString()
            val exp = Regex("\"exp\"\\s*:\\s*(\\d+)").find(decoded)?.groupValues?.get(1)
            exp?.toLong()?.times(1000L) // `exp` is epoch seconds; the store keeps millis.
        }.getOrNull()

        /** JWT segments are unpadded base64url; [Base64] requires the padding. */
        fun String.padBase64(): String = when (length % 4) {
            2 -> "$this=="
            3 -> "$this="
            else -> this
        }
    }
}
