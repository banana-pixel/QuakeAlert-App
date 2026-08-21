package id.web.quakealert.device

import android.content.Context
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

/**
 * The pair of outputs a test alert drives, behind one seam.
 *
 * Named as a capability rather than as "siren + vibrator" so [TestAlertPlayback] —
 * the part with the timing rules worth testing — never touches an Android class and
 * can be exercised on the JVM.
 */
interface AlertOutput {

    /** Begins the audible and tactile alert. Idempotent. */
    fun start()

    /**
     * Ends it and releases whatever [start] acquired. Must tolerate being called
     * without a matching [start], and twice: every teardown path funnels here.
     */
    fun stop()
}

/**
 * [AlertOutput] backed by the real device — [AlertSiren] for the tone and
 * [AlertVibrator] for the pulse.
 *
 * The siren is built per [start] and released on [stop] rather than held for the
 * object's life: `AlertSiren.release()` is terminal, so a second test after a first
 * needs a fresh player. The vibrator has no such state and is held once.
 */
class DeviceAlertOutput(private val context: Context) : AlertOutput {

    private var siren: AlertSiren? = null

    private val vibrator = AlertVibrator(context)

    override fun start() {
        val player = siren ?: AlertSiren(context).also { siren = it }
        player.start()
        vibrator.start()
    }

    override fun stop() {
        siren?.release()
        siren = null
        vibrator.stop()
    }
}

/**
 * The timing rules behind the Test Alert Sound modal (Figma node 144:1025): START
 * begins the alert, STOP ends it, and it ends itself after
 * [TEST_ALERT_DURATION_MS] whether or not anyone presses STOP.
 *
 * A plain class rather than composable state because the auto-stop is the part that
 * matters and the part that is easy to get wrong — a test alert that keeps sounding
 * because the user backed out of the dialog instead of pressing STOP is exactly the
 * public-place panic the modal's own copy warns about. Kept free of Compose and of
 * Android so those rules are unit-testable; the composable is a shell over it.
 *
 * @param scope the scope the auto-stop timer runs in. In the modal this is the
 *   composition's scope, so leaving the screen cancels the timer with it.
 * @param output the siren and vibrator pair to drive.
 * @param durationMs how long one test runs before stopping itself.
 */
class TestAlertPlayback(
    private val scope: CoroutineScope,
    private val output: AlertOutput,
    private val durationMs: Long = TEST_ALERT_DURATION_MS
) {

    private val _isPlaying = MutableStateFlow(false)

    /** Drives the START button's engaged fill. */
    val isPlaying: StateFlow<Boolean> = _isPlaying.asStateFlow()

    private val _remainingSeconds = MutableStateFlow(0)

    /**
     * Whole seconds left in the running test, counting down to 0; 0 while idle.
     *
     * Exposed because the modal prints it on the button ("START (5s)"): a siren with
     * no visible end looks like something the user has to escape, and the count is
     * what turns it into a demonstration they are watching. Emitted from the same
     * coroutine that owns the auto-stop, so the number on screen cannot disagree with
     * when the sound actually ends.
     */
    val remainingSeconds: StateFlow<Int> = _remainingSeconds.asStateFlow()

    /**
     * The armed auto-stop. Held so [stop] can cancel a pending one — otherwise a
     * stopped-then-restarted test would be cut short by the first test's timer.
     */
    private var timeout: Job? = null

    /**
     * Starts the test, or does nothing if one is already running. Idempotent so a
     * second tap on START neither restarts the tone from the top nor stacks a
     * second timer that would end the alert early.
     */
    fun start() {
        if (_isPlaying.value) return
        _isPlaying.value = true
        output.start()
        timeout = scope.launch {
            // Ticks down rather than sleeping once: the same Job then drives both the
            // label and the teardown, and cancelling it cancels both together.
            var remaining = ((durationMs + TICK_MS - 1) / TICK_MS).toInt()
            _remainingSeconds.value = remaining
            while (remaining > 0) {
                delay(TICK_MS)
                remaining--
                _remainingSeconds.value = remaining
            }
            stop()
        }
    }

    /**
     * Ends the test. Deliberately unconditional: this is also the dismissal and
     * disposal path, and a dialog torn down in a state this object did not expect
     * must still leave the device quiet. [AlertOutput.stop] is specified to
     * tolerate that.
     */
    fun stop() {
        timeout?.cancel()
        timeout = null
        _isPlaying.value = false
        _remainingSeconds.value = 0
        output.stop()
    }

    companion object {

        /**
         * One test alert lasts 5 s — long enough to confirm the tone is audible and
         * the phone buzzes, short enough that a user who triggered it somewhere
         * public is not stuck waiting. Unrelated to [AlertSiren]'s 90 s window,
         * which is a real alert's duration and not a demonstration's.
         */
        const val TEST_ALERT_DURATION_MS = 5_000L

        /** One countdown step. A second, because the label is printed in seconds. */
        private const val TICK_MS = 1_000L
    }
}
