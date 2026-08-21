package id.web.quakealert.ui.chat

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListState
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.tooling.preview.Preview
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import id.web.quakealert.domain.ServerConnectionState
import id.web.quakealert.ui.common.QuakeAppBar
import id.web.quakealert.ui.common.fadingEdges
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.QuakeAlertTheme

/**
 * Stateful entry point that connects [ChatViewModel] to the stateless
 * [ChatScreen]. Kept thin so the presentation layer stays testable.
 *
 * @param listState message-list scroll position, hoisted to
 *   [id.web.quakealert.ui.main.MainScreen] so it survives tab switches, rotation
 *   and process death.
 */
@Composable
fun ChatRoute(
    connectionState: ServerConnectionState,
    modifier: Modifier = Modifier,
    listState: LazyListState = rememberLazyListState(),
    viewModel: ChatViewModel = viewModel()
) {
    val uiState by viewModel.uiState.collectAsStateWithLifecycle()

    ChatScreen(
        uiState = uiState,
        connectionState = connectionState,
        onDraftChanged = viewModel::onDraftChanged,
        onSendClicked = viewModel::onSendClicked,
        onSwitchChannelClicked = viewModel::onSwitchChannelClicked,
        listState = listState,
        modifier = modifier
    )
}

/**
 * Stateless Chat screen (Figma node 1:925). Structure, top → bottom, mirrors the
 * History/Sensors layout so all tabs share behaviour:
 *  1. A static header [Column] pinned to the top: shared [QuakeAppBar] +
 *     [ChatChannelCard].
 *  2. A weighted [LazyColumn] filling the space between the header and the input
 *     bar, carrying the shared soft [fadingEdges] so bubbles dissolve at the
 *     scroll bounds.
 *  3. A pinned [ChatInputBar] at the bottom. [imePadding] lifts the whole column
 *     above the soft keyboard so the composer stays visible while typing.
 *
 * All state and events are hoisted to the caller ([ChatRoute] / [ChatViewModel]).
 */
@Composable
fun ChatScreen(
    uiState: ChatUiState,
    connectionState: ServerConnectionState = ServerConnectionState.CONNECTED,
    onDraftChanged: (String) -> Unit,
    onSendClicked: () -> Unit,
    onSwitchChannelClicked: () -> Unit,
    modifier: Modifier = Modifier,
    listState: LazyListState = rememberLazyListState()
) {
    Column(
        modifier = modifier
            .fillMaxSize()
            .imePadding()
            .padding(horizontal = Dimens.ScreenHorizontalPadding)
    ) {
        // --- Static header: title + channel card -----------------------------
        QuakeAppBar(title = "Chat", connectionState = connectionState)

        ChatChannelCard(
            channel = uiState.channel,
            onSwitchChannel = onSwitchChannelClicked,
            modifier = Modifier.padding(top = Dimens.HeaderSectionGap)
        )

        // --- Scrolling message list, bounded by header and input bar ---------
        LazyColumn(
            state = listState,
            modifier = Modifier
                .weight(1f)
                .fillMaxWidth()
                .fadingEdges(),
            contentPadding = PaddingValues(
                top = Dimens.ChatListTopPadding,
                bottom = Dimens.ChatListBottomPadding
            ),
            verticalArrangement = Arrangement.spacedBy(Dimens.ChatMessageSpacing)
        ) {
            items(
                items = uiState.items,
                key = { it.id },
                contentType = { it::class }
            ) { item ->
                when (item) {
                    is ChatListItem.Message -> ChatBubble(message = item.message)
                    is ChatListItem.DateSeparator ->
                        ChatDateSeparatorRow(separator = item.separator)
                }
            }
        }

        // --- Pinned composer -------------------------------------------------
        ChatInputBar(
            value = uiState.draft,
            onValueChange = onDraftChanged,
            onSend = onSendClicked,
            canSend = uiState.canSend,
            modifier = Modifier.padding(bottom = Dimens.ChatInputBottomPadding)
        )
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun ChatScreenPreview() {
    QuakeAlertTheme {
        ChatScreen(
            uiState = ChatUiState(
                items = listOf(
                    ChatListItem.DateSeparator(ChatDateSeparator("s", "Today")),
                    ChatListItem.Message(
                        ChatMessage("1", ChatAuthor.OTHER, "Rescue Team", "Anyone near Cimahi feeling the aftershocks?", "09:38")
                    ),
                    ChatListItem.Message(
                        ChatMessage("2", ChatAuthor.ME, "You", "Yes, felt a light tremor. Everyone safe here.", "09:39")
                    ),
                    ChatListItem.Message(
                        ChatMessage("3", ChatAuthor.OTHER, "Ayu", "Stay away from the old building on Jl. Merdeka.", "09:40")
                    )
                )
            ),
            onDraftChanged = {},
            onSendClicked = {},
            onSwitchChannelClicked = {}
        )
    }
}
