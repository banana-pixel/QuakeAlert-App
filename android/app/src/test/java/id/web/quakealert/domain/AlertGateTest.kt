package id.web.quakealert.domain

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The gate is the only thing standing between a nationwide broadcast and a siren
 * in the user's pocket (.clinerules/20 rule 2), so its edges are pinned here:
 * inside, outside, the intensity override, unknown position, and the two
 * coordinate seams where a naive distance formula goes wrong.
 *
 * The radius is no longer a parameter — [SafetyPolicy.ALERT_RADIUS_KM] decides,
 * and the boundary tests below are written against it rather than a literal so
 * they keep testing the boundary if the constant ever moves.
 */
class AlertGateTest {

    @Test
    fun `alarms when the centroid is inside the fixed radius`() {
        val decision = AlertGate.decide(
            userLocation = BANDUNG,
            centroidLat = JAKARTA.latitude,
            centroidLon = JAKARTA.longitude
        )

        assertTrue(decision.shouldAlarm)
        assertEquals(AlertGateReason.WITHIN_RADIUS, decision.reason)
        // ~117 km apart; the assertion is loose because the exact value belongs to
        // haversineKm, not to the gate.
        assertNotNull(decision.distanceKm)
        assertTrue(decision.distanceKm!! in 100.0..130.0)
        assertFalse(decision.isDistanceUnknown)
    }

    @Test
    fun `does not alarm when the centroid is outside the fixed radius`() {
        val decision = AlertGate.decide(
            userLocation = BANDUNG,
            centroidLat = MEDAN.latitude,
            centroidLon = MEDAN.longitude
        )

        assertFalse(decision.shouldAlarm)
        assertEquals(AlertGateReason.OUTSIDE_RADIUS, decision.reason)
        // Still reported: the banner says how far away it was even with no alarm.
        assertTrue(decision.distanceKm!! > 1_000.0)
    }

    @Test
    fun `fails open with an unknown position`() {
        val decision = AlertGate.decide(
            userLocation = null,
            centroidLat = MEDAN.latitude,
            centroidLon = MEDAN.longitude
        )

        // A missed warning is worse than a distant one: a user who never synced a
        // position must not be silently excluded from every alert.
        assertTrue(decision.shouldAlarm)
        assertEquals(AlertGateReason.LOCATION_UNKNOWN, decision.reason)
        assertNull(decision.distanceKm)
        assertTrue(decision.isDistanceUnknown)
    }

    @Test
    fun `measures across the antimeridian rather than around the globe`() {
        val west = UserLocation(latitude = 0.5, longitude = 179.9)
        val decision = AlertGate.decide(
            userLocation = west,
            centroidLat = 0.5,
            centroidLon = -179.9
        )

        // 0.2° apart across the seam ≈ 22 km. A formula that subtracted the raw
        // longitudes would read ~40 000 km and stay silent.
        assertTrue(decision.distanceKm!! < 30.0)
        assertTrue(decision.shouldAlarm)
    }

    @Test
    fun `measures across the equator`() {
        val north = UserLocation(latitude = 1.0, longitude = 120.0)
        val decision = AlertGate.decide(
            userLocation = north,
            centroidLat = -1.0,
            centroidLon = 120.0
        )

        // 2° of latitude ≈ 222 km, which also puts it just past the fixed radius. A
        // sign error would collapse the distance toward zero and alarm instead, so
        // the silence is part of the assertion rather than incidental to it.
        assertTrue(decision.distanceKm!! in 200.0..240.0)
        assertFalse(decision.shouldAlarm)
    }

    @Test
    fun `switches verdict across the radius boundary`() {
        // Asserted a metre either side rather than exactly on the radius: at the exact
        // boundary the verdict rests on the last bit of a double, which is not
        // behaviour worth pinning.
        val radius = SafetyPolicy.ALERT_RADIUS_KM.toDouble()

        val inside = AlertGate.decide(
            userLocation = EQUATOR_ORIGIN,
            centroidLat = 0.0,
            centroidLon = degreesEastOfOriginFor(kilometres = radius - 0.001)
        )
        assertTrue(inside.shouldAlarm)
        assertEquals(AlertGateReason.WITHIN_RADIUS, inside.reason)

        val outside = AlertGate.decide(
            userLocation = EQUATOR_ORIGIN,
            centroidLat = 0.0,
            centroidLon = degreesEastOfOriginFor(kilometres = radius + 0.001)
        )
        assertFalse(outside.shouldAlarm)
        assertEquals(AlertGateReason.OUTSIDE_RADIUS, outside.reason)
    }

