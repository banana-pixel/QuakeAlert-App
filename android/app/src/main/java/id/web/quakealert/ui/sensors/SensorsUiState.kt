package id.web.quakealert.ui.sensors

import androidx.compose.runtime.Immutable
import id.web.quakealert.data.UnitSystem
import id.web.quakealert.ui.common.ErrorCopy
import id.web.quakealert.ui.common.MapFocus
import id.web.quakealert.ui.common.MapMarker
import id.web.quakealert.ui.common.MapMarkerKind
import id.web.quakealert.ui.common.QuakeFilterState

/**
 * Trust/health state of a sensor station, driving the coloured status chip on a
 * [SensorItemCard]. The Figma design (node 1:1081) shows a green "Online" and a
 * red "Offline" treatment; [PENDING] is the third state migration 000005 added —
 * a provisioned node awaiting operator confirmation, which heartbeats but whose
 * readings never reach consensus.
 */
enum class SensorStatus { ONLINE, OFFLINE, PENDING }

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
 * @param latitude WGS84 latitude of the station, or null when the server holds no
 *   fix for it. Nullable rather than defaulted: `GET /sensors` answers 0.0 for a
 *   node that was provisioned without coordinates, and a dot at 0,0 would put the
 *   station in the Gulf of Guinea — a map that is confidently wrong is worse than a
 *   row with no dot, which is what a null produces.
 * @param longitude WGS84 longitude; see [latitude].
 */
@Immutable
data class SensorStationItem(
    val id: String,
    val stationId: String,
    val location: String,
    val chipLabel: String,
    val status: SensorStatus,
    val telemetry: SensorTelemetry,
    val latitude: Double? = null,
    val longitude: Double? = null
) {
    /** True when the station can be pinned on the map and focused. */
    val hasPosition: Boolean get() = latitude != null && longitude != null
}

/**
 * Summary overlay data for the map preview card (Figma node 1:1091), shared by
 * the Sensors screen and the Settings "Location & Coverage" section. The two
 * screens render the *same* linked map: identical location pill, range summary
 * and reactive coverage [geofenceFraction]. The only difference is the Sensors
 * screen's bottom-right settings shortcut, which Settings hides.
 *
 * @param locationLabel user-centred location pill (e.g. "Bandung, West Java, ID").
 * @param rangeKm the radius the user actually chose, in kilometres, or null when
 *   no radius is narrowing the query. Null is not a missing value to fill in with a
 *   default: printing the endpoint's own 500 km ceiling as "Range : 500 km" claims
 *   the user picked a radius they never picked, and reads as a limit on what they
 *   are being shown. [rangeLabel] says "All areas" instead.
 * @param sensorCount number of sensors within range.
 * @param geofenceFraction radius of the reactive coverage circle as a fraction
 *   (0f..1f) of the card's minimum side, so the visualised radius scales with the
 *   selected coverage.
 * @param latitude device latitude the basemap is centred on, or null when no
 *   position has ever been synced. Null rather than a default coordinate on
 *   purpose: a map confidently centred on somewhere the user is not is worse than
 *   a map that admits it has no fix, and [locationLabel] already says so.
 * @param longitude device longitude; see [latitude].
 */
@Immutable
data class SensorMapOverview(
    val locationLabel: String,
    val rangeKm: Int?,
    val sensorCount: Int,
    val geofenceFraction: Float = 0.9f,
    val latitude: Double? = null,
    val longitude: Double? = null
) {
    /**
     * The radius half of the badge, in the user's unit: "Range : 250 km" when a
     * radius is set, "All areas" when none is.
     *
     * The single place the app turns a radius into words, so every surface that
     * mentions one says it the same way: the word "Range", a colon, a number and a
     * unit, and no number at all when there is nothing to report.
     */
    fun rangeLabel(unitSystem: UnitSystem): String =
        rangeKm?.let { "Range : ${unitSystem.formatDistance(it)}" } ?: ALL_AREAS_LABEL

    /** The station-count half of the badge, pluralised. */
    val countLabel: String
        get() = if (sensorCount == 1) "1 sensor" else "$sensorCount sensors"

    /**
     * Both halves as the map badge renders them, e.g. "Range : 250 km · 4 sensors".
     *
     * Kept as two properties joined here rather than one format string: the radius
     * and the count answer different questions, and the comma that used to join them
     * read as though the count were scoped by nothing in particular.
     */
    fun summaryLabel(unitSystem: UnitSystem): String =
        "${rangeLabel(unitSystem)} · $countLabel"

    private companion object {
        /** Said instead of a radius when the query is not narrowed by one. */
        const val ALL_AREAS_LABEL = "All areas"
    }
}


