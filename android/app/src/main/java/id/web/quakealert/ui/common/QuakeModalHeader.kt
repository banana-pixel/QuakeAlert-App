package id.web.quakealert.ui.common

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.res.painterResource
import id.web.quakealert.R
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.ModalCloseFill
import id.web.quakealert.ui.theme.ModalTitle
import id.web.quakealert.ui.theme.TextPrimary

/**
 * Shared overlay header (Figma nodes 123:1003 / 124:1201 / 124:1614): the [title]
 * optically centered across the full card width with the circular close button
 * pinned to the trailing edge.
 *
 * Figma balances the title against an empty 20dp spacer on the leading side, which
 * leaves it a few dp off-centre; centering against the container instead honours
 * the design's intent and stays symmetric at every width. Extracted so the About,
 * Earthquake Details and Earthquake Possibility overlays all share one source of
 * truth for the [Dimens.ModalCloseSize] chrome.
 *
 * @param onDismiss invoked by the close button.
 * @param title centered overlay header text.
 */
@Composable
fun QuakeModalHeader(
    onDismiss: () -> Unit,
    title: String,
    modifier: Modifier = Modifier
) {
    val closeShape = remember { RoundedCornerShape(Dimens.ModalCloseRadius) }

    Box(modifier = modifier.fillMaxWidth(), contentAlignment = Alignment.Center) {
        Text(text = title, style = ModalTitle)

        Box(
            modifier = Modifier
                .align(Alignment.CenterEnd)
                .size(Dimens.ModalCloseSize)
                .clip(closeShape)
                .background(ModalCloseFill, closeShape)
                .clickable(onClick = onDismiss),
            contentAlignment = Alignment.Center
        ) {
            Icon(
                painter = painterResource(id = R.drawable.ic_close),
                contentDescription = "Close",
                tint = TextPrimary,
                modifier = Modifier.size(Dimens.ModalCloseIconSize)
            )
        }
    }
}