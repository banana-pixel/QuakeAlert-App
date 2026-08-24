package id.web.quakealert.data.network.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

/**
 * Request body of `POST /api/v1/nodes/provision`.
 *
 * `sensorModel` is a display label rendered on the station chip ("MPU 6050");
 * `locationName` is the public place name shown inside alerts and validated by the
 * server against its "Kecamatan, Kabupaten/Kota, Provinsi" convention — the wizard
 * validates the same rules client-side before ever sending this.
 */
@Serializable
data class ProvisionRequestDto(
    @SerialName("station_id") val stationId: String? = null,
    @SerialName("sensor_model") val sensorModel: String,
    @SerialName("location_name") val locationName: String,
    @SerialName("latitude") val latitude: Double,
    @SerialName("longitude") val longitude: Double
)

/**
 * Response of `POST /api/v1/nodes/provision` (`provisionResponse` in
 * `server/internal/api/api.go`).
 *
 * [provisioningSecret] is the per-node HMAC key and is displayed **exactly once** —
 * the server stores only ciphertext, so neither it nor anyone else can show it
 * again. It must reach the node's NVS via the local `/config` portal during this
 * wizard session; losing it means reprovisioning.
 */
@Serializable
data class ProvisionResponseDto(
    @SerialName("station_id") val stationId: String,
    @SerialName("provisioning_secret") val provisioningSecret: String,
    @SerialName("mqtt_broker") val mqttBroker: String,
    @SerialName("mqtt_port") val mqttPort: Int,
    @SerialName("mqtt_tls") val mqttTls: Boolean = true
)
