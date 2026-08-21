package id.web.quakealert.ui.common

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import id.web.quakealert.R
import id.web.quakealert.domain.ServerConnectionState
import id.web.quakealert.domain.isHealthy
import id.web.quakealert.ui.theme.CardBorder
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.HealthyBadgeFill
import id.web.quakealert.ui.theme.NunitoFontFamily
import id.web.quakealert.ui.theme.TextPrimary

/**
 * Shared screen header used by all five main tabs (Figma nodes 1:705 / 1:1082): a
 * large title on the left and the network-status badge on the right. Extracted to
 * [ui.common] so every screen shares a single source of truth for the layout and
 * token wiring instead of duplicating it.
 *
 * The badge is driven by the global [ServerConnectionState] rather than by anything
 * the calling screen loaded. That is the point: it reports whether the *backend* is
 * reachable, so an empty station roll or an all-offline node fleet cannot make one
 * tab claim the network is down while another says it is up. Per-screen health
 * (station chips, active-sensor counts) stays inside the screens that own it.
 *
 * @param title the screen title (e.g. "History", "Sensors").
 * @param connectionState global backend link state; the green [HealthyBadge] shows
 *   only while it [isHealthy].
 */
@Composable
fun QuakeAppBar(
    title: String,
    connectionState: ServerConnectionState,
    modifier: Modifier = Modifier
) {
    Row(
        modifier = modifier
            .fillMaxWidth()
            .statusBarsPadding(),
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalAlignment = Alignment.CenterVertically
    ) {
        Text(
            text = title,
            color = TextPrimary,
            fontFamily = NunitoFontFamily,
            fontWeight = FontWeight.ExtraBold,
            fontSize = 24.sp,
            lineHeight = 26.sp
        )

        if (connectionState.isHealthy) {
            HealthyBadge()
        }
    }
}

/** Green "Healthy" status pill with a globe glyph (Figma node 1:708). */
@Composable
private fun HealthyBadge(modifier: Modifier = Modifier) {
    Row(
        modifier = modifier
            .background(HealthyBadgeFill, RoundedCornerShape(Dimens.RadiusSmall))
            .border(Dimens.BorderThin, CardBorder, RoundedCornerShape(Dimens.RadiusSmall))
            .padding(
                horizontal = Dimens.BadgePaddingHorizontal,
                vertical = Dimens.BadgePaddingVertical
            ),
        horizontalArrangement = Arrangement.spacedBy(Dimens.BadgeIconGap),
        verticalAlignment = Alignment.CenterVertically
    ) {
        Icon(
            painter = painterResource(id = R.drawable.ic_globe),
            contentDescription = null,
            tint = TextPrimary,
            modifier = Modifier.size(16.dp)
        )
        Text(
            text = "Healthy",
            color = TextPrimary,
            fontFamily = NunitoFontFamily,
            fontWeight = FontWeight.Bold,
            fontSize = 15.sp
        )
    }
}
