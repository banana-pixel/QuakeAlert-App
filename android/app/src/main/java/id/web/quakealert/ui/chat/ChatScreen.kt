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
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.tooling.preview.Preview
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import id.web.quakealert.R
import id.web.quakealert.data.network.ServerHealth
import id.web.quakealert.ui.common.GenericErrorCopy
import id.web.quakealert.ui.common.QuakeAppBar
import id.web.quakealert.ui.common.QuakeEmptyState
import id.web.quakealert.ui.common.QuakeErrorState
import id.web.quakealert.ui.common.QuakeLoadingState
import id.web.quakealert.ui.common.fadingEdges
import id.web.quakealert.ui.theme.CardSubtitle
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.QuakeAlertTheme
import id.web.quakealert.ui.theme.TextSecondary

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
    health: ServerHealth,
    onOpenUpdates: () -> Unit = {},
    modifier: Modifier = Modifier,
    listState: LazyListState = rememberLazyListState(),
    viewModel: ChatViewModel = viewModel()
) {
    val uiState by viewModel.uiState.collectAsStateWithLifecycle()

    ChatScreen(
        uiState = uiState,
        health = health,
        onOpenUpdates = onOpenUpdates,
        onDraftChanged = viewModel::onDraftChanged,
        onSendClicked = viewModel::onSendClicked,
        onSwitchChannelClicked = viewModel::onSwitchChannelClicked,
        onRetry = viewModel::onRetry,
        onRetrySend = viewModel::onRetrySend,
        onLoadOlder = viewModel::onLoadOlder,
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
    health: ServerHealth = ServerHealth.HEALTHY,
    onOpenUpdates: () -> Unit = {},
    onDraftChanged: (String) -> Unit,
    onSendClicked: () -> Unit,
    onSwitchChannelClicked: () -> Unit,
    onRetry: () -> Unit = {},
    onRetrySend: (String) -> Unit = {},
    onLoadOlder: () -> Unit = {},
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
        QuakeAppBar(title = "Chat", health = health, onUpdatesClicked = onOpenUpdates)

        ChatChannelCard(
            channel = uiState.channel,
            onSwitchChannel = onSwitchChannelClicked,
            modifier = Modifier.padding(top = Dimens.HeaderSectionGap)
        )

        // One line naming the only limitation the user can act on: no regional room
        // until a position has been synced. Stated rather than left as an absence,
        // because a feature that is half there looks broken without the reason.
        uiState.notice?.let { notice ->
            Text(
                text = notice,
                style = CardSubtitle,
                color = TextSecondary,
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(top = Dimens.ChatListTopPadding)
            )
        }

        // Page upwards from the scroll position rather than from the first item's
        // composition: derivedStateOf recomputes only when the leading index crosses
        // the threshold, so onLoadOlder fires once per page instead of once per frame.
        val canLoadOlder = uiState.hasOlder && !uiState.isLoadingOlder && !uiState.isLoading
        val shouldLoadOlder by remember(listState, uiState.items.size, canLoadOlder) {
            derivedStateOf {
                val firstVisible = listState.layoutInfo.visibleItemsInfo.firstOrNull()?.index
                canLoadOlder && firstVisible != null && firstVisible <= LOAD_OLDER_THRESHOLD
            }
        }
        LaunchedEffect(shouldLoadOlder) {
            if (shouldLoadOlder) onLoadOlder()
        }

        // A room opens at its newest message, and a switch returns there. The stream is
        // oldest-first, so "newest" is the last index. Keyed on the channel and on
        // content merely appearing — not on the item count, which would drag the list
        // back down every time an older page landed above the user's reading position.
        val newestIndex = uiState.items.lastIndex
        LaunchedEffect(uiState.channel.channelName, newestIndex >= 0) {
            if (newestIndex >= 0) listState.scrollToItem(newestIndex)
        }

        // --- Body: loading / error / empty / the conversation ----------------
        val bodyModifier = Modifier
            .weight(1f)
            .fillMaxWidth()

        when {
            uiState.isLoading && uiState.items.isEmpty() ->
                QuakeLoadingState(modifier = bodyModifier, message = LOADING_MESSAGE)

            uiState.isError -> QuakeErrorState(
                copy = uiState.errorCopy ?: GenericErrorCopy,
                onRetry = onRetry,
                modifier = bodyModifier
            )

            // An empty room is a valid answer, not a failure: retention is 7 days, so
            // a quiet week really does leave nothing to show.
            uiState.isEmpty -> QuakeEmptyState(
                icon = R.drawable.ic_nav_chat,
                message = "No messages yet",
                subtitle = "Be the first to say what it is like where you are.",
                modifier = bodyModifier
            )

            else -> LazyColumn(
                state = listState,
                modifier = bodyModifier.fadingEdges(),
                contentPadding = PaddingValues(
                    top = Dimens.ChatListTopPadding,
                    bottom = Dimens.ChatListBottomPadding
                ),
                verticalArrangement = Arrangement.spacedBy(Dimens.ChatMessageSpacing)
            ) {
                // Above the oldest bubble, so the affordance sits where the content it
                // is fetching will appear.
                if (uiState.isLoadingOlder) {
                    item(key = "loading-older", contentType = "loading-older") {
                        Text(
                            text = LOADING_OLDER_MESSAGE,
                            style = CardSubtitle,
                            color = TextSecondary,
                            textAlign = TextAlign.Center,
                            modifier = Modifier.fillMaxWidth()
                        )
                    }
                }

                items(
                    items = uiState.items,
                    key = { it.id },
                    contentType = { it::class }
                ) { item ->
                    when (item) {
                        is ChatListItem.Message -> ChatBubble(
                            message = item.message,
                            onRetry = { onRetrySend(item.message.id) }
                        )

                        is ChatListItem.DateSeparator ->
                            ChatDateSeparatorRow(separator = item.separator)
                    }
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

/** Copy under the spinner while a room's first page is in flight. */
private const val LOADING_MESSAGE = "Loading messages..."

/** Copy shown above the oldest bubble while an upward page is in flight. */
private const val LOADING_OLDER_MESSAGE = "Loading older messages..."

/**
 * How close to the top of the list a scroll must come before the next older page is
 * requested. Two rows of lead time, so the page lands before the user reaches the
 * end of what is loaded.
 */
private const val LOAD_OLDER_THRESHOLD = 2
