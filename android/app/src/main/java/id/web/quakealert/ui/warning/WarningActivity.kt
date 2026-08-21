package id.web.quakealert.ui.warning

import android.content.Context
import android.content.Intent
import android.os.Build
import android.os.Bundle
import android.view.WindowManager
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.lifecycle.lifecycleScope
import id.web.quakealert.device.AlertSiren
import id.web.quakealert.device.TorchController
import id.web.quakealert.data.network.QuakeNetwork
import id.web.quakealert.domain.AlertType
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.QuakeAlertTheme
import id.web.quakealert.ui.theme.OnboardingBackgroundBrush
import kotlinx.coroutines.launch

/**
 * The full-screen earthquake alert, raised from a push notification's full-screen
 * intent.
 *
 * Deliberately an Activity and not a nav route inside [id.web.quakealert.MainActivity]:
 * it has to open from a process that was not running, over the lock screen, with no
 * user interaction — and a Compose destination cannot be reached from a dead process.
 *
 * **The gate has already run before this Activity exists.** [WarningNotifier] applies
 * [id.web.quakealert.domain.AlertGate] before it builds the notification, so
 * reaching `onCreate` already means "this quake is inside the user's radius". The
 * siren therefore starts here unconditionally.
 *
 * It owns its own [AlertSiren] and [TorchController] rather than sharing
 * [WarningViewModel]'s: that ViewModel belongs to MainActivity's screen, which may
 * not exist. Both effects are torn down in [onDestroy], so a torch cannot outlive
 * the alert that turned it on.
 */
class WarningActivity : ComponentActivity() {

    private val siren by lazy { AlertSiren(this) }

    private val torch by lazy { TorchController(this) }

    private var state by mutableStateOf(WarningUiState.ActiveAlert(
        eventId = "",
        intensityValue = "",
        distanceKm = null,
        locationName = ""
    ))

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        showOverLockScreen()
        enableEdgeToEdge()

        state = intent.toActiveAlert()
        siren.start()
        observeStandDown()

        setContent {
            QuakeAlertTheme {
                Column(
                    modifier = Modifier
                        .fillMaxSize()
                        .background(OnboardingBackgroundBrush)
                        .padding(Dimens.ScreenHorizontalPadding)
                ) {
                    ActiveAlertCard(
                        state = state,
                        onMuteClick = ::onMuteClick,
                        onSosLightClick = ::onSosLightClick,
                        modifier = Modifier.fillMaxSize()
                    )
                }
            }
        }
    }

    /**
     * A second alert while this screen is up (`launchMode="singleTop"`).
     *
     * A duplicate of the *same* event keeps the user's mute — silencing a siren must
     * not be undone by a redelivery of the alert that was silenced. A different
     * event id is a new quake and starts audible again.
     */
    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        setIntent(intent)
        val next = intent.toActiveAlert()
        val sameEvent = next.eventId.isNotBlank() && next.eventId == state.eventId
        state = next.copy(
            isMuted = if (sameEvent) state.isMuted else false,
            isSosLightOn = state.isSosLightOn,
            isSosLightUnavailable = state.isSosLightUnavailable
        )
        if (!state.isMuted) siren.start()
    }

    /**
     * Closes the screen when the server sends the all-clear for this event.
     *
     * Collecting the socket here also connects it, which is the point: a device woken
     * by push has no live connection, and without one the red screen would have no
     * way to ever learn the shaking is over except the user dismissing it.
     */
    private fun observeStandDown() {
        lifecycleScope.launch {
            QuakeNetwork.from(applicationContext).webSocketClient.alerts.collect { message ->
                val resolvesThis = message.type == AlertType.EVENT_RESOLVED &&
                    (message.eventId.isBlank() || message.eventId == state.eventId)
                if (resolvesThis) finish()
            }
        }
    }

    private fun onMuteClick() {
        val muted = !state.isMuted
        if (muted) siren.mute() else siren.unmute()
        state = state.copy(isMuted = muted)
    }

    private fun onSosLightClick() {
        if (state.isSosLightOn) {
            torch.stop()
            state = state.copy(isSosLightOn = false, isSosLightUnavailable = false)
            return
        }
        val started = torch.start(lifecycleScope)
        state = state.copy(isSosLightOn = started, isSosLightUnavailable = !started)
    }

    /**
     * Turns the screen on and shows over the keyguard.
     *
     * `setShowWhenLocked` / `setTurnScreenOn` are the API 27+ replacements for the
     * deprecated window flags, and `FLAG_KEEP_SCREEN_ON` is separate from both — it
     * keeps the display awake for the duration of the alert rather than letting the
     * usual timeout black it out mid-quake.
     */
    private fun showOverLockScreen() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O_MR1) {
            setShowWhenLocked(true)
            setTurnScreenOn(true)
        }
        window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
    }

    override fun onDestroy() {
        siren.release()
        torch.stop()
        super.onDestroy()
    }

    private fun Intent.toActiveAlert() = WarningUiState.ActiveAlert(
        eventId = getStringExtra(EXTRA_EVENT_ID).orEmpty(),
        intensityValue = getStringExtra(EXTRA_INTENSITY).orEmpty(),
        // -1 is the "unknown" sentinel because Intent has no nullable Int; the UI
        // renders null as "Distance unknown" rather than inventing a number.
        distanceKm = getIntExtra(EXTRA_DISTANCE_KM, UNKNOWN_DISTANCE).takeIf {
            it != UNKNOWN_DISTANCE
        },
        locationName = getStringExtra(EXTRA_LOCATION_NAME).orEmpty()
    )

    companion object {
        private const val EXTRA_EVENT_ID = "event_id"
        private const val EXTRA_INTENSITY = "intensity_value"
        private const val EXTRA_DISTANCE_KM = "distance_km"
        private const val EXTRA_LOCATION_NAME = "location_name"
        private const val UNKNOWN_DISTANCE = -1

        /**
         * Intent for the alert screen.
         *
         * `NEW_TASK` and `CLEAR_TOP` are both required: the notification launches this
         * from outside any task of ours, and a stale copy left behind by an earlier
         * quake must not sit under the new one.
         */
        fun intent(
            context: Context,
            eventId: String,
            intensityValue: String,
            locationName: String,
            distanceKm: Int?
        ): Intent = Intent(context, WarningActivity::class.java).apply {
            addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP)
            putExtra(EXTRA_EVENT_ID, eventId)
            putExtra(EXTRA_INTENSITY, intensityValue)
            putExtra(EXTRA_LOCATION_NAME, locationName)
            putExtra(EXTRA_DISTANCE_KM, distanceKm ?: UNKNOWN_DISTANCE)
        }
    }
}
