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

/**
 * The whole `/sensors` answer, not just its rows.
 *
 * The envelope's `range_km` and `active_sensors_count` used to be parsed and then
 * thrown away while the ViewModel recounted stations client-side — letting the map
 * badge claim a radius or a count the request never produced. Carrying the envelope
 * through makes the server's own numbers the single source for both, and
 * [hasStoredLocation] lets the health monitor distinguish "empty because the user
 * shares no position" from "empty because the fleet is silent".
 */
data class SensorNetworkSnapshot(
    /** The radius the *server* filtered by, in kilometres. */
    val rangeKm: Int,
    /** Stations the server counts as active — trusted and reporting within [rangeKm]. */
    val activeSensorsCount: Int,
    /** Every station in range, pending ones included. */
    val nodes: List<SensorNode>,
    /** Whether the caller holds a stored position at all. */
    val hasStoredLocation: Boolean
)

/**
 * A freshly minted node identity, as returned by `POST /api/v1/nodes/provision`.
 *
 * Everything here exists to be handed to the node's `/config` portal during the
 * wizard session — none of it is cached by the app.
 *
 * @param stationId the node's permanent identity: its MQTT topic suffix and its
 *   vote key in consensus. Born `verified = false` (Pending) until an operator
 *   confirms it via the admin endpoint.
 * @param provisioningSecret the per-node HMAC signing key, displayed exactly once
 *   (the server keeps only ciphertext). Must reach the node's NVS during this session.
 * @param mqttBroker broker hostname the node should connect to; server-owned so a
 *   fleet can be re-pointed without a reflash.
 * @param mqttPort broker port, paired with [mqttTls].
 * @param mqttTls whether the node should speak MQTTS or plaintext (dev brokers only).
 */
data class ProvisionedNode(
    val stationId: String,
    val provisioningSecret: String,
    val mqttBroker: String,
    val mqttPort: Int,
    val mqttTls: Boolean
)
