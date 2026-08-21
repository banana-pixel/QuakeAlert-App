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
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalInspectionMode
import androidx.compose.ui.viewinterop.AndroidView
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.MapAttributionScrim
import id.web.quakealert.ui.theme.MapAttributionText
import id.web.quakealert.ui.theme.MapSurfaceFallback
import id.web.quakealert.ui.theme.MicroCaption
import org.maplibre.android.MapLibre
import org.maplibre.android.camera.CameraPosition
import org.maplibre.android.geometry.LatLng
import org.maplibre.android.maps.MapView

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
 * Vector basemap behind the app's map cards, replacing the grey
 * [id.web.quakealert.ui.theme.MapPlaceholder] surface the screens stood on before
 * the SDK landed.
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
 * @param content overlays drawn over the basemap, in the same coordinate space as
 *   the card.
 */
@Composable
fun QuakeMap(
    focus: MapFocus?,
    modifier: Modifier = Modifier,
    attributionAlignment: Alignment = Alignment.BottomEnd,
    content: @Composable BoxScope.() -> Unit = {}
) {
    Box(modifier = modifier.background(MapSurfaceFallback)) {
        // LocalInspectionMode: Android Studio's preview renderer has no GL context,
        // so a MapView there fails the whole preview rather than the map card.
        val canRender = focus != null && !LocalInspectionMode.current
        if (canRender) {
            Basemap(focus = focus!!, modifier = Modifier.matchParentSize())
            MapAttribution(modifier = Modifier.align(attributionAlignment))
        }
        content()
    }
}

/** The GL surface itself, lifted out so [QuakeMap] reads as composition. */
@Composable
private fun Basemap(focus: MapFocus, modifier: Modifier = Modifier) {
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
                    map.setStyle(BASEMAP_STYLE_URL)
                }
            }
        },
        update = { view ->
            view.getMapAsync { map ->
                map.cameraPosition = CameraPosition.Builder()
                    .target(LatLng(focus.latitude, focus.longitude))
                    .zoom(focus.zoom)
                    .build()
            }
        },
        modifier = modifier
    )
}

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
        MapView(context)
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
