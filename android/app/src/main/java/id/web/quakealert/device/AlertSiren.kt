package id.web.quakealert.device

import android.content.Context
import android.media.AudioAttributes
import android.media.MediaPlayer
import android.media.RingtoneManager
import android.net.Uri
import android.os.Handler
import android.os.Looper
import android.util.Log

/**
 * The audible half of an active earthquake alert (Figma node 1:1043) — the siren
 * the screen's "MUTE ALERT" control silences.
 *
 * Three decisions worth knowing about before touching this:
 *
 *  - **No bundled audio asset.** The sound is the device's own default alarm tone,
 *    resolved through [RingtoneManager]. A user has already agreed that this tone
 *    means "wake up"; shipping a novel sound would be a quieter, less recognisable
 *    warning on every device whose owner has turned their alarm volume up.
 *  - **`USAGE_ALARM`, not `USAGE_NOTIFICATION`.** Alarm usage plays through the
 *    alarm stream, which Do Not Disturb and the ringer's silent mode do not mute by
 *    default. A life-safety siren that a silenced phone swallows is not a siren.
 *  - **The tone expires, the warning does not.** [start] arms a
 *    [SIREN_DURATION_MS] auto-stop, matching the 90 s alert window in
 *    docs/SYSTEM_SPEC.md and .clinerules/20 rule 3. Only the audio stops; the red
 *    screen stays up until `EVENT_RESOLVED` arrives, because the shaking ending is
 *    not the same as the event being over.
 *  - **Muting pauses, it does not release.** [mute] keeps the prepared player so
 *    [unmute] resumes instantly, and so a later [start] for the *same* alert does
 *    not undo the user's decision to silence it. Only [release] tears the player
 *    down, on stand-down or ViewModel clear.
 *
 * Every platform call is wrapped: a device with no default alarm tone, or a media
 * server that refuses the stream, must not take the visual alert down with it. The
 * screen is the part of the warning that has to survive.
 *
 * Not thread-safe — call it from the main thread (the ViewModel does).
 */
class AlertSiren(private val context: Context) {

    private var player: MediaPlayer? = null

    private val handler = Handler(Looper.getMainLooper())

    /**
     * The armed auto-stop. Held so a re-[start] cannot stack a second one and so
     * [release] can cancel a pending stop for a player that no longer exists.
     */
    private val expiry = Runnable { expire() }

    /** True once [start] has a prepared player, whether or not it is paused. */
    val isActive: Boolean
        get() = player != null

    /**
     * Starts the siren, or does nothing if it is already running or deliberately
     * muted. Idempotent so repeated frames for the same event do not restart the
     * tone from the top or resurrect a siren the user just silenced.
     */
    fun start() {
        if (player != null) return

        val uri = alarmUri() ?: run {
            Log.w(TAG, "No alarm/notification tone available; alert stays silent")
            return
        }

        player = runCatching {
            MediaPlayer().apply {
                setAudioAttributes(
                    AudioAttributes.Builder()
                        .setUsage(AudioAttributes.USAGE_ALARM)
                        .setContentType(AudioAttributes.CONTENT_TYPE_SONIFICATION)
                        .build()
                )
                setDataSource(context, uri)
                isLooping = true
                prepare()
                start()
            }
        }.onFailure { throwable ->
            Log.w(TAG, "Could not start alert siren", throwable)
        }.getOrNull()

        if (player != null) {
            handler.removeCallbacks(expiry)
            handler.postDelayed(expiry, SIREN_DURATION_MS)
        }
    }

    /**
     * The auto-stop firing. Tears the player down rather than pausing it: a paused
     * player would leave [isActive] true, and [unmute] would then let the tone
     * resume minutes after the window closed.
     */
    private fun expire() {
        if (player == null) return
        Log.i(TAG, "alert siren expired after ${SIREN_DURATION_MS}ms; visual alert continues")
        release()
    }

    /** Silences the siren while leaving it prepared, for "MUTE ALERT". */
    fun mute() {
        val current = player ?: return
        runCatching { if (current.isPlaying) current.pause() }
            .onFailure { Log.w(TAG, "Could not pause alert siren", it) }
    }

    /** Resumes a [mute]d siren. */
    fun unmute() {
        val current = player ?: return
        runCatching { if (!current.isPlaying) current.start() }
            .onFailure { Log.w(TAG, "Could not resume alert siren", it) }
    }

    /** Stops and tears down the player. Safe to call when nothing is playing. */
    fun release() {
        handler.removeCallbacks(expiry)
        val current = player ?: return
        player = null
        runCatching {
            if (current.isPlaying) current.stop()
            current.release()
        }.onFailure { Log.w(TAG, "Could not release alert siren", it) }
    }

    /**
     * Default alarm tone, falling back to the notification and ringtone defaults.
     * A device can legitimately have no alarm default set (a stripped ROM, or a
     * user who picked "None"), and any audible tone beats silence here.
     */
    private fun alarmUri(): Uri? = FALLBACK_TYPES.firstNotNullOfOrNull { type ->
        runCatching { RingtoneManager.getDefaultUri(type) }.getOrNull()
    }

    private companion object {
        const val TAG = "AlertSiren"

        /**
         * How long the tone may sound. 90 s is the server's alert window and cooldown
         * (docs/SYSTEM_SPEC.md): past it a still-sounding siren is no longer telling
         * the user anything new, and an unbounded alarm is one users learn to silence
         * pre-emptively.
         */
        const val SIREN_DURATION_MS = 90_000L

        val FALLBACK_TYPES = listOf(
            RingtoneManager.TYPE_ALARM,
            RingtoneManager.TYPE_NOTIFICATION,
            RingtoneManager.TYPE_RINGTONE
        )
    }
}
