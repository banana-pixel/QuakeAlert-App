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
    @SerialName("node_count") val nodeCount: Int = 0
)
