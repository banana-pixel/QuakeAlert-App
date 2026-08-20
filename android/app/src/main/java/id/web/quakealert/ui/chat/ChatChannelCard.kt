package id.web.quakealert.ui.chat

import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.material3.minimumInteractiveComponentSize
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import id.web.quakealert.R
import id.web.quakealert.ui.theme.CardBorder
import id.web.quakealert.ui.theme.CardSubtitle
import id.web.quakealert.ui.theme.CardTitle
import id.web.quakealert.ui.theme.ChatChannelCardGradient
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.QuakeAlertTheme
import id.web.quakealert.ui.theme.TextPrimary

/**
 * Pinned channel/network header card for the Chat screen (Figma node 1:934):
 * a fixed-height stadium card with a teal→olive horizontal gradient fill and a
 * 1dp white-10% stroke. Shows a leading globe glyph + channel name and an
 * "N users online" subtitle, with a trailing switch-channel icon button.
 *
 * @param channel active channel summary (name + online count).
 * @param onSwitchChannel invoked when the trailing switch icon is tapped.
 */
@Composable
fun ChatChannelCard(
    channel: ChatChannelInfo,
    onSwitchChannel: () -> Unit,
    modifier: Modifier = Modifier
) {
    val shape = RoundedCornerShape(Dimens.RadiusCard)

    Row(
        modifier = modifier
            .fillMaxWidth()
            .heightIn(min = Dimens.ChatChannelCardHeight)
            .clip(shape)
            .background(ChatChannelCardGradient, shape)
            .border(Dimens.BorderThin, CardBorder, shape)
            .padding(
                horizontal = Dimens.ChatChannelCardPaddingHorizontal,
                vertical = Dimens.ChatChannelCardPaddingVertical
            ),
        horizontalArrangement = Arrangement.spacedBy(Dimens.ChatChannelCardContentGap),
        verticalAlignment = Alignment.CenterVertically
    ) {
        Column(
            modifier = Modifier.weight(1f),
            verticalArrangement = Arrangement.spacedBy(Dimens.ChatChannelTitleGap)
        ) {
            Row(
                horizontalArrangement = Arrangement.spacedBy(Dimens.ChatChannelIconGap),
                verticalAlignment = Alignment.CenterVertically
            ) {
                Image(
                    painter = painterResource(id = R.drawable.ic_globe_network),
                    contentDescription = null,
                    modifier = Modifier.size(
                        width = Dimens.ChatChannelGlobeWidth,
                        height = Dimens.ChatChannelGlobeHeight
                    )
                )
                Text(text = channel.channelName, style = CardTitle)
            }
            Text(text = channel.onlineLabel, style = CardSubtitle)
        }

        // Switch-channel action: the glyph keeps its 22dp Figma token while
        // minimumInteractiveComponentSize lifts the touch box to the 48dp minimum,
        // and the tap carries the standard ripple (previously suppressed with
        // indication = null).
        Box(
            modifier = Modifier
                .minimumInteractiveComponentSize()
                .size(Dimens.ChatChannelSwitchIconSize)
                .clip(RoundedCornerShape(Dimens.RadiusSmall))
                .clickable(role = Role.Button, onClick = onSwitchChannel),
            contentAlignment = Alignment.Center
        ) {
            Icon(
                painter = painterResource(id = R.drawable.ic_switch_horizontal),
                contentDescription = "Switch channel",
                tint = TextPrimary,
                modifier = Modifier.size(Dimens.ChatChannelSwitchIconSize)
            )
        }
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun ChatChannelCardPreview() {
    QuakeAlertTheme {
        ChatChannelCard(
            channel = ChatChannelInfo(channelName = "West Java Mesh", usersOnline = 12),
            onSwitchChannel = {},
            modifier = Modifier.padding(16.dp)
        )
    }
}
