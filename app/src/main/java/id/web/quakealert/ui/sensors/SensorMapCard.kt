package id.web.quakealert.ui.sensors

import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
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
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.unit.dp
import id.web.quakealert.R
import id.web.quakealert.ui.theme.CardBorder
import id.web.quakealert.ui.theme.ChipLabel
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.GeofenceFill
import id.web.quakealert.ui.theme.GeofenceStroke
import id.web.quakealert.ui.theme.MapLocationPillFill
import id.web.quakealert.ui.theme.MapPlaceholder
import id.web.quakealert.ui.theme.MapRangeBadgeFill
import id.web.quakealert.ui.theme.MapSettingsShortcutBorder
import id.web.quakealert.ui.theme.MapSettingsShortcutFill
import id.web.quakealert.ui.theme.TextPrimary


/**
 * Map preview card shared by the Sensors screen (Figma node 1:1091) and the
 * Settings "Location & Coverage" section (Figma node 1:858). A placeholder map
 * surface hosts overlays:
 *  - a top-left location pill ([MapLocationPillFill] + pin glyph),
 *  - a bottom-left "Range : ... , N sensors" summary badge,
 *  - an optional bottom-right circular settings shortcut (Sensors only).
 *
 * When [geofenceFraction] is non-null a reactive coverage circle is drawn behind
 * the overlays, its radius scaling with the value (0f..1f of the card's minimum
 * side) so the Settings Coverage control visibly grows/shrinks the geofence. The
 * change is animated for a smooth transition between coverage steps.
 *
 * The real map SDK is deferred; the grey [MapPlaceholder] surface stands in for
 * the rendered map while preserving exact layout and overlay positioning.
 *
 * @param onSettingsShortcut when non-null renders the bottom-right settings
 *   shortcut (Sensors screen). Settings passes null to hide it.
 * @param geofenceFraction when non-null draws the reactive coverage circle.
 */
@Composable
fun SensorMapCard(
    overview: SensorMapOverview,
    modifier: Modifier = Modifier,
    onSettingsShortcut: (() -> Unit)? = null,
    geofenceFraction: Float? = null
) {
    val cardShape = remember { RoundedCornerShape(Dimens.RadiusCard) }
    val animatedFraction by animateFloatAsState(
        targetValue = geofenceFraction ?: 0f,
        animationSpec = tween(durationMillis = 300),
        label = "GeofenceFraction"
    )

    Box(
        modifier = modifier
            .fillMaxWidth()
            .height(Dimens.MapCardHeight)
            .clip(cardShape)
            .background(MapPlaceholder, cardShape)
            .border(Dimens.BorderThin, CardBorder, cardShape)
            .padding(Dimens.MapCardPadding)
    ) {
        // Reactive geofence coverage circle (Settings only).
        if (geofenceFraction != null) {
            Box(
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
        }

        // Top-left: user location pill.
        LocationPill(
            label = overview.locationLabel,
            modifier = Modifier.align(Alignment.TopStart)
        )

        // Bottom-left: range/sensor-count summary badge.
        RangeBadge(
            label = overview.summaryLabel,
            modifier = Modifier.align(Alignment.BottomStart)
        )

        // Bottom-right: settings shortcut (Sensors screen only).
        if (onSettingsShortcut != null) {
            SettingsShortcut(
                onClick = onSettingsShortcut,
                modifier = Modifier.align(Alignment.BottomEnd)
            )
        }
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

/** Circular settings shortcut button (Figma node 1:1101). */

@Composable
private fun SettingsShortcut(onClick: () -> Unit, modifier: Modifier = Modifier) {
    Box(
        modifier = modifier
            .size(Dimens.MapSettingsShortcutSize)
            .clip(CircleShape)
            .background(MapSettingsShortcutFill, CircleShape)
            .border(Dimens.BorderThin, MapSettingsShortcutBorder, CircleShape)
            .clickable(onClick = onClick),
        contentAlignment = Alignment.Center
    ) {
        Icon(
            painter = painterResource(id = R.drawable.ic_settings_sliders),
            contentDescription = "Open sensor settings",
            tint = Color.Black,
            modifier = Modifier.size(Dimens.MapSettingsShortcutIconSize)
        )
    }
}
