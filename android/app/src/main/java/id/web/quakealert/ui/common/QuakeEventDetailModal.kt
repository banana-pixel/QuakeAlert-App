package id.web.quakealert.ui.common

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import id.web.quakealert.data.UnitSystem
import id.web.quakealert.ui.history.MmiSeverity
import id.web.quakealert.ui.history.QuakeHistoryItem
import id.web.quakealert.ui.history.label
import id.web.quakealert.ui.history.timestampLabel
import id.web.quakealert.ui.theme.BorderLight
import id.web.quakealert.ui.theme.CardBorder
import id.web.quakealert.ui.theme.ChipLabel
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.EventDetailDividerColor
import id.web.quakealert.ui.theme.EventDetailLocation
import id.web.quakealert.ui.theme.EventDetailMeta
import id.web.quakealert.ui.theme.EventDetailModalGradient
import id.web.quakealert.ui.theme.EventDetailModalGradientSevere
import id.web.quakealert.ui.theme.EventDetailPulseInnerAlpha
import id.web.quakealert.ui.theme.EventDetailPulseMidAlpha
import id.web.quakealert.ui.theme.EventDetailPulseOuterAlpha
import id.web.quakealert.ui.theme.EventDetailShareFill
import id.web.quakealert.ui.theme.MetricLabel
import id.web.quakealert.ui.theme.MetricPanelFill
import id.web.quakealert.ui.theme.MetricValue
import id.web.quakealert.ui.theme.MmiBadgeValue
import id.web.quakealert.ui.theme.MmiCaption
import id.web.quakealert.ui.theme.MmiOrange
import id.web.quakealert.ui.theme.MmiOrangeContainer
import id.web.quakealert.ui.theme.MmiRed
import id.web.quakealert.ui.theme.MmiRedContainer
import id.web.quakealert.ui.theme.QuakeAlertTheme

/**
 * The Earthquake Details overlay hosted in its own [Dialog] window (Figma node
 * 123:743 / 124:1192). Sits on top of the host screen with the platform scrim
 * behind it; the close button, a back press and a tap outside the card all route
 * to [onDismiss], so no dismissal path leaves the overlay stuck open.
 *
 * Shared by History (default [title] "Earthquake Details") and the Warning
 * alert detail ([title] "Recent Earthquake", Figma node 124:1203).
 *
 * [DialogProperties.usePlatformDefaultWidth] is disabled so the card can span the
 * same content width as the screens beneath it, inset by the shared
 * [Dimens.ScreenHorizontalPadding] rather than Material's narrower dialog width.
 *
 * @param event the tapped history entry to describe.
 * @param unitSystem drives the "Distance from you" row unit ("km" / "mi").
 * @param title centered overlay header (Figma 123:1003 / 124:1203).
 * @param onDismiss invoked by the close button, a back press or an outside tap.
 * @param onShare invoked by the bottom "Share" action.
 */
@Composable
fun QuakeEventDetailModalDialog(
    event: QuakeHistoryItem,
    unitSystem: UnitSystem,
    onDismiss: () -> Unit,
    onShare: () -> Unit,
    modifier: Modifier = Modifier,
    title: String = "Earthquake Details"
) {
    Dialog(
        onDismissRequest = onDismiss,
        properties = DialogProperties(usePlatformDefaultWidth = false)
    ) {
        QuakeEventDetailModal(
            event = event,
            unitSystem = unitSystem,
            onDismiss = onDismiss,
            onShare = onShare,
            modifier = modifier.padding(Dimens.ScreenHorizontalPadding),
            title = title
        )
    }
}

/**
 * Stateless Earthquake Details card (Figma nodes 123:1002 / 124:1192): a dark
 * rounded surface filled with the severity-tinted vertical gradient (bronze for
 * [MmiSeverity.MODERATE], dark red for [MmiSeverity.SEVERE] — both stop on
 * [CardSurface]) and the shared white-10% stroke, stacking five sections
 * [Dimens.EventDetailSectionGap] apart:
 *  1. Header — centered [title] with a circular close button.
 *  2. Event banner — MMI badge beside the epicentre, timestamp and relative age.
 *  3. Map thumbnail — the epicentre expressed as concentric pulse rings.
 *  4. Seismic metrics — PGA, intensity and duration as three equal cells.
 *  5. Spatial info + the full-width "Share" action.
 *
 * The card scrolls internally so every section stays reachable on short viewports
 * (landscape, large font scales) instead of being clipped by the dialog window.
 *
 * Exposed separately from [QuakeEventDetailModalDialog] so it can be previewed and
 * tested without a dialog window.
 */
