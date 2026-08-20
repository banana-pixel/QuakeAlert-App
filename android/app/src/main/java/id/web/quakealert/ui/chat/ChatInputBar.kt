package id.web.quakealert.ui.chat

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import id.web.quakealert.R
import id.web.quakealert.ui.theme.CardBorder
import id.web.quakealert.ui.theme.CardTitle
import id.web.quakealert.ui.theme.ChatInputFill
import id.web.quakealert.ui.theme.ChatSendButtonFill
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.QuakeAlertTheme
import id.web.quakealert.ui.theme.TextPrimary
import id.web.quakealert.ui.theme.TextSecondary

/**
 * Bottom message-composer row for the Chat screen (Figma nodes 1:1013 field +
 * 1:1020 send button): a rounded, translucent input field on the left and a
 * fixed 50×50 cyan send button on the right.
 *
 * The field is a [BasicTextField] (not a Material `TextField`) so the flat dark
 * styling is fully custom, with a placeholder rendered only when [value] is
 * empty. The caller hoists the text state and both callbacks (UDF). Pressing the
 * IME send action forwards to [onSend].
 *
 * @param value current draft text.
 * @param onValueChange emitted as the user types.
 * @param onSend invoked by the send button and the IME send action.
 * @param canSend whether the send button is enabled (non-blank draft).
 */
@Composable
fun ChatInputBar(
    value: String,
    onValueChange: (String) -> Unit,
    onSend: () -> Unit,
    canSend: Boolean,
    modifier: Modifier = Modifier
) {
    Row(
        modifier = modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.spacedBy(Dimens.ChatInputRowGap),
        verticalAlignment = Alignment.CenterVertically
    ) {
        val fieldShape = RoundedCornerShape(Dimens.ChatInputFieldRadius)
        BasicTextField(
            value = value,
            onValueChange = onValueChange,
            modifier = Modifier
                .weight(1f)
                .clip(fieldShape)
                .background(ChatInputFill, fieldShape)
                .border(Dimens.BorderThin, CardBorder, fieldShape)
                .padding(
                    horizontal = Dimens.ChatInputFieldPaddingHorizontal,
                    vertical = Dimens.ChatInputFieldPaddingVertical
                ),
            textStyle = CardTitle,
            singleLine = true,
            cursorBrush = SolidColor(TextPrimary),
            keyboardOptions = KeyboardOptions(imeAction = ImeAction.Send),
            keyboardActions = KeyboardActions(onSend = { if (canSend) onSend() }),
            decorationBox = { innerTextField ->
                if (value.isEmpty()) {
                    Text(text = "Message the mesh...", style = CardTitle, color = TextSecondary)
                }
                innerTextField()
            }
        )

        SendButton(onSend = onSend, canSend = canSend)
    }
}

/**
 * Send action (Figma node 1:1020): a fixed 50×50 rounded square, already past the
 * 48dp minimum touch target. The tap carries the standard ripple (previously
 * suppressed with `indication = null`) and reports its enabled state to TalkBack.
 */
@Composable
private fun SendButton(
    onSend: () -> Unit,
    canSend: Boolean,
    modifier: Modifier = Modifier
) {
    val shape = RoundedCornerShape(Dimens.ChatSendButtonRadius)

    Box(
        modifier = modifier
            .size(Dimens.ChatSendButtonSize)
            .clip(shape)
            .background(ChatSendButtonFill, shape)
            .border(Dimens.BorderThin, CardBorder, shape)
            .clickable(
                enabled = canSend,
                role = Role.Button,
                onClick = onSend
            ),
        contentAlignment = Alignment.Center
    ) {
        Icon(
            painter = painterResource(id = R.drawable.ic_send),
            contentDescription = "Send message",
            tint = TextPrimary,
            modifier = Modifier.size(Dimens.ChatSendIconSize)
        )
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun ChatInputBarPreview() {
    QuakeAlertTheme {
        ChatInputBar(
            value = "",
            onValueChange = {},
            onSend = {},
            canSend = false,
            modifier = Modifier.padding(16.dp)
        )
    }
}
