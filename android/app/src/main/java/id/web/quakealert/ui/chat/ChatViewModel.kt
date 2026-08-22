package id.web.quakealert.ui.chat

import android.app.Application
import android.util.Log
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import id.web.quakealert.data.network.QuakeApiClient
import id.web.quakealert.data.network.QuakeNetwork
import id.web.quakealert.data.network.mapper.toChatListItems
import id.web.quakealert.domain.ChatChannel
import id.web.quakealert.domain.ChatChannelKind
import id.web.quakealert.domain.ChatMessageEntry
import id.web.quakealert.ui.common.errorCopy
import id.web.quakealert.ui.common.shouldReloadOnReconnect
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.time.Instant
import java.util.UUID

/**
 * Hosts the [ChatUiState] for the Chat screen, backed by the three chat endpoints
 * (`GET /chat/channels`, `GET /chat/messages`, `POST /chat/messages`) and the
 * `CHAT_MESSAGE` frames on the shared WebSocket.
 *
 * The division of labour between the two transports is the design, not an
 * optimisation: **REST is the source of truth** — sends go over it because it is
 * durable and repeatable, and history is read from it because a socket that was
 * closed while the app was backgrounded missed whatever was said — while the socket
 * only fans out what is already stored, as the fast path for a room the user is
 * looking at right now (docs/CHAT_DESIGN.md §5).
 *
 * Membership is never asserted here. The channel list comes from the server, which
 * derives the regional room from the last synced position; a client that assembled
 * `ID-jawa-barat` itself would ask for a room nobody is in the moment the server's
 * normalisation changes.
 */
class ChatViewModel(application: Application) : AndroidViewModel(application) {

    private val apiClient: QuakeApiClient = QuakeNetwork.from(application).apiClient

    private val webSocketClient = QuakeNetwork.from(application).webSocketClient

    private val networkMonitor = QuakeNetwork.from(application).networkMonitor

    private val _uiState = MutableStateFlow(ChatUiState(isLoading = true))
    val uiState: StateFlow<ChatUiState> = _uiState.asStateFlow()

    /**
     * The channels the server said this identity may read, in its order (`global`
     * first). Held outside [ChatUiState] because only the *active* one is rendered,
     * and a switcher over two tiers needs no more than the list itself.
     */
    private var channels: List<ChatChannel> = emptyList()

    /** The room currently on screen, or null until the channel list arrives. */
    private var activeChannelId: String? = null

    /**
     * Messages of [activeChannelId], oldest first, as the single backing list.
     *
     * Kept as domain entries rather than as the rendered [ChatListItem]s because
     * every mutation — an older page, a socket frame, a send that resolves — needs to
     * merge by `message_id` and re-derive the date separators, which the rendered
     * form cannot do.
     */
    private var entries: List<ChatMessageEntry> = emptyList()

    /** Delivery state of this device's in-flight sends, keyed by `client_message_id`. */
    private var sendStates: Map<String, ChatSendState> = emptyMap()

    init {
        loadChannels()
        observeSocket()
        observeConnectivity()
    }

    /** Retries whatever failed, from the error card's "Retry" action. */
    fun onRetry() {
        if (channels.isEmpty()) loadChannels() else loadMessages()
    }

    /** Updates the hoisted draft as the user types. */
    fun onDraftChanged(text: String) {
        // Trimmed to the contract's cap as it is typed rather than rejected on send:
        // the server counts 500 runes and answers 400 beyond it, and a composer that
        // silently accepts a 600-character message only to lose it is worse than one
        // that stops taking input.
        _uiState.update { it.copy(draft = text.take(QuakeApiClient.MAX_CHAT_BODY_LENGTH)) }
    }

    /**
     * Sends the draft, showing it immediately and reconciling with the server after.
     *
     * The optimistic bubble carries the `client_message_id` as its list key, which is
     * the whole reason the id is generated here and sent: the stored message comes
     * back twice — once as this call's response and once as a socket frame — and both
     * must land on the same row rather than adding two.
     */
    fun onSendClicked() {
        val state = _uiState.value
        val channelId = activeChannelId ?: return
        val body = state.draft.trim()
        if (body.isEmpty() || state.isSending) return

        val clientMessageId = UUID.randomUUID().toString()
        val ownId = ownUserId
        val pending = ChatMessageEntry(
            messageId = clientMessageId,
            channelId = channelId,
            senderId = ownId.orEmpty(),
            senderPseudonym = "You",
            senderLocationTag = null,
            body = body,
            isAdmin = false,
            isOwn = true,
            createdAt = Instant.now()
        )
        sendStates = sendStates + (clientMessageId to ChatSendState.SENDING)
        entries = entries + pending
        _uiState.update { it.copy(draft = "", isSending = true) }
        publish()

        viewModelScope.launch {
            apiClient.sendChatMessage(
                channelId = channelId,
                body = body,
                clientMessageId = clientMessageId
            ).fold(
                onSuccess = { stored ->
                    // Replace, not append: the pending row is dropped by id and the
                    // stored one merged in, so the socket echo of the same message
                    // finds it already present.
                    entries = entries.filterNot { it.messageId == clientMessageId }
                    sendStates = sendStates - clientMessageId
                    merge(stored)
                },
                onFailure = { throwable ->
                    Log.w(TAG, "could not send chat message", throwable)
                    // The bubble stays, marked failed: the user needs to see which
                    // message did not go out. Retrying reuses this same id, so a send
                    // that actually reached the server cannot become a duplicate.
                    sendStates = sendStates + (clientMessageId to ChatSendState.FAILED)
                    _uiState.update { it.copy(isSending = false) }
                    publish()
                }
            )
            _uiState.update { it.copy(isSending = false) }
        }
    }

