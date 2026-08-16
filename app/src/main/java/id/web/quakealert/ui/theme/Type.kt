package id.web.quakealert.ui.theme

import androidx.compose.material3.Typography
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.googlefonts.Font
import androidx.compose.ui.text.googlefonts.GoogleFont
import androidx.compose.ui.unit.sp
import id.web.quakealert.R

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
 *  - Regular (400)   : body / description text
 *  - Medium (500)    : supporting labels
 *  - SemiBold (600)  : emphasis
 *  - Bold (700)      : card titles, button labels, secondary copy
 *  - ExtraBold (800) : headline / title
 *
 * Bold (700) must be registered explicitly: several composables (permission
 * cards, buttons, badges) request FontWeight.Bold, and without a matching font
 * the renderer synthesizes a faux-bold that looks heavier/inconsistent versus
 * the real Nunito Bold.
 */
val NunitoFontFamily = FontFamily(
    Font(googleFont = nunitoGoogleFont, fontProvider = provider, weight = FontWeight.Normal),
    Font(googleFont = nunitoGoogleFont, fontProvider = provider, weight = FontWeight.Medium),
    Font(googleFont = nunitoGoogleFont, fontProvider = provider, weight = FontWeight.SemiBold),
    Font(googleFont = nunitoGoogleFont, fontProvider = provider, weight = FontWeight.Bold),
    Font(googleFont = nunitoGoogleFont, fontProvider = provider, weight = FontWeight.ExtraBold)
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
    color = TextPrimary
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
    color = TextSecondary
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
    color = TextPrimary
)


