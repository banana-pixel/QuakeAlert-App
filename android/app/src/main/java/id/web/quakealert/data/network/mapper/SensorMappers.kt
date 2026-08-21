package id.web.quakealert.data.network.mapper

import id.web.quakealert.data.network.model.SensorDto
import id.web.quakealert.domain.SensorNode
import id.web.quakealert.ui.sensors.SensorStationItem
import id.web.quakealert.ui.sensors.SensorStatus
import id.web.quakealert.ui.sensors.SensorTelemetry

/** Server's word for a reporting station (`Station.status` is "Online"/"Offline"). */
private const val STATUS_ONLINE = "Online"

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
    lastPing = lastPing?.takeIf { it.isNotBlank() },
    rssiDbm = rssiDbm,
    latencyMs = latencyMs
)

fun List<SensorDto>.toDomain(): List<SensorNode> = map { it.toDomain() }

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
    status = if (online) SensorStatus.ONLINE else SensorStatus.OFFLINE,
    telemetry = SensorTelemetry(
        lastPing = "Last Ping : ${lastPing.orDash()}",
        rssi = "RSSI : ${rssiDbm?.toString().orDash()} dBm",
        latency = "Latency : ${latencyMs?.toString().orDash()} ms"
    )
)

fun List<SensorNode>.toStationItems(): List<SensorStationItem> = map { it.toStationItem() }

private fun String?.orDash(): String =
    this?.takeIf { it.isNotBlank() } ?: QuakeFormat.UNAVAILABLE
