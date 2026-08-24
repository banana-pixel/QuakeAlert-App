package id.web.quakealert.data.network

import id.web.quakealert.domain.ServerConnectionState
import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * The full truth table of [evaluateServerHealth], one test per rule.
 *
 * The two HEALTHY cases that matter most are the ones the old per-screen
 * derivation got wrong: reachable with no stored location, and reachable before
 * any sensors answer has arrived. Both must read Healthy — an empty station roll
 * caused by a privacy choice is the server working, not a fault.
 */
class ServerHealthTest {

    private fun eval(
        online: Boolean = true,
        probe: ProbeOutcome = ProbeOutcome.OK,
        socket: ServerConnectionState = ServerConnectionState.DISCONNECTED,
        sensors: SensorNetworkStatus = SensorNetworkStatus.UNKNOWN
    ) = evaluateServerHealth(online, probe, socket, sensors)

    // --- rule 1: device offline outranks everything ---

    @Test
    fun `device offline is offline even with a stale ok probe`() {
        assertEquals(ServerHealth.OFFLINE, eval(online = false))
    }

    @Test
    fun `device offline is offline even with a live-looking socket state`() {
        assertEquals(ServerHealth.OFFLINE, eval(online = false, socket = ServerConnectionState.CONNECTED))
    }

    // --- rules 2-3: probe failure, softened only by a live socket ---

    @Test
    fun `failed probe without a socket is offline`() {
        assertEquals(ServerHealth.OFFLINE, eval(probe = ProbeOutcome.FAILED))
    }

    @Test
    fun `failed probe with a live socket degrades instead of going offline`() {
        // One dropped poll must not outweigh an open push channel.
        assertEquals(
            ServerHealth.LIMITED,
            eval(probe = ProbeOutcome.FAILED, socket = ServerConnectionState.CONNECTED)
        )
    }

    @Test
    fun `failed probe with a connecting socket is still offline`() {
        assertEquals(
            ServerHealth.OFFLINE,
            eval(probe = ProbeOutcome.FAILED, socket = ServerConnectionState.CONNECTING)
        )
    }

    // --- rule 4: nothing has answered yet ---

    @Test
    fun `unknown probe without a socket is checking`() {
        assertEquals(ServerHealth.CHECKING, eval(probe = ProbeOutcome.UNKNOWN))
    }

    @Test
    fun `checking is never re-entered once something answered`() {
        // An OK probe plus connected socket and silent fleet falls through to the
        // later rules; it can end at LIMITED but never back at CHECKING.
        val health = eval(
            probe = ProbeOutcome.OK,
            socket = ServerConnectionState.CONNECTED,
            sensors = SensorNetworkStatus.ALL_SILENT
        )
        assertEquals(ServerHealth.LIMITED, health)
    }

    // --- rule 5: named dependency down ---

    @Test
    fun `unhealthy dependency degrades without disconnecting`() {
        assertEquals(ServerHealth.LIMITED, eval(probe = ProbeOutcome.DEPENDENCIES_DEGRADED))
    }

    // --- rule 6: reachable server, silent fleet ---

    @Test
    fun `all stations silent while reachable is limited`() {
        assertEquals(ServerHealth.LIMITED, eval(sensors = SensorNetworkStatus.ALL_SILENT))
    }

    // --- rule 7: healthy ---

    @Test
    fun `reachable with no stored location is healthy - the original bug`() {
        assertEquals(ServerHealth.HEALTHY, eval(sensors = SensorNetworkStatus.NO_STORED_LOCATION))
    }

    @Test
    fun `reachable with unknown sensor network is healthy`() {
        assertEquals(ServerHealth.HEALTHY, eval())
    }

    @Test
    fun `reachable and reporting is healthy regardless of socket`() {
        assertEquals(ServerHealth.HEALTHY, eval(sensors = SensorNetworkStatus.REPORTING))
    }
}

class HealthOutcomeOfTest {

    @Test
    fun `plain text ok parses as up`() {
        assertEquals(ProbeOutcome.OK, healthOutcomeOf(200, "ok"))
    }

    @Test
    fun `garbage body from a 2xx endpoint is still up`() {
        assertEquals(ProbeOutcome.OK, healthOutcomeOf(200, "<html>under construction</html>"))
    }

    @Test
    fun `all-ok json parses as up`() {
        assertEquals(
            ProbeOutcome.OK,
            healthOutcomeOf(200, """{"status":"ok","database":"ok","mqtt":"ok"}""")
        )
    }

    @Test
    fun `a down dependency degrades the outcome`() {
        assertEquals(
            ProbeOutcome.DEPENDENCIES_DEGRADED,
            healthOutcomeOf(200, """{"status":"degraded","database":"down","mqtt":"ok"}""")
        )
    }

    @Test
    fun `non-2xx fails whatever the body says`() {
        assertEquals(ProbeOutcome.FAILED, healthOutcomeOf(503, """{"status":"ok"}"""))
        assertEquals(ProbeOutcome.FAILED, healthOutcomeOf(404, null))
    }
}

class SensorNetworkStatusOfTest {

    @Test
    fun `no stored location wins even at zero active`() {
        assertEquals(
            SensorNetworkStatus.NO_STORED_LOCATION,
            sensorNetworkStatusOf(hasStoredLocation = false, activeSensorsCount = 0)
        )
    }

    @Test
    fun `reporting beats silence by count alone`() {
        assertEquals(
            SensorNetworkStatus.REPORTING,
            sensorNetworkStatusOf(hasStoredLocation = true, activeSensorsCount = 1)
        )
    }

    @Test
    fun `located user with zero reporting stations is all silent`() {
        assertEquals(
            SensorNetworkStatus.ALL_SILENT,
            sensorNetworkStatusOf(hasStoredLocation = true, activeSensorsCount = 0)
        )
    }
}
