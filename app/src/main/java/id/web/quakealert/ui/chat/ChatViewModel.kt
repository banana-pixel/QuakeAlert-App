package id.web.quakealert.ui.chat

import androidx.lifecycle.ViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update

/**
 * Hosts the [ChatUiState] for the Chat screen and exposes it as a [StateFlow]
 * following unidirectional data flow. Seeded with mock messages mirroring the
 * Figma design (node 1:925) so the UI can be verified visually before a real
 * mesh-network transport is wired in.
 */
class ChatViewModel : ViewModel() {

    private val _uiState = MutableStateFlow(
        ChatUiState(items = mockConversation())
    )
    val uiState: StateFlow<ChatUiState> = _uiState.asStateFlow()

    /** Updates the hoisted draft as the user types. */
    fun onDraftChanged(text: String) {
        _uiState.update { it.copy(draft = text) }
    }

    /**
     * Appends the current draft as an outgoing message (if non-blank) and clears
     * the input. Uses a monotonic local id so list keys stay stable; a real
     * implementation would delegate to the transport layer.
     */
    fun onSendClicked() {
        _uiState.update { state ->
            if (!state.canSend) return@update state
            val sent = ChatMessage(
                id = "local-${nextLocalId++}",
                author = ChatAuthor.ME,
                senderName = "You",
                body = state.draft.trim(),
                time = "now"
            )
            state.copy(
                items = state.items + ChatListItem.Message(sent),
                draft = ""
            )
        }
    }

    /** Placeholder hook for tapping the channel switch icon. */
    fun onSwitchChannelClicked() {
        // Intentionally empty until channel switching is implemented.
    }

    private var nextLocalId = 0

    private companion object {
        fun mockConversation(): List<ChatListItem> = listOf(
            ChatListItem.DateSeparator(
                ChatDateSeparator(id = "sep-today", label = "Today")
            ),
            ChatListItem.Message(
                ChatMessage(
                    id = "m1",
                    author = ChatAuthor.OTHER,
                    senderName = "Rescue Team",
                    body = "Anyone near Cimahi feeling the aftershocks?",
                    time = "09:38"
                )
            ),
            ChatListItem.Message(
                ChatMessage(
                    id = "m2",
                    author = ChatAuthor.ME,
                    senderName = "You",
                    body = "Yes, felt a light tremor a minute ago. Everyone safe here.",
                    time = "09:39"
                )
            ),
            ChatListItem.Message(
                ChatMessage(
                    id = "m3",
                    author = ChatAuthor.OTHER,
                    senderName = "Ayu",
                    body = "Stay away from the old building on Jl. Merdeka, cracks widening.",
                    time = "09:40"
                )
            ),
            ChatListItem.Message(
                ChatMessage(
                    id = "m4",
                    author = ChatAuthor.ME,
                    senderName = "You",
                    body = "Noted. Sharing my location with the mesh now.",
                    time = "09:41"
                )
            ),
            ChatListItem.Message(
                ChatMessage(
                    id = "m5",
                    author = ChatAuthor.OTHER,
                    senderName = "Rescue Team",
                    body = "Copy. Medical unit is heading to the Cimahi sector, ETA 15 min.",
                    time = "09:43"
                )
            )
        )
    }
}
