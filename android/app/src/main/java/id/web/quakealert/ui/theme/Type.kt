package id.web.quakealert.ui.theme

import androidx.compose.material3.Typography
import androidx.compose.ui.text.PlatformTextStyle
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.googlefonts.Font
import androidx.compose.ui.text.googlefonts.GoogleFont
import androidx.compose.ui.text.style.LineHeightStyle
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.sp
import id.web.quakealert.R

/**
 * Optically-centered vertical metrics for compact single-line labels (chips,
 * pills, badges).
 *
 * Root cause of "bottom-heavy" text: Compose defaults `includeFontPadding` to
 * true, which bakes Nunito's asymmetric ascent/descent metrics into the line
 * box, and the default [LineHeightStyle] does not center the glyph within that
 * box. Inside a `contentAlignment = Center` capsule this makes the text sit low.
 *
 * Disabling font padding and forcing [LineHeightStyle.Alignment.Center] with
 * [LineHeightStyle.Trim.None] yields perfect geometric/optical centering that is
 * background-agnostic. Shared here so every capsule label stays consistent.
 */
private val CenteredPlatformStyle = PlatformTextStyle(includeFontPadding = false)
private val CenteredLineHeight = LineHeightStyle(
    alignment = LineHeightStyle.Alignment.Center,
    trim = LineHeightStyle.Trim.None
)


/**
 * Downloadable Google Fonts provider (backed by Google Play Services).
 * Certificates are declared in res/values/font_certs.xml.
 */
private val provider = GoogleFont.Provider(
    providerAuthority = "com.google.android.gms.fonts",
    providerPackage = "com.google.android.gms",
    certificates = R.array.com_google_android_gms_fonts_certs
)

private val nunitoGoogleFont = GoogleFont("Nunito")

/**
 * Nunito font family — the typeface used across the QuakeAlert
 * Onboarding design (Figma node 1:470). Weights map to the design:
 *  - Light (300)     : event-detail metadata (Figma nodes 124:1133 / 124:1159)
 *  - Regular (400)   : body / description text
 *  - Medium (500)    : supporting labels
 *  - SemiBold (600)  : emphasis
 *  - Bold (700)      : card titles, button labels, secondary copy
 *  - ExtraBold (800) : headline / title
 *  - Black (900)     : modal titles (About overlay, Figma node 4:669)
 *
 * Bold (700) must be registered explicitly: several composables (permission
 * cards, buttons, badges) request FontWeight.Bold, and without a matching font
 * the renderer synthesizes a faux-bold that looks heavier/inconsistent versus
 * the real Nunito Bold. The same reasoning applies to Black (900), requested by
 * [ModalTitle], and to Light (300) — including its italic — requested by
 * [EventDetailMeta], where a synthesized oblique would shear the wrong weight.
 */
val NunitoFontFamily = FontFamily(
    Font(googleFont = nunitoGoogleFont, fontProvider = provider, weight = FontWeight.Light),
    Font(
        googleFont = nunitoGoogleFont,
        fontProvider = provider,
        weight = FontWeight.Light,
        style = FontStyle.Italic
    ),
    Font(googleFont = nunitoGoogleFont, fontProvider = provider, weight = FontWeight.Normal),
    Font(googleFont = nunitoGoogleFont, fontProvider = provider, weight = FontWeight.Medium),
    Font(googleFont = nunitoGoogleFont, fontProvider = provider, weight = FontWeight.SemiBold),
    Font(googleFont = nunitoGoogleFont, fontProvider = provider, weight = FontWeight.Bold),
    Font(googleFont = nunitoGoogleFont, fontProvider = provider, weight = FontWeight.ExtraBold),
    Font(googleFont = nunitoGoogleFont, fontProvider = provider, weight = FontWeight.Black)
)

// Material typography wired to the Nunito family, with sizes reflecting
// the onboarding design tokens.
val Typography = Typography(
    // Title — "Welcome to QuakeAlert App." (Nunito ExtraBold 32/36)
    headlineMedium = TextStyle(
        fontFamily = NunitoFontFamily,
        fontWeight = FontWeight.ExtraBold,
        fontSize = 32.sp,
        lineHeight = 36.sp
    ),
    // Description body (Nunito Regular 14/24)
    bodyMedium = TextStyle(
        fontFamily = NunitoFontFamily,
        fontWeight = FontWeight.Normal,
        fontSize = 14.sp,
        lineHeight = 24.sp
    ),
    bodyLarge = TextStyle(
        fontFamily = NunitoFontFamily,
        fontWeight = FontWeight.Normal,
        fontSize = 16.sp,
        lineHeight = 24.sp,
        letterSpacing = 0.5.sp
    ),
    // Button label — "Start" (Nunito Bold 15/36)
    labelLarge = TextStyle(
        fontFamily = NunitoFontFamily,
        fontWeight = FontWeight.Bold,
        fontSize = 15.sp,
        lineHeight = 36.sp
    ),
    // Bottom-nav tab label (Figma style_b7774c0e: Nunito Bold 10/16, +0.05em)
    labelSmall = TextStyle(
        fontFamily = NunitoFontFamily,
        fontWeight = FontWeight.Bold,
        fontSize = 10.sp,
        lineHeight = 16.sp,
        letterSpacing = 0.5.sp
    )
)