@Composable
fun QuakeEventDetailModal(
    event: QuakeHistoryItem,
    unitSystem: UnitSystem,
    onDismiss: () -> Unit,
    onShare: () -> Unit,
    modifier: Modifier = Modifier,
    title: String = "Earthquake Details"
) {
    val shape = remember { RoundedCornerShape(Dimens.RadiusCard) }
    // Same severity → accent mapping the History card uses, so the overlay opens
    // in the colour of the row that raised it. The gradient follows the same
    // severity: bronze (123:1002) for moderate, dark red (124:1192) for severe.
    val accent = if (event.severity == MmiSeverity.SEVERE) MmiRed else MmiOrange
    val badgeContainer =
        if (event.severity == MmiSeverity.SEVERE) MmiRedContainer else MmiOrangeContainer
    val gradient =
        if (event.severity == MmiSeverity.SEVERE) EventDetailModalGradientSevere
        else EventDetailModalGradient

    Column(
        modifier = modifier
            .fillMaxWidth()
            .clip(shape)
            .background(gradient, shape)
            .border(Dimens.BorderThin, CardBorder, shape)
            .verticalScroll(rememberScrollState())
            .padding(Dimens.ModalPadding),
        verticalArrangement = Arrangement.spacedBy(Dimens.EventDetailSectionGap)
    ) {
        QuakeModalHeader(onDismiss = onDismiss, title = title)

        EventDetailBanner(
            event = event,
            accent = accent,
            badgeContainer = badgeContainer
        )

        EventDetailMap(
            accent = accent,
            focus = MapFocus(
                latitude = event.latitude,
                longitude = event.longitude,
                zoom = MapFocus.ZOOM_EVENT
            )
        )

        SeismicMetricsRow(event = event)

        SpatialInfoCard(event = event, unitSystem = unitSystem)

        ShareAction(onClick = onShare)
    }
}

/**
 * Primary event banner (Figma node 123:1069): the captioned MMI badge on the
 * leading side, then a fixed-height block spreading the epicentre, the timestamp
 * and the relative age evenly down its height.
 *
 * The badge column is pinned to [Dimens.CardLeadingColumnWidth] — the same 45dp
 * the History card's badge occupies — so the overlay's text column starts on the
 * same optical line as the card behind it.
 */
@Composable
private fun EventDetailBanner(
    event: QuakeHistoryItem,
    accent: Color,
    badgeContainer: Color,
    modifier: Modifier = Modifier
) {
    Row(
        modifier = modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.spacedBy(Dimens.EventDetailSectionGap)
    ) {
        MmiBadgeColumn(
            intensity = event.intensity,
            accent = accent,
            container = badgeContainer
        )

        Column(
            modifier = Modifier
                .weight(1f)
                .height(Dimens.EventDetailBannerHeight),
            verticalArrangement = Arrangement.SpaceBetween
        ) {
            Text(
                text = event.location,
                style = EventDetailLocation,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis
            )
            Text(text = event.timestampLabel, style = EventDetailMeta)
            Text(
                text = event.relativeTime,
                style = EventDetailMeta,
                fontStyle = FontStyle.Italic
            )
        }
    }
}

/**
 * "MMI" caption above the circular intensity badge (Figma nodes 124:1169 /
 * 123:1040). Reuses the History card's badge geometry — 45dp disc, 3dp accent ring
 * — and only steps the numeral up to [MmiBadgeValue]'s 16sp, matching Figma.
 */
@Composable
private fun MmiBadgeColumn(
    intensity: String,
    accent: Color,
    container: Color,
    modifier: Modifier = Modifier
) {
    Column(
        modifier = modifier.width(Dimens.CardLeadingColumnWidth),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(Dimens.EventDetailMmiColumnGap)
    ) {
        Text(text = "MMI", style = MmiCaption)

        Box(
            modifier = Modifier
                .size(Dimens.MmiBadgeSize)
                .clip(CircleShape)
                .background(container, CircleShape)
                .border(Dimens.MmiBadgeBorder, accent, CircleShape),
            contentAlignment = Alignment.Center
        ) {
            Text(text = intensity, style = MmiBadgeValue, color = accent)
        }
    }
}

/**
 * Radius fraction → fill alpha for the map's epicentre pulse rings, outermost
 * first. Hoisted out of the draw lambda so the table is allocated once instead of
 * on every frame `drawBehind` runs.
 */
private val PulseRings = listOf(
    1f to EventDetailPulseOuterAlpha,
    0.66f to EventDetailPulseMidAlpha,
    0.33f to EventDetailPulseInnerAlpha
)

