package id.web.quakealert.ui.sensors

import androidx.compose.runtime.Immutable
import id.web.quakealert.ui.common.QuakeFilter

/**
 * Connectivity state of a sensor station, driving the coloured status chip on a
 * [SensorItemCard]. The Figma design (node 1:1081) shows a green "Online" and a
 * red "Offline" treatment.
 */
enum class SensorStatus { ONLINE, OFFLINE }

/**
 * Live telemetry read-outs shown as pills on a station card. When a station is
 * offline these are rendered as placeholder dashes in the mock data.
 *
 * @param lastPing e.g. "Last Ping : 33s ago".
 * @param rssi e.g. "RSSI : -61 dBm".
 * @param latency e.g. "Latency : 2 ms".
 */
@Immutable
data class SensorTelemetry(
    val lastPing: String,
    val rssi: String,
    val latency: String
)

/**
 * A single sensor station row (Figma node 1:1111).
 *
 * @param id stable identity for list keys.
 * @param stationId station identifier suffix (e.g. "NODE-163A149F").
 * @param location human-readable placement (e.g. "Cimahi, West Java, ID").
 * @param chipLabel sensor module label rendered inside the chip badge (e.g. "MPU 6050").
 * @param status online/offline connectivity.
 * @param telemetry live metric pills.
 */
@Immutable
data class SensorStationItem(
    val id: String,
    val stationId: String,
    val location: String,
    val chipLabel: String,
    val status: SensorStatus,
    val telemetry: SensorTelemetry
)

/**
 * Summary overlay data for the map preview card (Figma node 1:1091), shared by
 * the Sensors screen and the Settings "Location & Coverage" section. The two
 * screens render the *same* linked map: identical location pill, range summary
 * and reactive coverage [geofenceFraction]. The only difference is the Sensors
 * screen's bottom-right settings shortcut, which Settings hides.
 *
 * @param locationLabel user-centred location pill (e.g. "Bandung, West Java, ID").
 * @param rangeKm covered radius in kilometres.
 * @param sensorCount number of sensors within range.
 * @param geofenceFraction radius of the reactive coverage circle as a fraction
 *   (0f..1f) of the card's minimum side, so the visualised radius scales with the
 *   selected coverage.
 */
@Immutable
data class SensorMapOverview(
    val locationLabel: String,
    val rangeKm: Int,
    val sensorCount: Int,
    val geofenceFraction: Float = 0.9f
) {
    /** Pre-formatted "Range : {km} km, {n} sensors" summary badge text. */
    val summaryLabel: String
        get() = "Range : $rangeKm km, $sensorCount sensors"
}


/**
 * Immutable UI state for the Sensors screen (Figma node 1:1081). Hoisted into
 * [SensorsViewModel] and consumed by the stateless [SensorsScreen]. The filter
 * uses the shared [QuakeFilter] enum common to History and Sensors.
 */
@Immutable
data class SensorsUiState(
    val isHealthy: Boolean = true,
    val overview: SensorMapOverview = SensorMapOverview(
        locationLabel = "Bandung, West Java, ID",
        rangeKm = 500,
        sensorCount = 2
    ),
    val selectedFilter: QuakeFilter = QuakeFilter.ALL,
    val nearRadiusKm: Int = 39,
    val sensors: List<SensorStationItem> = emptyList()
)
