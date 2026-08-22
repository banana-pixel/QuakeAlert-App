package id.web.quakealert.ui.chat

import androidx.compose.runtime.Immutable
import id.web.quakealert.domain.ChatChannel
import id.web.quakealert.domain.ChatChannelKind
import id.web.quakealert.ui.common.ErrorCopy

/**
 * A single chat message rendered as a bubble (Figma node 1:925). Messages are
 * either [ChatAuthor.ME] (outgoing, right-aligned cyan bubble) or
 * [ChatAuthor.OTHER] (incoming, left-aligned grey bubble showing the sender's
 * name).
 *
 * @param id stable identity for list keys. The server's `message_id` once stored,
 *   and the `client_message_id` while a send is still in flight — the same key
 *   either way, which is what lets the pending bubble be replaced by the stored one
 *   instead of a second bubble appearing.
 * @param author who sent the message, driving the bubble alignment + styling.
 * @param senderName display name shown above an incoming bubble. A snapshot of the
 *   sender's pseudonym as it was when they wrote, so a reroll does not retro-rename
 *   old messages.
 * @param body the message text.
 * @param time short send timestamp shown beside the bubble (e.g. "09:41").
 * @param sendState only ever anything but [ChatSendState.SENT] for an outgoing
 *   message this device is still trying to deliver.
 */
@Immutable
data class ChatMessage(
    val id: String,
    val author: ChatAuthor,
    val senderName: String,
    val body: String,
    val time: String,
    val sendState: ChatSendState = ChatSendState.SENT
)

/** Origin of a [ChatMessage], deciding bubble alignment and colour treatment. */
enum class ChatAuthor { ME, OTHER }

/**
 * Delivery state of an outgoing message.
 *
 * [FAILED] is a state and not a removal: a message that could not be sent stays on
 * screen so the user can see *which* one did not go out and retry it. Silently
 * dropping it is how someone comes to believe they reported a collapsed building.
 */
enum class ChatSendState { SENDING, SENT, FAILED }

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
 * Header summary for the active channel card (Figma node 1:934).
 *
 * The subtitle says *who is in the room*, not how many: there is no participant
 * count on the wire, and a made-up one ("12 users online", as the mock said) is the
 * kind of number a user would act on during an earthquake.
 *
 * @param channelName the server's `display_name`, so every member of a room sees
 *   the same title.
 * @param subtitle who this room reaches.
 * @param canSwitch whether a second channel exists to switch to. False for a user
 *   with no synced position, who has the global room only.
 * @param kind which tier is live, carried so the card can colour itself from the
 *   domain fact instead of matching on the display name — a room really called
 *   "Global" in some region would otherwise take the global styling.
 */
@Immutable
data class ChatChannelInfo(
    val channelName: String,
    val subtitle: String,
    val canSwitch: Boolean = false,
    val kind: ChatChannelKind = ChatChannelKind.GLOBAL
)

/** The two tiers as the header card describes them. */
internal fun ChatChannel.toChannelInfo(canSwitch: Boolean): ChatChannelInfo = ChatChannelInfo(
    channelName = displayName,
    subtitle = when (kind) {
        ChatChannelKind.GLOBAL -> "Everyone using QuakeAlert"
        ChatChannelKind.REGIONAL -> "People in your area"
    },
    canSwitch = canSwitch,
    kind = kind
)

/**
 * Immutable UI state for the Chat screen (Figma node 1:925). Hoisted into
 * [ChatViewModel] and consumed by the stateless [ChatScreen].
 *
 * @param channel active channel header summary.
 * @param items ordered stream of messages + date separators, oldest first.
 * @param draft the current text in the input field (hoisted for UDF).
 * @param isLoading first page of a channel in flight, with nothing to show yet.
 * @param isLoadingOlder an upward page in flight; the list stays put.
 * @param hasOlder whether an older page may exist. False once a short page comes
 *   back — retention is 7 days, so history genuinely ends.
 * @param isSending a send is in flight, so the composer's action is disabled while
 *   the optimistic bubble is already on screen.
 * @param notice one line explaining a limitation the user would otherwise read as a
 *   bug, e.g. having the global room only because no position has been synced.
 * @param errorCopy set only when the screen has nothing to show; a failure with
 *   messages already on screen is reported through the floating toast instead of
 *   replacing what the user is reading.
 */
@Immutable
data class ChatUiState(
    val channel: ChatChannelInfo = ChatChannelInfo(
        channelName = "Global",
        subtitle = "Everyone using QuakeAlert"
    ),
    val items: List<ChatListItem> = emptyList(),
    val draft: String = "",
    val isLoading: Boolean = false,
    val isLoadingOlder: Boolean = false,
    val hasOlder: Boolean = false,
    val isSending: Boolean = false,
    val notice: String? = null,
    val isError: Boolean = false,
    val errorCopy: ErrorCopy? = null
) {
    /**
     * True when the draft has non-blank content and can be sent.
     *
     * Blocked while a send is in flight: the rate limit is one message per two
     * seconds, so a second tap would be refused by the server, and a refusal the
     * user could have been spared is worse than a briefly disabled button.
     */
    val canSend: Boolean
        get() = draft.isNotBlank() && !isSending

    /** True when the list is empty for a reason other than a failure. */
    val isEmpty: Boolean
        get() = items.isEmpty() && !isLoading && !isError
}
