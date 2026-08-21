package id.web.quakealert.ui.warning

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import id.web.quakealert.data.AppSettingsRepository
import id.web.quakealert.data.UnitSystem
import id.web.quakealert.data.network.QuakeNetwork
import id.web.quakealert.data.network.mapper.QuakeFormat
import id.web.quakealert.data.network.mapper.intensityBannerLabel
import id.web.quakealert.data.network.mapper.intensityValueLabel
import id.web.quakealert.data.network.mapper.toHistoryItem
import id.web.quakealert.device.AlertSiren
import id.web.quakealert.device.TorchController
import id.web.quakealert.domain.AlertGate
import id.web.quakealert.domain.AlertType
import id.web.quakealert.domain.EarthquakeEvent
import id.web.quakealert.domain.EventStatus
import id.web.quakealert.domain.UserLocation
import id.web.quakealert.domain.WsAlertMessage
import id.web.quakealert.domain.distanceKmTo
import id.web.quakealert.service.WarningNotifier
import id.web.quakealert.ui.history.QuakeHistoryItem
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.time.Instant
import kotlin.math.roundToInt

/**
 * Hosts the [WarningUiState] for the Warning screen and exposes it as a
 * [StateFlow] following unidirectional data flow.
 *
 * Two sources feed it, and the split matters:
 *  - **REST**, once per load: the newest event from `GET /api/v1/events` decides
 *    which state the screen opens in, so a cold start *during* a quake opens on the
 *    emergency screen instead of a calm banner that waits for the next push.
 *  - **WebSocket / FCM**, continuously: `EARTHQUAKE_ALERT` raises
 *    [WarningUiState.ActiveAlert], `EVENT_RESOLVED` stands it down,
 *    `EARTHQUAKE_ADVISORY` only nudges the idle banner's possibility read.
 *
 * Both realtime channels share [onAlertReceived], because the FCM data payload is
 * the same shape as the socket frame (contracts/fcm/alert_payload.json) — one entry
 * point is what keeps a push-delivered alert and a socket-delivered alert from
 * producing two different screens.
 *
 * The ViewModel also owns the alert's two hardware effects, [AlertSiren] and
 * [TorchController], and that ownership is deliberate: both must outlive
 * recomposition and both must be torn down exactly once, on stand-down or in
 * [onCleared]. Driving them from the composable instead would tie a burning torch
 * to the lifetime of a composition.
 *
 * The persisted [UnitSystem] from [AppSettingsRepository] is folded into every
 * emission so the emergency card's proximity read, the detail overlay's "Distance
 * from you" row and the share text all render the unit the user picked in Settings.
 *
 * Sharing is deliberately absent here: firing `Intent.ACTION_SEND` needs a
 * `Context`, not app state, so it lives in [WarningRoute] alongside the other
 * composition-local work.
 */
class WarningViewModel(application: Application) : AndroidViewModel(application) {

    private val repository = AppSettingsRepository(application)

    private val network = QuakeNetwork.from(application)

    private val apiClient = network.apiClient

    private val siren = AlertSiren(application)

    private val torch = TorchController(application)

    private val _uiState = MutableStateFlow<WarningUiState>(
        WarningUiState.Idle(isLoading = true)
    )

    val uiState: StateFlow<WarningUiState> = combine(
        repository.unitSystem,
        _uiState
    ) { unit, state -> state.withUnitSystem(unit) }.stateIn(
        scope = viewModelScope,
        started = SharingStarted.WhileSubscribed(5_000),
        initialValue = WarningUiState.Idle(isLoading = true)
    )

    /**
     * Detail payload behind the idle banner's "SEE DETAILS" capsule, built from
     * whichever source last described a quake. Held outside [WarningUiState] because
     * it is not rendered until the overlay opens — putting it in the state would
     * recompose the screen for a value nothing is showing.
     */
    private var activeAlertDetails: QuakeHistoryItem? = null

    init {
        load()
        observeAlerts()
    }

    /**
     * Re-runs the alert-feed load after a failure, from
     * [id.web.quakealert.ui.common.QuakeErrorState]'s "Retry" action.
     */
    fun onRetry() {
        load()
    }