    @Test
    fun `the radius is fixed at 200 km and not a preference`() {
        // Pinned as a cross-repo contract: the same number is dispatch.AlertRadiusKm
        // on the server, which chose who received this event in the first place.
        // Changing it on one side only makes the two disagree about who gets woken.
        assertEquals(200, SafetyPolicy.ALERT_RADIUS_KM)
    }

    @Test
    fun `a severe quake alarms far outside the radius`() {
        val decision = AlertGate.decide(
            userLocation = BANDUNG,
            centroidLat = MEDAN.latitude,
            centroidLon = MEDAN.longitude,
            mmi = "VII"
        )

        assertTrue(decision.shouldAlarm)
        assertEquals(AlertGateReason.SEVERE_OVERRIDE, decision.reason)
        // The distance is still reported — the UI says how far, it just does not
        // decide anything.
        assertTrue(decision.distanceKm!! > 1_000.0)
    }

    @Test
    fun `PGA alone can trigger the override when MMI is unusable`() {
        // MMI travels as a Roman-numeral string, so a malformed one parses to 0 and
        // the acceleration is the only thing left to judge by.
        val decision = AlertGate.decide(
            userLocation = BANDUNG,
            centroidLat = MEDAN.latitude,
            centroidLon = MEDAN.longitude,
            mmi = "???",
            pgaGal = SafetyPolicy.OVERRIDE_PGA_GAL
        )

        assertTrue(decision.shouldAlarm)
        assertEquals(AlertGateReason.SEVERE_OVERRIDE, decision.reason)
    }

    @Test
    fun `a severe quake alarms even when the position is unknown`() {
        // The override is checked before the distance, so it wins over the fail-open
        // branch and the reason stays honest about why the siren went off.
        val decision = AlertGate.decide(
            userLocation = null,
            centroidLat = MEDAN.latitude,
            centroidLon = MEDAN.longitude,
            mmi = "IX"
        )

        assertTrue(decision.shouldAlarm)
        assertEquals(AlertGateReason.SEVERE_OVERRIDE, decision.reason)
        assertNull(decision.distanceKm)
    }

    @Test
    fun `an intensity below the override still obeys the radius`() {
        // The guard against an override that swallows the gate entirely: MMI VI and
        // 249.9 gal are each one step short, and a distant quake must stay silent.
        val decision = AlertGate.decide(
            userLocation = BANDUNG,
            centroidLat = MEDAN.latitude,
            centroidLon = MEDAN.longitude,
            mmi = "VI",
            pgaGal = SafetyPolicy.OVERRIDE_PGA_GAL - 0.1
        )

        assertFalse(decision.shouldAlarm)
        assertEquals(AlertGateReason.OUTSIDE_RADIUS, decision.reason)
    }

    @Test
    fun `shouldAlarm agrees with decide`() {
        listOf(BANDUNG, MEDAN, null).forEach { location ->
            listOf(null to 0.0, "VII" to 0.0, "V" to 400.0).forEach { (mmi, pga) ->
                val expected = AlertGate.decide(
                    location, JAKARTA.latitude, JAKARTA.longitude, mmi, pga
                )
                assertEquals(
                    expected.shouldAlarm,
                    AlertGate.shouldAlarm(
                        location, JAKARTA.latitude, JAKARTA.longitude, mmi, pga
                    )
                )
            }
        }
    }

    private companion object {
        val BANDUNG = UserLocation(latitude = -6.9175, longitude = 107.6191)
        val JAKARTA = UserLocation(latitude = -6.2088, longitude = 106.8456)
        val MEDAN = UserLocation(latitude = 3.5952, longitude = 98.6722)
        val EQUATOR_ORIGIN = UserLocation(latitude = 0.0, longitude = 100.0)

        /**
         * Longitude that is [kilometres] due east of [EQUATOR_ORIGIN]. Derived from
         * [EARTH_RADIUS_KM] rather than a magic 111.19 so the expectation cannot drift
         * from the radius the app actually uses.
         */
        fun degreesEastOfOriginFor(kilometres: Double): Double =
            EQUATOR_ORIGIN.longitude + Math.toDegrees(kilometres / EARTH_RADIUS_KM)
    }
}