/**
 * Map thumbnail (Figma node 123:1028): a 120dp-tall rounded card carrying the
 * shared white-10% stroke.
 *
 * Figma ships a rendered map raster here; [QuakeMap] supplies the real basemap,
 * pointed at the event's own centroid. The epicentre is still drawn on top as three
 * concentric pulse rings in the event's MMI accent plus a solid centroid dot, in a
 * single `drawBehind` pass rather than as nested composables so the whole focus
 * costs one draw node.
 *
 * The rings sit at the card's centre, which is the centroid because [QuakeMap]
 * locks the camera there — see its constraint 2. That also means the focus survives
 * a card opened with no network: the rings and the distance read stay, only the
 * terrain behind them is missing.
 */
@Composable
private fun EventDetailMap(
    accent: Color,
    focus: MapFocus,
    modifier: Modifier = Modifier
) {
    val shape = remember { RoundedCornerShape(Dimens.RadiusCard) }

    QuakeMap(
        focus = focus,
        attributionAlignment = Alignment.BottomStart,
        modifier = modifier
            .fillMaxWidth()
            .height(Dimens.EventDetailMapHeight)
            .clip(shape)
            .border(Dimens.BorderThin, CardBorder, shape)
    ) {
        Box(
            modifier = Modifier
                .fillMaxSize()
                .padding(Dimens.EventDetailMapPadding)
                .drawBehind {
                    val center = Offset(x = size.width / 2f, y = size.height / 2f)
                    val maxRadius = minOf(size.width, size.height) / 2f
                    val ringStroke = Stroke(width = Dimens.BorderThin.toPx())

                    // Outermost → innermost, so each ring's wash layers over the
                    // one before it and the focus reads as a gradient of alpha.
                    PulseRings.forEach { (fraction, alpha) ->
                        val radius = maxRadius * fraction
                        drawCircle(
                            color = accent.copy(alpha = alpha),
                            radius = radius,
                            center = center
                        )
                        drawCircle(
                            color = accent,
                            radius = radius,
                            center = center,
                            style = ringStroke
                        )
                    }

                    drawCircle(
                        color = accent,
                        radius = Dimens.EventDetailMapCentroidSize.toPx() / 2f,
                        center = center
                    )
                }
        )
    }
}

/**
 * Seismic metrics grid (Figma node 124:1088): PGA, intensity and duration as three
 * equal-weight cells on one row. The intensity cell reads from
 * [MmiSeverity.label] rather than a stored string, so it cannot disagree with the
 * badge colour above it.
 */
@Composable
private fun SeismicMetricsRow(
    event: QuakeHistoryItem,
    modifier: Modifier = Modifier
) {
    Row(
        modifier = modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.spacedBy(Dimens.EventDetailSectionGap)
    ) {
        MetricCell(
            label = "PGA (Max)",
            value = event.pgaLabel,
            modifier = Modifier.weight(1f)
        )
        MetricCell(
            label = "Intensity",
            value = event.severity.label,
            modifier = Modifier.weight(1f)
        )
        MetricCell(
            label = "Duration",
            value = event.durationLabel,
            modifier = Modifier.weight(1f)
        )
    }
}

/**
 * One metric cell (Figma node 124:1115): a 62dp-tall recessed panel with a 2dp
 * white-30% stroke, centering a [MetricLabel] over its [MetricValue].
 *
 * Both lines are given `TextAlign.Center` here rather than baked into the styles,
 * because the spatial info rows below reuse the very same two styles start-aligned.
 */
@Composable
private fun MetricCell(
    label: String,
    value: String,
    modifier: Modifier = Modifier
) {
    val shape = remember { RoundedCornerShape(Dimens.RadiusSmall) }

    Column(
        modifier = modifier
            .height(Dimens.EventDetailMetricCellHeight)
            .clip(shape)
            .background(MetricPanelFill, shape)
            .border(Dimens.BorderMedium, BorderLight, shape)
            .padding(horizontal = Dimens.EventDetailMetricCellPaddingHorizontal),
        verticalArrangement = Arrangement.spacedBy(
            space = Dimens.EventDetailMetricCellGap,
            alignment = Alignment.CenterVertically
        ),
        horizontalAlignment = Alignment.CenterHorizontally
    ) {
        Text(
            text = label,
            style = MetricLabel,
            textAlign = TextAlign.Center,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
            modifier = Modifier.fillMaxWidth()
        )
        Text(
            text = value,
            style = MetricValue,
            textAlign = TextAlign.Center,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
            modifier = Modifier.fillMaxWidth()
        )
    }
}

/**
 * Spatial info card (Figma node 124:1147): the distance-from-user reading and the
 * centroid coordinates as two start-aligned label/value rows, split by a #5D5D5D
 * hairline.
 *
 * The rule is a 1dp filled [Box] rather than Material's `HorizontalDivider`,
 * matching how [id.web.quakealert.ui.warning.WarningDivider] draws the app's other
 * rules.
 */
