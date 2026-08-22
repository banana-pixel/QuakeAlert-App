package id.web.quakealert.domain

import java.time.Instant

/**
 * A chat room the signed-in user may read and write.
 *
 * There are exactly two tiers and no ad-hoc rooms (docs/CHAT_DESIGN.md §3):
 * [ChatChannelKind.GLOBAL] — one room, everyone, and the only one that works before
 * a position fix exists — and [ChatChannelKind.REGIONAL], one room per admin-1 area,
 * whose membership is *derived* from the last synced position rather than joined.
 *
 * @param id the server's key. Opaque on purpose: the client passes it back
 *   verbatim and never assembles one.
 * @param displayName the title every member of the room sees, so two phones do not
 *   disagree about what the room is called.
 */
data class ChatChannel(
    val id: String,
    val kind: ChatChannelKind,
    val displayName: String
)

/** Which tier a [ChatChannel] belongs to. */
enum class ChatChannelKind { GLOBAL, REGIONAL }

/**
 * One message as the server stored it, from REST history or a socket frame.
 *
 * @param senderPseudonym the name captured when the message was written. Not
 *   refreshed later: a pseudonym can be rerolled, and history must not retro-rename.
 * @param senderLocationTag coarse place label of the sender, or null. Admin-1 at
 *   its finest — never a coordinate.
 * @param isOwn true when [senderId] matches this device's own `user_id`. The one
 *   fact the wire cannot carry, and what lets an optimistic bubble be replaced
 *   rather than duplicated when the socket echoes the message back.
 * @param createdAt when the server stored it — also the paging cursor.
 */
data class ChatMessageEntry(
    val messageId: String,
    val channelId: String,
    val senderId: String,
    val senderPseudonym: String,
    val senderLocationTag: String?,
    val body: String,
    val isAdmin: Boolean,
    val isOwn: Boolean,
    val createdAt: Instant
)
