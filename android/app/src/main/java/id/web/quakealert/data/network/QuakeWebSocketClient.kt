package id.web.quakealert.data.network

import android.util.Log
import id.web.quakealert.data.auth.AuthRepository
import id.web.quakealert.data.network.mapper.toDomain
import id.web.quakealert.data.network.mapper.toDomainOrNull
import id.web.quakealert.data.network.model.WsAlertMessageDto
import id.web.quakealert.data.network.model.WsChatMessageDto
import id.web.quakealert.data.network.model.WsFrameTypeDto
import id.web.quakealert.domain.ChatMessageEntry
import id.web.quakealert.domain.ServerConnectionState
import id.web.quakealert.domain.WsAlertMessage
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.channels.BufferOverflow
import kotlinx.coroutines.channels.ProducerScope
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.flow.buffer
import kotlinx.coroutines.flow.channelFlow
import kotlinx.coroutines.flow.shareIn
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.serialization.decodeFromString
import kotlinx.serialization.json.Json
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import java.util.concurrent.atomic.AtomicBoolean
import kotlin.coroutines.resume
import kotlin.random.Random

/**
 * The realtime half of the contract: `GET /ws`, which pushes
 * `EARTHQUAKE_ALERT` / `EARTHQUAKE_ADVISORY` / `EVENT_RESOLVED` frames
 * (server/internal/dispatch/ws.go).
 *
 * Exposed as one hot [SharedFlow]. The socket is opened when the first collector
 * arrives and closed a few seconds after the last one leaves, so the app is not
 * holding a connection open behind a screen nobody is looking at — while a short
 * grace period keeps it alive across a configuration change.
 *
 * `replay = 1` is deliberate: a recreated Warning screen must immediately learn
 * that a quake is in progress rather than sitting on a calm banner until the next
 * frame. Because a replayed frame can be arbitrarily old, consumers gate it with
 * [WsAlertMessage.isRecent] instead of trusting it blindly.
 *
 * Note for local runs: the server refuses the upgrade for **every** client unless
 * `WS_ALLOWED_ORIGINS` is set (server/cmd/quakealert/main.go — an empty allow-list
 * returns false), and OkHttp sends no `Origin` header. Dev needs
 * `WS_ALLOWED_ORIGINS=*`.
 *
 * @param scope application-lifetime scope that owns the sharing coroutine.
 */
