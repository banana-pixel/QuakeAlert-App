package id.web.quakealert.ui.sensors

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import id.web.quakealert.data.AppSettingsRepository
import id.web.quakealert.data.UnitSystem
import id.web.quakealert.data.network.QuakeApiClient
import id.web.quakealert.data.network.QuakeNetwork
import id.web.quakealert.data.network.mapper.toStationItems
import id.web.quakealert.domain.SafetyPolicy
import id.web.quakealert.ui.common.QuakeFilter
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.drop
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

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
     * Single entry point into the loading → content / error state machine, used by
     * both the initial load and [onRetry].
     */
    private fun load() {
        viewModelScope.launch {
            _uiState.update {
                it.copy(isLoading = true, isError = false, errorMessage = null)
            }
            try {
                val sensors = fetchSensors()
                val locationLabel = apiClient.currentUserLocation()?.locationName?.takeIf {
                    it.isNotBlank()
                }
                _uiState.update { state ->
                    state.copy(
                        sensors = sensors,
                        isLoading = false,
                        // The map badge counts *reporting* stations, matching the
                        // server's `active_sensors_count`; an offline node is in the
                        // list but is not coverage.
                        overview = state.overview.copy(
                            sensorCount = sensors.count { it.status == SensorStatus.ONLINE },
                            locationLabel = locationLabel ?: state.overview.locationLabel,
                            rangeKm = effectiveRangeKm(),
                            geofenceFraction = geofenceFraction(effectiveRangeKm())
                        )
                    )
                }
            } catch (cancellation: CancellationException) {
                // Never treat scope cancellation as a load failure — rethrow so the
                // coroutine machinery sees it and the screen keeps its last state.
                throw cancellation
            } catch (throwable: Throwable) {
                _uiState.update {
                    it.copy(
                        isLoading = false,
                        isError = true,
                        errorMessage = throwable.message ?: LOAD_ERROR_MESSAGE
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
     * server holds, so the widest honest answer is its own 500 km ceiling. "Near"
     * narrows to [SafetyPolicy.SENSORS_NEAR_RADIUS_KM], deliberately tighter than the
     * 200 km alert radius — this list answers "what is watching my area", and a
     * station 200 km away is not meaningfully watching it.
     */
    private fun effectiveRangeKm(): Int = when (_uiState.value.selectedFilter) {
        QuakeFilter.ALL -> QuakeApiClient.MAX_SENSOR_RANGE_KM
        QuakeFilter.NEAR -> SafetyPolicy.SENSORS_NEAR_RADIUS_KM
    }

    /** Coverage circle radius as a fraction of the map card, matching Settings. */
    private fun geofenceFraction(rangeKm: Int): Float {
        val span = (MAP_RANGE_CEILING_KM - SafetyPolicy.SENSORS_NEAR_RADIUS_KM).toFloat()
        val progress = (rangeKm - SafetyPolicy.SENSORS_NEAR_RADIUS_KM)
            .coerceAtLeast(0) / span
        return MIN_GEOFENCE_FRACTION +
            progress.coerceIn(0f, 1f) * (MAX_GEOFENCE_FRACTION - MIN_GEOFENCE_FRACTION)
    }

    /**
     * Switches between the "All" and "Near" filter pills, reloading the roll.
     *
     * The filter is applied server-side (`ST_DWithin` around the stored position),
     * so it re-queries rather than hiding rows: a station omitted from the wide
     * page cannot be recovered locally, and distance is not in the response.
     */
    fun onFilterSelected(filter: QuakeFilter) {
        if (_uiState.value.selectedFilter == filter) return
        _uiState.update { it.copy(selectedFilter = filter) }
        load()
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
        /** "All" spans the endpoint ceiling, so the circle is drawn against that. */
        const val MAP_RANGE_CEILING_KM = QuakeApiClient.MAX_SENSOR_RANGE_KM
        const val MIN_GEOFENCE_FRACTION = 0.35f
        const val MAX_GEOFENCE_FRACTION = 0.95f

        /** Fallback copy when a load failure carries no message of its own. */
        const val LOAD_ERROR_MESSAGE =
            "Could not reach the sensor network. Check your connection and try again."
    }
}
