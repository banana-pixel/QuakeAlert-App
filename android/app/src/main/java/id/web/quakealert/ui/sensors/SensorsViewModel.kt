package id.web.quakealert.ui.sensors

import android.app.Application
import android.util.Log
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import id.web.quakealert.data.AppSettingsRepository
import id.web.quakealert.data.UnitSystem
import id.web.quakealert.data.network.QuakeApiClient
import id.web.quakealert.data.network.QuakeNetwork
import id.web.quakealert.data.network.mapper.toStationItems
import id.web.quakealert.domain.SafetyPolicy
import id.web.quakealert.ui.common.QuakeFilterState
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/**
 * The slice of [roll] that survives the station-status criterion.
 *
 * An extension rather than a method on [QuakeFilterState] because the display model
 * lives in this package: `ui.common` must not learn about `ui.sensors` types just to
 * answer a question about them. [QuakeFilterState.acceptsStation] is the criterion;
 * this is only where it meets the list.
 */
private fun QuakeFilterState.narrow(
    roll: List<SensorStationItem>
): List<SensorStationItem> =
    roll.filter { acceptsStation(it.status == SensorStatus.ONLINE) }

/**
 * Hosts the [SensorsUiState] for the Sensors screen and exposes it as a
 * [StateFlow] following unidirectional data flow. Stations come from
 * `GET /api/v1/sensors` via [id.web.quakealert.data.network.QuakeApiClient].
 *
 * The persisted [UnitSystem] from [AppSettingsRepository] is folded into every
 * emission so the map's range badge and the "Near" filter pill render the same
 * unit the user picked in Settings.
 */
class SensorsViewModel(application: Application) : AndroidViewModel(application) {

    private val repository = AppSettingsRepository(application)

    private val apiClient = QuakeNetwork.from(application).apiClient

    private val _uiState = MutableStateFlow(SensorsUiState(isLoading = true))

    /**
     * The roll exactly as `/sensors` returned it, before the station-status
     * criterion narrows it.
     *
     * Kept because that criterion is answered locally: the response already carries
     * each station's status, so switching between "Online only" and "All stations"
     * must not cost a request, and the map's coverage count must keep counting the
     * whole roll rather than the slice on screen.
     */
    private var roll: List<SensorStationItem> = emptyList()

    val uiState: StateFlow<SensorsUiState> = combine(
        repository.unitSystem,
        _uiState
    ) { unit, state ->
        state.copy(unitSystem = unit)
    }.stateIn(
        scope = viewModelScope,
        started = SharingStarted.WhileSubscribed(5_000),
        initialValue = SensorsUiState(isLoading = true)
    )

    init {
        load()
    }

    /**
     * Re-runs the station-roll load after a failure, from
     * [id.web.quakealert.ui.common.QuakeErrorState]'s "Retry" action.
     */
    fun onRetry() {
        load()
    }

    /**
     * Re-queries the station roll from a pull-to-refresh gesture.
     *
     * Distinct from [load] in what it shows, not in what it fetches: the roll the
     * user pulled stays on screen under the indicator instead of being replaced by a
     * skeleton, because a refresh that blanks the list looks like a failure. Ignored
     * while any load is already in flight — the gesture is easy to repeat by accident.
     */
    fun onRefresh() {
        if (_uiState.value.isLoading || _uiState.value.isRefreshing) return
        load(isRefresh = true)
    }

    /**
     * Single entry point into the loading → content / error state machine, used by
     * the initial load, [onRetry] and [onRefresh].
     *
     * @param isRefresh routes the in-flight flag to [SensorsUiState.isRefreshing]
     *   instead of [SensorsUiState.isLoading], and keeps the current stations while
     *   the request runs.
     */
    private fun load(isRefresh: Boolean = false) {
        viewModelScope.launch {
            _uiState.update {
                it.copy(
                    isLoading = !isRefresh,
                    isRefreshing = isRefresh,
                    isError = false,
                    errorMessage = null
                )
            }
            try {
                roll = fetchSensors()
                val userLocation = apiClient.currentUserLocation()
                val locationLabel = userLocation?.locationName?.takeIf { it.isNotBlank() }
                _uiState.update { state ->
                    state.copy(
                        sensors = state.filter.narrow(roll),
                        isLoading = false,
                        isRefreshing = false,
                        // The map badge counts *reporting* stations in the whole
                        // roll, matching the server's `active_sensors_count`: an
                        // offline node is in the list but is not coverage, and a
                        // status filter hides rows without changing what is out
                        // there.
                        overview = state.overview.copy(
                            sensorCount = roll.count { it.status == SensorStatus.ONLINE },
                            locationLabel = locationLabel ?: state.overview.locationLabel,
                            // The radius the *user* chose, which is null in "All":
                            // the request still measures from the stored position,
                            // but the badge must not print the endpoint ceiling as
                            // though it were a choice.
                            rangeKm = _uiState.value.filter.sensorsRadiusKm,
                            geofenceFraction = geofenceFraction(effectiveRangeKm()),
                            // Left at the previous value when the position is
                            // unknown, so a transient null does not blank a basemap
                            // that was already centred correctly.
                            latitude = userLocation?.latitude ?: state.overview.latitude,
                            longitude = userLocation?.longitude ?: state.overview.longitude
                        )
                    )
                }
            } catch (cancellation: CancellationException) {
                // Never treat scope cancellation as a load failure — rethrow so the
                // coroutine machinery sees it and the screen keeps its last state.
                throw cancellation
            } catch (throwable: Throwable) {
                // A failed *refresh* keeps the roll it could not replace: the error
                // screen is for having nothing to show, and after a pull there is
                // still a list. Only a refresh over an empty roll surfaces it.
                val hadContent = isRefresh && _uiState.value.sensors.isNotEmpty()
                if (hadContent) Log.w(TAG, "could not refresh the sensor roll", throwable)
                _uiState.update {
                    it.copy(
                        isLoading = false,
                        isRefreshing = false,
                        isError = !hadContent,
                        errorMessage = if (hadContent) {
                            null
                        } else {
                            throwable.message ?: LOAD_ERROR_MESSAGE
                        }
                    )
                }
            }
        }
    }

