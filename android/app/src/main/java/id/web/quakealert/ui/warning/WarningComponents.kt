package id.web.quakealert.ui.warning

import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Text
import androidx.compose.material3.minimumInteractiveComponentSize
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.ColorFilter
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import id.web.quakealert.R
import id.web.quakealert.ui.common.QuakeModalHeader
import id.web.quakealert.ui.theme.AlertActionBorder
import id.web.quakealert.ui.theme.AlertBannerGradient
import id.web.quakealert.ui.theme.BannerMeta
import id.web.quakealert.ui.theme.BannerTitle
import id.web.quakealert.ui.theme.BannerValue
import id.web.quakealert.ui.theme.BorderLight
import id.web.quakealert.ui.theme.CardBorder
import id.web.quakealert.ui.theme.CardSubtitle
import id.web.quakealert.ui.theme.ChipLabel
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.EmergencyCtaBorder
import id.web.quakealert.ui.theme.EmergencyCtaFill
import id.web.quakealert.ui.theme.EventDetailDividerColor
import id.web.quakealert.ui.theme.EventDetailLocation
import id.web.quakealert.ui.theme.EventDetailMeta
import id.web.quakealert.ui.theme.MapPlaceholder
import id.web.quakealert.ui.theme.MetricLabel
import id.web.quakealert.ui.theme.MetricPanelFill
import id.web.quakealert.ui.theme.MetricValue
import id.web.quakealert.ui.theme.NunitoFontFamily
import id.web.quakealert.ui.theme.PossibilityBannerGradient
import id.web.quakealert.ui.theme.PossibilityDisclaimer
import id.web.quakealert.ui.theme.PossibilityModalGradient
import id.web.quakealert.ui.theme.PrepIconBorder
import id.web.quakealert.ui.theme.QuakeAlertTheme
import id.web.quakealert.ui.theme.TextPrimary
import id.web.quakealert.ui.theme.TextSecondary
import id.web.quakealert.ui.theme.WarningDividerColor

/**
 * Warning banner (Figma nodes 124:1297 / 124:1426): a fixed-height stadium card
 * whose gradient, glyph and text block all follow the [WarningBanner] variant —
 * an active alert is crimson with a waveform glyph and an intensity read, the
 * resting state is amber with a globe glyph and a possibility read. The variant
 * is identity; rendering is this component's job, mirroring how the event detail
 * modal keys its gradient off severity.
 *
 * @param banner alert summary content for the current state.
 * @param onSeeDetails invoked when the "SEE DETAILS" capsule is tapped.
 */
@Composable
fun AlertBanner(
    banner: WarningBanner,
    onSeeDetails: () -> Unit,
    modifier: Modifier = Modifier
) {
    val shape = RoundedCornerShape(Dimens.AlertBannerRadius)
    val (gradient, glyph) = when (banner) {
        is ActiveQuakeBanner -> AlertBannerGradient to R.drawable.ic_recording_02
        is PossibilityBanner -> PossibilityBannerGradient to R.drawable.ic_globe_04
    }

    Row(
        modifier = modifier
            .fillMaxWidth()
            .heightIn(min = Dimens.AlertBannerHeight)
            .clip(shape)
            .background(gradient, shape)
            .border(Dimens.BorderThin, CardBorder, shape)
            .padding(Dimens.AlertBannerPadding),
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalAlignment = Alignment.CenterVertically
    ) {
        Column(
            modifier = Modifier.weight(1f),
            verticalArrangement = Arrangement.spacedBy(Dimens.AlertBannerTitleGap)
        ) {
            Text(
                text = banner.title,
                style = BannerTitle,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis
            )
            when (banner) {
                is ActiveQuakeBanner -> {
                    Text(
                        text = banner.timeAgo,
                        style = BannerMeta,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis
                    )
                    Text(
                        text = banner.intensityLabel,
                        style = BannerValue,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis
                    )
                }
                is PossibilityBanner -> Text(
                    text = banner.possibilityLabel,
                    style = BannerValue,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis
                )
            }

            // "SEE DETAILS" capsule (Figma fill_a7745cf9): translucent stroke only.
            // minimumInteractiveComponentSize lifts the ~32dp capsule's touch box to
            // the 48dp minimum without touching its drawn geometry, and the tap
            // carries the standard ripple (previously suppressed).
            Box(
                modifier = Modifier
                    .padding(top = 6.dp)
                    .minimumInteractiveComponentSize()
                    .clip(RoundedCornerShape(Dimens.AlertActionRadius))
                    .border(
                        Dimens.BorderMedium,
                        AlertActionBorder,
                        RoundedCornerShape(Dimens.AlertActionRadius)
                    )
                    .clickable(role = Role.Button, onClick = onSeeDetails)
                    .padding(
                        horizontal = Dimens.AlertActionPaddingHorizontal,
                        vertical = Dimens.AlertActionPaddingVertical
                    )
            ) {
                Text(text = "SEE DETAILS", style = ChipLabel)
            }
        }

        Image(
            painter = painterResource(id = glyph),
            contentDescription = null,
            colorFilter = ColorFilter.tint(TextPrimary),
            modifier = Modifier.size(Dimens.AlertWaveIconSize)
        )
    }
}

