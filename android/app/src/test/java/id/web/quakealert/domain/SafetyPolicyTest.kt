package id.web.quakealert.domain

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The intensity override, pinned on both of its independent paths.
 *
 * The mirror of the server's `dispatch/severity_test.go`: the two implementations
 * decide the same thing about the same event, one choosing whether to broadcast
 * without a distance filter and the other whether to sound the siren anyway. A
 * disagreement here means a device discards the alert the server deliberately sent
 * it, so the thresholds are asserted exactly rather than approximately.
 */
class SafetyPolicyTest {

    @Test
    fun `MMI at and above the threshold is severe`() {
        assertTrue(SafetyPolicy.isSevere("VII", 0.0))
        assertTrue(SafetyPolicy.isSevere("VIII", 0.0))
        assertTrue(SafetyPolicy.isSevere("XII", 0.0))
    }

    @Test
    fun `MMI one step below the threshold is not`() {
        // The guard against an override so eager it replaces the radius gate.
        assertFalse(SafetyPolicy.isSevere("VI", 0.0))
        assertFalse(SafetyPolicy.isSevere("I", 0.0))
    }

    @Test
    fun `PGA at and above the threshold is severe`() {
        assertTrue(SafetyPolicy.isSevere("V", SafetyPolicy.OVERRIDE_PGA_GAL))
        assertTrue(SafetyPolicy.isSevere("V", 900.0))
    }

    @Test
    fun `PGA just below the threshold is not`() {
        assertFalse(SafetyPolicy.isSevere("V", SafetyPolicy.OVERRIDE_PGA_GAL - 0.1))
    }

    @Test
    fun `an unusable MMI is rescued by the PGA path`() {
        // Why the two signals are independent: the label can arrive missing or
        // malformed, and the acceleration is then the only thing left to judge by.
        assertTrue(SafetyPolicy.isSevere(null, 400.0))
        assertTrue(SafetyPolicy.isSevere("", 400.0))
        assertTrue(SafetyPolicy.isSevere("garbage", 400.0))

        // ...and with nothing usable on either side, it is not severe. Guessing high
        // here would mean a siren for every tremor with a typo in its label.
        assertFalse(SafetyPolicy.isSevere("garbage", 10.0))
        assertFalse(SafetyPolicy.isSevere(null, 0.0))
    }

    @Test
    fun `MMI parsing tolerates case and surrounding space`() {
        // Both have been seen from real payload producers; neither should cost a siren.
        assertTrue(SafetyPolicy.isSevere("vii", 0.0))
        assertTrue(SafetyPolicy.isSevere("  VII  ", 0.0))
    }

    @Test
    fun `an unrecognised numeral parses low, not high`() {
        listOf("", "XIII", "7", "M", "IIII", "V.5", null).forEach { input ->
            assertEquals("$input must not parse", 0, SafetyPolicy.romanToMmi(input))
        }
        assertEquals(SafetyPolicy.OVERRIDE_MMI, SafetyPolicy.romanToMmi("VII"))
    }

    @Test
    fun `the thresholds match the server constants`() {
        // dispatch.SevereMMI and dispatch.SeverePGAGal. Written as literals so a change
        // on this side cannot silently follow a change on the other.
        assertEquals(7, SafetyPolicy.OVERRIDE_MMI)
        assertEquals(250.0, SafetyPolicy.OVERRIDE_PGA_GAL, 0.0)
    }

    @Test
    fun `the browse filters are wider and narrower than the alert radius by intent`() {
        // Neither is a life-safety number, but both are easy to swap by accident, and
        // swapped they would make History too narrow and Sensors too broad to mean
        // anything.
        assertEquals(250, SafetyPolicy.HISTORY_NEAR_RADIUS_KM)
        assertEquals(150, SafetyPolicy.SENSORS_NEAR_RADIUS_KM)
        assertTrue(SafetyPolicy.HISTORY_NEAR_RADIUS_KM > SafetyPolicy.ALERT_RADIUS_KM)
        assertTrue(SafetyPolicy.SENSORS_NEAR_RADIUS_KM < SafetyPolicy.ALERT_RADIUS_KM)
    }
}
