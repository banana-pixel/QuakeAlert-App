package id.web.quakealert.domain

/**
 * Health of the app's link to the QuakeAlert backend, as observed on the alert
 * WebSocket (`GET /ws`).
 *
 * This is the *single* source of truth behind the top bar's network badge. It
 * deliberately lives outside every screen's UI state: server health is a property
 * of the process, not of a tab, and deriving it per screen is what let the Sensors
 * tab hide the badge while Warning showed it in the same session.
 *
 * The socket is the right signal rather than the REST feed because it is the
 * channel an earthquake alert actually arrives on — if it is up, the backend can
 * reach this device.
 */
enum class ServerConnectionState {

    /** No socket yet: first attempt in flight, or waiting out a reconnect backoff. */
    CONNECTING,

    /** The socket reached OPEN and has not closed or failed since. */
    CONNECTED,

    /** The last attempt closed or failed; a retry is scheduled. */
    DISCONNECTED
}

/**
 * Whether the badge should read "Healthy".
 *
 * Only [ServerConnectionState.CONNECTED] qualifies: a connection that is merely
 * being attempted is not health, and the design ships a single green top-bar
 * variant, so the other two states hide the badge rather than invent a colour for
 * it. Defined once here so no composable re-derives the rule.
 */
val ServerConnectionState.isHealthy: Boolean
    get() = this == ServerConnectionState.CONNECTED