    /**
     * Re-sends a message that failed, under its original `client_message_id`.
     *
     * Reusing the id is what makes the retry safe: the first attempt may have been
     * stored before the connection dropped, and the server answers a repeat of the
     * same id with the same message rather than a second one.
     */
    fun onRetrySend(messageId: String) {
        if (sendStates[messageId] != ChatSendState.FAILED) return
        val pending = entries.firstOrNull { it.messageId == messageId } ?: return
        val channelId = activeChannelId ?: return

        sendStates = sendStates + (messageId to ChatSendState.SENDING)
        _uiState.update { it.copy(isSending = true) }
        publish()

        viewModelScope.launch {
            apiClient.sendChatMessage(
                channelId = channelId,
                body = pending.body,
                clientMessageId = messageId
            ).fold(
                onSuccess = { stored ->
                    entries = entries.filterNot { it.messageId == messageId }
                    sendStates = sendStates - messageId
                    merge(stored)
                },
                onFailure = { throwable ->
                    Log.w(TAG, "could not resend chat message", throwable)
                    sendStates = sendStates + (messageId to ChatSendState.FAILED)
                }
            )
            _uiState.update { it.copy(isSending = false) }
            publish()
        }
    }

    /**
     * Moves to the other channel.
     *
     * A toggle rather than a picker because there are exactly two tiers and at most
     * two rooms: a sheet listing two items would be a step the user has no reason to
     * take. With only the global room available the switch is disabled and the header
     * says why instead.
     */
    fun onSwitchChannelClicked() {
        if (channels.size < 2) return
        val index = channels.indexOfFirst { it.id == activeChannelId }
        val next = channels[(index + 1) % channels.size]
        if (next.id == activeChannelId) return
        activeChannelId = next.id
        entries = emptyList()
        sendStates = emptyMap()
        publish()
        loadMessages()
    }

    /** Loads an older page, from the list scrolling to its top. */
    fun onLoadOlder() {
        val state = _uiState.value
        if (state.isLoading || state.isLoadingOlder || !state.hasOlder) return
        val channelId = activeChannelId ?: return
        val oldest = entries.minByOrNull { it.createdAt }?.createdAt ?: return

        _uiState.update { it.copy(isLoadingOlder = true) }
        viewModelScope.launch {
            try {
                apiClient.fetchChatMessages(
                    channelId = channelId,
                    before = oldest,
                    ownUserId = ownUserId
                ).fold(
                    onSuccess = { page ->
                        merge(page)
                        _uiState.update { it.copy(hasOlder = page.size >= PAGE_SIZE) }
                    },
                    onFailure = { throwable ->
                        // Paging stops rather than replacing the conversation with an
                        // error card: what is already on screen is still correct.
                        Log.w(TAG, "could not load older messages", throwable)
                        _uiState.update { it.copy(hasOlder = false) }
                    }
                )
            } finally {
                _uiState.update { it.copy(isLoadingOlder = false) }
            }
        }
    }

