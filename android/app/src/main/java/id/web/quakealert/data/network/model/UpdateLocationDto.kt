package id.web.quakealert.data.network.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

/**
 * Body of `PUT /api/v1/users/location`.
 *
 * Two contract details drive the shape:
 *
 *  - the server decodes with `DisallowUnknownFields`, so this class must carry
 *    *only* the three documented keys — an extra field is a 400, not a warning.
 *    That is why there is no radius here: the alert radius is fixed by
 *    [id.web.quakealert.domain.SafetyPolicy], the server no longer accepts one, and
 *    sending it anyway would fail the whole position sync.
 *  - the endpoint is a true PUT (replace): sending the body without
 *    [locationName] **clears** any previously stored label. The client's Json is
 *    configured with `explicitNulls = false` so a null label is omitted from the
 *    wire, which the server treats the same way — as "no label".
 */
@Serializable
data class UpdateLocationRequestDto(
    @SerialName("latitude") val latitude: Double,
    @SerialName("longitude") val longitude: Double,
    @SerialName("location_name") val locationName: String? = null,
    /**
     * ISO-3166 alpha-2 country of the fix, from the same reverse-geocode as
     * [locationName]. Sent with the position rather than through an endpoint of its
     * own because the region is a fact *about* that position.
     */
    @SerialName("country_iso") val countryIso: String? = null,
    /**
     * Admin-1 area (province/state) of the fix, raw. The server normalises the
     * pair into the regional chat channel key — a client that builds the key
     * itself would ask for a room nobody else is in the moment that normalisation
     * changes.
     *
     * Omitting the pair means "leave the stored region alone", which is why both
     * fields are null rather than empty when the geocode failed: a transient
     * lookup failure must not evict someone from their regional channel.
     */
    @SerialName("admin_area") val adminArea: String? = null
)

/**
 * Response of `PUT /api/v1/users/location` (HTTP 200).
 *
 * Every field defaults, so a server that adds one this build does not know about
 * still parses: the position is already stored by the time the body is read, and
 * failing the sync over an unparseable echo would be the worse outcome.
 */
@Serializable
data class UpdateLocationResponseDto(
    @SerialName("user_id") val userId: String = "",
    @SerialName("latitude") val latitude: Double = 0.0,
    @SerialName("longitude") val longitude: Double = 0.0,
    @SerialName("location_name") val locationName: String? = null,
    /**
     * The regional chat channel that applies after this update, or null when the
     * area could not be normalised — that user has the global channel only.
     */
    @SerialName("region_code") val regionCode: String? = null,
    @SerialName("updated_at") val updatedAt: String = ""
)

/**
 * Request body of `PUT /api/v1/users/fcm-token`
 * (contracts/openapi/openapi.yaml, `UpdateFcmTokenRequest`).
 *
 * The server rejects unknown fields outright (`DisallowUnknownFields`), so this
 * carries exactly the one property the schema names.
 */
@Serializable
data class UpdateFcmTokenRequestDto(
    @SerialName("fcm_token") val fcmToken: String
)

/**
 * Response of `POST /api/v1/users/pseudonym/reroll` (HTTP 200).
 *
 * The endpoint is rate-limited to once per 60 s per user and answers `429` when
 * called sooner, so the client has to treat that status as a cooldown rather than
 * an error worth retrying.
 */
@Serializable
data class RerollPseudonymResponseDto(
    @SerialName("pseudonym") val pseudonym: String = "",
    @SerialName("updated_at") val updatedAt: String = ""
)
