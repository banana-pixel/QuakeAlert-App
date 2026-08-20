package id.web.quakealert.ui.sensors

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import id.web.quakealert.data.AppSettingsRepository
import id.web.quakealert.data.UnitSystem
import id.web.quakealert.ui.common.QuakeFilter
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/**
 * Hosts the [SensorsUiState] for the Sensors screen and exposes it as a
 * [StateFlow] following unidirectional data flow. Seeded with mock data
 * mirroring the Figma design (node 1:1081) so the UI can be verified visually
 * before a real sensor-network data source is wired in.
 *
 * The persisted [UnitSystem] from [AppSettingsRepository] is folded into every
 * emission so the map's range badge and the "Near" filter pill render the same
 * unit the user picked in Settings.
 */
class SensorsViewModel(application: Application) : AndroidViewModel(application) {

    private val repository = AppSettingsRepository(application)

    private val _uiState = MutableStateFlow(SensorsUiState(isLoading = true))

    val uiState: StateFlow<SensorsUiState> = combine(
        repository.unitSystem,
        _uiState
    ) { unit, state -> state.copy(unitSystem = unit) }.stateIn(
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
     * both the initial load and [onRetry]. [fetchSensors] is the only seam a real
     * REST/WS repository has to replace.
     */
    private fun load() {
        viewModelScope.launch {
            _uiState.update {
                it.copy(isLoading = true, isError = false, errorMessage = null)
            }
            try {
                val sensors = fetchSensors()
                _uiState.update { it.copy(sensors = sensors, isLoading = false) }
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
     * Fetches the sensor station roll. Currently returns the Figma-mirroring mock
     * fixture; `suspend` so swapping in the REST/WS source is a body change rather
     * than a signature change.
     */
    private suspend fun fetchSensors(): List<SensorStationItem> = mockSensors()

    /** Switches between the "All" and "Near" filter pills. */
    fun onFilterSelected(filter: QuakeFilter) {
        _uiState.update { it.copy(selectedFilter = filter) }
    }

    /** Placeholder hook for the calendar/date-range picker button. */
    fun onCalendarClicked() {
        // Intentionally empty until a date-range picker is implemented.
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
        /** Fallback copy when a load failure carries no message of its own. */
        const val LOAD_ERROR_MESSAGE =
            "Could not reach the sensor network. Check your connection and try again."

        private val OFFLINE_TELEMETRY = SensorTelemetry(
            lastPing = "Last Ping : - s ago",
            rssi = "RSSI : - dBm",
            latency = "Latency : - ms"
        )

        fun mockSensors(): List<SensorStationItem> = listOf(
            SensorStationItem(
                id = "1",
                stationId = "NODE-163A149F",
                location = "Cimahi, West Java, ID",
                chipLabel = "MPU 6050",
                status = SensorStatus.ONLINE,
                telemetry = SensorTelemetry(
                    lastPing = "Last Ping : 33s ago",
                    rssi = "RSSI : -61 dBm",
                    latency = "Latency : 2 ms"
                )
            ),
            SensorStationItem(
                id = "2",
                stationId = "NODE-53FC66GH",
                location = "Bandung, West Java, ID",
                chipLabel = "MPU 6050",
                status = SensorStatus.OFFLINE,
                telemetry = OFFLINE_TELEMETRY
            ),
            SensorStationItem(
                id = "3",
                stationId = "NODE-53FC66GH",
                location = "Bandung, West Java, ID",
                chipLabel = "MPU 6050",
                status = SensorStatus.ONLINE,
                telemetry = SensorTelemetry(
                    lastPing = "Last Ping : 20 s ago",
                    rssi = "RSSI : -53 dBm",
                    latency = "Latency : 5 ms"
                )
            ),
            SensorStationItem(
                id = "4",
                stationId = "NODE-8D2B7E4C",
                location = "Jakarta, DKI Jakarta, ID",
                chipLabel = "MPU 6050",
                status = SensorStatus.ONLINE,
                telemetry = SensorTelemetry(
                    lastPing = "Last Ping : 12s ago",
                    rssi = "RSSI : -48 dBm",
                    latency = "Latency : 3 ms"
                )
            ),
            SensorStationItem(
                id = "5",
                stationId = "NODE-9F3E2D1A",
                location = "Surabaya, East Java, ID",
                chipLabel = "MPU 6050",
                status = SensorStatus.OFFLINE,
                telemetry = OFFLINE_TELEMETRY
            ),
            SensorStationItem(
                id = "6",
                stationId = "NODE-B7295C8F",
                location = "Yogyakarta, Special Region of Yogyakarta, ID",
                chipLabel = "MPU 6050",
                status = SensorStatus.ONLINE,
                telemetry = SensorTelemetry(
                    lastPing = "Last Ping : 7s ago",
                    rssi = "RSSI : -55 dBm",
                    latency = "Latency : 4 ms"
                )
            ),
            SensorStationItem(
                id = "7",
                stationId = "NODE-1A4D3C7E",
                location = "Medan, North Sumatra, ID",
                chipLabel = "MPU 6050",
                status = SensorStatus.ONLINE,
                telemetry = SensorTelemetry(
                    lastPing = "Last Ping : 15s ago",
                    rssi = "RSSI : -60 dBm",
                    latency = "Latency : 6 ms"
                )
            )
        )
    }
}
