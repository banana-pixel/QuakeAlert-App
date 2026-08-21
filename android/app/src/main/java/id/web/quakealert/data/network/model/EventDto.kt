package id.web.quakealert.data.network.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

/**
 * One confirmed earthquake event from `GET /api/v1/events`.
 *
 * Field names follow the server's `eventDTO` (server/internal/api/api.go) exactly.
 * Two of them are easy to get wrong and worth stating plainly:
 *
 *  - there is **no magnitude** in this system. A surface MEMS network measures
 *    ground acceleration, so severity is expressed as [pga] (gal) plus the derived
 *    [mmi] / [intensityLabel] — never as a Richter/Mw number.
 *  - [latitude]/[longitude] are the weighted **centroid** of the triggering
 *    stations, not an epicentre.
 *
 * @param pga max PGA in gal (cm/s²), the canonical unit.
 * @param mmi Modified Mercalli intensity as a Roman numeral, e.g. "V".
 * @param intensityLabel "light" | "moderate" | "strong" (server-derived from PGA).
 * @param depthKm always null by contract — a 2D centroid carries no depth. Present
 *   so the model survives a future catalogue integration unchanged.
 * @param createdAt RFC3339 UTC, mapped from `started_at`; the feed's DESC sort key.
 * @param resolvedAt absent from the payload while [status] is "HAPPENING", hence
 *   defaulted rather than required.
 */
@Serializable
data class EventDto(
    @SerialName("event_id") val eventId: String,
    @SerialName("status") val status: String,
    @SerialName("pga") val pga: Double,
    @SerialName("mmi") val mmi: String,
    @SerialName("intensity_label") val intensityLabel: String = "",
    @SerialName("latitude") val latitude: Double,
    @SerialName("longitude") val longitude: Double,
    @SerialName("depth_km") val depthKm: Double? = null,
    @SerialName("location_name") val locationName: String,
    @SerialName("triggered_nodes_count") val triggeredNodesCount: Int = 0,
    @SerialName("created_at") val createdAt: String,
    @SerialName("resolved_at") val resolvedAt: String? = null
)

/**
 * Paginated envelope of `GET /api/v1/events`.
 *
 * @param count number of events **on this page**, not the total available — do not
 *   drive an "end of list" decision off it; compare `events.size` against the
 *   requested limit instead.
 * @param rangeKm radius actually applied when the spatial filter was active, null
 *   otherwise.
 */
@Serializable
data class EventsResponseDto(
    @SerialName("limit") val limit: Int = 0,
    @SerialName("offset") val offset: Int = 0,
    @SerialName("count") val count: Int = 0,
    @SerialName("range_km") val rangeKm: Int? = null,
    @SerialName("events") val events: List<EventDto> = emptyList()
)
