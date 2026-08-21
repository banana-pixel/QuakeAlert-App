package id.web.quakealert.device

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.Job
import kotlinx.coroutines.cancelChildren
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Rules for [TestAlertPlayback], the timing behind the Test Alert Sound modal.
 *
 * Worth pinning on the JVM rather than leaving to an instrumented test, because the
 * failure mode is loud in the most literal sense: a demonstration siren that does
 * not stop is the public-place panic the modal's own copy warns about, and none of
 * these paths are visible when they break — the dialog looks identical whether or
 * not the tone underneath it was released.
 *
 * The fake counts calls rather than recording a transcript: what matters is that the
 * device ends up quiet exactly once per test, not the order platform calls happened
 * to arrive in.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class TestAlertPlaybackTest {

    private class FakeOutput : AlertOutput {
        var starts = 0
            private set
        var stops = 0
            private set

        override fun start() { starts++ }
        override fun stop() { stops++ }
    }

    @Test
    fun `START begins the alert`() = runTest {
        val output = FakeOutput()
        val playback = TestAlertPlayback(this, output)

        playback.start()

        assertTrue(playback.isPlaying.value)
        assertEquals(1, output.starts)
        assertEquals(0, output.stops)

        playback.stop()
    }

    @Test
    fun `STOP ends the alert`() = runTest {
        val output = FakeOutput()
        val playback = TestAlertPlayback(this, output)

        playback.start()
        playback.stop()

        assertFalse(playback.isPlaying.value)
        assertEquals(1, output.stops)
    }

    @Test
    fun `stops itself after the timeout with no one pressing STOP`() = runTest {
        val output = FakeOutput()
        val playback = TestAlertPlayback(this, output)

        playback.start()

        // One millisecond short: the timeout must not fire early, or the test alert
        // is cut off before the user has heard enough of it to judge.
        advanceTimeBy(TestAlertPlayback.TEST_ALERT_DURATION_MS - 1)
        runCurrent()
        assertTrue(playback.isPlaying.value)
        assertEquals(0, output.stops)

        advanceTimeBy(1)
        runCurrent()
        assertFalse(playback.isPlaying.value)
        assertEquals(1, output.stops)
    }

    @Test
    fun `the timeout is five seconds`() = runTest {
        // Pinned because the number is a product decision, not an implementation
        // detail: long enough to confirm the tone, short enough to escape from.
        assertEquals(5_000L, TestAlertPlayback.TEST_ALERT_DURATION_MS)
    }

    @Test
    fun `a second START does not restart the alert or stack a timer`() = runTest {
        val output = FakeOutput()
        val playback = TestAlertPlayback(this, output)

        playback.start()
        advanceTimeBy(1_000)
        runCurrent()
        playback.start()

        assertEquals("the tone must not restart from the top", 1, output.starts)

        // The second call must not have armed a timer of its own: the alert ends on
        // the *first* one's schedule, 4 s from here, and not a moment later.
        advanceTimeBy(TestAlertPlayback.TEST_ALERT_DURATION_MS - 1_000)
        runCurrent()
        assertFalse(playback.isPlaying.value)
        assertEquals(1, output.stops)
    }

    @Test
    fun `a restarted alert gets the full timeout again`() = runTest {
        val output = FakeOutput()
        val playback = TestAlertPlayback(this, output)

        playback.start()
        advanceTimeBy(4_000)
        runCurrent()
        playback.stop()
        playback.start()

        // The cancelled timer must not survive its own alert: at 4 s into the second
        // run the first one's deadline has passed, and a leaked timer would have
        // ended a test that should still be sounding.
        advanceTimeBy(4_000)
        runCurrent()
        assertTrue(playback.isPlaying.value)

        playback.stop()
    }

    @Test
    fun `stopping without starting still leaves the device quiet`() = runTest {
        val output = FakeOutput()
        val playback = TestAlertPlayback(this, output)

        // The disposal path: a dialog can be torn down having never played anything,
        // or twice, and neither may leave a half-released player behind.
        playback.stop()
        playback.stop()

        assertFalse(playback.isPlaying.value)
        assertEquals(0, output.starts)
        assertEquals(2, output.stops)
    }

    @Test
    fun `a cancelled scope cannot fire the timeout afterwards`() = runTest {
        val output = FakeOutput()
        // Mirrors disposal: the modal's playback runs in the composition's scope, so
        // leaving the screen cancels the timer. The explicit stop() on dispose is
        // what silences the device; this asserts the timer cannot also fire later
        // and stop a *subsequent* test alert.
        val scope = CoroutineScope(coroutineContext + Job())
        val playback = TestAlertPlayback(scope, output)

        playback.start()
        scope.coroutineContext[Job]?.cancelChildren()
        playback.stop()

        advanceTimeBy(TestAlertPlayback.TEST_ALERT_DURATION_MS * 2)
        runCurrent()
        assertEquals("only the explicit stop may have run", 1, output.stops)
    }

    @Test
    fun `countdown ticks 5 to 0 and clears on stop`() = runTest {
        val output = FakeOutput()
        val playback = TestAlertPlayback(this, output)

        // Idle is 0, not 5: the label must not promise a test that has not begun.
        assertEquals(0, playback.remainingSeconds.value)

        playback.start()
        runCurrent()
        assertEquals(5, playback.remainingSeconds.value)

        advanceTimeBy(1_000)
        runCurrent()
        assertEquals(4, playback.remainingSeconds.value)

        advanceTimeBy(3_000)
        runCurrent()
        assertEquals(1, playback.remainingSeconds.value)

        // The final tick is also the auto-stop, so the count reaching 0 and the device
        // going quiet cannot disagree.
        advanceTimeBy(1_000)
        runCurrent()
        assertEquals(0, playback.remainingSeconds.value)
        assertFalse(playback.isPlaying.value)
        assertEquals(1, output.stops)
    }

    @Test
    fun `a manual STOP mid-count resets the label`() = runTest {
        val output = FakeOutput()
        val playback = TestAlertPlayback(this, output)

        playback.start()
        advanceTimeBy(2_000)
        runCurrent()
        assertEquals(3, playback.remainingSeconds.value)

        playback.stop()
        assertEquals(0, playback.remainingSeconds.value)

        // And the cancelled timer cannot cut a second test short.
        playback.start()
        runCurrent()
        assertEquals(5, playback.remainingSeconds.value)
        advanceTimeBy(4_000)
        runCurrent()
        assertEquals(1, playback.remainingSeconds.value)
        assertTrue(playback.isPlaying.value)

        playback.stop()
    }
}
