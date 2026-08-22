package id.web.quakealert.ui.common

import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * The floor that keeps a pull-to-refresh indicator retractable.
 *
 * `PullToRefreshBox` parks its spinner on release and retracts it only on an
 * `isRefreshing` true → false transition that composition observes, so a refresh
 * completing inside one frame would leave it parked forever.
 */
class RefreshIndicatorTest {

    @Test
    fun `an answer inside a single frame is held long enough to be seen`() {
        // The case that used to strand the spinner: a pull with no network fails in
        // about a millisecond.
        assertEquals(MIN_REFRESH_VISIBLE_MS - 1, remainingRefreshHoldMs(elapsedMs = 1))
        assertEquals(MIN_REFRESH_VISIBLE_MS, remainingRefreshHoldMs(elapsedMs = 0))
    }

    @Test
    fun `a request that outlived the floor is not delayed further`() {
        assertEquals(0L, remainingRefreshHoldMs(elapsedMs = MIN_REFRESH_VISIBLE_MS))
        assertEquals(0L, remainingRefreshHoldMs(elapsedMs = 30_000))
    }

    @Test
    fun `the hold is clamped to the floor at both ends`() {
        // A negative elapsed time must not turn a 400 ms floor into a 5 s wait.
        assertEquals(MIN_REFRESH_VISIBLE_MS, remainingRefreshHoldMs(elapsedMs = -5_000))
        assertEquals(0L, remainingRefreshHoldMs(elapsedMs = 5, minimumMs = 0))
    }
}
