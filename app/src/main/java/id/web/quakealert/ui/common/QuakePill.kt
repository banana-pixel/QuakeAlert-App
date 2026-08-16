package id.web.quakealert.ui.common

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Shape
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import id.web.quakealert.ui.theme.CardBorder
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.PillFill
import id.web.quakealert.ui.theme.PillLabel
import id.web.quakealert.ui.theme.TextPrimary

/**
 * Shared stadium-capsule badge used identically across the History card's
 * "X km Away" distance badge (Figma node 1:726) and the Sensor card's telemetry
 * / status pills (Figma nodes 1:1123, 1:1126).
 *
 * Single source of truth for capsule chrome so both cards stay pixel-consistent:
 *  - fill: [PillFill] (#373737 stadium fill), overridable for the status variant
 *  - stroke: 1dp [CardBorder] (white 10%) so every capsule edge reads crisply
 *  - shape: fully-rounded [Dimens.RadiusStadium] stadium capsule
 *  - padding: [Dimens.PillPaddingHorizontal] x [Dimens.PillPaddingVertical]
 *  - label: [PillLabel] (Nunito Medium 11/12)
 *
 * @param text the capsule label.
 * @param fill capsule background; defaults to the shared [PillFill].
 * @param dotColor when non-null, renders a solid leading connectivity dot
 *   (e.g. Online/Offline status). When null the capsule is text-only.
 */
@Composable
fun QuakePill(
    text: String,
    modifier: Modifier = Modifier,
    fill: Color = PillFill,
    dotColor: Color? = null
) {
    val shape: Shape = RoundedCornerShape(Dimens.RadiusStadium)
    Row(
        modifier = modifier
            .height(Dimens.PillHeight)
            .clip(shape)
            .background(fill, shape)
            .border(Dimens.BorderThin, CardBorder, shape)
            .padding(
                horizontal = Dimens.PillPaddingHorizontal,
                vertical = Dimens.PillPaddingVertical
            ),
        horizontalArrangement = Arrangement.spacedBy(Dimens.PillDotGap),
        verticalAlignment = Alignment.CenterVertically
    ) {
        if (dotColor != null) {
            Box(
                modifier = Modifier
                    .size(Dimens.PillDotSize)
                    .clip(RoundedCornerShape(Dimens.RadiusStadium))
                    .background(dotColor)
            )
        }
        Text(
            text = text,
            style = PillLabel,
            color = TextPrimary,
            textAlign = TextAlign.Center,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
            softWrap = false
        )
    }
}
