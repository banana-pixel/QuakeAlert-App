package id.web.quakealert.domain

/**
 * Kind of realtime message pushed over the WebSocket (`AlertMessage.type` in
 * server/internal/dispatch/ws.go).
 *
 * The three variants carry different confidence and therefore different
 * life-safety weight — collapsing them would either under-warn on a confirmed
 * quake or over-warn on a two-node tremor:
 *
 *  - [EARTHQUAKE_ALERT]    ≥ 3 nodes, consensus CONFIRMED. Full alert.
 *  - [EARTHQUAKE_ADVISORY] 1–2 nodes, unconfirmed. Not persisted server-side and
 *    carries an empty `event_id`.
 *  - [EVENT_RESOLVED]      all-clear for a previously alerted event.
 */
enum class AlertType {
    EARTHQUAKE_ALERT,
    EARTHQUAKE_ADVISORY,
    EVENT_RESOLVED
}

/**
 * A realtime alert received over `GET /ws`, in canonical units.
 *
 * Shape matches the FCM data-only payload as well (contracts/fcm/alert_payload.json),
 * so foreground WebSocket delivery and background push can share one parse and one
 * de-duplication key.
 *
 * @param eventId de-duplication key across the WS and FCM channels. Empty for
 *   [AlertType.EARTHQUAKE_ADVISORY], which the server never persists — callers
 *   must not use it as a map key without checking.
 * @param pgaGal peak ground acceleration in gal (cm/s²).
 * @param centroidLat weighted centroid of triggering stations, **not** the
 *   epicentre.
 * @param centroidLon see [centroidLat].
 * @param timestampMs event time in milliseconds since the Unix epoch, UTC.
 */
data class WsAlertMessage(
    val type: AlertType,
    val eventId: String,
    val mmi: String,
    val intensityLabel: String,
    val pgaGal: Double,
    val centroidLat: Double,
    val centroidLon: Double,
    val locationName: String,
    val timestampMs: Long,
    val nodeCount: Int
) {

    /**
     * True when this alert is recent enough to still be actionable, used to keep a
     * replayed or backlogged message from raising a fresh alarm.
     *
     * The WebSocket flow replays its last value to new subscribers so a screen
     * that is re-entered immediately sees the current alert. Without this guard
     * that same replay would resurrect an hours-old quake as an active warning.
     */
    fun isRecent(nowMs: Long = System.currentTimeMillis(), windowMs: Long = RECENT_WINDOW_MS): Boolean {
        val age = nowMs - timestampMs
        // Negative age = clock skew between server and device; treat as current
        // rather than stale, since discarding a live alert is the worse failure.
        return age <= windowMs
    }

    companion object {
        /**
         * How long an alert is treated as active (15 minutes). Chosen to outlast the
         * server's 90 s alert cooldown and a typical aftershock sequence's opening
         * minutes, while still expiring well within one app session.
         */
        const val RECENT_WINDOW_MS: Long = 15 * 60 * 1000L
    }
}
