package id.web.quakealert.ui.onboarding

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.LinkAnnotation
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.TextLinkStyles
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.text.withLink
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import id.web.quakealert.ui.theme.NunitoFontFamily
import id.web.quakealert.ui.theme.TextLink
import id.web.quakealert.ui.theme.TextSecondary

/** Project repository — opened from the "GitHub" bug-report link. */
private const val GITHUB_REPO_URL = "https://github.com/banana-pixel/QuakeAlert-App"

/** Author profile — opened from the "@banana-pixel" credit link. */
private const val GITHUB_PROFILE_URL = "https://github.com/banana-pixel"

/**
 * Closing copy for Onboarding Page 7 (Figma node 1:453), split into three
 * distinct paragraphs to match the design:
 *  1. Body — plain sensor-availability explanation.
 *  2. Bug report — "Report bugs here " + underlined "GitHub" link + " if you
 *     find one!". The accent colour/underline is scoped to just the word
 *     "GitHub".
 *  3. Credit — an underlined "@banana-pixel" link on its own line.
 *
 * Links use the modern [LinkAnnotation.Url] API, which the framework opens via
 * the platform URI handler automatically.
 */
@Composable
fun ReadyText(modifier: Modifier = Modifier) {
    // Matches the description style used on every other onboarding page
    // (Nunito Regular 14/24) so the copy reads consistently across the flow.
    val bodyStyle = SpanStyle(
        color = TextSecondary,
        fontFamily = NunitoFontFamily,
        fontWeight = FontWeight.Normal,
        fontSize = 14.sp
    )
    val linkStyle = TextLinkStyles(
        style = SpanStyle(
            color = TextLink,
            fontFamily = NunitoFontFamily,
            fontWeight = FontWeight.Normal,
            fontSize = 14.sp,
            textDecoration = TextDecoration.Underline
        )
    )

    Column(
        modifier = modifier.fillMaxWidth(),
        verticalArrangement = Arrangement.spacedBy(12.dp)
    ) {
        // Paragraph 1 — body copy.
        Text(
            text = "You will receive earthquake warning depends on sensor " +
                "availability in your area. If theres no sensors ready in your " +
                "area, this app wont be working. You can check for sensors " +
                "availibility on Sensors page.",
            color = TextSecondary,
            fontFamily = NunitoFontFamily,
            fontWeight = FontWeight.Normal,
            fontSize = 14.sp,
            lineHeight = 24.sp,
            modifier = Modifier.fillMaxWidth()
        )

        // Paragraph 2 — bug report with scoped "GitHub" link.
        Text(
            text = buildAnnotatedString {
                withStyle(bodyStyle) { append("Report bugs here ") }
                withLink(LinkAnnotation.Url(url = GITHUB_REPO_URL, styles = linkStyle)) {
                    append("GitHub")
                }
                withStyle(bodyStyle) { append(" if you find one!") }
            },
            lineHeight = 24.sp,
            modifier = Modifier.fillMaxWidth()
        )

        // Paragraph 3 — author credit link on its own line.
        Text(
            text = buildAnnotatedString {
                withStyle(bodyStyle) { append("by ") }
                withLink(LinkAnnotation.Url(url = GITHUB_PROFILE_URL, styles = linkStyle)) {
                    append("@banana-pixel")
                }
            },
            lineHeight = 24.sp,
            modifier = Modifier.fillMaxWidth()
        )
    }
}