    /**
     * Single entry point into the loading → content / error state machine, used by
     * both the initial load and [onRetry].
     *
     * Every write back into [_uiState] is guarded on the state still being
     * [WarningUiState.Idle]. A socket frame can raise a real emergency while this
     * request is in flight, and neither a stale "no recent earthquake" snapshot nor
     * a network error may take that screen down.
     */
    private fun load() {
        viewModelScope.launch {
            _uiState.update { state ->
                if (state is WarningUiState.Idle) {
                    state.copy(isLoading = true, isError = false, errorMessage = null)
                } else {
                    state
                }
            }
            try {
                when (val outcome = fetchWarning()) {
                    is LoadOutcome.Emergency -> raise(outcome.alert)
                    is LoadOutcome.DistantEmergency -> _uiState.update { state ->
                        if (state is WarningUiState.Idle) {
                            state.copy(
                                banner = outcome.snapshot.banner,
                                sectionTitle = outcome.snapshot.sectionTitle,
                                tips = outcome.snapshot.tips,
                                isLoading = false
                            )
                        } else {
                            state
                        }
                    }
                    is LoadOutcome.Resting -> _uiState.update { state ->
                        if (state is WarningUiState.Idle) {
                            state.copy(
                                banner = outcome.snapshot.banner,
                                sectionTitle = outcome.snapshot.sectionTitle,
                                tips = outcome.snapshot.tips,
                                isLoading = false
                            )
                        } else {
                            state
                        }
                    }
                }
            } catch (cancellation: CancellationException) {
                // Never treat scope cancellation as a load failure — rethrow so the
                // coroutine machinery sees it and the screen keeps its last state.
                throw cancellation
            } catch (throwable: Throwable) {
                _uiState.update { state ->
                    if (state is WarningUiState.Idle) {
                        state.copy(
                            isLoading = false,
                            isError = true,
                            errorMessage = throwable.message ?: LOAD_ERROR_MESSAGE
                        )
                    } else {
                        state
                    }
                }
            }
        }
    }

    /**
     * Resolves the opening state from the newest stored event.
     *
     * A single event is enough: the feed is sorted `created_at DESC`, and only an
     * unresolved quake inside the recent window can justify opening on the emergency
     * screen. An older or already-resolved event means the idle state.
     */
    private suspend fun fetchWarning(): LoadOutcome {
        val latest = apiClient.fetchEvents(limit = 1).getOrThrow().firstOrNull()
        if (latest == null || !latest.isOngoing()) {
            activeAlertDetails = null
            return LoadOutcome.Resting(restingSnapshot())
        }

        val userLocation = apiClient.currentUserLocation()
        activeAlertDetails = latest.toHistoryItem(userLocation)

        // The same gate the realtime path uses. A cold start during a quake on the
        // other side of the country must open on the banner, not the siren.
        val decision = AlertGate.decide(
            userLocation = userLocation,
            centroidLat = latest.latitude,
            centroidLon = latest.longitude,
            mmi = latest.mmi,
            pgaGal = latest.pgaGal
        )
        if (!decision.shouldAlarm) {
            return LoadOutcome.DistantEmergency(
                activeSnapshot(
                    intensityLabel = latest.intensityBannerLabel(),
                    timeAgo = QuakeFormat.relativeTime(latest.createdAt, Instant.now())
                )
            )
        }
        return LoadOutcome.Emergency(latest.toActiveAlert(userLocation))
    }

    /**
     * Collects the realtime stream for the ViewModel's lifetime.
     *
     * The flow reconnects internally, so a dropped socket is not an error state
     * here — a genuinely unreachable alert network already shows through the
     * [load] failure path.
     */
    private fun observeAlerts() {
        viewModelScope.launch {
            network.webSocketClient.alerts.collect { message -> onAlertReceived(message) }
        }
    }

