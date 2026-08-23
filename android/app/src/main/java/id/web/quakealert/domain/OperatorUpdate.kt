package id.web.quakealert.domain

import java.time.Instant

/**
 * One announcement published by an operator — never a consequence of the consensus
 * engine, and never rendered like one.
 *
 * The type exists separately from [WsAlertMessage] rather than being folded into it
 * with a fourth [AlertType]: everything downstream of an alert exists to interrupt
 * (the siren, the full-screen intent, [AlertGate], [AlertDedup]), and an operator
 * notice must not be able to reach any of it by accident. Two types means the
 * compiler enforces that separation instead of a `when` branch having to remember it.
 *
 * @param id the server's `broadcast_id`, also the notification dedup key: the socket
 *   frame and its push copy describe one announcement.
 * @param regionCode the `user_profiles.region_code` this was targeted at, or null for
 *   a national notice. Kept so the list can say *why* the user was told, which is the
 *   difference between "this concerns your province" and "this concerns everyone".
 * @param publishedAt when the server stored it. Announcements have no expiry — unlike
 *   an alert, an old one is stale rather than dangerous — so nothing here is gated on
 *   a recent window.
 */
data class OperatorUpdate(
    val id: String,
    val title: String,
    val body: String,
    val regionCode: String?,
    val publishedAt: Instant
) {
    /** True when this went to every device rather than one province. */
    val isNational: Boolean get() = regionCode.isNullOrBlank()
}