/**
 * Short centered drag-handle divider between the banner and the preparedness
 * tips (Figma node 1:1037): a 100dp rounded bar in white 40%.
 */
@Composable
fun WarningDivider(modifier: Modifier = Modifier) {
    Box(
        modifier = modifier
            .fillMaxWidth()
            .padding(vertical = Dimens.WarningDividerPaddingVertical),
        contentAlignment = Alignment.Center
    ) {
        Box(
            modifier = Modifier
                .size(width = Dimens.WarningDividerWidth, height = Dimens.WarningDividerThickness)
                .clip(RoundedCornerShape(Dimens.WarningDividerThickness))
                .background(WarningDividerColor)
        )
    }
}

/**
 * A single preparedness tip row (Figma node 1:1038): a circular white-outline
 * glyph on the left and a bold title + dimmed description in the text column.
 */
@Composable
fun PrepTipRow(
    tip: PreparednessTip,
    modifier: Modifier = Modifier
) {
    Row(
        modifier = modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.spacedBy(Dimens.PrepTipContentGap),
        verticalAlignment = Alignment.CenterVertically
    ) {
        Box(
            modifier = Modifier
                .size(Dimens.PrepIconCircleSize)
                .clip(RoundedCornerShape(Dimens.RadiusStadium))
                .border(Dimens.BorderThin, PrepIconBorder, RoundedCornerShape(Dimens.RadiusStadium)),
            contentAlignment = Alignment.Center
        ) {
            Image(
                painter = painterResource(id = tip.icon),
                contentDescription = null,
                modifier = Modifier.size(Dimens.PrepIconGlyphSize)
            )
        }

        Column(
            modifier = Modifier.weight(1f),
            verticalArrangement = Arrangement.spacedBy(Dimens.PrepTipTextGap)
        ) {
            Text(
                text = tip.title,
                color = TextPrimary,
                fontFamily = NunitoFontFamily,
                fontWeight = FontWeight.Bold,
                fontSize = 16.sp,
                lineHeight = 20.sp
            )
            Text(text = tip.description, style = CardSubtitle)
        }
    }
}

/**
 * Bottom emergency call-to-action (Figma nodes 124:1312 / 124:1441): a fixed-height
 * stadium button with a translucent wine fill and a white 30% stroke. Label
 * follows the current design's "SHELTER & EMERGENCY INFO"; the action is a stub
 * until the emergency-resource flow is defined.
 *
 * Full-width, so its touch area already clears the 48dp minimum on the horizontal
 * axis; [minimumInteractiveComponentSize] lifts the 34dp height to 48dp too while
 * the drawn capsule keeps its token. This is the app's most safety-critical tap
 * target, so it gets the same treatment as the small icon buttons.
 */
@Composable
fun EmergencyCta(
    onClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    val shape = RoundedCornerShape(Dimens.EmergencyCtaRadius)

    Box(
        modifier = modifier
            .fillMaxWidth()
            .minimumInteractiveComponentSize()
            .height(Dimens.EmergencyCtaHeight)
            .clip(shape)
            .background(EmergencyCtaFill, shape)
            .border(Dimens.BorderMedium, EmergencyCtaBorder, shape)
            .clickable(role = Role.Button, onClick = onClick),
        contentAlignment = Alignment.Center
    ) {
        Text(
            text = "SHELTER & EMERGENCY INFO",
            style = ChipLabel
        )
    }
}

/**
 * The Earthquake Possibility overlay hosted in its own [Dialog] window (Figma
 * node 124:1605), opened from the resting banner's "SEE DETAILS" action. Same
 * chrome as [id.web.quakealert.ui.common.QuakeEventDetailModalDialog] — platform
 * width disabled so the card spans the screens' content column, and every
 * dismissal path (close button, back press, outside tap) routes to [onDismiss].
 */
@Composable
fun EarthquakePossibilityModal(
    possibility: EarthquakePossibility,
    onDismiss: () -> Unit,
    modifier: Modifier = Modifier
) {
    Dialog(
        onDismissRequest = onDismiss,
        properties = DialogProperties(usePlatformDefaultWidth = false)
    ) {
        EarthquakePossibilityCard(
            possibility = possibility,
            onDismiss = onDismiss,
            modifier = modifier.padding(Dimens.ScreenHorizontalPadding)
        )
    }
}

