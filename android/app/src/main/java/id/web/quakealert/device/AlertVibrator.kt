package id.web.quakealert.device

import android.content.Context
import android.os.Build
import android.os.VibrationEffect
import android.os.Vibrator
import android.os.VibratorManager
import android.util.Log

/**
 * The tactile half of an alert — the pulse that runs alongside [AlertSiren].
 *
 * It exists because the siren alone is not reliable: a phone face-down in a bag,
 * on a desk in a noisy room, or paired to headphones the user is not wearing gives
 * no warning at all. A pattern the user can feel through a pocket costs nothing and
 * covers those cases.
 *
 * The pattern is deliberately unlike a notification buzz: a long pulse, a short
 * gap, repeated. Two short taps read as "a message arrived"; this has to read as
 * "look at your phone now".
 *
 * As in [AlertSiren], every platform call is wrapped — a device with no vibrator,
 * or one whose motor the OS has parked, must not take the audible or visual halves
 * of the warning down with it. Not thread-safe: main thread only.
 */
class AlertVibrator(context: Context) {

    /**
     * Resolved once at construction. `VibratorManager` is the only supported route
     * from Android 12 (API 31); the deprecated system service is still the only one
     * below it, and this app's `minSdk` is 28.
     */
    private val vibrator: Vibrator? = runCatching {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
            val manager = context.getSystemService(VibratorManager::class.java)
            manager?.defaultVibrator
        } else {
            @Suppress("DEPRECATION")
            context.getSystemService(Context.VIBRATOR_SERVICE) as? Vibrator
        }
    }.getOrNull()

    /** True while [start] has handed a repeating pattern to the motor. */
    var isActive: Boolean = false
        private set

    /**
     * Starts the repeating pulse, or does nothing if it is already running or the
     * device has no usable vibrator. Idempotent for the same reason [AlertSiren.start]
     * is: a repeated call for one alert must not restart the pattern from the top.
     */
    fun start() {
        if (isActive) return
        val motor = vibrator?.takeIf { it.hasVibrator() } ?: return
        runCatching {
            motor.vibrate(
                VibrationEffect.createWaveform(PATTERN_MS, REPEAT_FROM_INDEX)
            )
            isActive = true
        }.onFailure { Log.w(TAG, "could not start the alert vibration", it) }
    }

    /**
     * Stops the pulse. Safe to call when nothing is running — every teardown path
     * (STOP, dismissal, timeout, stand-down) funnels through here, so it has to be
     * unconditional rather than assume a matching [start].
     */
    fun stop() {
        isActive = false
        runCatching { vibrator?.cancel() }
            .onFailure { Log.w(TAG, "could not stop the alert vibration", it) }
    }

    private companion object {
        const val TAG = "AlertVibrator"

        /**
         * `off, on, off, on, …` in milliseconds. Starts on immediately, then a
         * 600 ms pulse and a 400 ms gap — slow enough to feel like one insistent
         * signal rather than a stutter.
         */
        val PATTERN_MS = longArrayOf(0L, 600L, 400L)

        /** Repeat from the first *off* slot, so the pattern loops until cancelled. */
        const val REPEAT_FROM_INDEX = 0
    }
}
