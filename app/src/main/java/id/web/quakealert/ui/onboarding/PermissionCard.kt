package id.web.quakealert.ui.onboarding

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import id.web.quakealert.R
import id.web.quakealert.ui.common.QuakeCard
import id.web.quakealert.ui.theme.BorderFaint
import id.web.quakealert.ui.theme.BorderLight
import id.web.quakealert.ui.theme.NunitoFontFamily
import id.web.quakealert.ui.theme.OverlayLight
import id.web.quakealert.ui.theme.SuccessGreen
import id.web.quakealert.ui.theme.SuccessGreenTranslucent
import id.web.quakealert.ui.theme.TextPrimary
import id.web.quakealert.ui.theme.TextSecondary

/**
 * Dark rounded card used on the onboarding permission pages (Figma node 1:363)
 * to request a runtime permission. Built on the shared [QuakeCard] so its
 * chrome (surface, 1dp border, radius, padding) is byte-identical to the
 * Settings setting rows. Shows a title and a status badge underneath:
 *  - Granted  → green badge with a check-circle and "Allowed".
 *  - Not yet  → neutral badge prompting the user to tap to allow.
 *
 * The whole card is clickable while not yet granted; tapping triggers
 * [onClick], which the caller wires to the system permission launcher.
 */
@Composable
fun PermissionCard(
    title: String,
    isGranted: Boolean,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    grantedLabel: String = "Allowed"
) {
    QuakeCard(
        title = title,
        modifier = modifier,
        onClick = if (isGranted) null else onClick,
        detail = { StatusBadge(isGranted = isGranted, grantedLabel = grantedLabel) }
    )
}

@Composable
private fun StatusBadge(isGranted: Boolean, grantedLabel: String) {
    val backgroundColor = if (isGranted) SuccessGreenTranslucent else OverlayLight
    val borderColor = if (isGranted) BorderFaint else BorderLight
    val label = if (isGranted) grantedLabel else "Tap to allow"

    Row(
        modifier = Modifier
            .clip(RoundedCornerShape(10.dp))
            .background(backgroundColor)
            .border(
                width = 1.dp,
                color = borderColor,
                shape = RoundedCornerShape(10.dp)
            )
            .padding(horizontal = 8.dp, vertical = 2.dp),
        horizontalArrangement = Arrangement.spacedBy(5.dp),
        verticalAlignment = Alignment.CenterVertically
    ) {
        if (isGranted) {
            Icon(
                painter = painterResource(id = R.drawable.ic_check_circle),
                contentDescription = null,
                tint = SuccessGreen,
                modifier = Modifier.size(16.dp)
            )
        }
        Text(
            text = label,
            color = if (isGranted) TextPrimary else TextSecondary,
            fontFamily = NunitoFontFamily,
            fontWeight = FontWeight.Bold,
            fontSize = 14.sp,
            lineHeight = 20.sp
        )
    }
}