    /**
     * Applies one realtime frame, from either the WebSocket or an FCM data payload.
     *
     * Public because push delivery arrives outside this ViewModel's collection: a
     * `FirebaseMessagingService` parses the payload into the same [WsAlertMessage]
     * and hands it here, so background push and foreground socket converge on one
     * state machine instead of two.
     *
     * [WsAlertMessage.isRecent] gates the alert path because the stream replays its
     * last frame to a new subscriber: without the guard, re-entering the screen
     * hours later would resurrect a finished quake as an active emergency.
     */
    fun onAlertReceived(message: WsAlertMessage) {
        viewModelScope.launch {
            when (message.type) {
                AlertType.EARTHQUAKE_ALERT -> if (message.isRecent()) raiseAlert(message)

                // Deliberately *not* the emergency screen: an advisory is 1–2 nodes
                // and unconfirmed, and escalating it would train users to ignore the
                // real thing. It also must never *downgrade* a live alert, hence the
                // Idle guard.
                AlertType.EARTHQUAKE_ADVISORY -> _uiState.update { state ->
                    if (state is WarningUiState.Idle) {
                        state.copy(banner = advisoryBanner(), isLoading = false)
                    } else {
                        state
                    }
                }

                AlertType.EVENT_RESOLVED -> standDown()
            }
        }
    }

    /**
     * Raises the emergency screen for a confirmed alert — but only after the
     * distance gate agrees.
     *
     * The gate is not a nicety: `/ws` and the FCM topic are broadcast channels, so
     * this device receives every event in the country. Without [AlertGate] a tremor
     * 800 km away sounds the same siren as one under the user's building, which is
     * how a life-safety app teaches its users to ignore it.
     *
     * A gated-out alert is not discarded — it becomes the idle "Recent Earthquake"
     * banner, with the details still available behind "SEE DETAILS". The user is
     * informed, just not woken.
     */
    private suspend fun raiseAlert(message: WsAlertMessage) {
        val userLocation: UserLocation? = apiClient.currentUserLocation()
        activeAlertDetails = message.toHistoryItem(userLocation)

        val decision = AlertGate.decide(
            userLocation = userLocation,
            centroidLat = message.centroidLat,
            centroidLon = message.centroidLon,
            mmi = message.mmi,
            pgaGal = message.pgaGal
        )

        if (!decision.shouldAlarm) {
            _uiState.update { state ->
                if (state is WarningUiState.Idle) {
                    val snapshot = distantSnapshot(message)
                    state.copy(
                        banner = snapshot.banner,
                        sectionTitle = snapshot.sectionTitle,
                        tips = snapshot.tips,
                        isLoading = false,
                        isError = false,
                        errorMessage = null
                    )
                } else {
                    state
                }
            }
            return
        }

        // Cross-channel de-duplication. A false result means this exact event was
        // already acted on — by the push handler (so WarningActivity is up with its
        // own siren) or by an earlier instance of this ViewModel (so the socket is
        // replaying its last frame to a re-entered screen). Either way the visual
        // alert is still correct and still raised; what must not happen is a second
        // siren starting over the first, or an old alert becoming audible again.
        val alreadyRaised = !network.alertDedup.markIfNew(message)

        // ActiveAlert.proximityLabel already renders "Distance unknown" when the gate
        // failed open on a missing position, so nothing extra is needed for that case.
        raise(message.toActiveAlert(userLocation), startSiren = !alreadyRaised)
    }

    /**
     * Switches the screen to [WarningUiState.ActiveAlert] and starts the siren.
     *
     * Re-raising the *same* event (a socket reconnect replaying its last frame, or
     * an FCM copy of a frame already delivered) preserves the user's mute and torch
     * choices — silencing a siren must not be undone by a duplicate of the alert
     * that was silenced. A genuinely new `event_id` starts audible again, because
     * the previous quake's mute says nothing about this one.
     */
    private fun raise(alert: WarningUiState.ActiveAlert, startSiren: Boolean = true) {
        _uiState.update { state ->
            val carried = (state as? WarningUiState.ActiveAlert)
                ?.takeIf { it.eventId.isNotBlank() && it.eventId == alert.eventId }

            alert.copy(
                isMuted = carried?.isMuted ?: false,
                isSosLightOn = carried?.isSosLightOn ?: torch.isOn,
                isSosLightUnavailable = carried?.isSosLightUnavailable ?: false,
                unitSystem = state.unitSystem
            )
        }

        // Idempotent, and a no-op while a carried-over mute is in effect.
        if (startSiren && (_uiState.value as? WarningUiState.ActiveAlert)?.isMuted != true) {
            siren.start()
        }
    }

