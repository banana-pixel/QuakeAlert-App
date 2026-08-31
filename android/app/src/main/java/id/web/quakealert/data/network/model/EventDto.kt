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
 *   so the model survives future catalogue integration unchanged.
 * @param createdAt RFC3339 UTC, mapped from `started_at`; the feed's DESC sort key.
 * @param resolvedAt absent from the payload while [status] is "HAPPENING", hence
 *   defaulted rather than required.
 * @param eventState Phase 3 lifecycle state. Absent on pre-Phase-3 rows — null means
 *   "unknown", not a specific named state. Never UNCONFIRMED in this feed.
 * @param eventRevision monotone revision counter for this event; absent on pre-Phase-3
 *   rows, defaults to 0.
 * @param originTs onset of shaking in ms epoch UTC. Distinct from [createdAt] (which
 *   is when the server row was written). Absent on pre-Phase-3 rows, defaults to 0.
 * @param originTsSource provenance of [originTs]: "SENSOR" (measured) or
 *   "PUBLISH_BOUND" (upper bound only). Absent on pre-Phase-3 rows, defaults to "".
 * @param independentCellCount number of mutually independent contributors (§7.3).
 *   Always >= 2 for events in this feed. Absent on pre-Phase-3 rows, defaults to 0.
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
    @SerialName("resolved_at") val resolvedAt: String? = null,
    // Phase 3 fields — all optional so pre-Phase-3 responses decode without error.
    @SerialName("event_state") val eventState: String? = null,
    @SerialName("event_revision") val eventRevision: Int = 0,
    @SerialName("origin_ts") val originTs: Long = 0L,
    @SerialName("origin_ts_source") val originTsSource: String = "",
    @SerialName("independent_cell_count") val independentCellCount: Int = 0,
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