class QuakeWebSocketClient(
    private val client: OkHttpClient,
    private val json: Json,
    private val authRepository: AuthRepository,
    scope: CoroutineScope
) {

    /**
     * Backing flow for [chatMessages]. Dropping the oldest on overflow matches the
     * server's own priority rule: a chat frame may be lost under pressure, never an
     * alert, because a lost message reappears through `GET /chat/messages` while a
     * lost warning has no way back.
     */
    private val _chatMessages = MutableSharedFlow<ChatMessageEntry>(
        replay = 0,
        extraBufferCapacity = BUFFER_CAPACITY,
        onBufferOverflow = BufferOverflow.DROP_OLDEST
    )

    /**
     * Backing state for [connectionState], written from the points in the
     * reconnect loop that already know about every transition.
     */
    private val _connectionState = MutableStateFlow(ServerConnectionState.DISCONNECTED)

    /**
     * Chat frames from the same socket, in arrival order.
     *
     * A separate flow rather than a second connection, and separate from [alerts]
     * rather than one union type, for one reason each:
     *
     *  - one socket, because alerts and chat then share a single auth and a single
     *     reconnect (docs/CLIENT_SPEC.md §5.4);
     *  - a separate flow, because [alerts] replays its last value so a recreated
     *     Warning screen learns a quake is in progress. A chat message must never be
     *     able to occupy that replay slot.
     *
     * `replay = 0`: history comes from `GET /chat/messages`, which is the source of
     * truth. The socket is the fast path, so a frame that arrives while nothing is
     * collecting is simply the page load's job instead.
     */
    val chatMessages: SharedFlow<ChatMessageEntry> = _chatMessages.asSharedFlow()

    /** Live alert frames, newest last, with the most recent one replayed. */
    val alerts: SharedFlow<WsAlertMessage> = channelFlow {
        // Reset on every successful open, so a long-lived healthy connection that
        // eventually drops reconnects in ~1 s rather than inheriting the backoff
        // from some unrelated outage hours earlier. (This is exactly what a
        // `retryWhen`-based reconnect gets wrong: its attempt counter never resets.)
        var attempt = 0
        while (true) {
            _connectionState.value = ServerConnectionState.CONNECTING
            val outcome = try {
                session()
            } catch (cancellation: CancellationException) {
                throw cancellation
            } catch (throwable: Throwable) {
                // Includes a failed token bootstrap: no identity, no handshake.
                Outcome(opened = false, unauthenticated = false, cause = throwable)
            }
            _connectionState.value = ServerConnectionState.DISCONNECTED

            if (outcome.opened) attempt = 0
            if (outcome.unauthenticated) {
                // The server rejected the token at handshake time; drop it so the
                // next attempt bootstraps a fresh identity instead of replaying a
                // credential we already know is dead.
                Log.w(TAG, "websocket handshake unauthenticated, re-bootstrapping identity")
                authRepository.invalidate()
            }

            attempt++
            val backoff = backoffMillis(attempt)
            Log.d(TAG, "websocket reconnecting in ${backoff}ms (attempt $attempt)", outcome.cause)
            delay(backoff)
        }
    }.buffer(capacity = BUFFER_CAPACITY, onBufferOverflow = BufferOverflow.DROP_OLDEST)
        .shareIn(
            scope = scope,
            started = SharingStarted.WhileSubscribed(stopTimeoutMillis = STOP_TIMEOUT_MS),
            replay = 1
        )

    /**
     * Health of the socket, and the single source of truth behind every tab's
     * top-bar status badge.
     *
     * Collecting this **also holds a subscription on [alerts]**, and that is the
     * whole design rather than an implementation detail: [alerts] is shared
     * `WhileSubscribed`, and its only other collector is the Warning screen. Without
     * the inner subscription the socket would be open on the Warning tab alone, so a
     * badge bound to this flow would report `DISCONNECTED` on Sensors and History —
     * reproducing the very inconsistency it exists to remove.
     *
     * Because all five tabs render the badge, the practical effect is that the alert
     * channel stays up for as long as the app's UI is alive — the right trade for an
     * early-warning app — while the shared `WhileSubscribed` grace period still lets
     * it close once nothing is looking.
     */
    val connectionState: StateFlow<ServerConnectionState> = channelFlow {
        // The frames themselves are irrelevant here; holding the subscription open
        // is the point.
        launch { alerts.collect { } }
        _connectionState.collect { send(it) }
    }.stateIn(
        scope = scope,
        started = SharingStarted.WhileSubscribed(stopTimeoutMillis = STOP_TIMEOUT_MS),
        initialValue = ServerConnectionState.CONNECTING
    )

    /**
     * Runs one connection to completion.
     *
     * Suspends until the socket closes or fails, so the caller's loop is a plain
     * sequence of connection attempts rather than a callback tangle.
     */
    private suspend fun ProducerScope<WsAlertMessage>.session(): Outcome {
        val token = authRepository.ensureToken()
        val request = Request.Builder()
            .url(QuakeApiConfig.webSocketUrl())
            // Set explicitly rather than via AuthInterceptor: the handshake needs the
            // token that was just awaited, and the interceptor skips requests that
            // already carry the header.
            .header(HEADER_AUTHORIZATION, "Bearer $token")
            .build()

        return suspendCancellableCoroutine { continuation ->
            val opened = AtomicBoolean(false)
            val finished = AtomicBoolean(false)

            fun finish(outcome: Outcome) {
                // onClosed and onFailure are mutually exclusive in practice, but
                // resuming twice would crash rather than reconnect.
                if (finished.compareAndSet(false, true) && continuation.isActive) {
                    continuation.resume(outcome)
                }
            }

            val socket = client.newWebSocket(request, object : WebSocketListener() {
                override fun onOpen(webSocket: WebSocket, response: Response) {
                    opened.set(true)
                    _connectionState.value = ServerConnectionState.CONNECTED
                    Log.i(TAG, "websocket connected")
                }

                override fun onMessage(webSocket: WebSocket, text: String) {
                    emit(text)
                }

                override fun onClosing(webSocket: WebSocket, code: Int, reason: String) {
                    // Complete the closing handshake so the server's writer goroutine
                    // ends cleanly instead of waiting out its pong deadline.
                    webSocket.close(CLOSE_NORMAL, null)
                }

                override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
                    Log.i(TAG, "websocket closed ($code ${reason.ifBlank { "no reason" }})")
                    finish(Outcome(opened = opened.get(), unauthenticated = false, cause = null))
                }

                override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                    finish(
                        Outcome(
                            opened = opened.get(),
                            unauthenticated = response?.code == HTTP_UNAUTHORIZED,
                            cause = t
                        )
                    )
                }
            })

            continuation.invokeOnCancellation {
                // cancel(), not close(): the collector is gone, so there is nothing
                // to wait a closing handshake for.
                socket.cancel()
            }
        }
    }

    /**
     * Decodes one frame and forwards it, dropping anything unusable.
     *
     * A malformed or unknown frame must not tear down the connection — the next
     * frame may be the one that matters.
     */
    private fun ProducerScope<WsAlertMessage>.emit(text: String) {
        // Sorted by `type` before anything is decoded in full. Decoding a chat frame
        // as an alert would otherwise succeed — every alert field has a default — and
        // be reported below as a dropped alert of unknown type.
        val frameType = runCatching { json.decodeFromString<WsFrameTypeDto>(text) }
            .getOrNull()
            ?.type
        if (frameType == null) {
            Log.w(TAG, "dropping unparseable websocket frame")
            return
        }
        if (frameType.equals(TYPE_CHAT_MESSAGE, ignoreCase = true)) {
            emitChat(text)
            return
        }

        val dto = runCatching { json.decodeFromString<WsAlertMessageDto>(text) }.getOrNull()
        if (dto == null) {
            Log.w(TAG, "dropping unparseable websocket frame")
            return
        }
        val message = dto.toDomainOrNull()
        if (message == null) {
            Log.w(TAG, "dropping websocket frame with unknown type '${dto.type}'")
            return
        }
        // trySend, not send: this runs on OkHttp's reader thread, which cannot
        // suspend. The buffer drops the oldest frame under pressure, so this always
        // succeeds — a stale frame is the right thing to lose, never the newest one.
        trySend(message)
    }

    /**
     * Decodes one `CHAT_MESSAGE` frame into [chatMessages].
     *
     * `isOwn` is resolved here from the identity the live socket authenticated with
     * ([AuthRepository.peekUserId], a cache read — this runs on OkHttp's reader
     * thread and cannot suspend). Getting it right is what lets the chat screen
     * *replace* its optimistic bubble when its own message comes back rather than
     * showing it twice.
     */
    private fun emitChat(text: String) {
        val dto = runCatching { json.decodeFromString<WsChatMessageDto>(text) }.getOrNull()
        if (dto == null || dto.messageId.isBlank()) {
            Log.w(TAG, "dropping unusable chat frame")
            return
        }
        // tryEmit, not emit: the reader thread cannot suspend. The buffer drops the
        // oldest frame under pressure, so this always succeeds.
        _chatMessages.tryEmit(dto.toDomain(ownUserId = authRepository.peekUserId()))
    }

    /** Outcome of a single connection attempt. */
    private class Outcome(
        /** True if the socket ever reached OPEN — the signal to reset the backoff. */
        val opened: Boolean,
        /** True when the upgrade itself was rejected with 401. */
        val unauthenticated: Boolean,
        val cause: Throwable?
    )

    private companion object {
        const val TAG = "QuakeWebSocket"
        const val HEADER_AUTHORIZATION = "Authorization"
        const val HTTP_UNAUTHORIZED = 401
        const val CLOSE_NORMAL = 1000

        /** Envelope discriminator for a chat frame (server dispatch.ChatMessage). */
        const val TYPE_CHAT_MESSAGE = "CHAT_MESSAGE"

        /** Grace period so a rotation does not tear the socket down and rebuild it. */
        const val STOP_TIMEOUT_MS = 5_000L

        /** Frames held while a slow collector catches up; the oldest is dropped first. */
        const val BUFFER_CAPACITY = 16

        const val INITIAL_BACKOFF_MS = 1_000L
        const val MAX_BACKOFF_MS = 30_000L
        const val MAX_JITTER_MS = 500L

        /**
         * Exponential backoff capped at [MAX_BACKOFF_MS], plus jitter.
         *
         * The cap matters for a life-safety app: after a long outage the client must
         * still be retrying every 30 s, not every 20 minutes. The jitter keeps a
         * whole city's phones from reconnecting on the same tick after a server
         * restart.
         */
        fun backoffMillis(attempt: Int): Long {
            val exponential = INITIAL_BACKOFF_MS shl (attempt - 1).coerceIn(0, 16)
            return exponential.coerceAtMost(MAX_BACKOFF_MS) + Random.nextLong(MAX_JITTER_MS)
        }
    }
}
