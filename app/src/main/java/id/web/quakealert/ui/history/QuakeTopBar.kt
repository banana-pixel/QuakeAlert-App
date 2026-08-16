package id.web.quakealert.ui.history

import androidx.compose.foundation.border
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
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
import id.web.quakealert.ui.theme.CardBorder

import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.HealthyBadgeFill
import id.web.quakealert.ui.theme.NunitoFontFamily
import id.web.quakealert.ui.theme.TextPrimary

/**
 * History screen header (Figma node 1:705): the "History" title on the left and
 * a network-status badge on the right. The badge shows a globe icon plus a
 * "Healthy" label when [isHealthy] is true.
 */
@Composable
fun QuakeTopBar(
    isHealthy: Boolean,
    modifier: Modifier = Modifier
) {
    Row(
        modifier = modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalAlignment = Alignment.CenterVertically
    ) {
        Text(
            text = "History",
            color = TextPrimary,
            fontFamily = NunitoFontFamily,
            fontWeight = FontWeight.ExtraBold,
            fontSize = 24.sp,
            lineHeight = 26.sp
        )

        if (isHealthy) {
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
