package id.web.quakealert.data.network

import okhttp3.OkHttpClient
import okhttp3.Request

/**
 * One reachability question, asked with one request.
 *
 * Deliberately tiny. Everything decision-shaped about the answer lives in
 * [healthOutcomeOf] (pure, unit-tested); this class only owns the HTTP mechanics —
 * build the request, await it, hand over `(code, body)` and never throw except on
 * cancellation, matching [QuakeApiClient]'s `guarded` contract so a cancelled
 * monitor never masquerades as a failed probe.
 */
class HealthProbe(private val client: OkHttpClient) {

    /**
     * Asks `GET /healthz` once.
     *
     * Any thrown [okhttp3.IOException] becomes [ProbeOutcome.FAILED]: for the badge,
     * "the server did not answer" has exactly one meaning regardless of whether the
     * TCP connect timed out or DNS failed.
     */
    suspend fun probe(): ProbeOutcome {
        val request = Request.Builder().url(QuakeApiConfig.url(QuakeApiConfig.PATH_HEALTHZ)).build()
        return try {
            client.newCall(request).awaitResponse { response ->
                healthOutcomeOf(response.code, response.body?.string())
            }
        } catch (e: kotlinx.coroutines.CancellationException) {
            throw e
        } catch (_: Exception) {
            ProbeOutcome.FAILED
        }
    }
}
