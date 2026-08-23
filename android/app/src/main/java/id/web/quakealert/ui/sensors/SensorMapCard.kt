package id.web.quakealert.ui.sensors

import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import id.web.quakealert.R
import id.web.quakealert.data.UnitSystem
import id.web.quakealert.ui.common.MapFocus
import id.web.quakealert.ui.common.MapMarker
import id.web.quakealert.ui.common.QuakeMap
import id.web.quakealert.ui.theme.CardBorder
import id.web.quakealert.ui.theme.ChipLabel
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.GeofenceFill
import id.web.quakealert.ui.theme.GeofenceStroke
import id.web.quakealert.ui.theme.MapLocationPillFill
import id.web.quakealert.ui.theme.MapRangeBadgeFill
import id.web.quakealert.ui.theme.TextPrimary


/**
 * Map preview card shared by the Sensors screen (Figma node 1:1091) and the
 * Settings "Location & Coverage" section (Figma node 1:858). Both screens render
 * the *same* linked map, so overlays are driven from the shared [overview]:
 *  - a top-left location pill ([MapLocationPillFill] + pin glyph),
 *  - a bottom-left "Range : ... , N sensors" summary badge (Sensors only, see
 *    [showRangeBadge]),
 *  - a reactive coverage geofence circle whose radius scales with
 *    [SensorMapOverview.geofenceFraction] (0f..1f of the card's minimum side),
 *  - and nothing else. The bottom-right settings shortcut is gone: the Settings
 *    screen now carries this same map inside its "Sync Location Now" card, so a
 *    button whose only job was to send the user there was pointing at a screen that
 *    shows the very card it sat on.
 *
 * The geofence radius change is animated for a smooth transition between coverage
 * steps.
 *
 * The basemap itself comes from [QuakeMap], centred on the device position carried
 * by [overview]; every overlay above stays plain Compose so it keeps the design
 * system's tokens. When no position has ever been synced there is nothing to centre
 * on, so the card falls back to [QuakeMap]'s dark ground and the location pill's
 * own "not set" copy carries the news.
 *
 * @param unitSystem drives the "Range : ..." summary badge unit ("km" / "mi").
 * @param showRangeBadge paints the "Range : ... , N sensors" badge. False in the
 *   Settings "Sync Location Now" card: range and sensor counts are the Sensors
 *   screen's subject, and repeating them on a card whose only claim is *where the
 *   last fix landed* invited the reading that Settings owns the search radius too.
 * @param showGeofence paints the coverage circle. False inside the Settings
 *   "Sync Location Now" card, where the map is a 130dp confirmation of *where* the
 *   fix landed and the circle — drawn from the card's shorter side — would fill it
 *   edge to edge and say nothing about coverage.
 * @param height the card's height. Defaults to the Sensors screen's full preview;
 *   the inline Settings map passes [Dimens.MapCardInlineHeight].
 * @param markers dots pinned to the ground: the device position on both screens,
 *   plus the station roll on Sensors. Passed through to [QuakeMap] rather than drawn
 *   as overlays here, because an overlay is only in the right place at the exact
 *   centre of the camera — which is true of the coverage circle and the pill, and
 *   false of every station.
 * @param pillLabel overrides the top-left pill's text. Defaults to the device place
 *   carried by [overview], which is what the Settings map wants; the Sensors screen
 *   passes [id.web.quakealert.ui.sensors.mapPillLabel] so the pill names the station the
 *   camera has moved to instead of a place the camera is no longer over.
 * @param focus overrides where the camera points. Null means "the device position
 *   carried by [overview]", which is what both screens want until a station row is
 *   tapped; the Sensors screen then passes the selected station's own framing.
 */
