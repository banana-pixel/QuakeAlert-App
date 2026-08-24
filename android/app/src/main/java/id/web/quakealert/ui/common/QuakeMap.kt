package id.web.quakealert.ui.common

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxScope
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.Immutable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.toArgb
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalInspectionMode
import androidx.compose.ui.viewinterop.AndroidView
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.MapAttributionScrim
import id.web.quakealert.ui.theme.MapAttributionText
import id.web.quakealert.ui.theme.AccentBlue
import id.web.quakealert.ui.theme.MapSurfaceFallback
import id.web.quakealert.ui.theme.MicroCaption
import id.web.quakealert.ui.theme.StatusOfflineDot
import id.web.quakealert.ui.theme.StatusOnlineDot
import id.web.quakealert.ui.theme.TextPrimary
import org.maplibre.android.MapLibre
import org.maplibre.android.camera.CameraPosition
import org.maplibre.android.geometry.LatLng
import org.maplibre.android.maps.MapLibreMapOptions
import org.maplibre.android.maps.MapView
import org.maplibre.android.style.expressions.Expression
import org.maplibre.android.style.layers.CircleLayer
import org.maplibre.android.style.layers.PropertyFactory
import org.maplibre.android.style.sources.GeoJsonSource
import org.maplibre.geojson.Feature
import org.maplibre.geojson.FeatureCollection
import org.maplibre.geojson.Point

/**
 * Where a map card is pointed: a WGS84 coordinate plus the zoom that frames it.
 *
 * Its own type rather than two loose doubles because "the map has nothing to point
 * at" is a real state on every one of the three cards — the user may never have
 * synced a position, and an alert may arrive before the centroid does — and a
 * nullable [MapFocus] expresses that once instead of asking each caller to keep two
 * nullable numbers in agreement.
 *
 * @param zoom MapLibre zoom level. The three named defaults below are the only
 *   values the app uses; a card should pick one rather than invent a number, so the
 *   same kind of subject is framed the same way everywhere.
 */
@Immutable
data class MapFocus(
    val latitude: Double,
    val longitude: Double,
    val zoom: Double = ZOOM_REGION
) {
    companion object {
        /** Epicentre / single-event framing: a city and its surroundings. */
        const val ZOOM_EVENT: Double = 8.0

        /** Region framing — a province-sized area around the subject. */
        const val ZOOM_REGION: Double = 6.5

        /** Coverage framing: wide enough that a 200 km ring fits on a short card. */
        const val ZOOM_COVERAGE: Double = 5.5
    }
}

/**
 * What a [MapMarker] stands for, which is the only thing that decides how it is
 * drawn: an enum rather than colours and radii on the marker itself, so a station
 * dot cannot be painted one way on the Sensors map and another way in Settings.
 *
 * Declaration order is paint order (see [toCircleLayer]), so it is not arbitrary:
 * where dots overlap, a reporting station wins over a dead one, and the tapped
 * station and the device position win over both.
 */
enum class MapMarkerKind {
    /** A provisioned station that is not reporting. */
    STATION_OFFLINE,

    /** A station the server currently counts as reporting. */
    STATION_ONLINE,

    /** The station whose row the user tapped; drawn larger and on top. */
    SELECTED,

    /** The device's own last synced position. */
    USER
}

/**
 * A single dot drawn on the basemap in map coordinates.
 *
 * Deliberately *not* Compose [content]: an overlay drawn in the card's own
 * coordinate space is only correct at the exact centre of the camera, which is why
 * the coverage circle and the location pill can stay Compose but a station 40 km
 * east of the user cannot. These go through MapLibre so they stay pinned to the
 * ground the camera is looking at.
 *
 * @param id stable feature identity, used only to keep the collection diffable.
 * @param latitude WGS84 latitude; callers must not pass a placeholder.
 * @param longitude WGS84 longitude.
 * @param kind drives colour and size; see [MapMarkerKind].
 */
@Immutable
data class MapMarker(
    val id: String,
    val latitude: Double,
    val longitude: Double,
    val kind: MapMarkerKind
)

