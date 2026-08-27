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

    private val seen = LinkedHashMap<String, Entry>()

    /**
     * Records [message] and reports whether it is new.
     *
     * A frame with no `event_id` has no usable key — those always count as new. Since
     * server Phase 3 every frame carries one, including an advisory; before it,
     * advisories carried an empty id, and that older behaviour is the reason this
     * branch exists rather than an assertion. It is safe either way because an
     * advisory only updates a banner; it never starts a siren or an activity.
     *
     * A *lower* `event_revision` for a key already seen is suppressed as well, not
     * just an exact repeat. The two channels are unordered relative to each other —
     * an FCM copy of revision 2 can arrive after the socket delivered revision 3 —
     * and acting on the older frame would re-raise a state the event has already left.
     * Revision 0 means the server did not say, which is every pre-Phase-3 frame, so it
     * must keep behaving exactly as before: first frame wins, repeats suppressed.
     */
    fun markIfNew(message: WsAlertMessage): Boolean {
        val key = key(message) ?: return true
        synchronized(seen) {
            val previous = seen[key]
            if (previous != null && message.eventRevision <= previous.revision) return false
            seen[key] = Entry(timestampMs = message.timestampMs, revision = message.eventRevision)
            // Bounded so a long-lived process cannot grow this without limit; the
            // oldest insertion is the least likely to be re-delivered.
            while (seen.size > MAX_ENTRIES) {
                val oldest = seen.keys.firstOrNull() ?: break
                seen.remove(oldest)
            }
            return true
        }
    }

    /**
     * Whether [message] has already been recorded, without recording it. Follows the
     * same revision rule as [markIfNew]: a newer revision of a known event has NOT
     * been seen.
     */
    fun hasSeen(message: WsAlertMessage): Boolean {
        val key = key(message) ?: return false
        return synchronized(seen) {
            val previous = seen[key] ?: return@synchronized false
            message.eventRevision <= previous.revision
        }
    }

    /**
     * Keyed on type as well as `event_id`: an `EVENT_RESOLVED` shares its event id
     * with the `EARTHQUAKE_ALERT` it clears, and treating the all-clear as a
     * duplicate of the alarm would leave the red screen up forever.
     */
    private fun key(message: WsAlertMessage): String? =
        message.eventId.takeIf { it.isNotBlank() }?.let { "${message.type.name}:$it" }

    /**
     * What is remembered per key. The revision is what makes an out-of-order
     * re-delivery distinguishable from news; the timestamp is kept for the same
     * diagnostic reason it always was.
     */
    private data class Entry(val timestampMs: Long, val revision: Int)

    private companion object {
        const val MAX_ENTRIES = 64
    }
}