    /**
     * Fetches the station roll and maps it to the display model.
     *
     * `getOrThrow()` re-raises the client's [Result] failure so the `try`/`catch`
     * above stays the single place that turns a failure into UI state. Unlike the
     * events feed this endpoint *requires* a token, so a failed bootstrap surfaces
     * here as the same error state.
     */
    private suspend fun fetchSensors(): List<SensorStationItem> =
        apiClient.fetchSensors(rangeKm = effectiveRangeKm()).getOrThrow().toStationItems()

    /**
     * The radius sent as `range_km`.
     *
     * "All" is not "unfiltered": the endpoint always measures from the position the
     * server holds, so the widest honest answer is its own 500 km ceiling — which is
     * also why "Near" uses [QuakeFilterState.sensorsRadiusKm] rather than the raw
     * choice: a 1000 km browse radius is legal on `/events` and rejected here, so it
     * is clamped, and the sheet says so rather than letting the tab quietly answer a
     * narrower question than the one on screen.
     */
    private fun effectiveRangeKm(): Int =
        _uiState.value.filter.sensorsRadiusKm ?: QuakeApiClient.MAX_SENSOR_RANGE_KM

    /** Coverage circle radius as a fraction of the map card, matching Settings. */
    private fun geofenceFraction(rangeKm: Int): Float {
        val span = (MAP_RANGE_CEILING_KM - SafetyPolicy.SENSORS_NEAR_RADIUS_KM).toFloat()
        val progress = (rangeKm - SafetyPolicy.SENSORS_NEAR_RADIUS_KM)
            .coerceAtLeast(0) / span
        return MIN_GEOFENCE_FRACTION +
            progress.coerceIn(0f, 1f) * (MAX_GEOFENCE_FRACTION - MIN_GEOFENCE_FRACTION)
    }

    /**
     * Adopts the shared filter, re-narrowing the roll and re-querying it when the
     * radius changed.
     *
     * Pushed in by [SensorsRoute] from
     * [id.web.quakealert.ui.common.QuakeFilterViewModel], the same instance the
     * History tab reads, so switching tabs never changes the question being asked.
     *
     * Only the radius reaches `/sensors`, and the sheet offers this tab nothing else
     * that could: a station has no intensity and no time of occurrence, so those
     * criteria are not shown here rather than silently ignored.
     *
     * The radius is applied server-side (`ST_DWithin` around the stored position),
     * so it re-queries rather than hiding rows: a station omitted from the wide page
     * cannot be recovered locally, and distance is not in the response. Station
     * status is the opposite case and is applied locally.
     */
    fun applyFilter(filter: QuakeFilterState) {
        val current = _uiState.value.filter
        if (current == filter) return
        _uiState.update { it.copy(filter = filter, sensors = filter.narrow(roll)) }
        // Only the radius is answered by the server. A station-status change is a
        // question about the roll already in hand, so re-querying for it would blank
        // the list and spend a request to reach the same rows.
        if (filter.sensorsRadiusKm != current.sensorsRadiusKm) load()
    }

    /**
     * Placeholder hook for tapping a station card. The tapped [item] is accepted
     * now so the call site does not change once a station-detail destination
     * exists; it is deliberately unused until then.
     */
    @Suppress("UNUSED_PARAMETER")
    fun onSensorClicked(item: SensorStationItem) {
        // Intentionally empty until a station-detail screen is implemented.
    }

    private companion object {
        const val TAG = "SensorsViewModel"

        /** "All" spans the endpoint ceiling, so the circle is drawn against that. */
        const val MAP_RANGE_CEILING_KM = QuakeApiClient.MAX_SENSOR_RANGE_KM
        const val MIN_GEOFENCE_FRACTION = 0.35f
        const val MAX_GEOFENCE_FRACTION = 0.95f

        /** Fallback copy when a load failure carries no message of its own. */
        const val LOAD_ERROR_MESSAGE =
            "Could not reach the sensor network. Check your connection and try again."
    }
}