/**
 * Vector basemap behind the app's map cards, replacing the grey placeholder
 * surface the screens stood on before the SDK landed.
 *
 * Three deliberate constraints:
 *
 *  1. **Non-interactive.** Every gesture is disabled. All three cards live inside a
 *     scrolling parent, and a map that pans would swallow the scroll the user
 *     actually meant — the History and Sensors feeds are the primary content, not
 *     the thumbnail. It also means the basemap never drifts away from the overlays
 *     drawn on top of it, which is what lets those overlays stay plain Compose.
 *  2. **Overlays stay in Compose.** Callers draw the epicentre rings, coverage
 *     circle, pins and badges as [content] in the same [Box], centred on the card —
 *     which *is* [focus], because the camera is locked there. Rendering them as
 *     MapLibre annotations would need a second dependency and would put the design
 *     system's tokens behind a style JSON.
 *  3. **Honest when offline.** A null [focus], a preview, or tiles that never
 *     arrive all leave [MapSurfaceFallback] showing rather than a blank white
 *     rectangle. The card keeps its shape and its overlays either way, so a user
 *     reading a warning with no signal still sees the distance and intensity that
 *     matter; only the terrain behind them is missing.
 *
 * @param focus where to point the camera, or null when nothing is known yet — in
 *   which case no GL surface is created at all.
 * @param attributionAlignment corner for the required provider credit. Defaults to
 *   bottom-end; cards that already own that corner move it.
 * @param markers dots pinned to the ground rather than to the card — stations, the
 *   device position — drawn as MapLibre circle layers. Empty by default, so a card
 *   that has nothing to pin (the event-detail map, which is already centred on its
 *   one subject) pays nothing and renders exactly as before.
 * @param content overlays drawn over the basemap, in the same coordinate space as
 *   the card.
 */
@Composable
fun QuakeMap(
    focus: MapFocus?,
    modifier: Modifier = Modifier,
    attributionAlignment: Alignment = Alignment.BottomEnd,
    markers: List<MapMarker> = emptyList(),
    content: @Composable BoxScope.() -> Unit = {}
) {
    Box(modifier = modifier.background(MapSurfaceFallback)) {
        // LocalInspectionMode: Android Studio's preview renderer has no GL context,
        // so a MapView there fails the whole preview rather than the map card.
        val canRender = focus != null && !LocalInspectionMode.current
        if (canRender) {
            Basemap(
                focus = focus!!,
                markers = markers,
                modifier = Modifier.matchParentSize()
            )
            MapAttribution(modifier = Modifier.align(attributionAlignment))
        }
        content()
    }
}

/** The GL surface itself, lifted out so [QuakeMap] reads as composition. */
@Composable
private fun Basemap(
    focus: MapFocus,
    markers: List<MapMarker>,
    modifier: Modifier = Modifier
) {
    val mapView = rememberMapView()

    AndroidView(
        factory = {
            mapView.apply {
                getMapAsync { map ->
                    map.uiSettings.apply {
                        // See constraint 1 above. The logo and attribution widgets
                        // are MapLibre's own chrome; the credit is re-rendered as
                        // [MapAttribution] in the app's own type instead.
                        setAllGesturesEnabled(false)
                        isLogoEnabled = false
                        isAttributionEnabled = false
                        isCompassEnabled = false
                    }
                    map.setStyle(BASEMAP_STYLE_URL) { style ->
                        // Source and layers are created once, here, and only ever
                        // re-fed below: adding a layer per emission would stack
                        // duplicates and throw on the second identical id.
                        style.addSource(
                            GeoJsonSource(MARKER_SOURCE_ID, markers.toFeatureCollection())
                        )
                        MapMarkerKind.entries.forEach { style.addLayer(it.toCircleLayer()) }
                    }
                }
            }
        },
        update = { view ->
            view.getMapAsync { map ->
                map.cameraPosition = CameraPosition.Builder()
                    .target(LatLng(focus.latitude, focus.longitude))
                    .zoom(focus.zoom)
                    .build()
                // getStyle's callback fires only once the style has finished
                // loading, so a marker list that arrives before the tiles do is
                // applied when they land instead of being dropped on the floor.
                map.getStyle { style ->
                    style.getSourceAs<GeoJsonSource>(MARKER_SOURCE_ID)
                        ?.setGeoJson(markers.toFeatureCollection())
                }
            }
        },
        modifier = modifier
    )
}

/**
 * Markers → GeoJSON, each carrying its [MapMarkerKind] name as a feature property
 * so one source can feed every layer and a change of kind is a data change rather
 * than a layer rebuild.
 */
private fun List<MapMarker>.toFeatureCollection(): FeatureCollection =
    FeatureCollection.fromFeatures(
        map { marker ->
            Feature.fromGeometry(
                Point.fromLngLat(marker.longitude, marker.latitude),
                null,
                marker.id
            ).apply { addStringProperty(MARKER_KIND_PROPERTY, marker.kind.name) }
        }
    )

/**
 * One filtered circle layer per kind, rather than one layer with data-driven
 * expressions for colour and radius.
 *
 * Two reasons: the draw order is then explicit — [MapMarkerKind.entries] order is
 * the paint order, which is what keeps the user dot and the tapped station on top
 * of a cluster of station dots — and the styling reads as plain Kotlin against the
 * same theme tokens the status chips use, instead of a `match` expression that
 * would have to be kept in step with the enum by hand.
 */