// ============================================================
// Shared card typography — single source of truth for the list
// cards on History (node 1:715) and Sensors (node 1:1111).
// ============================================================

/**
 * Primary card title used identically by the History card's location line
 * ("Bandung, West Java, ID") and the Sensor card's station header
 * ("Station NODE-xxxx"). Nunito Bold 16/18 so both share an identical base
 * font size and baseline.
 */
val CardTitle = TextStyle(
    fontFamily = NunitoFontFamily,
    fontWeight = FontWeight.Bold,
    fontSize = 16.sp,
    lineHeight = 18.sp,
    color = TextPrimary,
    platformStyle = CenteredPlatformStyle,
    lineHeightStyle = CenteredLineHeight
)


/**
 * Dimmed secondary card line shared by the History card's date/time metadata
 * and the Sensor card's location subtitle. Nunito SemiBold 13/16 in the
 * secondary (dimmed white) colour.
 */
val CardSubtitle = TextStyle(
    fontFamily = NunitoFontFamily,
    fontWeight = FontWeight.SemiBold,
    fontSize = 13.sp,
    lineHeight = 16.sp,
    color = TextSecondary,
    platformStyle = CenteredPlatformStyle,
    lineHeightStyle = CenteredLineHeight
)


/**
 * Label inside a shared QuakePill capsule (History "km Away" badge, Sensor
 * status + telemetry pills). Nunito Medium 11/12 to keep the capsules compact
 * and identical across both cards.
 */
val PillLabel = TextStyle(
    fontFamily = NunitoFontFamily,
    fontWeight = FontWeight.Medium,
    fontSize = 11.sp,
    lineHeight = 12.sp,
    color = TextPrimary,
    platformStyle = CenteredPlatformStyle,
    lineHeightStyle = CenteredLineHeight
)

/**
 * Bold label used by the filter chips ("All", "Near - 39km") and the map-card
 * overlay badges (location pill, range summary). Nunito Bold 13/16 with the
 * shared centered metrics so the glyphs sit optically centered in their
 * fixed-height capsules rather than drifting toward the bottom edge.
 */
val ChipLabel = TextStyle(
    fontFamily = NunitoFontFamily,
    fontWeight = FontWeight.Bold,
    fontSize = 13.sp,
    lineHeight = 16.sp,
    color = TextPrimary,
    platformStyle = CenteredPlatformStyle,
    lineHeightStyle = CenteredLineHeight
)

// ============================================================
// Modal / overlay typography — About overlay (Figma node 4:654)
// ============================================================

/**
 * Centered modal title ("About", Figma node 4:669): Nunito Black 16/22. Heavier
 * than [CardTitle] so the overlay header outweighs the card titles behind it.
 */
val ModalTitle = TextStyle(
    fontFamily = NunitoFontFamily,
    fontWeight = FontWeight.Black,
    fontSize = 16.sp,
    lineHeight = 22.sp,
    color = TextPrimary,
    textAlign = TextAlign.Center,
    platformStyle = CenteredPlatformStyle,
    lineHeightStyle = CenteredLineHeight
)

/**
 * Centered multi-paragraph modal body copy (Figma node 4:672): Nunito Bold
 * 16/22. Font padding is disabled so the 22sp line height lands on the Figma
 * rhythm instead of being inflated by Nunito's asymmetric metrics, but the
 * default [LineHeightStyle] is kept — per-line centering is only wanted inside
 * fixed-height capsules, not in a wrapping paragraph.
 */
val ModalBodyText = TextStyle(
    fontFamily = NunitoFontFamily,
    fontWeight = FontWeight.Bold,
    fontSize = 16.sp,
    lineHeight = 22.sp,
    color = TextPrimary,
    textAlign = TextAlign.Center,
    platformStyle = CenteredPlatformStyle
)

// ============================================================
// Earthquake Details overlay typography (Figma node 123:1002)
// ============================================================

/**
 * Caption above the MMI badge in the detail banner (Figma node 124:1169): Nunito
 * Bold 12/16. One step below [ChipLabel] so it reads as a field label for the
 * badge instead of competing with it.
 */
val MmiCaption = TextStyle(
    fontFamily = NunitoFontFamily,
    fontWeight = FontWeight.Bold,
    fontSize = 12.sp,
    lineHeight = 16.sp,
    color = TextPrimary,
    textAlign = TextAlign.Center,
    platformStyle = CenteredPlatformStyle,
    lineHeightStyle = CenteredLineHeight
)

