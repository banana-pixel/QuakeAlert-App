package id.web.quakealert.domain

/**
 * A sensor station in the network as reported by `GET /api/v1/sensors`
 * (`Station` in contracts/openapi/openapi.yaml).
 *
 * Telemetry fields are nullable because the contract marks them optional: the
 * server omits them for a station that has never reported a heartbeat, and an
 * offline station's last-known values are not meaningful. Rendering the "—"
 * placeholders for those cases belongs to
 * [id.web.quakealert.data.network.mapper.toStationItem], not here.
 *
 * @param online true when the server reports status "Online". Stored as a boolean
 *   rather than the raw string so an unexpected value can never leak into the UI
 *   as a third, unhandled state.
 * @param verified operator confirmation (migration 000005): false means the node
 *   renders as Pending — visible, but not trusted infrastructure and never counted
 *   among active sensors.
 * @param lastPing human-readable relative time straight from the server
 *   (e.g. "33s ago") — the server owns this wording so every client agrees.
 * @param rssiDbm signal strength in dBm (negative integer).
 * @param latencyMs broker round-trip latency in milliseconds.
 */
data class SensorNode(
    val stationId: String,
    val sensorModel: String,
    val locationName: String,
    val latitude: Double,
    val longitude: Double,
    val online: Boolean,
    val verified: Boolean = false,
    val lastPing: String?,
    val rssiDbm: Int?,
    val latencyMs: Int?
)
