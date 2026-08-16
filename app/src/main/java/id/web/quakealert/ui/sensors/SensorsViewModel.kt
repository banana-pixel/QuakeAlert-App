package id.web.quakealert.ui.sensors

import androidx.lifecycle.ViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update

/**
 * Hosts the [SensorsUiState] for the Sensors screen and exposes it as a
 * [StateFlow] following unidirectional data flow. Seeded with mock data
 * mirroring the Figma design (node 1:1081) so the UI can be verified visually
 * before a real sensor-network data source is wired in.
 */
class SensorsViewModel : ViewModel() {

    private val _uiState = MutableStateFlow(SensorsUiState(sensors = mockSensors()))
    val uiState: StateFlow<SensorsUiState> = _uiState.asStateFlow()

    /** Switches between the "All" and "Near" filter pills. */
    fun onFilterSelected(filter: SensorFilter) {
        _uiState.update { it.copy(selectedFilter = filter) }
    }

    /** Placeholder hook for the calendar/date-range picker button. */
    fun onCalendarClicked() {
        // Intentionally empty until a date-range picker is implemented.
    }

    /** Placeholder hook for tapping a station card. */
    fun onSensorClicked(item: SensorStationItem) {
        // Intentionally empty until a station-detail screen is implemented.
    }

    private companion object {
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
