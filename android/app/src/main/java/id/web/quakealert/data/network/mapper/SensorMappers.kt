package id.web.quakealert.data.network.mapper

import id.web.quakealert.data.network.model.SensorDto
import id.web.quakealert.domain.ProvisionedNode
import id.web.quakealert.data.network.model.ProvisionResponseDto
import id.web.quakealert.domain.SensorNode
import id.web.quakealert.ui.sensors.SensorStationItem
import id.web.quakealert.ui.sensors.SensorStatus
import id.web.quakealert.ui.sensors.SensorTelemetry

/** Server's word for a reporting station (`Station.status` is "Online"/"Offline"). */
private const val STATUS_ONLINE = "Online"

/**
 * Server's word for a provisioned node awaiting operator confirmation
 * (`Station.status` is "Pending", migration 000005). Checked before [STATUS_ONLINE]:
 * trust is the question asked before health, so an unverified node never renders as
 * Online no matter how fresh its heartbeat.
 */
private const val STATUS_PENDING = "Pending"

/**
 * Wire → domain.
 *
 * `status` becomes a boolean here so an unexpected value cannot reach the UI as a
 * third, unrendered state — anything that is not exactly "Online" counts as down,
 * which is the safe direction for a network-health readout.
 */
fun SensorDto.toDomain(): SensorNode = SensorNode(
    stationId = stationId,
    sensorModel = sensorModel,
    locationName = locationName,
    latitude = latitude,
    longitude = longitude,
    online = status.equals(STATUS_ONLINE, ignoreCase = true),
    verified = verified,
    lastPing = lastPing?.takeIf { it.isNotBlank() },
    rssiDbm = rssiDbm,
    latencyMs = latencyMs
)

fun List<SensorDto>.toDomain(): List<SensorNode> = map { it.toDomain() }

/**
 * Provisioning wire → domain. Field-for-field: the response carries exactly the
 * handoff the node's `/config` portal needs, and nothing here is defaulted away —
 * a missing broker or secret must fail loudly, not silently configure a node that
 * can never connect.
 */
fun ProvisionResponseDto.toDomain(): ProvisionedNode = ProvisionedNode(
    stationId = stationId,
    provisioningSecret = provisioningSecret,
    mqttBroker = mqttBroker,
    mqttPort = mqttPort,
    mqttTls = mqttTls
)

/**
 * Domain → the Sensors screen's UI model.
 *
 * Telemetry pills keep the design's `"Label : value"` shape, and an offline or
 * never-reporting station renders dashes rather than stale numbers — an "RSSI :
 * -61 dBm" pill under a red "Offline" chip would imply a link that is not there.
 *
 * [SensorNode.lastPing] is passed through verbatim: the relative wording ("33s
 * ago") is the server's, so every client agrees on how fresh a heartbeat looks.
 */
fun SensorNode.toStationItem(): SensorStationItem = SensorStationItem(
    // station_id is unique in `iot_nodes`, so it doubles as the list key.
    id = stationId,
    stationId = stationId,
    location = locationName,
    chipLabel = sensorModel,
    status = when {
        !verified -> SensorStatus.PENDING
        online -> SensorStatus.ONLINE
        else -> SensorStatus.OFFLINE
    },
    // Null Island is treated as "no fix", not as a location. The contract defaults
    // both coordinates to 0.0 for a node provisioned without one, and a station dot
    // 5000 km off West Africa is a lie the map would tell confidently; a station
    // genuinely at 0,0 is in open ocean, so nothing real is lost.
    latitude = latitude.takeUnless { hasNoFix() },
    longitude = longitude.takeUnless { hasNoFix() },
    telemetry = SensorTelemetry(
        lastPing = "Last Ping : ${lastPing.orDash()}",
        rssi = "RSSI : ${rssiDbm?.toString().orDash()} dBm",
        latency = "Latency : ${latencyMs?.toString().orDash()} ms"
    )
)

fun List<SensorNode>.toStationItems(): List<SensorStationItem> = map { it.toStationItem() }

/** See the coordinate comment in [toStationItem]. */
private fun SensorNode.hasNoFix(): Boolean = latitude == 0.0 && longitude == 0.0

private fun String?.orDash(): String =
    this?.takeIf { it.isNotBlank() } ?: QuakeFormat.UNAVAILABLE
