package id.web.quakealert.data.network.mapper

import id.web.quakealert.data.network.model.ChatChannelDto
import id.web.quakealert.data.network.model.ChatMessageDto
import id.web.quakealert.data.network.model.WsChatMessageDto
import id.web.quakealert.domain.ChatChannelKind
import id.web.quakealert.domain.ChatMessageEntry
import id.web.quakealert.ui.chat.ChatAuthor
import id.web.quakealert.ui.chat.ChatListItem
import id.web.quakealert.ui.chat.ChatSendState
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneId

/**
 * The chat wire is two transports for one conversation — REST with RFC3339 stamps
 * and a WebSocket with ms-epoch ones — so the mapping is where the two are made to
 * agree. The list renderer is pure on purpose: day grouping and own-message
 * authorship are the two things a reader would notice being wrong, and neither
 * needs a device to test.
 */
class ChatMappersTest {

    private val zone: ZoneId = ZoneId.of("Asia/Jakarta")
    private val today: LocalDate = LocalDate.of(2026, 8, 22)

    @Test
    fun `keeps an unknown channel kind as regional rather than dropping the room`() {
        // A tier this build does not know about is still a room the server says the
        // user is in; hiding it would lose messages, showing it costs a wrong subtitle.
        val channels = listOf(
            ChatChannelDto(channelId = "global", kind = "GLOBAL", displayName = "Global"),
            ChatChannelDto(channelId = "ID-jawa-barat", kind = "PROVINCIAL", displayName = ""),
            ChatChannelDto(channelId = "  ", kind = "GLOBAL", displayName = "Nameless")
        ).toDomain()

        assertEquals(2, channels.size)
        assertEquals(ChatChannelKind.GLOBAL, channels[0].kind)
        assertEquals(ChatChannelKind.REGIONAL, channels[1].kind)
        // A blank display name falls back to the key, so the card is never empty.
        assertEquals("ID-jawa-barat", channels[1].displayName)
    }

    @Test
    fun `marks only the device's own messages as outgoing`() {
        val mine = ChatMessageDto(
            messageId = "m1",
            channelId = "global",
            senderId = "me",
            senderPseudonym = "Quakezen-1",
            message = "Aman",
            createdAt = "2026-08-22T09:41:00Z"
        ).toDomain(ownUserId = "me")
        val theirs = mine.copy(messageId = "m2", senderId = "you", isOwn = false)

        assertTrue(mine.isOwn)
        assertFalse(theirs.isOwn)
        assertEquals(Instant.parse("2026-08-22T09:41:00Z"), mine.createdAt)
    }

    @Test
    fun `reads the socket frame's millisecond stamp`() {
        val entry = WsChatMessageDto(
            messageId = "m1",
            channelId = "global",
            senderId = "me",
            senderPseudonym = "Quakezen-1",
            message = "Aman",
            timestamp = 1_755_855_660_000L
        ).toDomain(ownUserId = "me")

        assertEquals(Instant.ofEpochMilli(1_755_855_660_000L), entry.createdAt)
        assertTrue(entry.isOwn)
    }

    @Test
    fun `orders oldest first and separates each calendar day`() {
        val items = toChatListItems(
            entries = listOf(
                entry("m3", "Latest", "2026-08-22T02:00:00Z"),
                entry("m1", "Two days ago", "2026-08-20T02:00:00Z"),
                entry("m2", "Yesterday", "2026-08-21T02:00:00Z")
            ),
            zone = zone,
            today = today
        )

        // Three days, three separators, each immediately before its message.
        assertEquals(6, items.size)
        assertEquals(
            listOf("20 Aug 2026", "Two days ago", "Yesterday", "Yesterday", "Today", "Latest"),
            items.map { item ->
                when (item) {
                    is ChatListItem.DateSeparator -> item.separator.label
                    is ChatListItem.Message -> item.message.body
                }
            }
        )
    }

    @Test
    fun `applies the send state of a message still in flight`() {
        val items = toChatListItems(
            entries = listOf(
                entry("pending", "Not out yet", "2026-08-22T02:00:00Z"),
                entry("other", "Theirs", "2026-08-22T03:00:00Z", own = false)
            ),
            sendStates = mapOf("pending" to ChatSendState.FAILED),
            zone = zone,
            today = today
        )
        val messages = items.filterIsInstance<ChatListItem.Message>().map { it.message }

        assertEquals(ChatSendState.FAILED, messages[0].sendState)
        assertEquals(ChatAuthor.ME, messages[0].author)
        assertEquals("You", messages[0].senderName)
        // Anything without a recorded state is stored, which is the common case.
        assertEquals(ChatSendState.SENT, messages[1].sendState)
        assertEquals(ChatAuthor.OTHER, messages[1].author)
        assertEquals("Quakezen-1", messages[1].senderName)
    }

    private fun entry(
        id: String,
        body: String,
        createdAt: String,
        own: Boolean = true
    ): ChatMessageEntry = ChatMessageEntry(
        messageId = id,
        channelId = "global",
        senderId = if (own) "me" else "you",
        senderPseudonym = "Quakezen-1",
        senderLocationTag = null,
        body = body,
        isAdmin = false,
        isOwn = own,
        createdAt = Instant.parse(createdAt)
    )
}