private fun MapMarkerKind.toCircleLayer(): CircleLayer {
    val fill: Color
    val radius: Float
    val strokeColor: Color
    val strokeWidth: Float
    when (this) {
        MapMarkerKind.STATION_ONLINE -> {
            fill = StatusOnlineDot; radius = 5f
            strokeColor = MapSurfaceFallback; strokeWidth = 1.5f
        }
        MapMarkerKind.STATION_OFFLINE -> {
            fill = StatusOfflineDot; radius = 5f
            strokeColor = MapSurfaceFallback; strokeWidth = 1.5f
        }
        // The tapped station: same hue as its status would give it is not enough to
        // find in a cluster, so it is drawn larger with a white ring.
        MapMarkerKind.SELECTED -> {
            fill = AccentBlue; radius = 8f
            strokeColor = TextPrimary; strokeWidth = 2.5f
        }
        MapMarkerKind.USER -> {
            fill = TextPrimary; radius = 6f
            strokeColor = AccentBlue; strokeWidth = 3f
        }
    }
    return CircleLayer("$MARKER_LAYER_PREFIX${name.lowercase()}", MARKER_SOURCE_ID)
        .withProperties(
            PropertyFactory.circleColor(fill.toArgb()),
            PropertyFactory.circleRadius(radius),
            PropertyFactory.circleStrokeColor(strokeColor.toArgb()),
            PropertyFactory.circleStrokeWidth(strokeWidth)
        )
        .withFilter(
            Expression.eq(Expression.get(MARKER_KIND_PROPERTY), Expression.literal(name))
        )
}

/** Single GeoJSON source every marker layer reads. */
private const val MARKER_SOURCE_ID = "quake-markers"

/** Feature property holding a [MapMarkerKind] name; the layers filter on it. */
private const val MARKER_KIND_PROPERTY = "kind"

/** Layer ids are derived from the kind, so they are stable and collision-free. */
private const val MARKER_LAYER_PREFIX = "quake-marker-"

/**
 * A [MapView] bound to the current lifecycle.
 *
 * The `started` / `resumed` flags are not defensive noise: the composable can be
 * disposed while the host is still resumed (the user leaves the tab) *or* long
 * after it stopped (the activity went to background first), and MapLibre's renderer
 * expects the pause/stop pair exactly once before `onDestroy`. Tracking what was
 * actually delivered is what keeps both paths from double-calling it.
 */
@Composable
private fun rememberMapView(): MapView {
    val context = LocalContext.current
    val mapView = remember {
        // Idempotent, and cheap after the first call. Kept here rather than in
        // Application.onCreate so a build that never shows a map never loads the
        // native library.
        MapLibre.getInstance(context)
        // Texture mode, not the default GLSurfaceView. A SurfaceView punches its own
        // hole through the window and is composited by the system, so it cannot be
        // drawn into an offscreen render layer — and every map in this app sits in
        // one: the scroll containers apply a RenderEffect while the stretch
        // overscroll animates, and fadingEdges() composites its content offscreen to
        // mask the edges. The map vanished for exactly as long as that lasted, which
        // is the blink seen when overscrolling. A TextureView draws inside the view
        // hierarchy and composes correctly with both, for a fill-rate cost a small
        // static basemap does not notice.
        MapView(context, MapLibreMapOptions.createFromAttributes(context).textureMode(true))
    }
    val lifecycleOwner = LocalLifecycleOwner.current

    DisposableEffect(lifecycleOwner, mapView) {
        var started = false
        var resumed = false

        mapView.onCreate(null)
        val observer = LifecycleEventObserver { _, event ->
            when (event) {
                Lifecycle.Event.ON_START -> if (!started) {
                    mapView.onStart()
                    started = true
                }
                Lifecycle.Event.ON_RESUME -> if (!resumed) {
                    mapView.onResume()
                    resumed = true
                }
                Lifecycle.Event.ON_PAUSE -> if (resumed) {
                    mapView.onPause()
                    resumed = false
                }
                Lifecycle.Event.ON_STOP -> if (started) {
                    mapView.onStop()
                    started = false
                }
                else -> Unit
            }
        }
        lifecycleOwner.lifecycle.addObserver(observer)

        onDispose {
            lifecycleOwner.lifecycle.removeObserver(observer)
            if (resumed) mapView.onPause()
            if (started) mapView.onStop()
            mapView.onDestroy()
        }
    }

    return mapView
}

/**
 * Provider credit. Not optional decoration — the basemap's data is OpenStreetMap
 * under ODbL and the tiles are served by CARTO, and both require the notice to be
 * visible wherever the map is.
 */
