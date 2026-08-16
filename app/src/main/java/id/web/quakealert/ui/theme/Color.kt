package id.web.quakealert.ui.theme

import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color

// ============================================================
// Flat Minimalist Dark palette — QuakeAlert Onboarding (Figma)
// ============================================================

// Background gradient stops
// linear-gradient(180deg, rgba(0, 24, 42, 1) 0%, rgba(0, 0, 0, 1) 100%)
val BackgroundGradientTop = Color(0xFF00182A)
val BackgroundGradientBottom = Color(0xFF000000)

// Primary accent — button fill rgba(0, 85, 125, 0.5)
val AccentBlue = Color(0xFF00557D)
val AccentBlueTranslucent = Color(0x8000557D) // 50% alpha

// Neutrals / text
val TextPrimary = Color(0xFFFFFFFF)
val TextSecondary = Color(0x85FFFFFF) // white 52% (0.52 * 255 ≈ 0x85)

// Surfaces & borders
val SurfaceDark = Color(0xFF00182A)
val BorderLight = Color(0x4DFFFFFF) // white 30%
val OverlayLight = Color(0x0DFFFFFF) // white 5% (progress track "Black" overlay)

// Permission card (Onboarding Page 3)
val CardDark = Color(0xFF222222) // card fill #222222
val BorderFaint = Color(0x1AFFFFFF) // white 10%
val SuccessGreenTranslucent = Color(0x78163B00) // rgba(22, 59, 0, 0.47)
val SuccessGreen = Color(0xFF4CAF50) // granted badge accent

// Clickable text links (Onboarding Page 7)
val TextLink = Color(0xFF4FC3D9) // teal/cyan link accent

// ============================================================
// History screen palette — QuakeAlert (Figma node 1:701)
// ============================================================

// Page & card surfaces
val HistoryBackground = Color(0xFF000000)       // frame fill #000000
val CardSurface = Color(0xFF222222)             // history card fill #222222
val CardBorder = Color(0x1AFFFFFF)              // white 10% card stroke

// Top-bar "Healthy" status badge
val HealthyBadgeFill = Color(0xFF0C3600)        // green badge fill #0C3600

// Filter pills
val FilterActiveFill = Color(0xFF003346)        // "All" active fill #003346
val FilterInactiveFill = Color(0xFF222222)      // inactive fill #222222

// MMI (intensity) badge — moderate (orange) vs severe (red)
val MmiOrange = Color(0xFFF39F1D)               // intensity accent / text
val MmiOrangeContainer = Color(0xFF462C08)      // orange badge fill
val MmiRed = Color(0xFFF31D1D)                  // intensity accent / text
val MmiRedContainer = Color(0xFF460808)         // red badge fill

// Card sub-elements
val MapPlaceholder = Color(0xFFD9D9D9)          // map thumbnail placeholder
val DistanceBadgeFill = Color(0xFF373737)       // "X km Away" pill fill
val ShareButtonFill = Color(0xFF24505D)         // share icon button fill

// Bottom navigation
val NavBarFill = Color(0xFF232323)              // nav container fill #232323
val NavBarBorder = Color(0x26FFFFFF)            // white 15% nav stroke
val NavActiveFill = Color(0x6300677E)           // active tab pill rgba(0,103,126,0.39)
val NavActiveText = Color(0xFF5C98AB)           // active tab label/icon tint
val NavLabel = Color(0x99FFFFFF)                // inactive tab label (dimmed white)




/**
 * Vertical background gradient used across onboarding screens,
 * mirroring the Figma frame fill.
 */
val OnboardingBackgroundBrush = Brush.verticalGradient(
    colors = listOf(BackgroundGradientTop, BackgroundGradientBottom)
)
