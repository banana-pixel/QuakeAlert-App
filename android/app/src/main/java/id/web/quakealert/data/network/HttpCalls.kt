package id.web.quakealert.data.network

import id.web.quakealert.data.network.model.ApiErrorDto
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withContext
import kotlinx.serialization.decodeFromString
import kotlinx.serialization.json.Json
import okhttp3.Call
import okhttp3.Callback
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.Response
import java.io.IOException
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException

/** `application/json` media type used for every request body. */
internal val JSON_MEDIA_TYPE = "application/json; charset=utf-8".toMediaType()

/**
 * The JSON codec shared by the REST client and the WebSocket client.
 *
 *  - `ignoreUnknownKeys`: the server may add fields (contract-first, ADR-0004);
 *    a new key must not crash an older client mid-earthquake.
 *  - `explicitNulls = false`: on the way out it omits null `location_name` instead
 *    of sending `null` — required because `PUT /users/location` decodes with
 *    `DisallowUnknownFields` and treats an absent label as "no label". On the way
 *    in it lets an explicit `null` fall back to the property default.
 *  - `encodeDefaults = false`: keeps request bodies to exactly the documented keys.
 */
internal fun quakeJson(): Json = Json {
    ignoreUnknownKeys = true
    explicitNulls = false
    encodeDefaults = false
}

/**
 * Awaits an OkHttp [Call] without blocking a thread, cancelling the underlying
 * HTTP call when the calling coroutine is cancelled — so a ViewModel whose scope
 * dies does not leave a socket open.
 */
internal suspend fun Call.await(): Response = suspendCancellableCoroutine { continuation ->
    continuation.invokeOnCancellation { cancel() }
    enqueue(object : Callback {
        override fun onFailure(call: Call, e: IOException) {
            // A cancelled call reports failure too; resuming then would overwrite
            // the CancellationException with a misleading "Canceled" IOException.
            if (!continuation.isCancelled) continuation.resumeWithException(e)
        }

        override fun onResponse(call: Call, response: Response) {
            continuation.resume(response)
        }
    })
}

/**
 * Awaits [Call] and hands the response to [block] on [Dispatchers.IO], closing it
 * afterwards.
 *
 * The dispatcher is the point of this helper, not a detail. [await] resumes on
 * whatever dispatcher the caller is on, and every caller here is ultimately a
 * `viewModelScope` coroutine, so that is the main thread; `Response.body.string()`
 * then reads the socket, which for a chunked response is a real blocking read and
 * `StrictMode` answers it with `NetworkOnMainThreadException`. Doing the whole
 * response-handling block on IO means a caller cannot reintroduce that by reading
 * the body one line lower down.
 */
internal suspend fun <T> Call.awaitResponse(block: (Response) -> T): T =
    withContext(Dispatchers.IO) { await().use(block) }

/**
 * Parses the uniform `{ code, message }` error body, returning null when the
 * response carried no usable body. Never throws: an unparseable error body must
 * still surface as the underlying HTTP failure rather than as a JSON exception.
 */
internal fun Json.decodeApiError(body: String?): ApiErrorDto? {
    if (body.isNullOrBlank()) return null
    return runCatching { decodeFromString<ApiErrorDto>(body) }.getOrNull()
}
