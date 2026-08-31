package id.web.quakealert.domain

import java.time.Instant

/**
 * Lifecycle of a confirmed earthquake event, mirroring `earthquake_events.status`
 * on the server (`HAPPENING` → `RESOLVED`).
 */
enum class EventStatus { HAPPENING, RESOLVED }

/**
 * A confirmed earthquake event as the server models it
 * (`GET /api/v1/events`, `EarthquakeEvent` in contracts/openapi/openapi.yaml).
 *
 * This is the transport-agnostic domain shape: canonical units only (PGA in gal,
 * coordinates WGS84, timestamps as [Instant]), no display formatting. Turning it
 * into the strings the History screen renders is
 * [id.web.quakealert.data.network.mapper.toHistoryItem]'s job, so the same event
 * can back a list row, a detail overlay and a share sheet without three
 * different roundings.
 *
 * Only CONFIRMED events (≥ 3 unique nodes) are persisted server-side, so every
 * instance here has already passed consensus — ADVISORY tremors arrive over the
 * WebSocket as [WsAlertMessage] instead and are never part of this history.
 *
 * @param pgaGal peak ground acceleration in gal (cm/s²). Convert to `g` at render
 *   time only (1 g ≈ 980.665 gal).
 * @param mmi Modified Mercalli intensity as a Roman numeral (e.g. "V").
 * @param intensityLabel server-side severity word ("light" / "moderate" / "strong").
 * @param latitude weighted centroid of the triggering stations — **not** the
 *   epicentre. Never present it as a precise epicentre location.
 * @param longitude see [latitude].
 * @param depthKm always null: a surface MEMS network estimates a 2D centroid, not
 *   a hypocentre. Kept so the model stays stable if a catalogue source is integrated
 *   as an optional post-event reference. Must never be rendered as "0 km".
 * @param createdAt start of the event (server maps this from `started_at`); the
 *   DESC sort key of the history feed.
 * @param resolvedAt null while [status] is [EventStatus.HAPPENING].
 * @param eventState Phase 3 lifecycle state (CONFIRMED, RESOLVED, or CANCELLED).
 *   Null for pre-Phase-3 rows — absence means unknown, not a specific state.
 *   Never UNCONFIRMED in this feed (unconfirmed events are not published here).
 * @param eventRevision monotone revision counter; 0 for pre-Phase-3 rows.
 * @param originTsMs onset of shaking in ms epoch UTC. Distinct from [createdAt].
 *   0 for pre-Phase-3 rows.
 * @param originTsSource provenance of [originTsMs]: "SENSOR" (measured) or
 *   "PUBLISH_BOUND" (upper bound). Empty for pre-Phase-3 rows.
 * @param independentCellCount number of mutually independent contributors (§7.3).
 *   Always >= 2 for events in this feed. 0 for pre-Phase-3 rows.
 */
data class EarthquakeEvent(
    val eventId: String,
    val status: EventStatus,
    val pgaGal: Double,
    val mmi: String,
    val intensityLabel: String,
    val latitude: Double,
    val longitude: Double,
    val depthKm: Double?,
    val locationName: String,
    val triggeredNodesCount: Int,
    val createdAt: Instant,
    val resolvedAt: Instant?,
    // Phase 3 fields — safe defaults so existing call sites compile without change.
    val eventState: EventState? = null,
    val eventRevision: Int = 0,
    val originTsMs: Long = 0L,
    val originTsSource: String = "",
    val independentCellCount: Int = 0,
)