/**
 * Stateless Earthquake Possibility card (Figma node 124:1605): a near-flat dark
 * surface (so the overlay reads as a lifted panel, not an alert) carrying five
 * sections [Dimens.EventDetailSectionGap] apart:
 *  1. Header — centered "Earthquake Possibility" with the shared circular close.
 *  2. Location + possibility read.
 *  3. Map placeholder — same deferral of the map SDK the other screens make.
 *  4. Stats card — most recent quake and local count within the coverage radius.
 *  5. Accuracy disclaimer in light italic.
 *
 * The card scrolls internally so every section stays reachable on short
 * viewports. Exposed separately from [EarthquakePossibilityModal] so it can be
 * previewed without a dialog window.
 */
@Composable
fun EarthquakePossibilityCard(
    possibility: EarthquakePossibility,
    onDismiss: () -> Unit,
    modifier: Modifier = Modifier
) {
    val shape = remember { RoundedCornerShape(Dimens.RadiusCard) }
    val statsShape = remember { RoundedCornerShape(Dimens.RadiusSmall) }

    Column(
        modifier = modifier
            .fillMaxWidth()
            .clip(shape)
            .background(PossibilityModalGradient, shape)
            .border(Dimens.BorderThin, CardBorder, shape)
            .verticalScroll(rememberScrollState())
            .padding(Dimens.ModalPadding),
        verticalArrangement = Arrangement.spacedBy(Dimens.EventDetailSectionGap)
    ) {
        QuakeModalHeader(onDismiss = onDismiss, title = "Earthquake Possibility")

        Column(
            modifier = Modifier.fillMaxWidth(),
            verticalArrangement = Arrangement.spacedBy(Dimens.EventDetailMmiColumnGap)
        ) {
            Text(
                text = possibility.location,
                style = EventDetailLocation,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis
            )
            Text(
                text = possibility.possibilityLabel,
                style = EventDetailMeta,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis
            )
        }

        Box(
            modifier = Modifier
                .fillMaxWidth()
                .height(Dimens.EventDetailMapHeight)
                .clip(shape)
                .background(MapPlaceholder, shape)
                .border(Dimens.BorderThin, CardBorder, shape)
        )

        Column(
            modifier = Modifier
                .fillMaxWidth()
                .clip(statsShape)
                .background(MetricPanelFill, statsShape)
                .border(Dimens.BorderMedium, BorderLight, statsShape)
                .padding(Dimens.EventDetailInfoPadding),
            verticalArrangement = Arrangement.spacedBy(Dimens.EventDetailInfoGap)
        ) {
            PossibilityStatRow(
                label = possibility.recentEarthquakeLabel,
                value = possibility.recentEarthquakeValue
            )

            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .height(Dimens.BorderThin)
                    .background(EventDetailDividerColor)
            )

            PossibilityStatRow(
                label = possibility.earthquakeCountLabel,
                value = possibility.earthquakeCountValue
            )
        }

        Text(
            text = possibility.disclaimer,
            style = PossibilityDisclaimer,
            modifier = Modifier.fillMaxWidth()
        )
    }
}

/**
 * One stat row of the possibility card (Figma node 124:1652): a fixed 44dp block
 * that Figma splits into two 22dp halves. [Arrangement.SpaceEvenly] reproduces
 * that split from the two line boxes — the same treatment the Earthquake Details
 * overlay's spatial rows use.
 */
@Composable
private fun PossibilityStatRow(
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
        Text(
            text = label,
            style = MetricLabel,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis
        )
        Text(
            text = value,
            style = MetricValue,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis
        )
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun PossibilityBannerPreview() {
    QuakeAlertTheme {
        AlertBanner(
            banner = PossibilityBanner(
                title = "No Recent Earthquake",
                possibilityLabel = "Possibility : High Risk"
            ),
            onSeeDetails = {},
            modifier = Modifier.padding(16.dp)
        )
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun ActiveQuakeBannerPreview() {
    QuakeAlertTheme {
        AlertBanner(
            banner = ActiveQuakeBanner(
                title = "Recent Earthquake Alert",
                timeAgo = "20 minutes ago",
                intensityLabel = "Intensity : IV (moderate)"
            ),
            onSeeDetails = {},
            modifier = Modifier.padding(16.dp)
        )
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun PrepTipRowPreview() {
    QuakeAlertTheme {
        PrepTipRow(
            tip = PreparednessTip(
                id = "kit",
                icon = R.drawable.ic_prep_kit,
                title = "Build a 72-Hour Kit",
                description = "Pack water, non-perishable food, flashlights, extra batteries, and a first-aid kit in an easy-to-reach bag."
            ),
            modifier = Modifier.padding(16.dp)
        )
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun EarthquakePossibilityCardPreview() {
    QuakeAlertTheme {
        EarthquakePossibilityCard(
            possibility = EarthquakePossibility(),
            onDismiss = {},
            modifier = Modifier.padding(Dimens.ScreenHorizontalPadding)
        )
    }
}