@Composable
private fun MapAttribution(modifier: Modifier = Modifier) {
    val shape = remember { RoundedCornerShape(Dimens.RadiusSmall) }
    Box(
        modifier = modifier
            .padding(Dimens.MapAttributionInset)
            .background(MapAttributionScrim, shape)
            .padding(
                horizontal = Dimens.MapAttributionPaddingHorizontal,
                vertical = Dimens.MapAttributionPaddingVertical
            )
    ) {
        Text(text = BASEMAP_ATTRIBUTION, style = MicroCaption, color = MapAttributionText)
    }
}

/**
 * Basemap style. CARTO's "dark matter" is a hosted OpenStreetMap vector style that
 * needs no API key, which is what keeps a commercial token out of the APK; its
 * near-black palette also lands close enough to the app's own background that the
 * cards do not turn into bright rectangles in a dark UI.
 */
private const val BASEMAP_STYLE_URL =
    "https://basemaps.cartocdn.com/gl/dark-matter-gl-style/style.json"

/** The notice [MapAttribution] renders. Wording fixed by the two licences. */
private const val BASEMAP_ATTRIBUTION = "© OpenStreetMap · © CARTO"

// ============================================================
// Interactive map variant (Add Sensor wizard, Location step)
// ============================================================

/**
 * The interactive counterpart of [QuakeMap]: the same dark basemap, but pan and
 * zoom stay live and the location is whatever the frame is centred on.
 *
 * Why a fixed centre pin rather than tap-to-place: the pin is drawn by the caller
 * over the exact centre of the frame, so what the user sees under the crosshair is
 * definitionally what gets reported. A tap target can miss by a thumb width, and on
 * a 175dp map a thumb width is a neighbourhood.
 *
 * The coordinate is reported on camera *idle* rather than on every frame, so a long
 * pan produces one geocode lookup at the end instead of thirty on the way. The
 * camera honours [focus] only when it lands somewhere else entirely (> ~100 m from
 * what is framed), which is exactly the GPS-sync case: a sync reads as "take me
 * there", a pan as "the sensor is here".
 *
 * Gestures stay with MapLibre rather than a Compose `pointerInput` wrapper: the view
 * owns its touch pipeline, so intercepting in Compose would either steal the pan or
 * lose the race with it.
 *
 * @param focus where the camera should sit; honoured only when it differs from what
 *   the camera currently frames (see above).
 * @param onCenterSettled invoked with the framed WGS84 coordinate once a gesture
 *   ends.
 */
@Composable
fun LocationPickerMap(
    focus: MapFocus,
    onCenterSettled: (Double, Double) -> Unit,
    modifier: Modifier = Modifier
) {
    val mapView = rememberMapView()
    // The listener registers once against the map; this keeps it calling the
    // *latest* callback across recompositions instead of a stale capture.
    val currentOnCenterSettled by rememberUpdatedState(onCenterSettled)

    AndroidView(
        factory = {
            mapView.apply {
                getMapAsync { map ->
                    map.uiSettings.apply {
                        // Rotate and tilt add nothing to "where is this sensor" and
                        // make accidental two-finger twists disorienting; pan, zoom
                        // and double-tap zoom all stay live.
                        setAllGesturesEnabled(true)
                        isRotateGesturesEnabled = false
                        isTiltGesturesEnabled = false
                        isLogoEnabled = false
                        isAttributionEnabled = false
                        isCompassEnabled = false
                    }

                    // Registered exactly once: MapLibre owns its camera pipeline, so
                    // re-adding per composition would stack duplicate callbacks.
                    map.addOnCameraIdleListener {
                        map.cameraPosition.target?.let { target ->
                            currentOnCenterSettled(target.latitude, target.longitude)
                        }
                    }

                    map.setStyle(BASEMAP_STYLE_URL)
                }
            }
        },
        update = { view ->
            view.getMapAsync { map ->
                // Camera: honour focus only when it moved meaningfully, so user pans
                // are never fought by the recomposition their own pan triggered.
                val target = map.cameraPosition.target
                val moved = target == null ||
                    kotlin.math.abs(target.latitude - focus.latitude) > RECENTER_THRESHOLD_DEG ||
                    kotlin.math.abs(target.longitude - focus.longitude) > RECENTER_THRESHOLD_DEG
                if (moved) {
                    map.cameraPosition = CameraPosition.Builder()
                        .target(LatLng(focus.latitude, focus.longitude))
                        .zoom(focus.zoom)
                        .build()
                }
            }
        },
        modifier = modifier
    )
}

/**
 * How far (in degrees) a new focus must sit from the framed centre before the
 * camera is allowed to jump. ~100 m of ground: well under any GPS-sync hop,
 * comfortably over map-tap jitter.
 */
private const val RECENTER_THRESHOLD_DEG = 0.001
