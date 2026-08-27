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
 * Lifecycle state of the event a frame describes (server `event_state`, Phase 3).
 *
 * A SEPARATE enum from [AlertType], and that separation is the whole design: the
 * wire `type` enum is frozen at its three values because an installed client drops
 * a frame whose `type` it does not recognise
 * (id.web.quakealert.data.network.mapper.toDomainOrNull returns null), which for a
 * withdrawal would mean leaving a full-screen alert up for an event the server has
 * already retracted. State travels in an additive field instead, so an un-updated
 * install loses the explanation and never the protection.
 *
 * `when` exhaustiveness over this enum is a compile-time property of *this* build,
 * so a state added later cannot break an installed one — only the next build has to
 * handle it. For that to hold, an unrecognised value must map to null (absent)
 * rather than drop the frame; see the mapper.
 *
 *  - [UNCONFIRMED] 1–2 nodes. Real shaking as far as the network knows, but not
 *    confirmed by separated stations. Arrives as [AlertType.EARTHQUAKE_ADVISORY].
 *  - [CONFIRMED]   ≥ 3 nodes across ≥ 2 separated cells. Arrives as
 *    [AlertType.EARTHQUAKE_ALERT].
 *  - [RESOLVED]    the event ended without new evidence.
 *  - [CANCELLED]   the report was WITHDRAWN — its evidence was invalidated or an
 *    operator retracted it. Not the same claim as [RESOLVED], and the copy must not
 *    say the shaking stopped.
 */
enum class EventState {
    UNCONFIRMED,
    CONFIRMED,
    RESOLVED,
    CANCELLED
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
 * @param isTest true only for a drill (`POST /api/v1/admin/test-alert`). Defaults to
 *   false so a payload that carries no flag can never be mistaken for a drill — the
 *   direction that matters, since treating a real quake as a test would suppress it.
 *   Release builds never see one at all: the mapper drops such a frame before it
 *   becomes a [WsAlertMessage]
 *   (id.web.quakealert.data.network.mapper.toDomainOrNull).
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
    val nodeCount: Int,
    val isTest: Boolean = false,
    /**
     * Lifecycle state, or null when the server did not say — a pre-Phase-3 server,
     * or a state this build has never heard of. Null means "unknown", never a state
     * by default: [AlertType] already carries the confirmed-versus-advisory
     * distinction this field only refines.
     */
    val eventState: EventState? = null,
    /**
     * Monotonic per-event revision, 0 when unknown. Comparable only within one
     * `event_id`.
     */
    val eventRevision: Int = 0,
    /** Estimated onset of shaking in ms since the epoch, UTC; 0 when unknown. */
    val originTsMs: Long = 0L,
    /**
     * "SENSOR" (measured) or "PUBLISH_BOUND" (an upper bound from a legacy v1
     * observation, possibly later than the real onset); empty when unknown. Kept as
     * the server's own string because nothing on this client computes with it — it
     * exists so a displayed onset can be qualified rather than trusted.
     */
    val originTsSource: String = "",
    /** Separated spatial cells that contributed evidence; 0 when unknown. */
    val independentCellCount: Int = 0
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
