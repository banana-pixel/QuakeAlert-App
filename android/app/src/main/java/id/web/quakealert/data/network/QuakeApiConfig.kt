package id.web.quakealert.data.network

import id.web.quakealert.BuildConfig

/**
 * Single source of truth for where the client talks to the backend.
 *
 * [BASE_URL] comes from `BuildConfig` so the dev and production transports cannot
 * be confused at runtime: the debug build points at the emulator loopback
 * (`http://10.0.2.2:8080/`, cleartext-allowed only for that host by
 * res/xml/network_security_config.xml) while release points at
 * `https://api.quakealert.id/` per ADR-0003.
 *
 * All REST paths sit under [API_PREFIX]; the WebSocket deliberately does not —
 * it is registered at the server root (`GET /ws` in server/internal/api/router.go).
 */
object QuakeApiConfig {

    /** Base URL, always with a trailing slash. */
    const val BASE_URL: String = BuildConfig.QUAKE_BASE_URL

    /** Version prefix shared by every REST endpoint. */
    const val API_PREFIX: String = "api/v1"

    const val PATH_AUTH_ANONYMOUS: String = "$API_PREFIX/auth/anonymous"
    const val PATH_EVENTS: String = "$API_PREFIX/events"
    const val PATH_SENSORS: String = "$API_PREFIX/sensors"
    const val PATH_USER_LOCATION: String = "$API_PREFIX/users/location"
    const val PATH_USER_FCM_TOKEN: String = "$API_PREFIX/users/fcm-token"
    const val PATH_USER_PSEUDONYM_REROLL: String = "$API_PREFIX/users/pseudonym/reroll"
    const val PATH_CHAT_CHANNELS: String = "$API_PREFIX/chat/channels"
    const val PATH_CHAT_MESSAGES: String = "$API_PREFIX/chat/messages"
    const val PATH_BROADCASTS: String = "$API_PREFIX/broadcasts"

    /** WebSocket path — mounted at the server root, outside [API_PREFIX]. */
    const val PATH_WS: String = "ws"

    /** Absolute URL for a REST [path], e.g. `url(PATH_EVENTS)`. */
    fun url(path: String): String = BASE_URL + path

    /**
     * Absolute WebSocket URL.
     *
     * OkHttp accepts `http`/`https` for WebSocket requests and upgrades them
     * internally, so scheme rewriting to `ws`/`wss` is unnecessary — and doing it
     * would break `HttpUrl.parse`, which rejects those schemes.
     */
    fun webSocketUrl(): String = BASE_URL + PATH_WS
}
