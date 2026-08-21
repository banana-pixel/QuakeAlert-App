package id.web.quakealert.domain

/**
 * Remembers which alerts this process has already acted on.
 *
 * The two delivery channels overlap by design: an alert arrives over the WebSocket
 * *and* as an FCM push, and the socket also replays its last frame to every new
 * subscriber. Without a shared memory of what has been handled, one earthquake
 * launches the full-screen activity twice and restarts a siren the user just
 * silenced.
 *
 * Process-lifetime, deliberately not persisted: after a restart the app has no
 * siren running and no activity showing, so re-acting on a still-recent alert is
 * the correct behaviour rather than a duplicate.
 *
 * Thread-safe — the FCM service delivers on a binder thread while the socket
 * delivers on an IO dispatcher.
 */
class AlertDedup {

    private val seen = LinkedHashMap<String, Long>()

    /**
     * Records [message] and reports whether it is new.
     *
     * An `EARTHQUAKE_ADVISORY` carries an empty `event_id` (the server does not
     * persist advisories), so it has no usable key — those always count as new. That
     * is safe because an advisory only updates a banner; it never starts a siren or
     * an activity.
     */
    fun markIfNew(message: WsAlertMessage): Boolean {
        val key = key(message) ?: return true
        synchronized(seen) {
            if (seen.containsKey(key)) return false
            seen[key] = message.timestampMs
            // Bounded so a long-lived process cannot grow this without limit; the
            // oldest insertion is the least likely to be re-delivered.
            while (seen.size > MAX_ENTRIES) {
                val oldest = seen.keys.firstOrNull() ?: break
                seen.remove(oldest)
            }
            return true
        }
    }

    /** Whether [message] has already been recorded, without recording it. */
    fun hasSeen(message: WsAlertMessage): Boolean {
        val key = key(message) ?: return false
        return synchronized(seen) { seen.containsKey(key) }
    }

    /**
     * Keyed on type as well as `event_id`: an `EVENT_RESOLVED` shares its event id
     * with the `EARTHQUAKE_ALERT` it clears, and treating the all-clear as a
     * duplicate of the alarm would leave the red screen up forever.
     */
    private fun key(message: WsAlertMessage): String? =
        message.eventId.takeIf { it.isNotBlank() }?.let { "${message.type.name}:$it" }

    private companion object {
        const val MAX_ENTRIES = 64
    }
}
