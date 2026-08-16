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


/**
 * Vertical background gradient used across onboarding screens,
 * mirroring the Figma frame fill.
 */
val OnboardingBackgroundBrush = Brush.verticalGradient(
    colors = listOf(BackgroundGradientTop, BackgroundGradientBottom)
)
