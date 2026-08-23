package id.web.quakealert.data.network.mapper

import id.web.quakealert.BuildConfig
import id.web.quakealert.data.network.model.WsAlertMessageDto
import id.web.quakealert.domain.WsAlertMessage

/**
 * FCM data payload → the same [WsAlertMessage] the WebSocket produces.
 *
 * Every value in an FCM `data` map is a string — that is a platform constraint, not
 * a contract choice (contracts/fcm/alert_payload.json) — so the numeric fields are
 * parsed here rather than by the serializer. Converging on [WsAlertMessage] is the
 * point: push and socket then share one state machine, one distance gate and one
 * de-duplication key instead of drifting into two alert paths.
 *
 * Returns null only when `type` is missing or unrecognised, which is the one field
 * with no safe default: it decides between raising an alarm, showing a banner and
 * clearing an alert.
 *
 * @param nowMs used when `timestamp` is missing or unparseable. A malformed
 *   timestamp becomes *now* rather than 0, because epoch-0 would fail
 *   [WsAlertMessage.isRecent] and silently discard what may be a live alert.
 * @param allowTestAlerts forwarded to [toDomainOrNull], which drops a drill frame on
 *   a release build. Threaded through rather than read there twice, so push and
 *   socket cannot end up on different sides of the fence.
 */
fun Map<String, String>.toWsAlertMessageOrNull(
    nowMs: Long = System.currentTimeMillis(),
    allowTestAlerts: Boolean = BuildConfig.DEBUG
): WsAlertMessage? {
    val type = this["type"]?.trim().orEmpty()
    if (type.isEmpty()) return null

    return WsAlertMessageDto(
        type = type,
        // Empty on an advisory, which the server never persists — kept as "" rather
        // than defaulted, so dedup can recognise it as "no key".
        eventId = this["event_id"].orEmpty().trim(),
        mmi = this["mmi"].orEmpty().trim(),
        intensityLabel = this["intensity_label"].orEmpty().trim(),
        pgaGal = this["pga_gal"]?.trim()?.toDoubleOrNull() ?: 0.0,
        centroidLat = this["centroid_lat"]?.trim()?.toDoubleOrNull() ?: 0.0,
        centroidLon = this["centroid_lon"]?.trim()?.toDoubleOrNull() ?: 0.0,
        locationName = this["location_name"].orEmpty().trim(),
        timestamp = this["timestamp"]?.trim()?.toLongOrNull() ?: nowMs,
        // Absent from the push contract entirely; the type already carries the
        // confirmed-versus-advisory distinction that node count would imply.
        nodeCount = this["node_count"]?.trim()?.toIntOrNull() ?: 0,
        // Only the exact string the server sends counts as a drill. Anything else —
        // "1", "TRUE", a typo, an absent key — reads as a real alert, which is the
        // direction to fail in: a real quake mis-read as a drill would be dropped
        // outright on a release build.
        isTest = this["is_test"]?.trim() == "true"
    ).toDomainOrNull(allowTestAlerts = allowTestAlerts)
}