    /**
     * Asks the server which rooms this identity may read, then loads the first one.
     *
     * The list is the membership: `global` is always present, and a regional room
     * appears only once a position has been synced and its area could be normalised.
     * A user with one channel is a valid state, not an empty screen — the header says
     * why rather than showing a room that is not there.
     */
    private fun loadChannels() {
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, isError = false, errorCopy = null) }
            ownUserId = apiClient.currentUserId()
            apiClient.fetchChatChannels().fold(
                onSuccess = { available ->
                    channels = available
                    val active = available.firstOrNull { it.id == activeChannelId }
                        ?: available.firstOrNull()
                    if (active == null) {
                        // No rooms at all is a server-side state the client cannot act
                        // on, so it reads as a failure rather than as an empty room.
                        _uiState.update {
                            it.copy(
                                isLoading = false,
                                isError = true,
                                errorCopy = errorCopy(IllegalStateException("no channels"))
                            )
                        }
                        return@fold
                    }
                    activeChannelId = active.id
                    _uiState.update {
                        it.copy(
                            channel = active.toChannelInfo(canSwitch = available.size > 1),
                            notice = noticeFor(available)
                        )
                    }
                    loadMessages()
                },
                onFailure = { throwable ->
                    Log.w(TAG, "could not load chat channels", throwable)
                    _uiState.update {
                        it.copy(
                            isLoading = false,
                            isError = true,
                            errorCopy = errorCopy(throwable)
                        )
                    }
                }
            )
        }
    }

    /** Loads the newest page of [activeChannelId], replacing whatever is on screen. */
    private fun loadMessages() {
        val channelId = activeChannelId ?: return
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, isError = false, errorCopy = null) }
            try {
                apiClient.fetchChatMessages(
                    channelId = channelId,
                    ownUserId = ownUserId
                ).fold(
                    onSuccess = { page ->
                        // Assigned rather than merged: this is the newest page of a
                        // room being opened, so anything held from a previous channel
                        // must not survive into it.
                        entries = page.sortedBy { it.createdAt }
                        _uiState.update { it.copy(hasOlder = page.size >= PAGE_SIZE) }
                        publish()
                    },
                    onFailure = { throwable ->
                        Log.w(TAG, "could not load chat messages", throwable)
                        // A failure with messages already on screen is not worth
                        // replacing them for: the room is still readable.
                        val hadContent = entries.isNotEmpty()
                        _uiState.update {
                            it.copy(
                                isError = !hadContent,
                                errorCopy = if (hadContent) null else errorCopy(throwable)
                            )
                        }
                    }
                )
            } finally {
                // The single owner of the flag, so no exit path — including scope
                // cancellation — can leave a spinner running with nothing behind it.
                _uiState.update { it.copy(isLoading = false) }
            }
        }
    }

    /**
     * Appends frames arriving on the shared socket for the room on screen.
     *
     * Frames for the *other* channel are dropped rather than buffered: opening that
     * room reads its history from REST anyway, so keeping them would only be a second
     * copy of the same page waiting to disagree with it.
     */
    private fun observeSocket() {
        viewModelScope.launch {
            webSocketClient.chatMessages.collect { message ->
                if (message.channelId != activeChannelId) return@collect
                merge(message)
            }
        }
    }

    /**
     * Reloads when connectivity returns, so an outage that ended does not leave the
     * room sitting on an error card until someone taps Retry. Only over a failure and
     * only with nothing in flight — see [shouldReloadOnReconnect].
     */
    private fun observeConnectivity() {
        viewModelScope.launch {
            networkMonitor.onlineRegained.collect {
                val state = _uiState.value
                val busy = state.isLoading || state.isLoadingOlder || state.isSending
                if (shouldReloadOnReconnect(isError = state.isError, isBusy = busy)) onRetry()
            }
        }
    }

    /**
     * Folds [incoming] into [entries] by `message_id`, newest last.
     *
     * De-duplication by id is what makes three independent arrival paths safe: the
     * POST response, the socket echo of that same message, and an older page that
     * overlaps the newest one all name the same row.
     */
    private fun merge(incoming: List<ChatMessageEntry>) {
        if (incoming.isEmpty()) return
        val byId = LinkedHashMap<String, ChatMessageEntry>(entries.size + incoming.size)
        entries.forEach { byId[it.messageId] = it }
        incoming.forEach { byId[it.messageId] = it }
        entries = byId.values.sortedBy { it.createdAt }
        publish()
    }

    private fun merge(incoming: ChatMessageEntry) = merge(listOf(incoming))

    /** Re-derives the rendered stream — date separators included — from [entries]. */
    private fun publish() {
        val items = toChatListItems(entries = entries, sendStates = sendStates)
        _uiState.update { it.copy(items = items) }
    }

    /**
     * The one limitation worth naming on screen: no regional room yet.
     *
     * Said rather than shown as an absence, because the global room working while the
     * regional one is missing looks like a broken feature unless the reason is given
     * — and the reason is something the user can act on (sync a position in Settings).
     */
    private fun noticeFor(available: List<ChatChannel>): String? =
        if (available.none { it.kind == ChatChannelKind.REGIONAL }) {
            "Sync your location in Settings to join your area's channel."
        } else {
            null
        }

    /**
     * This device's `user_id`, read once at start-up.
     *
     * Needed to tell an own message from a stranger's, which decides bubble
     * alignment. Null before the identity bootstrap has finished, which renders
     * everything as incoming — the safe way to be wrong.
     */
    private var ownUserId: String? = null

    private companion object {
        const val TAG = "ChatViewModel"

        /**
         * Page size for `GET /chat/messages`, and the threshold for "there may be
         * more": a short page means the room's history has ended.
         */
        const val PAGE_SIZE = QuakeApiClient.DEFAULT_CHAT_LIMIT
    }
}
