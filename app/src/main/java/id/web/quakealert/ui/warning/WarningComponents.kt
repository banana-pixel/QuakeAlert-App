package id.web.quakealert.ui.warning

import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.ColorFilter
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import id.web.quakealert.R
import id.web.quakealert.ui.theme.AlertActionBorder
import id.web.quakealert.ui.theme.AlertBannerGradient
import id.web.quakealert.ui.theme.CardSubtitle
import id.web.quakealert.ui.theme.ChipLabel
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.EmergencyCtaBorder
import id.web.quakealert.ui.theme.EmergencyCtaFill
import id.web.quakealert.ui.theme.NunitoFontFamily
import id.web.quakealert.ui.theme.PrepIconBorder
import id.web.quakealert.ui.theme.QuakeAlertTheme
import id.web.quakealert.ui.theme.TextPrimary
import id.web.quakealert.ui.theme.TextSecondary
import id.web.quakealert.ui.theme.WarningDividerColor

/**
 * Active-alert banner (Figma node 1:1035): a fixed-height stadium card with a
 * deep crimson vertical gradient. Left column carries the alert title, relative
 * time and a short guidance line plus a "SEE DETAILS" capsule; the right side
 * shows a seismograph waveform glyph.
 *
 * @param banner alert summary content.
 * @param onSeeDetails invoked when the "SEE DETAILS" capsule is tapped.
 */
@Composable
fun AlertBanner(
    banner: AlertBannerInfo,
    onSeeDetails: () -> Unit,
    modifier: Modifier = Modifier
) {
    val shape = RoundedCornerShape(Dimens.AlertBannerRadius)
    val interactionSource = remember { MutableInteractionSource() }

    Row(
        modifier = modifier
            .fillMaxWidth()
            .height(Dimens.AlertBannerHeight)
            .clip(shape)
            .background(AlertBannerGradient, shape)
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
                color = TextPrimary,
                fontFamily = NunitoFontFamily,
                fontWeight = FontWeight.ExtraBold,
                fontSize = 20.sp,
                lineHeight = 24.sp
            )
            Text(text = banner.intensityLabel, style = ChipLabel)
            Text(
                text = banner.timeAgo,
                style = CardSubtitle,
                color = TextPrimary
            )
            Text(
                text = banner.description,
                style = CardSubtitle,
                color = TextSecondary,
                modifier = Modifier.padding(top = 2.dp)
            )

            // "SEE DETAILS" capsule (Figma fill_a7745cf9): translucent stroke only.
            Box(
                modifier = Modifier
                    .padding(top = 6.dp)
                    .clip(RoundedCornerShape(Dimens.AlertActionRadius))
                    .border(
                        Dimens.BorderThin,
                        AlertActionBorder,
                        RoundedCornerShape(Dimens.AlertActionRadius)
                    )
                    .clickable(
                        interactionSource = interactionSource,
                        indication = null,
                        onClick = onSeeDetails
                    )
                    .padding(
                        horizontal = Dimens.AlertActionPaddingHorizontal,
                        vertical = Dimens.AlertActionPaddingVertical
                    )
            ) {
                Text(text = "SEE DETAILS", style = ChipLabel)
            }
        }

        Image(
            painter = painterResource(id = R.drawable.ic_recording_wave),
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
 * Bottom emergency call-to-action (Figma node 1:1039): a fixed-height stadium
 * button with a translucent wine fill and a white 30% stroke.
 */
@Composable
fun EmergencyCta(
    onClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    val shape = RoundedCornerShape(Dimens.EmergencyCtaRadius)
    val interactionSource = remember { MutableInteractionSource() }

    Box(
        modifier = modifier
            .fillMaxWidth()
            .height(Dimens.EmergencyCtaHeight)
            .clip(shape)
            .background(EmergencyCtaFill, shape)
            .border(Dimens.BorderThin, EmergencyCtaBorder, shape)
            .clickable(
                interactionSource = interactionSource,
                indication = null,
                onClick = onClick
            ),
        contentAlignment = Alignment.Center
    ) {
        Text(
            text = "Call Emergency Services",
            style = ChipLabel
        )
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun AlertBannerPreview() {
    QuakeAlertTheme {
        AlertBanner(
            banner = AlertBannerInfo(
                title = "Earthquake Detected",
                intensityLabel = "MMI VII · Strong",
                timeAgo = "20 minutes ago",
                description = "Strong shaking expected. Stay calm and take cover."
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
                title = "Prepare an Emergency Kit",
                description = "Water, food, flashlight, and a first-aid kit for 3 days."
            ),
            modifier = Modifier.padding(16.dp)
        )
    }
}