/**
 * Roman-numeral MMI value inside the detail banner's badge (Figma node 123:1041):
 * Nunito Bold 16. One step larger than the History list card's 15sp badge because
 * the overlay's badge is the primary read of the event. Colour is supplied per
 * severity by the call site, so it is deliberately left unset here.
 */
val MmiBadgeValue = TextStyle(
    fontFamily = NunitoFontFamily,
    fontWeight = FontWeight.Bold,
    fontSize = 16.sp,
    lineHeight = 20.sp,
    textAlign = TextAlign.Center,
    platformStyle = CenteredPlatformStyle,
    lineHeightStyle = CenteredLineHeight
)

/**
 * Epicentre headline in the detail banner (Figma node 123:1058): Nunito Bold
 * 15/20. Deliberately not [CardTitle] (16sp) — the overlay pairs this line with
 * two 12sp metadata lines inside a fixed-height block, and Figma tightens it to
 * 15sp so that block does not crowd.
 */
val EventDetailLocation = TextStyle(
    fontFamily = NunitoFontFamily,
    fontWeight = FontWeight.Bold,
    fontSize = 15.sp,
    lineHeight = 20.sp,
    color = TextPrimary,
    platformStyle = CenteredPlatformStyle,
    lineHeightStyle = CenteredLineHeight
)

/**
 * Timestamp and relative-age metadata under the epicentre headline (Figma nodes
 * 124:1133 / 124:1159): Nunito Light 12/16 in full white — unlike [CardSubtitle]
 * the overlay does not dim these lines, it lightens their weight. The relative-age
 * line reuses this style with `fontStyle = FontStyle.Italic` at the call site,
 * which is why that italic is registered in [NunitoFontFamily].
 */
val EventDetailMeta = TextStyle(
    fontFamily = NunitoFontFamily,
    fontWeight = FontWeight.Light,
    fontSize = 12.sp,
    lineHeight = 16.sp,
    color = TextPrimary,
    platformStyle = CenteredPlatformStyle,
    lineHeightStyle = CenteredLineHeight
)

/**
 * Field label inside the overlay's seismic metric cells and spatial info rows
 * (Figma styles `style_297198bb` / `style_51a47a12`): Nunito Regular 11/16.
 * Alignment is left unset — the metric grid passes `TextAlign.Center` while the
 * spatial rows keep the default start alignment, exactly as Figma has them.
 */
val MetricLabel = TextStyle(
    fontFamily = NunitoFontFamily,
    fontWeight = FontWeight.Normal,
    fontSize = 11.sp,
    lineHeight = 16.sp,
    color = TextPrimary,
    platformStyle = CenteredPlatformStyle,
    lineHeightStyle = CenteredLineHeight
)

/**
 * Value paired with [MetricLabel] (Figma styles `style_8aae760f` /
 * `style_acdce2d9`): Nunito Black 12/16. Only one step up in size from its label,
 * so the jump to Black — not the size — carries the emphasis. Alignment is left
 * unset for the same reason as [MetricLabel].
 */
val MetricValue = TextStyle(
    fontFamily = NunitoFontFamily,
    fontWeight = FontWeight.Black,
    fontSize = 12.sp,
    lineHeight = 16.sp,
    color = TextPrimary,
    platformStyle = CenteredPlatformStyle,
    lineHeightStyle = CenteredLineHeight
)

// ============================================================
// Warning screen typography (Figma nodes 124:1297 / 124:1426)
// ============================================================

/**
 * Section title above the tip list ("Stay alert for aftershocks" / "Stay
 * prepared for an earthquake", Figma 124:1311): Nunito Bold 20/22. Promoted from
 * the old hardcoded 18sp inline style so every Warning state shares one token.
 */
val SectionTitle = TextStyle(
    fontFamily = NunitoFontFamily,
    fontWeight = FontWeight.Bold,
    fontSize = 20.sp,
    lineHeight = 22.sp,
    color = TextPrimary,
    platformStyle = CenteredPlatformStyle,
    lineHeightStyle = CenteredLineHeight
)

/**
 * Accuracy disclaimer at the foot of the Earthquake Possibility overlay (Figma
 * 124:1708): Nunito Light Italic 11/16. Same weight/italic pairing as
 * [EventDetailMeta]'s relative-age line, stepped down a size so the disclaimer
 * recedes behind the data rows above it.
 */
val PossibilityDisclaimer = TextStyle(
    fontFamily = NunitoFontFamily,
    fontWeight = FontWeight.Light,
    fontSize = 11.sp,
    lineHeight = 16.sp,
    color = TextPrimary,
    fontStyle = FontStyle.Italic,
    platformStyle = CenteredPlatformStyle,
    lineHeightStyle = CenteredLineHeight
)



