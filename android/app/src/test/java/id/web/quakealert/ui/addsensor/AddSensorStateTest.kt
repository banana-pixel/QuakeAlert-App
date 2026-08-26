package id.web.quakealert.ui.addsensor

import id.web.quakealert.domain.ProvisionedNode
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Truth tables for the wizard's pure decision layer: the name rules mirrored from
 * the server, and the step gates. The ViewModel owns every side effect; nothing in
 * these tests touches a network or a clock.
 */
class AddSensorStateTest {

    // --- SensorNameRules.normalize / validate (mirror of nodeLocationName) ---

    @Test
    fun `name normalization matches the server - trim and collapse whitespace`() {
        assertEquals("Lembang, Bandung Barat", SensorNameRules.normalize("  Lembang, \t Bandung Barat "))
    }

    @Test
    fun `valid place names pass`() {
        assertNull(SensorNameRules.validate("Lembang, Kab. Bandung Barat, Jawa Barat"))
        assertNull(SensorNameRules.validate("Cimahi"))
    }

    @Test
    fun `blank and overlong names are rejected`() {
        assertEquals(DetailsError.NAME_REQUIRED, SensorNameRules.validate("   "))
        val long = "Kecamatan yang sangat panjang ".repeat(6).trim()
        assertTrue(long.length > SensorNameRules.MAX_LENGTH)
        assertEquals(DetailsError.NAME_TOO_LONG, SensorNameRules.validate(long))
    }

    @Test
    fun `a name carrying the station id pattern is rejected`() {
        // The exact bug class from "Bandung AAAAAAAA": identity is not a place.
        assertEquals(DetailsError.NAME_HAS_NODE_ID, SensorNameRules.validate("Bandung NODE-AAAAAAAA"))
        // Case-insensitive on purpose: the server uppercases before matching.
        assertEquals(DetailsError.NAME_HAS_NODE_ID, SensorNameRules.validate("Cimahi node-163a149f"))
    }

    @Test
    fun `keyboard mash is rejected by the character-run rule`() {
        assertEquals(DetailsError.NAME_NOT_PLACE_LIKE, SensorNameRules.validate("Bandung AAAAAAAA"))
        assertEquals(DetailsError.NAME_NOT_PLACE_LIKE, SensorNameRules.validate("Purwakarta 11111"))
        // Four identical characters is still plausible ("Cisaat IV"?), five is not.
        assertNull(SensorNameRules.validate("Kota Baru IIII"))
    }

    // --- DETAILS gate ---

    @Test
    fun `details gate advances when name and position are valid`() {
        val state = AddSensorState(locationName = "Cimahi", latitude = -6.87, longitude = 107.54)
            .advanceIfDetailsValid()
        assertEquals(AddSensorWizardStep.CREDENTIALS, state.currentStep)
        assertNull(state.detailsError)
    }

    @Test
    fun `details gate refuses a missing pin without losing the typed name`() {
        val state = AddSensorState(locationName = "Cimahi").advanceIfDetailsValid()
        assertEquals(AddSensorWizardStep.WELCOME, state.currentStep)
        assertEquals(DetailsError.POSITION_MISSING, state.detailsError)
        assertEquals("Cimahi", state.locationName)
    }

    @Test
    fun `details gate surfaces the name rule it broke`() {
        val state = AddSensorState(
            locationName = "Bandung AAAAAAAA",
            latitude = -6.87,
            longitude = 107.54
        ).advanceIfDetailsValid()
        assertEquals(DetailsError.NAME_NOT_PLACE_LIKE, state.detailsError)
        assertFalse(state.detailsValid)
    }

    // --- LINK gate + selection ---

    @Test
    fun `link gate requires an ssid and bounds the password length`() {
        var state = AddSensorState().advanceIfLinkValid()
        assertEquals(LinkError.SSID_REQUIRED, state.linkError)

        state = AddSensorState(selectedSsid = "HomeWifi", wifiPassword = "x".repeat(65))
            .advanceIfLinkValid()
        assertEquals(LinkError.PASSWORD_TOO_SHORT, state.linkError)

        // Open networks are legitimate: empty password passes.
        state = AddSensorState(selectedSsid = "FreeNet").advanceIfLinkValid()
        assertNull(state.linkError)
    }

