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
 *   a hypocentre. Kept so the model stays stable if a catalogue source (e.g. BMKG)
 *   is integrated later. Must never be rendered as "0 km".
 * @param createdAt start of the event (server maps this from `started_at`); the
 *   DESC sort key of the history feed.
 * @param resolvedAt null while [status] is [EventStatus.HAPPENING].
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
    val resolvedAt: Instant?
)
