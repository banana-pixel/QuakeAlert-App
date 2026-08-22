package id.web.quakealert.ui.chat

import androidx.compose.animation.AnimatedContent
import androidx.compose.animation.animateColorAsState
import androidx.compose.animation.core.FastOutSlowInEasing
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.tween
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.togetherWith
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
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.rotate
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import id.web.quakealert.R
import id.web.quakealert.domain.ChatChannelKind
import id.web.quakealert.ui.theme.CardBorder
import id.web.quakealert.ui.theme.CardSubtitle
import id.web.quakealert.ui.theme.CardTitle
import id.web.quakealert.ui.theme.ChatChannelGradientEnd
import id.web.quakealert.ui.theme.ChatChannelGradientStart
import id.web.quakealert.ui.theme.ChatRegionalGradientEnd
import id.web.quakealert.ui.theme.ChatRegionalGradientStart
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.QuakeAlertTheme
import id.web.quakealert.ui.theme.TextPrimary

/**
 * Pinned channel/network header card for the Chat screen (Figma node 1:934):
 * a fixed-height stadium card with a horizontal gradient fill and a 1dp white-10%
 * stroke. Shows a leading globe glyph + channel name and a subtitle naming who the
 * room reaches, with a trailing switch-channel icon button.
 *
 * **The fill names the tier**: the Figma teal→olive for Global, warm amber→ember for
 * a regional room. That is not decoration — the tier decides who hears you, and this
 * card is the only thing on screen that says which audience is live.
 *
 * Everything about a switch is animated, because the room's first page takes a round
 * trip and an un-animated header would leave the tap looking unacknowledged until it
 * lands: the two gradient stops cross-fade, the name and subtitle swap through
 * [AnimatedContent], and the switch glyph turns 180° so the press registers on the
 * control the finger is still on.
 *
 * The switch icon is hidden — not merely disabled — when there is nowhere to switch
 * to: a user with no synced position has the global room only, and a control that
 * does nothing when tapped reads as a bug.
 *
 * @param channel active channel summary.
 * @param onSwitchChannel invoked when the trailing switch icon is tapped.
 */
@Composable
fun ChatChannelCard(
    channel: ChatChannelInfo,
    onSwitchChannel: () -> Unit,
    modifier: Modifier = Modifier
) {
    val shape = RoundedCornerShape(Dimens.RadiusCard)
    val regional = channel.kind == ChatChannelKind.REGIONAL
    val spec = tween<Color>(durationMillis = TRANSITION_MS, easing = FastOutSlowInEasing)

    // The stops are animated, not the Brush: a Brush is not an animatable value, so it
    // is rebuilt each frame from two colours that are.
    val gradientStart by animateColorAsState(
        targetValue = if (regional) ChatRegionalGradientStart else ChatChannelGradientStart,
        animationSpec = spec,
        label = "channelGradientStart"
    )
    val gradientEnd by animateColorAsState(
        targetValue = if (regional) ChatRegionalGradientEnd else ChatChannelGradientEnd,
        animationSpec = spec,
        label = "channelGradientEnd"
    )
    // Half a turn per switch, absolute rather than incremental, so the glyph cannot
    // drift out of alignment over many taps.
    val iconRotation by animateFloatAsState(
        targetValue = if (regional) SWITCH_ICON_ROTATION else 0f,
        animationSpec = tween(durationMillis = TRANSITION_MS, easing = FastOutSlowInEasing),
        label = "channelSwitchRotation"
    )

    Row(
        modifier = modifier
            .fillMaxWidth()
            .heightIn(min = Dimens.ChatChannelCardHeight)
            .clip(shape)
            .background(Brush.horizontalGradient(listOf(gradientStart, gradientEnd)), shape)
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
                // Keyed on the name, so re-rendering the same room for any other reason
                // does not blink the title. A plain cross-fade rather than a slide: the
                // card does not move, and content sliding inside a stationary card reads
                // as a glitch rather than as a transition.
                AnimatedContent(
                    targetState = channel.channelName,
                    transitionSpec = { fadeIn(textSpec()) togetherWith fadeOut(textSpec()) },
                    label = "channelName"
                ) { name ->
                    Text(text = name, style = CardTitle)
                }
            }
            AnimatedContent(
                targetState = channel.subtitle,
                transitionSpec = { fadeIn(textSpec()) togetherWith fadeOut(textSpec()) },
                label = "channelSubtitle"
            ) { subtitle ->
                Text(text = subtitle, style = CardSubtitle)
            }
        }

        // Switch-channel action: the glyph keeps its 22dp Figma token while
        // minimumInteractiveComponentSize lifts the touch box to the 48dp minimum,
        // and the tap carries the standard ripple (previously suppressed with
        // indication = null).
        if (channel.canSwitch) {
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
                    // Names the room the tap will move to, not the control's mechanism:
                    // a screen reader user gets the same information the colour gives.
                    contentDescription = if (regional) {
                        "Switch to the global channel"
                    } else {
                        "Switch to your area's channel"
                    },
                    tint = TextPrimary,
                    modifier = Modifier
                        .size(Dimens.ChatChannelSwitchIconSize)
                        .rotate(iconRotation)
                )
            }
        }
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun ChatChannelCardPreview() {
    QuakeAlertTheme {
        Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
            ChatChannelCard(
                channel = ChatChannelInfo(
                    channelName = "Global",
                    subtitle = "Everyone using QuakeAlert",
                    canSwitch = true,
                    kind = ChatChannelKind.GLOBAL
                ),
                onSwitchChannel = {},
                modifier = Modifier.padding(horizontal = 16.dp)
            )
            ChatChannelCard(
                channel = ChatChannelInfo(
                    channelName = "Jawa Barat",
                    subtitle = "People in your area",
                    canSwitch = true,
                    kind = ChatChannelKind.REGIONAL
                ),
                onSwitchChannel = {},
                modifier = Modifier.padding(horizontal = 16.dp)
            )
        }
    }
}

/** Shared fade for the title/subtitle swap, at the card's own transition length. */
private fun textSpec() = tween<Float>(durationMillis = TRANSITION_MS, easing = FastOutSlowInEasing)

/**
 * How long a tier change takes, fill and text alike.
 *
 * Long enough to be seen as a change rather than a repaint, short enough that the card
 * has settled before the new room's first page arrives.
 */
private const val TRANSITION_MS = 250

/** Half a turn: the switch glyph is symmetric, so 180° reads as "swapped". */
private const val SWITCH_ICON_ROTATION = 180f