/**
 * Immutable UI state for the Sensors screen (Figma node 1:1081). Hoisted into
 * [SensorsViewModel] and consumed by the stateless [SensorsScreen]. The filter
 * uses the shared [QuakeFilterState] common to History and Sensors.
 *
 * [isLoading], [isError] and [errorCopy] form the screen's state machine: the
 * station list region renders exactly one of loading / error / empty / content.
 *
 * The header's network badge is deliberately *not* derived here: it reads the global
 * [id.web.quakealert.domain.ServerConnectionState], so an empty roll or an
 * all-offline fleet no longer hides a badge that the other tabs are showing. Station
 * connectivity is expressed where the design puts it — the per-row [SensorStatus]
 * chips and the map's active-sensor count.
 *
 * @param isLoading true while the station roll is in flight for the first time,
 *   which swaps the list for a skeleton.
 * @param isRefreshing true while a pull-to-refresh is in flight. Its own flag
 *   rather than a reuse of [isLoading]: the roll the user pulled stays on screen
 *   under the indicator instead of being replaced.
 * @param isError true when the last load failed; pairs with [errorCopy].
 * @param errorCopy classified failure copy from
 *   [id.web.quakealert.ui.common.errorCopy], rendered by
 *   [id.web.quakealert.ui.common.QuakeErrorState], or null when there is no error.
 * @param selectedStationId the station whose row was tapped, or null when none is.
 *   Held as an id rather than the item so the list and the map read one fact, and so
 *   a refreshed roll cannot leave a stale copy of a station selected.
 * @param needsPosition true when nothing has synced a device position yet, so there
 *   is no query to make. `GET /sensors` measures every roll from the position the
 *   *server* holds and answers an empty list when it holds none — in every mode, not
 *   just "Near". Without this flag a first launch reads "No Sensors In This Area" and
 *   offers to widen a radius that cannot help, which blames the network for a
 *   question that was never asked.
 */
@Immutable
data class SensorsUiState(
    val isLoading: Boolean = false,
    val isRefreshing: Boolean = false,
    val isError: Boolean = false,
    val errorCopy: ErrorCopy? = null,
    val needsPosition: Boolean = false,
    // Empty rather than a sample station: the roll is genuinely unknown until
    // `GET /sensors` answers, and a placeholder count would claim coverage the
    // user may not have (the endpoint returns nothing without a stored position).
    val overview: SensorMapOverview = SensorMapOverview(
        locationLabel = "Location not set",
        // Null, not the default "Near" radius: the filter opens in "All", so a
        // number here would badge a radius the user has not chosen.
        rangeKm = null,
        sensorCount = 0
    ),
    val filter: QuakeFilterState = QuakeFilterState(),
    val unitSystem: UnitSystem = UnitSystem.METRIC,
    val sensors: List<SensorStationItem> = emptyList(),
    val selectedStationId: String? = null
)

/**
 * Every dot the Sensors map shows: one per station that has a fix, plus the device
 * position.
 *
 * Derived here rather than assembled in the composable so it can be tested without
 * a GL context, and so the map and the list cannot disagree about which station is
 * selected — both read the same [selectedStationId].
 *
 * Stations with no coordinates contribute nothing. The user dot comes from the same
 * [SensorMapOverview] coordinates the camera is pointed at, so the dot and the
 * centre cannot drift apart.
 */
fun SensorsUiState.mapMarkers(): List<MapMarker> = buildList {
    sensors.forEach { station ->
        val latitude = station.latitude ?: return@forEach
        val longitude = station.longitude ?: return@forEach
        add(
            MapMarker(
                id = station.id,
                latitude = latitude,
                longitude = longitude,
                kind = when {
                    station.id == selectedStationId -> MapMarkerKind.SELECTED
                    station.status == SensorStatus.ONLINE -> MapMarkerKind.STATION_ONLINE
                    // PENDING shares the offline dot on the map: until an operator
                    // vouches for the node it is not counted as network capacity,
                    // and the row's chip is where the trust distinction is spelled.
                    else -> MapMarkerKind.STATION_OFFLINE
                }
            )
        )
    }
    val latitude = overview.latitude
    val longitude = overview.longitude
    if (latitude != null && longitude != null) {
        add(
            MapMarker(
                id = USER_MARKER_ID,
                latitude = latitude,
                longitude = longitude,
                kind = MapMarkerKind.USER
            )
        )
    }
}

/**
 * Where the Sensors map points: the selected station when there is one, otherwise
 * the device position, otherwise nowhere.
 *
 * A selected station is framed at [MapFocus.ZOOM_EVENT] — one step in from the
 * coverage framing — because the question a tap asks is "where is *this* node",
 * not "what does my area look like".
 */
fun SensorsUiState.mapFocus(): MapFocus? {
    val selected = sensors.firstOrNull { it.id == selectedStationId }
    val selectedLatitude = selected?.latitude
    val selectedLongitude = selected?.longitude
    if (selectedLatitude != null && selectedLongitude != null) {
        return MapFocus(
            latitude = selectedLatitude,
            longitude = selectedLongitude,
            zoom = MapFocus.ZOOM_EVENT
        )
    }
    val latitude = overview.latitude ?: return null
    val longitude = overview.longitude ?: return null
    return MapFocus(latitude = latitude, longitude = longitude, zoom = MapFocus.ZOOM_COVERAGE)
}

/**
 * What the map card's location pill says: the selected station when there is one,
 * otherwise the device position the camera is resting on.
 *
 * Derived here beside [mapFocus] for the same reason: the pill names what the camera is
 * looking at, so the two must be computed from one fact. A pill left on the user's own
 * place while the camera sits on a station was the whole complaint — the map moved and
 * refused to say where to.
 */
fun SensorsUiState.mapPillLabel(): String {
    val selected = sensors.firstOrNull { it.id == selectedStationId } ?: return overview.locationLabel
    if (selected.latitude == null || selected.longitude == null) return overview.locationLabel
    return "Station ${selected.stationId}"
}

/**
 * Feature id for the device dot. A fixed string rather than a station id, because
 * there is exactly one and it must never collide with a station's key.
 */
private const val USER_MARKER_ID = "user-position"
