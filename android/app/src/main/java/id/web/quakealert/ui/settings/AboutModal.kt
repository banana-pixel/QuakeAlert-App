package id.web.quakealert.ui.settings

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
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.ColorFilter
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import id.web.quakealert.R
import id.web.quakealert.ui.common.QuakeModalHeader
import id.web.quakealert.ui.theme.AboutActionDonateFill
import id.web.quakealert.ui.theme.AboutActionEmailFill
import id.web.quakealert.ui.theme.AboutActionGithubFill
import id.web.quakealert.ui.theme.AboutLogoCoreFill
import id.web.quakealert.ui.theme.AboutLogoHaloFill
import id.web.quakealert.ui.theme.AboutLogoRingFill
import id.web.quakealert.ui.theme.AboutModalGradient
import id.web.quakealert.ui.theme.BorderLight
import id.web.quakealert.ui.theme.CardBorder
import id.web.quakealert.ui.theme.ChipLabel
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.ModalBodyText
import id.web.quakealert.ui.theme.QuakeAlertTheme
import id.web.quakealert.ui.theme.TextPrimary

/**
 * External destinations opened from the About overlay's action buttons. Kept in
 * one place so the URLs are not scattered across the UI layer.
 */
object AboutLinks {
    /** Project GitHub Pages site — "GitHub Pages" action. */
    const val GITHUB_PAGES = "https://banana-pixel.github.io/QuakeAlert-App/"

    /** Author contact — "Email" action. Pre-fills a subject line. */
    const val EMAIL = "mailto:wiratara006@gmail.com?subject=QuakeAlert%20Feedback"

    /** Support the project — "Donate" action. */
    const val DONATE = "https://github.com/sponsors/banana-pixel"
}

/** Mission statement (Figma node 4:672, first paragraph). */
private const val ABOUT_MISSION =
    "QuakeAlert is built to provide a warning system that can be accessed for " +
        "everyone, especially for the countries or places that don’t have early " +
        "warning system for earthquake. I hope this app can save lives."

/** Feedback invitation (Figma node 4:672, second paragraph). */
private const val ABOUT_FEEDBACK =
    "If you have some suggestion or found any bugs, feel free to contact me."

/** Author attribution (Figma node 4:672, closing line). */
private const val ABOUT_ATTRIBUTION = "by @banana-pixel (Vito Wiratara)"

/**
 * The About overlay hosted in its own [Dialog] window (Figma node 4:654). Sits on
 * top of the Settings screen with the platform scrim behind it; back press and
 * taps outside the card both route to [onDismiss], so navigation is never
 * trapped.
 *
 * [DialogProperties.usePlatformDefaultWidth] is disabled so the card can span the
 * full content width the design calls for, inset by the shared
 * [Dimens.ScreenHorizontalPadding] rather than Material's narrower dialog width.
 *
 * @param onDismiss invoked by the close button, back press or an outside tap.
 * @param onGithubClick invoked by the "GitHub Pages" action.
 * @param onEmailClick invoked by the "Email" action.
 * @param onDonateClick invoked by the "Donate" action.
 */
@Composable
fun AboutModalDialog(
    onDismiss: () -> Unit,
    onGithubClick: () -> Unit,
    onEmailClick: () -> Unit,
    onDonateClick: () -> Unit
) {
    Dialog(
        onDismissRequest = onDismiss,
        properties = DialogProperties(usePlatformDefaultWidth = false)
    ) {
        AboutModal(
            onDismiss = onDismiss,
            onGithubClick = onGithubClick,
            onEmailClick = onEmailClick,
            onDonateClick = onDonateClick,
            modifier = Modifier.padding(Dimens.ScreenHorizontalPadding)
        )
    }
}

/**
 * Stateless About modal card (Figma node 4:668): a dark rounded surface filled
 * with the teal → near-black vertical gradient and a subtle white-10% stroke,
 * stacking four sections 26dp apart:
 *  1. Header — the shared [QuakeModalHeader]: centered "About" title with a
 *     circular close (X) button trailing.
 *  2. Logo badge — concentric glowing discs around the seismograph glyph.
 *  3. Body copy — mission, feedback note and the author attribution.
 *  4. Actions — "GitHub Pages" + "Email" on one row, full-width "Donate" below.
 *
 * The card scrolls internally so the copy stays reachable on short viewports
 * (landscape, large font scales) instead of being clipped by the dialog window.
 *
 * Exposed separately from [AboutModalDialog] so it can be previewed and tested
 * without a dialog window.
 */
@Composable
fun AboutModal(
    onDismiss: () -> Unit,
    onGithubClick: () -> Unit,
    onEmailClick: () -> Unit,
    onDonateClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    val shape = RoundedCornerShape(Dimens.RadiusCard)

    Column(
        modifier = modifier
            .fillMaxWidth()
            .clip(shape)
            .background(AboutModalGradient, shape)
            .border(Dimens.BorderThin, CardBorder, shape)
            .verticalScroll(rememberScrollState())
            .padding(Dimens.ModalPadding),
        verticalArrangement = Arrangement.spacedBy(Dimens.AboutModalSectionGap)
    ) {
        QuakeModalHeader(onDismiss = onDismiss, title = "About")
        AboutLogoBadge()
        AboutModalBody()
        AboutModalActions(
            onGithubClick = onGithubClick,
            onEmailClick = onEmailClick,
            onDonateClick = onDonateClick
        )
    }
}