@Composable
private fun SpatialInfoCard(
    event: QuakeHistoryItem,
    unitSystem: UnitSystem,
    modifier: Modifier = Modifier
) {
    val shape = remember { RoundedCornerShape(Dimens.RadiusSmall) }

    Column(
        modifier = modifier
            .fillMaxWidth()
            .clip(shape)
            .background(MetricPanelFill, shape)
            .border(Dimens.BorderMedium, BorderLight, shape)
            .padding(Dimens.EventDetailInfoPadding),
        verticalArrangement = Arrangement.spacedBy(Dimens.EventDetailInfoGap)
    ) {
        SpatialInfoRow(
            label = "Distance from you",
            value = "${unitSystem.formatDistance(event.distanceKm)} Away"
        )

        Box(
            modifier = Modifier
                .fillMaxWidth()
                .height(Dimens.BorderThin)
                .background(EventDetailDividerColor)
        )

        SpatialInfoRow(label = "Coordinates (Centroid)", value = event.coordinates)
    }
}

/**
 * One row of [SpatialInfoCard] (Figma node 124:1153): a fixed 44dp block that
 * Figma splits into two 22dp halves. [Arrangement.SpaceEvenly] reproduces that
 * split from the two line boxes without hard-coding either half's height.
 */
@Composable
private fun SpatialInfoRow(
    label: String,
    value: String,
    modifier: Modifier = Modifier
) {
    Column(
        modifier = modifier
            .fillMaxWidth()
            .height(Dimens.EventDetailInfoRowHeight),
        verticalArrangement = Arrangement.SpaceEvenly
    ) {
        Text(text = label, style = MetricLabel)
        Text(
            text = value,
            style = MetricValue,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis
        )
    }
}

/**
 * Full-width bronze "Share" action (Figma node 124:1085). Geometry, stroke and
 * label come from the shared [Dimens.ModalActionHeight] overlay-action chrome the
 * About modal's buttons also use; only the [EventDetailShareFill] wash is its own.
 */
@Composable
private fun ShareAction(onClick: () -> Unit, modifier: Modifier = Modifier) {
    val shape = remember { RoundedCornerShape(Dimens.RadiusSmall) }

    Box(
        modifier = modifier
            .fillMaxWidth()
            .height(Dimens.ModalActionHeight)
            .clip(shape)
            .background(EventDetailShareFill, shape)
            .border(Dimens.BorderMedium, BorderLight, shape)
            .clickable(onClick = onClick)
            .padding(horizontal = Dimens.ModalActionPaddingHorizontal),
        contentAlignment = Alignment.Center
    ) {
        Text(text = "Share", style = ChipLabel)
    }
}

/** Mock event mirroring the values Figma annotates on node 123:1002. */
private val PreviewEvent = QuakeHistoryItem(
    id = "preview",
    intensity = "VII",
    severity = MmiSeverity.MODERATE,
    location = "Lembang, West Java, ID",
    date = "20 Jun 2026",
    time = "07:19:18 WIB",
    distanceKm = 60,
    relativeTime = "2 months ago",
    pgaLabel = "61.5 gal",
    durationLabel = "7 sec",
    coordinates = "41.40338, 2.17403",
    latitude = 41.40338,
    longitude = 2.17403
)

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun EventDetailModalPreview() {
    QuakeAlertTheme {
        QuakeEventDetailModal(
            event = PreviewEvent,
            unitSystem = UnitSystem.METRIC,
            onDismiss = {},
            onShare = {},
            modifier = Modifier.padding(Dimens.ScreenHorizontalPadding)
        )
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun EventDetailModalSeverePreview() {
    QuakeAlertTheme {
        QuakeEventDetailModal(
            event = PreviewEvent.copy(
                intensity = "IX",
                severity = MmiSeverity.SEVERE,
                pgaLabel = "142.0 gal",
                durationLabel = "23 sec"
            ),
            unitSystem = UnitSystem.METRIC,
            onDismiss = {},
            onShare = {},
            modifier = Modifier.padding(Dimens.ScreenHorizontalPadding)
        )
    }
}

/**
 * Warning-tab instance of the shared overlay (Figma node 124:1192): the severe
 * (XI) event mirroring the design's annotation with the "Recent Earthquake"
 * title and the dark-red alert gradient, opened from the Warning banner's
 * "SEE DETAILS" action.
 */
@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun WarningRecentEarthquakeModalPreview() {
    QuakeAlertTheme {
        QuakeEventDetailModal(
            event = PreviewEvent.copy(
                intensity = "XI",
                severity = MmiSeverity.SEVERE
            ),
            unitSystem = UnitSystem.METRIC,
            onDismiss = {},
            onShare = {},
            title = "Recent Earthquake",
            modifier = Modifier.padding(Dimens.ScreenHorizontalPadding)
        )
    }
}
