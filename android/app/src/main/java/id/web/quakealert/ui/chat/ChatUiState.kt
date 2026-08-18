package id.web.quakealert.ui.chat

import androidx.compose.runtime.Immutable

/**
 * A single chat message rendered as a bubble (Figma node 1:925). Messages are
 * either [ChatAuthor.ME] (outgoing, right-aligned cyan bubble) or
 * [ChatAuthor.OTHER] (incoming, left-aligned grey bubble showing the sender's
 * name).
 *
 * @param id stable identity for list keys.
 * @param author who sent the message, driving the bubble alignment + styling.
 * @param senderName display name shown above an incoming bubble (e.g. "Rescue Team").
 * @param body the message text.
 * @param time short send timestamp shown beside the bubble (e.g. "09:41").
 */
@Immutable
data class ChatMessage(
    val id: String,
    val author: ChatAuthor,
    val senderName: String,
    val body: String,
    val time: String
)

/** Origin of a [ChatMessage], deciding bubble alignment and colour treatment. */
enum class ChatAuthor { ME, OTHER }

/**
 * A distinct calendar-day separator inserted between message groups (Figma date
 * pill e.g. "Today", "Yesterday").
 *
 * @param id stable identity for list keys.
 * @param label human-readable day label.
 */
@Immutable
data class ChatDateSeparator(
    val id: String,
    val label: String
)

/**
 * Discriminated union of the two row types the chat list can render: a message
 * bubble or a date separator. Modelled as a sealed interface so the [ChatScreen]
 * LazyColumn can lay out a single ordered stream while keeping each variant
 * strongly typed.
 */
sealed interface ChatListItem {
    val id: String

    @Immutable
    data class Message(val message: ChatMessage) : ChatListItem {
        override val id: String get() = message.id
    }

    @Immutable
    data class DateSeparator(val separator: ChatDateSeparator) : ChatListItem {
        override val id: String get() = separator.id
    }
}

/**
 * Header summary for the active chat channel/network card (Figma node 1:934):
 * the connected mesh/network name and how many users are currently online.
 *
 * @param channelName active channel name (e.g. "West Java Mesh").
 * @param usersOnline number of participants currently connected.
 */
@Immutable
data class ChatChannelInfo(
    val channelName: String,
    val usersOnline: Int
) {
    /** Pre-formatted "N users online" subtitle. */
    val onlineLabel: String
        get() = "$usersOnline users online"
}

/**
 * Immutable UI state for the Chat screen (Figma node 1:925). Hoisted into
 * [ChatViewModel] and consumed by the stateless [ChatScreen].
 *
 * @param isHealthy drives the shared [id.web.quakealert.ui.common.QuakeAppBar]
 *   network-status badge.
 * @param channel active channel header summary.
 * @param items ordered stream of messages + date separators to render.
 * @param draft the current text in the input field (hoisted for UDF).
 */
@Immutable
data class ChatUiState(
    val isHealthy: Boolean = true,
    val channel: ChatChannelInfo = ChatChannelInfo(
        channelName = "West Java Mesh",
        usersOnline = 12
    ),
    val items: List<ChatListItem> = emptyList(),
    val draft: String = ""
) {
    /** True when the draft has non-blank content and can be sent. */
    val canSend: Boolean
        get() = draft.isNotBlank()
}