    /**
     * Returns the screen to its idle state on an all-clear, and takes both hardware
     * effects down with it.
     *
     * The torch is switched off rather than left burning: its only control lives on
     * the emergency card, so a torch that survived the stand-down would be one the
     * user has no way left to turn off.
     */
    private fun standDown() {
        siren.release()
        torch.stop()
        // Also takes down the ongoing push notification, which is deliberately
        // non-dismissible: the all-clear is the thing that removes it.
        WarningNotifier.clear(getApplication())
        activeAlertDetails = null
        val snapshot = restingSnapshot()
        _uiState.update { state ->
            WarningUiState.Idle(
                banner = snapshot.banner,
                sectionTitle = snapshot.sectionTitle,
                tips = snapshot.tips,
                unitSystem = state.unitSystem
            )
        }
    }

    /**
     * Silences or re-enables the siren from the card's "MUTE ALERT" control (Figma
     * node 1:1073).
     *
     * A toggle rather than a one-way mute, and the visual alert is untouched either
     * way: the user asked for quiet, not to stop being warned.
     */
    fun onMuteClick() {
        val current = _uiState.value as? WarningUiState.ActiveAlert ?: return
        val muted = !current.isMuted
        if (muted) siren.mute() else siren.unmute()
        _uiState.update { state ->
            if (state is WarningUiState.ActiveAlert) state.copy(isMuted = muted) else state
        }
    }

    /**
     * Toggles the SOS torch strobe from the card's "SOS LIGHT" control (Figma node
     * 1:1076).
     *
     * No runtime permission is requested because none exists for this:
     * `CameraManager.setTorchMode` is permission-free, and the app deliberately does
     * not declare `CAMERA` (see [TorchController]). What can fail is availability —
     * no flash unit, or the camera held by another app — and that failure is written
     * into the state so the control can say the light did not come on rather than
     * looking engaged over a dark LED.
     */
    fun onSosLightClick() {
        val current = _uiState.value as? WarningUiState.ActiveAlert ?: return

        if (current.isSosLightOn) {
            torch.stop()
            _uiState.update { state ->
                if (state is WarningUiState.ActiveAlert) {
                    state.copy(isSosLightOn = false, isSosLightUnavailable = false)
                } else {
                    state
                }
            }
            return
        }

        val started = torch.start(viewModelScope)
        _uiState.update { state ->
            if (state is WarningUiState.ActiveAlert) {
                state.copy(isSosLightOn = started, isSosLightUnavailable = !started)
            } else {
                state
            }
        }
    }

    /**
     * Raises the overlay behind the idle banner's "SEE DETAILS" capsule, dispatched
     * by the current variant: [ActiveQuakeBanner] opens the "Recent Earthquake"
     * event detail (Figma 124:1192), [PossibilityBanner] opens the "Earthquake
     * Possibility" card (Figma 124:1605). Keeping the dispatch here means the screen
     * never needs to know which overlay a tap raises.
     */
    fun onSeeDetailsClicked() {
        val idle = _uiState.value as? WarningUiState.Idle ?: return
        when (idle.banner) {
            is ActiveQuakeBanner -> activeAlertDetails?.let { details ->
                _uiState.update { state ->
                    if (state is WarningUiState.Idle) {
                        state.copy(selectedEventDetails = details)
                    } else {
                        state
                    }
                }
            }
            is PossibilityBanner -> _uiState.update { state ->
                if (state is WarningUiState.Idle) {
                    state.copy(selectedPossibility = EarthquakePossibility())
                } else {
                    state
                }
            }
        }
    }

    /**
     * Closes the "Recent Earthquake" detail overlay. Called for every dismissal
     * path — the close (X) button, a back press and a tap outside the card.
     */
    fun onDetailDismissed() {
        _uiState.update { state ->
            if (state is WarningUiState.Idle) state.copy(selectedEventDetails = null) else state
        }
    }

    /**
     * Closes the "Earthquake Possibility" overlay. Called for every dismissal
     * path — the close (X) button, a back press and a tap outside the card.
     */
    fun onPossibilityDismissed() {
        _uiState.update { state ->
            if (state is WarningUiState.Idle) state.copy(selectedPossibility = null) else state
        }
    }

    /** Placeholder hook for tapping the bottom "Emergency" call-to-action. */
    fun onEmergencyClicked() {
        // Intentionally empty until the emergency-resource flow is defined.
    }

    /**
     * Releases both hardware effects. `viewModelScope` is cancelled around this
     * point, which would cancel the strobe loop anyway — but the torch is switched
     * off explicitly rather than left to that ordering, because a leaked LED is the
     * one failure here the user cannot recover from inside the app.
     */
    override fun onCleared() {
        siren.release()
        torch.stop()
        super.onCleared()
    }

