package id.web.quakealert.ui.common

/**
 * Shortest time the pull-to-refresh indicator stays on screen, in milliseconds.
 *
 * Not cosmetic. `PullToRefreshBox` parks its indicator at the threshold the moment
 * the finger lifts and retracts it *only* when the `isRefreshing` flag it was given
 * goes true → false — `PullToRefreshElement.update` calls the node's `update()` only
 * when the value actually changed. Compose reads state once per frame, so a refresh
 * that begins and ends inside a single frame is invisible to it: the flag was false
 * before and false after, nothing changed, and the spinner is left parked with no
 * further transition coming to retract it.
 *
 * That is not a rare case. A pull with no network fails in a millisecond or two, and
 * the "Near with no fix" path answers without a request at all.
 *
 * A floor on how long the flag stays true makes the transition span at least one
 * frame in every case, which is also why the value is far above a frame: it doubles
 * as the usual anti-flicker floor, so a fast answer reads as a refresh that happened
 * rather than a twitch.
 */
internal const val MIN_REFRESH_VISIBLE_MS: Long = 400L

/**
 * How much longer the refresh flag must stay set, given how long it has been set
 * already. Zero once the request has outlived the floor, which is the common case.
 *
 * Clamped at both ends: never negative, and never longer than the floor itself, so
 * an elapsed time that reads as negative — a wrong clock, or a hold asked about
 * before the indicator was ever raised — cannot turn a floor into a long wait.
 *
 * @param elapsedMs time since the indicator was shown.
 * @param minimumMs the floor, defaulting to [MIN_REFRESH_VISIBLE_MS].
 */
internal fun remainingRefreshHoldMs(
    elapsedMs: Long,
    minimumMs: Long = MIN_REFRESH_VISIBLE_MS
): Long = (minimumMs - elapsedMs).coerceIn(0L, minimumMs)