    @Test
    fun `selecting an ssid clears the stale password`() {
        val state = AddSensorState(selectedSsid = "Old", wifiPassword = "secret")
            .onSsidSelected("New")
        assertEquals("New", state.selectedSsid)
        assertEquals("", state.wifiPassword)
    }

    // --- provisioning + confirm transitions ---

    private fun provisionedState() = AddSensorState(
        locationName = "Cimahi",
        latitude = -6.87,
        longitude = 107.54
    ).onProvisioned(
        id.web.quakealert.domain.ProvisionedNode(
            stationId = "NODE-163A149F",
            provisioningSecret = "sec_test",
            mqttBroker = "broker.quakealert.id",
            mqttPort = 8883,
            mqttTls = true
        )
    )

    @Test
    fun `node configured moves to confirm with the echoed id as authority`() {
        // The portal's echo wins over the minted id: if the node carried a
        // different NVS identity, that is what will heartbeat.
        val state = provisionedState().onNodeConfigured(effectiveStationId = null)
        assertEquals(AddSensorWizardStep.FINISHING, state.currentStep)
        assertTrue(state.nodeConfigured)
        assertEquals("NODE-163A149F", state.effectiveStationId)
    }

    @Test
    fun `confirm poll reads pending then online from the roll`() {
        var state = provisionedState().onNodeConfigured(null)

        state = state.onConfirmPoll(mapOf("NODE-163A149F" to "Pending"))
        assertEquals(ConfirmState.PENDING, state.confirmState)

        state = state.onConfirmPoll(mapOf("NODE-163A149F" to "Online"))
        assertEquals(ConfirmState.ONLINE, state.confirmState)
    }

    @Test
    fun `absence spends budget and exhaustion sets the message once`() {
        var state = provisionedState().onNodeConfigured(null)
        val fresh = state.attemptsLeft

        state = state.onConfirmPoll(emptyMap())
        assertEquals(fresh - 1, state.attemptsLeft)
        assertNull(state.failure)

        repeat(state.attemptsLeft) { state = state.onConfirmPoll(emptyMap()) }
        assertEquals(0, state.attemptsLeft)
        assertEquals(WizardFailure.SENSOR_NEVER_CHECKED_IN, state.failure)
    }

    // --- navigation and exit ---

    @Test
    fun `only the location step can be stepped back out of`() {
        assertEquals(
            AddSensorWizardStep.WELCOME,
            AddSensorState(currentStep = AddSensorWizardStep.LOCATION).previousStep
        )
        // Past LOCATION the identity exists on the server and cannot be un-minted,
        // so there is nowhere safe to step back to; null asks for the exit question.
        assertNull(AddSensorState(currentStep = AddSensorWizardStep.CREDENTIALS).previousStep)
        assertNull(AddSensorState(currentStep = AddSensorWizardStep.WLAN).previousStep)
        assertNull(AddSensorState().previousStep)
    }

    @Test
    fun `leaving costs progress exactly when something was minted`() {
        assertFalse(AddSensorState().exitLosesProgress)
        assertFalse(AddSensorState(currentStep = AddSensorWizardStep.RATE_LIMIT).exitLosesProgress)
        assertTrue(AddSensorState(currentStep = AddSensorWizardStep.CREDENTIALS).exitLosesProgress)
        assertTrue(AddSensorState(currentStep = AddSensorWizardStep.WLAN).exitLosesProgress)

        // A finished flow is already saved; a still-waiting one is not.
        assertTrue(
            AddSensorState(currentStep = AddSensorWizardStep.FINISHING).exitLosesProgress
        )
        assertFalse(
            AddSensorState(
                currentStep = AddSensorWizardStep.FINISHING,
                confirmState = ConfirmState.ONLINE
            ).exitLosesProgress
        )
    }

    // --- copy hygiene: the house rule, as a test ---