/**
 * Central logo badge (Figma node 4:670). The design ships a raster logo here; it
 * is rebuilt from Compose primitives as three concentric cyan discs of rising
 * alpha (halo → ring → core) wrapping the shared `ic_recording_wave` seismograph
 * glyph, which keeps the badge resolution-independent and tied to the palette.
 *
 * Figma's `0 4px 30px rgba(0,0,0,0.25)` drop shadow is intentionally dropped: on
 * the near-black card a dark blur is invisible, and the alpha-stepped rings
 * already supply the glow it was there to imply.
 */
@Composable
private fun AboutLogoBadge(modifier: Modifier = Modifier) {
    Box(
        modifier = modifier
            .fillMaxWidth()
            .height(Dimens.AboutModalLogoSize),
        contentAlignment = Alignment.Center
    ) {
        ConcentricDisc(size = Dimens.AboutModalLogoSize, fill = AboutLogoHaloFill) {
            ConcentricDisc(size = Dimens.AboutModalLogoRingSize, fill = AboutLogoRingFill) {
                ConcentricDisc(
                    size = Dimens.AboutModalLogoCoreSize,
                    fill = AboutLogoCoreFill,
                    bordered = true
                ) {
                    Image(
                        painter = painterResource(id = R.drawable.ic_recording_wave),
                        contentDescription = null,
                        colorFilter = ColorFilter.tint(TextPrimary),
                        modifier = Modifier.size(Dimens.AboutModalLogoGlyphSize)
                    )
                }
            }
        }
    }
}

/**
 * One ring of [AboutLogoBadge]: a clipped circle of [size] filled with [fill],
 * centering its [content]. [bordered] adds the white-30% hairline the innermost
 * core disc carries.
 */
@Composable
private fun ConcentricDisc(
    size: Dp,
    fill: Color,
    modifier: Modifier = Modifier,
    bordered: Boolean = false,
    content: @Composable () -> Unit
) {
    val base = modifier
        .size(size)
        .clip(CircleShape)
        .background(fill, CircleShape)

    Box(
        modifier = if (bordered) {
            base.border(Dimens.BorderThin, BorderLight, CircleShape)
        } else {
            base
        },
        contentAlignment = Alignment.Center
    ) {
        content()
    }
}

/**
 * Modal body copy (Figma node 4:672): a single centered text block whose three
 * paragraphs are separated by blank lines, so the gaps inherit the 22sp line
 * height rather than needing their own spacing token.
 *
 * The closing attribution is highlighted by stepping up to ExtraBold against the
 * surrounding Bold. A colour accent is deliberately avoided — the app reserves
 * tinted text for tappable links (`TextLink`), and this line is not one.
 */
@Composable
private fun AboutModalBody(modifier: Modifier = Modifier) {
    Text(
        text = buildAnnotatedString {
            append(ABOUT_MISSION)
            append("\n\n")
            append(ABOUT_FEEDBACK)
            append("\n\n")
            withStyle(SpanStyle(fontWeight = FontWeight.ExtraBold)) {
                append(ABOUT_ATTRIBUTION)
            }
        },
        style = ModalBodyText,
        modifier = modifier.fillMaxWidth()
    )
}

/**
 * Bottom action block (Figma node 4:673): "GitHub Pages" and "Email" share a row
 * as equal halves, with the full-width "Donate" button beneath. Rows and the
 * in-row gap both use the design's 20dp spacing.
 */
@Composable
private fun AboutModalActions(
    onGithubClick: () -> Unit,
    onEmailClick: () -> Unit,
    onDonateClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    Column(
        modifier = modifier.fillMaxWidth(),
        verticalArrangement = Arrangement.spacedBy(Dimens.AboutModalActionGap)
    ) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.spacedBy(Dimens.AboutModalActionGap),
            verticalAlignment = Alignment.CenterVertically
        ) {
            AboutActionButton(
                label = "GitHub Pages",
                fill = AboutActionGithubFill,
                onClick = onGithubClick,
                modifier = Modifier.weight(1f)
            )
            AboutActionButton(
                label = "Email",
                fill = AboutActionEmailFill,
                onClick = onEmailClick,
                modifier = Modifier.weight(1f)
            )
        }

        AboutActionButton(
            label = "Donate",
            fill = AboutActionDonateFill,
            onClick = onDonateClick,
            modifier = Modifier.fillMaxWidth()
        )
    }
}

/**
 * A single About action button (Figma nodes 4:677 / 4:681 / 4:686): a fixed 34dp
 * stadium-cornered box with a 31%-alpha [fill], a 2dp white-30% stroke and a
 * centered [ChipLabel]. Only the fill hue distinguishes the three actions, so
 * they share one implementation.
 */
@Composable
private fun AboutActionButton(
    label: String,
    fill: Color,
    onClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    val shape = RoundedCornerShape(Dimens.RadiusSmall)

    Box(
        modifier = modifier
            .height(Dimens.ModalActionHeight)
            .clip(shape)
            .background(fill, shape)
            .border(Dimens.BorderMedium, BorderLight, shape)
            .clickable(onClick = onClick)
            .padding(horizontal = Dimens.ModalActionPaddingHorizontal),
        contentAlignment = Alignment.Center
    ) {
        Text(text = label, style = ChipLabel)
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun AboutModalPreview() {
    QuakeAlertTheme {
        AboutModal(
            onDismiss = {},
            onGithubClick = {},
            onEmailClick = {},
            onDonateClick = {},
            modifier = Modifier.padding(Dimens.ScreenHorizontalPadding)
        )
    }
}
