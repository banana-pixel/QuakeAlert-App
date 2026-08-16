package id.web.quakealert.ui.theme

import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.runtime.Composable

/**
 * Flat Minimalist Dark color scheme derived from the QuakeAlert
 * Onboarding design (Figma node 1:470).
 */
private val DarkColorScheme = darkColorScheme(
    primary = AccentBlue,
    onPrimary = TextPrimary,
    secondary = AccentBlueTranslucent,
    onSecondary = TextPrimary,
    tertiary = AccentBlue,
    onTertiary = TextPrimary,
    background = BackgroundGradientBottom,
    onBackground = TextPrimary,
    surface = SurfaceDark,
    onSurface = TextPrimary,
    surfaceVariant = SurfaceDark,
    onSurfaceVariant = TextSecondary,
    outline = BorderLight
)

@Composable
fun QuakeAlertTheme(
    content: @Composable () -> Unit
) {
    // The design is a fixed Flat Minimalist Dark theme, so we always
    // apply the branded dark scheme (no dynamic color / light variant).
    MaterialTheme(
        colorScheme = DarkColorScheme,
        typography = Typography,
        content = content
    )
}