    @Test
    fun `no wizard copy leaks transport detail`() {
        val banned = listOf(
            "Exception", "IOException", "HTTP", "http", "socket", "Socket",
            "SoftAP", "portal", "null", "MQTT", "/config", "/sensors", "429"
        )
        val sentences = WizardFailure.entries.flatMap { failure ->
            val copy = failureCopy(failure)
            listOf(copy.title, copy.message)
        } +
            AddSensorWizardStep.entries.flatMap { listOf(it.headline(), it.helperText()) } +
            DetailsError.entries.map { it.message() } +
            LinkError.entries.map { it.message() }

        for (sentence in sentences) {
            for (word in banned) {
                assertFalse("leaked '\$word' in: \$sentence", sentence.contains(word))
            }
            // Em dashes are banned app-wide; the wizard is where copy is densest.
            assertFalse("em dash in: \$sentence", sentence.contains('\u2014'))
        }
    }

    // --- shouldRevokeOnExit: the truth table behind the revoke-on-cancel rule ---

    private fun provisionedNode() = ProvisionedNode(
        stationId = "NODE-163A149F",
        provisioningSecret = "secret",
        mqttBroker = "b",
        mqttPort = 8883,
        mqttTls = true
    )

    @Test
    fun `cancel before provisioning never revokes`() {
        // No identity minted, no capability held: nothing to withdraw and nothing
        // to authorize the withdrawal with.
        assertFalse(AddSensorState(currentStep = AddSensorWizardStep.LOCATION).shouldRevokeOnExit)
        assertFalse(AddSensorState(currentStep = AddSensorWizardStep.WELCOME).shouldRevokeOnExit)
        assertFalse(AddSensorState(currentStep = AddSensorWizardStep.RATE_LIMIT).shouldRevokeOnExit)
    }

    @Test
    fun `cancel after provisioning but before configuration revokes`() {
        val credentials = AddSensorState(
            currentStep = AddSensorWizardStep.CREDENTIALS,
            provisioned = provisionedNode()
        )
        assertTrue(credentials.shouldRevokeOnExit)

        // WLAN step too: joined the node network but /config has not succeeded,
        // so `effectiveStationId` is still null — the server row is withdrawable.
        assertTrue(
            AddSensorState(
                currentStep = AddSensorWizardStep.WLAN,
                provisioned = provisionedNode()
            ).shouldRevokeOnExit
        )

        // A portal rejection keeps the session on WLAN with no echo: still
        // unconfigured, so exit must still revoke.
        assertTrue(
            AddSensorState(
                currentStep = AddSensorWizardStep.WLAN,
                provisioned = provisionedNode(),
                failure = WizardFailure.SETTINGS_NOT_ACCEPTED
            ).shouldRevokeOnExit
        )
    }

    @Test
    fun `successful completion never revokes`() {
        // The portal echoed an effective station id: the node's NVS now carries
        // this identity. Deleting the row would brick a configured device until a
        // factory reset, so configuration is the client-side point of no return.
        val finishing = AddSensorState(
            currentStep = AddSensorWizardStep.FINISHING,
            provisioned = provisionedNode(),
            effectiveStationId = "NODE-163A149F",
            confirmState = ConfirmState.ONLINE
        )
        assertFalse(finishing.shouldRevokeOnExit)

        // Same at the moment of handoff (WLAN success applying onNodeConfigured).
        val justConfigured = AddSensorState(
            currentStep = AddSensorWizardStep.FINISHING,
            provisioned = provisionedNode(),
            effectiveStationId = "NODE-163A149F"
        )
        assertFalse(justConfigured.shouldRevokeOnExit)
    }

    @Test
    fun `a node that was configured but never checked in is not revoked`() {
        // SENSOR_NEVER_CHECKED_IN means the node IS configured and keeps retrying
        // on its own; the row must survive for it to check in to.
        val timedOut = AddSensorState(
            currentStep = AddSensorWizardStep.FINISHING,
            provisioned = provisionedNode(),
            effectiveStationId = "NODE-163A149F",
            attemptsLeft = 0,
            failure = WizardFailure.SENSOR_NEVER_CHECKED_IN
        )
        assertFalse(timedOut.shouldRevokeOnExit)
    }
}
