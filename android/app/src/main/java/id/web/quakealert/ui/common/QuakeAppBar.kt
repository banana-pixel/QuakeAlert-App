package id.web.quakealert.ui.common

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import id.web.quakealert.R
import id.web.quakealert.data.network.ServerHealth
import id.web.quakealert.ui.theme.CardBorder
import id.web.quakealert.ui.theme.ConnectingBadgeFill
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.HealthyBadgeFill
import id.web.quakealert.ui.theme.NunitoFontFamily
import id.web.quakealert.ui.theme.OfflineBadgeFill
import id.web.quakealert.ui.theme.PillFill
import id.web.quakealert.ui.theme.TextPrimary

/**
 * Shared screen header used by all five main tabs (Figma nodes 1:705 / 1:1082): a
 * large title on the left and the server-status badge on the right. Extracted to
 * [ui.common] so every screen shares a single source of truth for the layout and
 * token wiring instead of duplicating it.
 *
 * The badge is driven by the global [ServerHealth] verdict rather than by anything
 * the calling screen loaded. That is the point: it reports whether the *backend* can
 * do its job, so an empty station roll or an all-offline node fleet cannot make one
 * tab claim the network is down while another says it is up. Per-screen health
 * (station chips, active-sensor counts) stays inside the screens that own it.
 *
 * It is always present, in one of four states, rather than shown only while healthy.
 * A badge that vanishes when the link drops says nothing at all — and "nothing" is
 * indistinguishable from a healthy app with a quiet network, which is the one reading
 * this app must never allow. On Warning the badge is joined by
 * [id.web.quakealert.ui.warning.WarningOfflineNotice], which spells out what a dropped
 * link means for the alerts themselves.
 *
 * @param title the screen title (e.g. "History", "Sensors").
 * @param health the global verdict from [id.web.quakealert.data.network.ServerHealthMonitor];
 *   the label is rendered verbatim, so the UI never sees the internal word for degradation —
 *   users get "Limited", not jargon.
 * @param onUpdatesClicked opens the Updates overlay (Figma node 158-1645 places the
 *   notification-text glyph beside the server-status pill). Null — the default — renders no
 *   control, so a caller that cannot host the overlay simply leaves it out.
 */
@Composable
fun QuakeAppBar(
    title: String,
    health: ServerHealth,
    modifier: Modifier = Modifier,
    onUpdatesClicked: (() -> Unit)? = null
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

        Row(verticalAlignment = Alignment.CenterVertically) {
            if (onUpdatesClicked != null) {
                UpdatesIconButton(onClick = onUpdatesClicked)
                // One badge-gap of air between the glyph and the status pill, so the
                // pair reads as two separate controls rather than one crowded cluster.
                Spacer(Modifier.width(Dimens.BadgeIconGap))
            }
            ServerHealthBadge(health = health)
        }
    }
}

/**
 * The Updates entry point in the app bar (Figma node 158-1645): tinted like every other
 * bar glyph. [IconButton] supplies the 48dp minimum touch target Material requires
 * without inflating the visible mark.
 */
@Composable
private fun UpdatesIconButton(
    onClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    IconButton(onClick = onClick, modifier = modifier) {
        Icon(
            painter = painterResource(id = R.drawable.ic_updates_notification),
            contentDescription = "Open Updates",
            tint = TextPrimary,
            modifier = Modifier.size(24.dp)
        )
    }
}

/**
 * Server-status pill (Figma node 1:708 is the healthy variant). The other three
 * variants reuse its geometry unchanged and differ only in fill, glyph and word, so
 * the badge stays one recognisable object across all four.
 *
 * The severity ramp reads grey → green → amber → red: Checking has no verdict yet,
 * Healthy needs no explanation, Limited says "something behind the server is not
 * right" without borrowing Offline's alarm (the globe keeps standing for a place
 * that answers; the triangle is reserved for the one state where alerts cannot
 * arrive).
 */
@Composable
private fun ServerHealthBadge(
    health: ServerHealth,
    modifier: Modifier = Modifier
) {
    val (fill, glyph, label) = when (health) {
        // Neutral grey, not amber: no verdict yet must not read as a caution.
        ServerHealth.CHECKING ->
            Triple(PillFill, R.drawable.ic_globe, "Checking…")
        ServerHealth.HEALTHY ->
            Triple(HealthyBadgeFill, R.drawable.ic_globe, "Healthy")
        ServerHealth.LIMITED ->
            Triple(ConnectingBadgeFill, R.drawable.ic_globe, "Limited")
        ServerHealth.OFFLINE ->
            Triple(OfflineBadgeFill, R.drawable.ic_alert_triangle, "Offline")
    }

    Row(
        modifier = modifier
            .semantics { contentDescription = "Server status: $label" }
            .background(fill, RoundedCornerShape(Dimens.RadiusSmall))
            .border(Dimens.BorderThin, CardBorder, RoundedCornerShape(Dimens.RadiusSmall))
            .padding(
                horizontal = Dimens.BadgePaddingHorizontal,
                vertical = Dimens.BadgePaddingVertical
            ),
        horizontalArrangement = Arrangement.spacedBy(Dimens.BadgeIconGap),
        verticalAlignment = Alignment.CenterVertically
    ) {
        Icon(
            painter = painterResource(id = glyph),
            // The row's semantics already announce the whole statement once; a
            // contentDescription here would make TalkBack read the glyph first.
            contentDescription = null,
            tint = TextPrimary,
            modifier = Modifier.size(16.dp)
        )
        Text(
            text = label,
            color = TextPrimary,
            fontFamily = NunitoFontFamily,
            fontWeight = FontWeight.Bold,
            fontSize = 15.sp
        )
    }
}
