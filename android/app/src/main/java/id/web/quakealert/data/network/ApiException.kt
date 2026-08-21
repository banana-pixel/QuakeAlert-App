package id.web.quakealert.data.network

import id.web.quakealert.data.network.model.ApiErrorDto
import java.io.IOException

/**
 * A non-2xx REST response, carrying the server's uniform `{ code, message }` body
 * when one was present.
 *
 * Extends [IOException] so it travels the same path as transport failures through
 * the existing ViewModel `try`/`catch` state machines, and its [message] is the
 * server's own copy — the History and Sensors screens surface `throwable.message`
 * directly in their error state.
 *
 * @param code machine code from the body (INVALID_ARGUMENT, UNAUTHENTICATED,
 *   RATE_LIMITED, INTERNAL, …), or null when the body was empty or unparseable.
 */
class ApiException(
    val httpCode: Int,
    val code: String? = null,
    message: String
) : IOException(message) {

    /**
     * True when the server rejected our credentials. The API client re-bootstraps
     * the anonymous identity once on this, per docs/CLIENT_SPEC.md §6.
     */
    val isUnauthenticated: Boolean get() = httpCode == 401

    companion object {
        fun from(httpCode: Int, error: ApiErrorDto?, fallback: String): ApiException =
            ApiException(
                httpCode = httpCode,
                code = error?.code,
                message = error?.message?.takeIf { it.isNotBlank() } ?: fallback
            )
    }
}
