package id.web.quakealert.data.network.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

/**
 * Realtime alert frame pushed over `GET /ws` (server `dispatch.AlertMessage`).
 *
 * The same shape is used by the MQTT `alerts/earthquake` contract and — as
 * all-string values — by the FCM data-only payload, so one parse covers every
 * delivery channel (docs/CLIENT_SPEC.md §5.2).
 *
 * @param type "EARTHQUAKE_ALERT" | "EARTHQUAKE_ADVISORY" | "EVENT_RESOLVED".
 * @param eventId de-duplication key; empty string on ADVISORY frames, which the
 *   server does not persist.
 * @param pgaGal peak ground acceleration in gal — note the `_gal` suffix here
 *   versus the REST feed's bare `pga`; the two channels name the same unit
 *   differently.
 * @param centroidLat station centroid, not the epicentre.
 * @param timestamp **milliseconds** since the Unix epoch, UTC (not seconds).
 * @param isTest true only on a drill published by `POST /api/v1/admin/test-alert`.
 *   Absent from a real event's payload entirely (the server omits it), so the
 *   default is the safe read: a frame that says nothing is not a drill.
 * @param eventState lifecycle state of the event this frame describes
 *   ("UNCONFIRMED" | "CONFIRMED" | "RESOLVED" | "CANCELLED"), added in server
 *   Phase 3. The wire `type` enum was deliberately NOT extended, so this is the
 *   only field that tells a withdrawal apart from an all-clear — both arrive as
 *   "EVENT_RESOLVED". Empty from a pre-Phase-3 server.
 * @param eventRevision monotonic revision of the event, 1 for its first public
 *   frame. Lets an out-of-order re-delivery be recognised as stale rather than as
 *   news; 0 means the server did not say.
 * @param originTs estimated onset of shaking in **milliseconds** since the Unix
 *   epoch — not the same instant as [timestamp], which is when the frame's
 *   decision was taken. 0 means unknown.
 * @param originTsSource how [originTs] was obtained: "SENSOR" (measured on a
 *   node) or "PUBLISH_BOUND" (an upper bound derived from a legacy v1
 *   observation's publish time, so it may be later than the real onset). Empty
 *   means unknown; no arithmetic here may treat a bound as a measurement.
 * @param independentCellCount how many separated spatial cells contributed
 *   evidence. 0 means the server did not say.
 */
@Serializable
data class WsAlertMessageDto(
    @SerialName("type") val type: String,
    @SerialName("event_id") val eventId: String = "",
    @SerialName("mmi") val mmi: String = "",
    @SerialName("intensity_label") val intensityLabel: String = "",
    @SerialName("pga_gal") val pgaGal: Double = 0.0,
    @SerialName("centroid_lat") val centroidLat: Double = 0.0,
    @SerialName("centroid_lon") val centroidLon: Double = 0.0,
    @SerialName("location_name") val locationName: String = "",
    @SerialName("timestamp") val timestamp: Long = 0L,
    @SerialName("node_count") val nodeCount: Int = 0,
    @SerialName("is_test") val isTest: Boolean = false,
    // Phase 3 lifecycle fields. Defaulted like every field above, which is what
    // makes an older server's frame parse unchanged — the same tolerance
    // `ignoreUnknownKeys = true` gives this client for fields it has never heard
    // of, applied in the other direction.
    @SerialName("event_state") val eventState: String = "",
    @SerialName("event_revision") val eventRevision: Int = 0,
    @SerialName("origin_ts") val originTs: Long = 0L,
    @SerialName("origin_ts_source") val originTsSource: String = "",
    @SerialName("independent_cell_count") val independentCellCount: Int = 0
)
