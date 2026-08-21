package id.web.quakealert.data.network.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

/**
 * Response of `POST /api/v1/auth/anonymous` (HTTP 201), the one public endpoint.
 *
 * There is no refresh flow: when [expiresAt] passes, the client calls the same
 * endpoint again and receives a *new* identity — [userId] and [pseudonym] change
 * with it (docs/CLIENT_SPEC.md §2).
 *
 * @param token JWT HS256 whose only meaningful claims are `sub` (= [userId]),
 *   `iat` and `exp`. Never parse identity out of anything but `sub`.
 * @param tokenType always "Bearer"; kept so the client sends back exactly the
 *   scheme the server issued instead of hardcoding it in two places.
 * @param expiresAt RFC3339 UTC, equal to the `exp` claim.
 */
@Serializable
data class AuthResponseDto(
    @SerialName("token") val token: String,
    @SerialName("token_type") val tokenType: String = "Bearer",
    @SerialName("expires_at") val expiresAt: String? = null,
    @SerialName("user_id") val userId: String,
    @SerialName("pseudonym") val pseudonym: String,
    @SerialName("created_at") val createdAt: String? = null
)

/**
 * Uniform error body every non-2xx REST response carries
 * (`{ "code": ..., "message": ... }`).
 *
 * @param code stable machine code: INVALID_ARGUMENT, UNAUTHENTICATED,
 *   STATION_ALREADY_EXISTS, RATE_LIMITED, INTERNAL.
 */
@Serializable
data class ApiErrorDto(
    @SerialName("code") val code: String,
    @SerialName("message") val message: String
)