@Composable
fun SensorMapCard(
    overview: SensorMapOverview,
    modifier: Modifier = Modifier,
    unitSystem: UnitSystem = UnitSystem.METRIC,
    showGeofence: Boolean = true,
    showRangeBadge: Boolean = true,
    height: Dp = Dimens.MapCardHeight,
    markers: List<MapMarker> = emptyList(),
    focus: MapFocus? = null,
    pillLabel: String = overview.locationLabel
) {
    val cardShape = remember { RoundedCornerShape(Dimens.RadiusCard) }
    val animatedFraction by animateFloatAsState(
        targetValue = overview.geofenceFraction,
        animationSpec = tween(durationMillis = 300),
        label = "GeofenceFraction"
    )
    val devicePosition = remember(overview.latitude, overview.longitude) {
        val latitude = overview.latitude
        val longitude = overview.longitude
        if (latitude != null && longitude != null) {
            MapFocus(latitude = latitude, longitude = longitude, zoom = MapFocus.ZOOM_COVERAGE)
        } else {
            null
        }
    }

    QuakeMap(
        focus = focus ?: devicePosition,
        markers = markers,
        // Bottom-end belongs to the settings shortcut on the Sensors screen, and
        // bottom-start to the range badge on both, so the credit takes the one
        // corner no overlay claims.
        attributionAlignment = Alignment.TopEnd,
        // No inset and no outline on the map itself: the basemap fills the clipped
        // bounds. Insetting here shrank the GL surface inside the clip, so the
        // card's own ground showed through as a ring around the tiles, and the
        // hairline drew a second edge on top of it. The 12dp breathing room the
        // overlays need is theirs alone, applied to each of them below.
        modifier = modifier
            .fillMaxWidth()
            .height(height)
            .clip(cardShape)
    ) {
        // Reactive geofence coverage circle.
        if (showGeofence) Box(
            modifier = Modifier
                .fillMaxSize()
                .drawBehind {
                    val radius = (minOf(size.width, size.height) / 2f) * animatedFraction
                    val center = androidx.compose.ui.geometry.Offset(
                        x = size.width / 2f,
                        y = size.height / 2f
                    )
                    drawCircle(color = GeofenceFill, radius = radius, center = center)
                    drawCircle(
                        color = GeofenceStroke,
                        radius = radius,
                        center = center,
                        style = Stroke(width = 2.dp.toPx())
                    )
                }
        )


        // Top-left: user location pill.
        LocationPill(
            label = pillLabel,
            modifier = Modifier
                .align(Alignment.TopStart)
                .padding(Dimens.MapCardPadding)
        )

        // Bottom-left: range/sensor-count summary badge.
        if (showRangeBadge) RangeBadge(
            label = overview.summaryLabel(unitSystem),
            modifier = Modifier
                .align(Alignment.BottomStart)
                .padding(Dimens.MapCardPadding)
        )

    }
}

/** Location pill overlay (Figma node 1:1094). */
@Composable
private fun LocationPill(label: String, modifier: Modifier = Modifier) {
    val shape = RoundedCornerShape(Dimens.RadiusSmall)
    Row(
        modifier = modifier
            .height(Dimens.MapLocationPillHeight)
            .clip(shape)
            .background(MapLocationPillFill, shape)
            .border(Dimens.BorderThin, CardBorder, shape)
            .padding(
                horizontal = Dimens.MapLocationPillPaddingHorizontal,
                vertical = Dimens.MapLocationPillPaddingVertical
            ),
        horizontalArrangement = Arrangement.spacedBy(Dimens.MapLocationPillGap),
        verticalAlignment = Alignment.CenterVertically
    ) {
        Icon(
            painter = painterResource(id = R.drawable.ic_pin_location),
            contentDescription = null,
            tint = TextPrimary,
            modifier = Modifier.size(Dimens.MapPinIconSize)
        )
        // Shared ChipLabel with centered metrics so the label is optically
        // centered against the pin glyph instead of sitting low.
        Text(text = label, style = ChipLabel)
    }
}


/** Range/sensor-count summary badge (Figma node 1:1099). */
@Composable
private fun RangeBadge(label: String, modifier: Modifier = Modifier) {
    val shape = RoundedCornerShape(Dimens.RadiusSmall)
    Box(
        modifier = modifier
            .height(Dimens.MapRangeBadgeHeight)
            .clip(shape)
            .background(MapRangeBadgeFill, shape)
            .border(Dimens.BorderThin, CardBorder, shape)
            .padding(
                horizontal = Dimens.MapRangeBadgePaddingHorizontal,
                vertical = Dimens.MapRangeBadgePaddingVertical
            ),
        contentAlignment = Alignment.Center
    ) {
        // Shared ChipLabel: centered metrics keep the summary text vertically
        // centered within the fixed-height badge.
        Text(text = label, style = ChipLabel)
    }
}
