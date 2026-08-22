package id.web.quakealert.ui.chat

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.wrapContentWidth
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import id.web.quakealert.ui.theme.CardBorder
import id.web.quakealert.ui.theme.CardSubtitle
import id.web.quakealert.ui.theme.CardTitle
import id.web.quakealert.ui.theme.ChatIncomingFill
import id.web.quakealert.ui.theme.ChatOutgoingBorder
import id.web.quakealert.ui.theme.ChatOutgoingFill
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.QuakeAlertTheme
import id.web.quakealert.ui.theme.SectionHeaderPillFill
import id.web.quakealert.ui.theme.TextSecondary

/**
 * A single chat message row (Figma node 1:925). Incoming messages
 * ([ChatAuthor.OTHER]) align to the start with a grey [ChatIncomingFill] bubble
 * that shows the sender's name; outgoing messages ([ChatAuthor.ME]) align to
 * the end with a cyan [ChatOutgoingFill] bubble.
 *
 * Following modern chat-UI convention, the timestamp lives *inside* the bubble,
 * locked to the bottom-end corner via a trailing [Row] aligned with
 * [Alignment.End]. Layout order inside the bubble: sender name (incoming only),
 * message body, then the bottom-end timestamp.
 *
 * The bubble is width-constrained to [Dimens.ChatBubbleMaxWidthFraction] of the
 * row so long messages wrap instead of stretching edge-to-edge.
 */
@Composable
fun ChatBubble(
    message: ChatMessage,
    onRetry: () -> Unit = {},
    modifier: Modifier = Modifier
) {
    val isMine = message.author == ChatAuthor.ME
    val bubbleShape = RoundedCornerShape(Dimens.ChatBubbleRadius)

    Row(
        modifier = modifier.fillMaxWidth(),
        horizontalArrangement = if (isMine) Arrangement.End else Arrangement.Start
    ) {
        val bubbleModifier = Modifier
            .fillMaxWidth(Dimens.ChatBubbleMaxWidthFraction)
            .wrapContentWidth(if (isMine) Alignment.End else Alignment.Start)
            .clip(bubbleShape)
            .then(
                if (isMine) {
                    Modifier
                        .background(ChatOutgoingFill, bubbleShape)
                        .border(Dimens.BorderThin, ChatOutgoingBorder, bubbleShape)
                } else {
                    Modifier
                        .background(ChatIncomingFill, bubbleShape)
                        .border(Dimens.BorderThin, CardBorder, bubbleShape)
                }
            )
            .padding(
                horizontal = Dimens.ChatBubblePaddingHorizontal,
                vertical = Dimens.ChatBubblePaddingVertical
            )
            .then(
                if (message.sendState == ChatSendState.FAILED) {
                    Modifier.clickable(role = Role.Button, onClick = onRetry)
                } else {
                    Modifier
                }
            )

        Column(
            modifier = bubbleModifier,
            verticalArrangement = Arrangement.spacedBy(Dimens.ChatBubbleContentGap)
        ) {
            if (!isMine) {
                Text(
                    text = message.senderName,
                    style = CardSubtitle,
                    fontWeight = FontWeight.Bold
                )
            }
            Text(text = message.body, style = CardTitle, fontWeight = FontWeight.Normal)

            // Timestamp locked to the bottom-end corner inside the bubble, with the
            // delivery state beside it for a message still on its way out. A failed
            // send says what to do about it rather than only that it failed, and the
            // whole bubble is the retry target — a 12sp word is not a touch target.
            Row(
                modifier = Modifier.align(Alignment.End),
                horizontalArrangement = Arrangement.spacedBy(Dimens.ChatBubbleContentGap),
                verticalAlignment = Alignment.CenterVertically
            ) {
                when (message.sendState) {
                    ChatSendState.SENDING -> Timestamp("Sending...")
                    ChatSendState.FAILED -> Timestamp("Not sent. Tap to retry")
                    ChatSendState.SENT -> Unit
                }
                Timestamp(message.time)
            }
        }
    }
}

@Composable
private fun Timestamp(time: String, modifier: Modifier = Modifier) {
    Text(text = time, style = CardSubtitle, color = TextSecondary, modifier = modifier)
}

/**
 * A centered date-separator pill (Figma date chip) marking a new calendar day in
 * the message stream (e.g. "Today", "Yesterday").
 *
 * Styled consistently with the Settings [SectionHeaderPill] ("Location &
 * Coverage") — same [Dimens.RadiusStadium] capsule, [SectionHeaderPillFill] fill,
 * [CardBorder] stroke, [Dimens.SectionHeaderPillHeight] height,
 * [Dimens.SectionHeaderPillPaddingHorizontal] internal padding and [CardTitle]
 * label — so date chips read as the same design element as section headers.
 */
@Composable
fun ChatDateSeparatorRow(
    separator: ChatDateSeparator,
    modifier: Modifier = Modifier
) {
    Box(
        modifier = modifier.fillMaxWidth(),
        contentAlignment = Alignment.Center
    ) {
        val shape = RoundedCornerShape(Dimens.RadiusStadium)
        Box(
            modifier = Modifier
                .height(Dimens.SectionHeaderPillHeight)
                .clip(shape)
                .background(SectionHeaderPillFill, shape)
                .border(Dimens.BorderThin, CardBorder, shape)
                .padding(horizontal = Dimens.SectionHeaderPillPaddingHorizontal),
            contentAlignment = Alignment.Center
        ) {
            Text(
                text = separator.label,
                style = CardTitle,
                textAlign = TextAlign.Center
            )
        }
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun ChatBubblePreview() {
    QuakeAlertTheme {
        Column(
            modifier = Modifier.padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp)
        ) {
            ChatDateSeparatorRow(ChatDateSeparator("s", "Today"))
            ChatBubble(
                ChatMessage("1", ChatAuthor.OTHER, "Rescue Team", "Anyone near Cimahi feeling the aftershocks?", "09:38")
            )
            ChatBubble(
                ChatMessage("2", ChatAuthor.ME, "You", "Yes, felt a light tremor. Everyone safe here.", "09:39")
            )
            ChatBubble(
                ChatMessage(
                    id = "3",
                    author = ChatAuthor.ME,
                    senderName = "You",
                    body = "Road is blocked near the market.",
                    time = "09:40",
                    sendState = ChatSendState.FAILED
                )
            )
        }
    }
}
