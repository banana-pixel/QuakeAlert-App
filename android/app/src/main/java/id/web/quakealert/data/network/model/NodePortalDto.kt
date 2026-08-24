package id.web.quakealert.data.network.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

/**
 * Body of the node's `POST /config` portal call (firmware/src/network.cpp).
 *
 * Field-for-field with what the portal accepts: Wi-Fi credentials, placement,
 * the per-node HMAC key minted by provisioning, and — the wizard's handoff —
 * the identity plus broker endpoint the server issued.
 */
@Serializable
data class NodePortalConfigDto(
    @SerialName("ssid") val ssid: String,
    @SerialName("password") val password: String,
    @SerialName("lat") val latitude: Double,
    @SerialName("lon") val longitude: Double,
    @SerialName("hmac_key") val hmacKey: String,
    @SerialName("station_id") val stationId: String? = null,
    @SerialName("mqtt_broker") val mqttBroker: String? = null,
    @SerialName("mqtt_port") val mqttPort: Int? = null,
    @SerialName("mqtt_tls") val mqttTls: Boolean? = null
)

/**
 * Response of the portal's `/config`. [stationId] is the node's *effective*
 * identity — its own NVS value after this write — and is authoritative for the
 * confirm step even when it differs from what provisioning minted.
 */
@Serializable
data class NodePortalConfigResponseDto(
    @SerialName("status") val status: String,
    @SerialName("message") val message: String? = null,
    @SerialName("station_id") val stationId: String? = null
)
