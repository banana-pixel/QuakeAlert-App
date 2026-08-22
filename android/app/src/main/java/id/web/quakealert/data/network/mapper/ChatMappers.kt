package id.web.quakealert.data.network.mapper

import id.web.quakealert.data.network.model.ChatChannelDto
import id.web.quakealert.data.network.model.ChatChannelsResponseDto
import id.web.quakealert.data.network.model.ChatMessageDto
import id.web.quakealert.data.network.model.WsChatMessageDto
import id.web.quakealert.domain.ChatChannel
import id.web.quakealert.domain.ChatChannelKind
import id.web.quakealert.domain.ChatMessageEntry
import id.web.quakealert.ui.chat.ChatAuthor
import id.web.quakealert.ui.chat.ChatDateSeparator
import id.web.quakealert.ui.chat.ChatListItem
import id.web.quakealert.ui.chat.ChatMessage
import id.web.quakealert.ui.chat.ChatSendState
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneId

/**
 * Wire → domain for the channel list.
 *
 * An unrecognised `kind` maps to [ChatChannelKind.REGIONAL] rather than being
 * dropped: the server only lists rooms this identity may read, so a tier this build
 * does not know about should still be readable — it just does not get the global
 * room's "always available" framing.
 */
fun ChatChannelDto.toDomain(): ChatChannel = ChatChannel(
    id = channelId,
    kind = if (kind.equals("GLOBAL", ignoreCase = true)) {
        ChatChannelKind.GLOBAL
    } else {
        ChatChannelKind.REGIONAL
    },
    displayName = displayName.ifBlank { channelId }
)

/** Drops entries with no `channel_id`: an unnamed room cannot be requested. */
fun List<ChatChannelDto>.toDomain(): List<ChatChannel> =
    filter { it.channelId.isNotBlank() }.map { it.toDomain() }

/** See [ChatChannelsResponseDto]; `global` arrives first and that order is kept. */
fun ChatChannelsResponseDto.toDomain(): List<ChatChannel> = channels.toDomain()

/**
 * Wire → domain for a stored message.
 *
 * @param ownUserId this device's `user_id`, or null before the identity bootstrap.
 *   Null means nothing is marked as own, which renders every bubble as incoming —
 *   the safe way to be wrong, since a stranger's message styled as yours is worse
 *   than your own message styled as a stranger's.
 */
fun ChatMessageDto.toDomain(ownUserId: String?): ChatMessageEntry = ChatMessageEntry(
    messageId = messageId,
    channelId = channelId,
    senderId = senderId,
    senderPseudonym = senderPseudonym,
    senderLocationTag = senderLocationTag?.takeIf { it.isNotBlank() },
    body = message,
    isAdmin = isAdmin,
    isOwn = ownUserId != null && senderId == ownUserId,
    createdAt = createdAt.toInstantOrEpoch()
)

fun List<ChatMessageDto>.toDomain(ownUserId: String?): List<ChatMessageEntry> =
    map { it.toDomain(ownUserId) }

/**
 * Socket frame → domain, so a message that arrives live and the same message read
 * back from `GET /chat/messages` are indistinguishable downstream.
 *
 * The only difference on the wire is the clock: the socket sends ms epoch, REST
 * sends RFC3339 (docs/CLIENT_SPEC.md §5.4 vs §4.6).
 */
fun WsChatMessageDto.toDomain(ownUserId: String?): ChatMessageEntry = ChatMessageEntry(
    messageId = messageId,
    channelId = channelId,
    senderId = senderId,
    senderPseudonym = senderPseudonym,
    senderLocationTag = senderLocationTag?.takeIf { it.isNotBlank() },
    body = message,
    isAdmin = isAdmin,
    isOwn = ownUserId != null && senderId == ownUserId,
    createdAt = Instant.ofEpochMilli(timestamp)
)

/**
 * An unparseable or absent `created_at` becomes the epoch rather than throwing.
 *
 * The message text is the part the user needs; losing a whole bubble over a
 * timestamp the server should not have malformed would be the worse trade. Such a
 * row sorts to the top of the history, where it is visibly odd instead of silently
 * misplaced among today's messages.
 */
private fun String.toInstantOrEpoch(): Instant =
    runCatching { Instant.parse(this) }.getOrDefault(Instant.EPOCH)

/**
 * Domain → the ordered stream the chat list renders: messages **ascending** (oldest
 * first, newest at the bottom) with a date pill inserted whenever the calendar day
 * changes.
 *
 * Pure, and separated from the ViewModel for that reason — the separator rule is the
 * one piece of judgement in the chat pipeline, and it depends on a clock and a zone
 * that a test needs to control.
 *
 * @param entries in any order; sorted here so a socket frame appended to a REST page
 *   cannot leave the list out of order.
 * @param zone the device zone. Day boundaries are the user's, not UTC's: a message
 *   sent at 07:00 WIB is "Today" even though UTC still says yesterday.
 */
fun toChatListItems(
    entries: List<ChatMessageEntry>,
    sendStates: Map<String, ChatSendState> = emptyMap(),
    zone: ZoneId = ZoneId.systemDefault(),
    today: LocalDate = LocalDate.now(zone)
): List<ChatListItem> {
    val ordered = entries.sortedBy { it.createdAt }
    val items = ArrayList<ChatListItem>(ordered.size + 2)
    var lastDay: LocalDate? = null

    ordered.forEach { entry ->
        val day = entry.createdAt.atZone(zone).toLocalDate()
        if (day != lastDay) {
            items += ChatListItem.DateSeparator(
                ChatDateSeparator(id = "sep-$day", label = dayLabel(day, today, zone))
            )
            lastDay = day
        }
        items += ChatListItem.Message(entry.toUiMessage(sendStates, zone))
    }
    return items
}

/**
 * One entry as a bubble.
 *
 * The key is [ChatMessageEntry.messageId] — the server's for a stored message, the
 * `client_message_id` for one still in flight. That is what makes the optimistic
 * bubble *replaceable*: when the socket echoes the message back, the ViewModel swaps
 * the pending entry for the stored one under the same list key instead of appending a
 * second bubble.
 */
private fun ChatMessageEntry.toUiMessage(
    sendStates: Map<String, ChatSendState>,
    zone: ZoneId
): ChatMessage = ChatMessage(
    id = messageId,
    author = if (isOwn) ChatAuthor.ME else ChatAuthor.OTHER,
    senderName = if (isOwn) "You" else senderPseudonym.ifBlank { "Anonymous" },
    body = body,
    time = QuakeFormat.chatTime(createdAt, zone),
    sendState = sendStates[messageId] ?: ChatSendState.SENT
)

/**
 * "Today" / "Yesterday" for the two days a user is actually reading, and a date for
 * anything older. Retention is 7 days, so "older" is at most a week.
 */
private fun dayLabel(day: LocalDate, today: LocalDate, zone: ZoneId): String = when (day) {
    today -> "Today"
    today.minusDays(1) -> "Yesterday"
    else -> QuakeFormat.date(day.atStartOfDay(zone).toInstant(), zone)
}
