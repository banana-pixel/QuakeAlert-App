package id.web.quakealert.data.network.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

/**
 * One sensor station from `GET /api/v1/sensors` (server `stationDTO`).
 *
 * The telemetry trio is optional in the contract, so all three default to null: a
 * freshly provisioned node that has never sent a heartbeat simply omits them.
 *
 * Note there is **no battery field** — the ESP32 nodes are mains-powered and the
 * contract exposes link health ([rssiDbm], [latencyMs]) rather than charge.
 *
 * @param status "Online" | "Offline" | "Pending", capitalised exactly as the
 *   server sends it. "Pending" marks a provisioned node the operator has not yet
 *   confirmed (migration 000005): it heartbeats, but its readings never reach
 *   consensus and it is excluded from `active_sensors_count`.
 * @param verified operator confirmation state; the server always sends it, so the
 *   false default only fires on a malformed payload — trust is never granted by
 *   absence.
 * @param lastPing human-readable relative time owned by the server, e.g. "33s ago".
 * @param rssiDbm signal strength in dBm (negative).
 */
@Serializable
data class SensorDto(
    @SerialName("station_id") val stationId: String,
    @SerialName("sensor_model") val sensorModel: String = "",
    @SerialName("location_name") val locationName: String = "",
    @SerialName("latitude") val latitude: Double = 0.0,
    @SerialName("longitude") val longitude: Double = 0.0,
    @SerialName("status") val status: String,
    @SerialName("last_ping") val lastPing: String? = null,
    @SerialName("rssi_dbm") val rssiDbm: Int? = null,
    @SerialName("latency_ms") val latencyMs: Int? = null,
    @SerialName("verified") val verified: Boolean = false
)

/**
 * Envelope of `GET /api/v1/sensors`.
 *
 * @param rangeKm radius the server filtered by, relative to the user's stored
 *   location (default 50 km).
 * @param activeSensorsCount stations the server counts as online — kept from the
 *   envelope rather than recomputed client-side so the map badge and the list
 *   cannot disagree about how many nodes are up.
 */
@Serializable
data class SensorsResponseDto(
    @SerialName("range_km") val rangeKm: Int = 0,
    @SerialName("active_sensors_count") val activeSensorsCount: Int = 0,
    @SerialName("stations") val stations: List<SensorDto> = emptyList()
)
