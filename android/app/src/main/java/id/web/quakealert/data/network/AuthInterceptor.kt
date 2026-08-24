package id.web.quakealert.data.network

import id.web.quakealert.data.auth.AuthRepository
import okhttp3.Interceptor
import okhttp3.Response

/**
 * Attaches `Authorization: Bearer <token>` to outgoing requests.
 *
 * The header is read from [AuthRepository]'s in-memory cache, never from disk or
 * the network: interceptors run on OkHttp's own threads, and bootstrapping an
 * identity from here would mean a blocking nested call inside a call. Minting the
 * token is therefore the caller's job — [QuakeApiClient] awaits
 * [AuthRepository.ensureToken] before it issues a request, and this interceptor
 * only stamps what already exists.
 *
 * Three requests are deliberately left bare:
 *  - `POST /api/v1/auth/anonymous`, the public bootstrap, which is what *produces*
 *    the token. Sending a stale one would be noise at best.
 *  - anything that already carries an `Authorization` header, so the WebSocket
 *    handshake (which sets its own) is not rewritten.
 *  - `GET /healthz`, the reachability probe. It is public, and it must stay an
 *    identity-free signal: a probe running inside a token-expiry window would
 *    otherwise tangle "is the server up" with "am I logged in", two questions
 *    with entirely different remedies.
 */
class AuthInterceptor(private val authRepository: AuthRepository) : Interceptor {

    override fun intercept(chain: Interceptor.Chain): Response {
        val request = chain.request()

        val skip = request.header(HEADER_AUTHORIZATION) != null ||
            request.url.encodedPath.endsWith(QuakeApiConfig.PATH_AUTH_ANONYMOUS) ||
            // /healthz is public; the probe must stay an identity-free signal.
            request.url.encodedPath.endsWith(QuakeApiConfig.PATH_HEALTHZ)
        if (skip) return chain.proceed(request)

        val token = authRepository.peekToken() ?: return chain.proceed(request)

        return chain.proceed(
            request.newBuilder()
                .header(HEADER_AUTHORIZATION, "Bearer $token")
                .build()
        )
    }

    private companion object {
        const val HEADER_AUTHORIZATION = "Authorization"
    }
}
