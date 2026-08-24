package id.web.quakealert.domain

/**
 * Health of the app's link to the QuakeAlert backend, as observed on the alert
 * WebSocket (`GET /ws`).
 *
 * This is the socket-side *input* to the badge verdict:
 * [id.web.quakealert.data.network.ServerHealthMonitor] combines it with device
 * connectivity, a polled `GET /healthz` and the sensor-fleet state through
 * [id.web.quakealert.data.network.evaluateServerHealth] — a live socket softens one
 * dropped probe to Limited instead of Offline, which is the one judgement a raw
 * connection enum cannot express. It deliberately lives outside every screen's UI
 * state: link health is a property of the process, not of a tab.
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
