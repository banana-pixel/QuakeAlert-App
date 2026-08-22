package id.web.quakealert.data.network.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

/**
 * One channel from `GET /api/v1/chat/channels`.
 *
 * The client never *builds* a channel id. There are two tiers — `global` and one
 * regional room keyed `<ISO2>-<admin1-slug>` — and the regional key is whatever the
 * server's normalisation produced from the place sent with the last position sync.
 * A client that assembled `ID-jawa-barat` itself would ask for a room nobody is in
 * the moment that normalisation changes (docs/CHAT_DESIGN.md §3).
 *
 * @param kind "GLOBAL" or "REGIONAL". Unknown values are tolerated by the mapper
 *   rather than dropped: a channel the server offers is a channel the user may read.
 */
@Serializable
data class ChatChannelDto(
    @SerialName("channel_id") val channelId: String = "",
    @SerialName("kind") val kind: String = "",
    @SerialName("display_name") val displayName: String = ""
)

/** Response of `GET /api/v1/chat/channels`; `global` is always first. */
@Serializable
data class ChatChannelsResponseDto(
    @SerialName("channels") val channels: List<ChatChannelDto> = emptyList()
)

/**
 * One stored message, shared by `GET`/`POST /api/v1/chat/messages`.
 *
 * @param senderPseudonym a **snapshot** taken when the message was stored, not a
 *   live join: a user who rerolls their pseudonym does not retro-rename their own
 *   history.
 * @param senderId the caller matches this against its own `user_id` to recognise
 *   its own messages, which is what lets an optimistic bubble be replaced instead
 *   of duplicated when the socket echoes it back.
 * @param createdAt RFC3339 UTC — also the paging cursor: the oldest `created_at`
 *   held becomes the next request's `before`.
 */
@Serializable
data class ChatMessageDto(
    @SerialName("message_id") val messageId: String = "",
    @SerialName("channel_id") val channelId: String = "",
    @SerialName("sender_id") val senderId: String = "",
    @SerialName("sender_pseudonym") val senderPseudonym: String = "",
    @SerialName("sender_location_tag") val senderLocationTag: String? = null,
    @SerialName("message") val message: String = "",
    @SerialName("is_admin") val isAdmin: Boolean = false,
    @SerialName("created_at") val createdAt: String = ""
)

/**
 * Response of `GET /api/v1/chat/messages` — **descending** (newest first).
 *
 * `count < limit` means there is no older page; retention is 7 days, so the history
 * genuinely ends rather than paging forever.
 */
@Serializable
data class ChatMessagesResponseDto(
    @SerialName("channel_id") val channelId: String = "",
    @SerialName("limit") val limit: Int = 0,
    @SerialName("count") val count: Int = 0,
    @SerialName("messages") val messages: List<ChatMessageDto> = emptyList()
)

/**
 * Body of `POST /api/v1/chat/messages`.
 *
 * [clientMessageId] is the idempotency key and is always sent: a client that times
 * out does not know whether its first attempt landed, and a duplicate in a public
 * room cannot be taken back. Re-posting the same id returns the *same* message.
 */
@Serializable
data class CreateChatMessageRequestDto(
    @SerialName("channel_id") val channelId: String,
    @SerialName("message") val message: String,
    @SerialName("client_message_id") val clientMessageId: String
)

/**
 * The `CHAT_MESSAGE` frame on the shared `GET /ws` socket
 * (docs/CLIENT_SPEC.md §5.4).
 *
 * Same fields as [ChatMessageDto] except the timestamp: the socket speaks **ms
 * epoch** like every other frame on it, while REST answers RFC3339.
 */
@Serializable
data class WsChatMessageDto(
    @SerialName("type") val type: String = "",
    @SerialName("message_id") val messageId: String = "",
    @SerialName("channel_id") val channelId: String = "",
    @SerialName("sender_id") val senderId: String = "",
    @SerialName("sender_pseudonym") val senderPseudonym: String = "",
    @SerialName("sender_location_tag") val senderLocationTag: String? = null,
    @SerialName("message") val message: String = "",
    @SerialName("is_admin") val isAdmin: Boolean = false,
    @SerialName("timestamp") val timestamp: Long = 0L
)

/**
 * Just the discriminator, so one socket carrying several envelopes can be sorted
 * before anything is decoded in full.
 *
 * Decoding a chat frame as an alert would otherwise *succeed* — every alert field
 * has a default — and only be caught by the alert mapper's unknown-type check,
 * which logs it as a dropped alert. Reading `type` first keeps that log honest and
 * keeps unknown types genuinely ignored.
 */
@Serializable
data class WsFrameTypeDto(
    @SerialName("type") val type: String = ""
)