    /** What a REST load resolved to: an emergency, or an idle snapshot. */
    private sealed interface LoadOutcome {
        data class Emergency(val alert: WarningUiState.ActiveAlert) : LoadOutcome

        /**
         * An ongoing quake that failed the distance gate: shown as a recent-quake
         * banner rather than the emergency screen.
         */
        data class DistantEmergency(val snapshot: WarningSnapshot) : LoadOutcome
        data class Resting(val snapshot: WarningSnapshot) : LoadOutcome
    }

    /**
     * The three fields an idle load resolves together. Bundled so the banner
     * variant, its section title and its tip set can never be applied piecemeal
     * and leave the screen showing aftershock tips under a resting banner.
     */
    private data class WarningSnapshot(
        val banner: WarningBanner,
        val sectionTitle: String,
        val tips: List<PreparednessTip>
    )

    private companion object {
        /** Fallback copy when a load failure carries no message of its own. */
        const val LOAD_ERROR_MESSAGE =
            "Could not reach the alert network. Check your connection and try again."

        /** Copy for the idle banner variants, matching the design (Figma 124:1297 / 124:1426). */
        const val TITLE_ACTIVE = "Recent Earthquake Alert"
        const val TITLE_RESTING = "No Recent Earthquake"
        const val SECTION_ACTIVE = "Stay alert for aftershocks"
        const val SECTION_RESTING = "Stay prepared for an earthquake"
        const val POSSIBILITY_DEFAULT = "Possibility : High Risk"

        fun activeSnapshot(intensityLabel: String, timeAgo: String) = WarningSnapshot(
            banner = ActiveQuakeBanner(
                title = TITLE_ACTIVE,
                intensityLabel = intensityLabel,
                timeAgo = timeAgo
            ),
            sectionTitle = SECTION_ACTIVE,
            tips = activeQuakeTips()
        )

        /** A confirmed alert that the distance gate kept off the emergency screen. */
        fun distantSnapshot(message: WsAlertMessage) = activeSnapshot(
            intensityLabel = message.intensityBannerLabel(),
            timeAgo = QuakeFormat.relativeTime(
                Instant.ofEpochMilli(message.timestampMs),
                Instant.now()
            )
        )

        fun restingSnapshot() = WarningSnapshot(
            banner = PossibilityBanner(
                title = TITLE_RESTING,
                possibilityLabel = POSSIBILITY_DEFAULT
            ),
            sectionTitle = SECTION_RESTING,
            tips = noActiveQuakeTips()
        )

        /**
         * Idle banner shown while an unconfirmed tremor is being evaluated. Same
         * variant as the resting banner, so the layout is untouched — only the
         * read-out changes.
         */
        fun advisoryBanner() = PossibilityBanner(
            title = "Possible Tremor Detected",
            possibilityLabel = POSSIBILITY_DEFAULT
        )

        /** Unresolved and inside the same window the realtime path uses. */
        fun EarthquakeEvent.isOngoing(nowMs: Long = System.currentTimeMillis()): Boolean =
            status == EventStatus.HAPPENING &&
                nowMs - createdAt.toEpochMilli() <= WsAlertMessage.RECENT_WINDOW_MS

        /**
         * Stored event → emergency screen state. Distance stays null when the device
         * position is unknown, rather than the 0 the History card falls back to: "0 km
         * away" on a full-screen alert reads as *at the epicentre*.
         */
        fun EarthquakeEvent.toActiveAlert(userLocation: UserLocation?) =
            WarningUiState.ActiveAlert(
                eventId = eventId,
                intensityValue = intensityValueLabel(),
                distanceKm = userLocation.distanceKmTo(latitude, longitude)?.roundToInt(),
                locationName = locationName
            )

        /** Realtime frame → emergency screen state. See the stored-event twin above. */
        fun WsAlertMessage.toActiveAlert(userLocation: UserLocation?) =
            WarningUiState.ActiveAlert(
                eventId = eventId,
                intensityValue = intensityValueLabel(),
                distanceKm = userLocation.distanceKmTo(centroidLat, centroidLon)?.roundToInt(),
                locationName = locationName
            )
    }
}
