package id.web.quakealert.ui.addsensor

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
        assertEquals(WizardStep.CREDENTIALS, state.step)
        assertNull(state.detailsError)
    }

    @Test
    fun `details gate refuses a missing pin without losing the typed name`() {
        val state = AddSensorState(locationName = "Cimahi").advanceIfDetailsValid()
        assertEquals(WizardStep.DETAILS, state.step)
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
    ).copy(step = WizardStep.CREDENTIALS).onProvisioned(
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
        assertEquals(WizardStep.CONFIRM, state.confirmStateOrNull())
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
        assertNull(state.errorMessage)

        repeat(state.attemptsLeft) { state = state.onConfirmPoll(emptyMap()) }
        assertEquals(0, state.attemptsLeft)
        assertTrue(!state.errorMessage.isNullOrBlank())
    }

    private fun AddSensorState.confirmStateOrNull(): WizardStep = step
}
