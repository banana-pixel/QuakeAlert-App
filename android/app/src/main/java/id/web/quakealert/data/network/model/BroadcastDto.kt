package id.web.quakealert.data.network.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

/**
 * One announcement from `GET /api/v1/broadcasts`.
 *
 * @param regionCode nullable on the wire, and genuinely null for a national notice
 *   (server/internal/api/admin.go `broadcastDTO`) — not an empty string, so the
 *   default here is null rather than "".
 * @param createdAt RFC3339 UTC, like every other REST timestamp in the contract.
 */
@Serializable
data class BroadcastDto(
    @SerialName("broadcast_id") val broadcastId: String = "",
    @SerialName("title") val title: String = "",
    @SerialName("body") val body: String = "",
    @SerialName("region_code") val regionCode: String? = null,
    @SerialName("created_at") val createdAt: String = ""
)

/** Response of `GET /api/v1/broadcasts` — newest first, national and regional mixed. */
@Serializable
data class BroadcastsResponseDto(
    @SerialName("broadcasts") val broadcasts: List<BroadcastDto> = emptyList()
)

/**
 * The `ADMIN_BROADCAST` frame on the shared `GET /ws` socket
 * (docs/CLIENT_SPEC.md §5.7).
 *
 * Same fields as [BroadcastDto] except the clock: the socket speaks ms epoch while
 * REST answers RFC3339. `region_code` is `omitempty` on this frame rather than
 * nullable, so an absent field and an empty string both mean national.
 */
@Serializable
data class WsBroadcastMessageDto(
    @SerialName("type") val type: String = "",
    @SerialName("broadcast_id") val broadcastId: String = "",
    @SerialName("title") val title: String = "",
    @SerialName("body") val body: String = "",
    @SerialName("region_code") val regionCode: String = "",
    @SerialName("timestamp") val timestamp: Long = 0L
)
