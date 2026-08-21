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
 * inside, outside, unknown position, and the two coordinate seams where a naive
 * distance formula goes wrong.
 */
class AlertGateTest {

    @Test
    fun `alarms when the centroid is inside the coverage radius`() {
        val decision = AlertGate.decide(
            userLocation = BANDUNG,
            centroidLat = JAKARTA.latitude,
            centroidLon = JAKARTA.longitude,
            coverageRadiusKm = 150
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
    fun `does not alarm when the centroid is outside the coverage radius`() {
        val decision = AlertGate.decide(
            userLocation = BANDUNG,
            centroidLat = MEDAN.latitude,
            centroidLon = MEDAN.longitude,
            coverageRadiusKm = 300
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
            centroidLon = MEDAN.longitude,
            coverageRadiusKm = 50
        )

        // A missed warning is worse than a distant one — the tightest radius and the
        // furthest quake must still alarm when the position is unknown.
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
            centroidLon = -179.9,
            coverageRadiusKm = 50
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
            centroidLon = 120.0,
            coverageRadiusKm = 300
        )

        // 2° of latitude ≈ 222 km; a sign error would collapse this toward zero.
        assertTrue(decision.distanceKm!! in 200.0..240.0)
        assertTrue(decision.shouldAlarm)
    }

    @Test
    fun `switches verdict across the radius boundary`() {
        // Asserted a metre either side rather than exactly on the radius: at the exact
        // boundary the verdict rests on the last bit of a double, which is not
        // behaviour worth pinning.
        val inside = AlertGate.decide(
            userLocation = EQUATOR_ORIGIN,
            centroidLat = 0.0,
            centroidLon = degreesEastOfOriginFor(kilometres = 149.999),
            coverageRadiusKm = 150
        )
        assertTrue(inside.shouldAlarm)
        assertEquals(AlertGateReason.WITHIN_RADIUS, inside.reason)

        val outside = AlertGate.decide(
            userLocation = EQUATOR_ORIGIN,
            centroidLat = 0.0,
            centroidLon = degreesEastOfOriginFor(kilometres = 150.001),
            coverageRadiusKm = 150
        )
        assertFalse(outside.shouldAlarm)
        assertEquals(AlertGateReason.OUTSIDE_RADIUS, outside.reason)
    }

    @Test
    fun `clamps a corrupt radius to the settings bounds`() {
        // A zero or negative stored radius means a corrupt preference, not consent to
        // silence every alert: it must behave as the 50 km floor.
        val nearby = AlertGate.decide(
            userLocation = EQUATOR_ORIGIN,
            centroidLat = 0.0,
            centroidLon = degreesEastOfOriginFor(kilometres = 40.0),
            coverageRadiusKm = 0
        )
        assertTrue(nearby.shouldAlarm)

        val outsideFloor = AlertGate.decide(
            userLocation = EQUATOR_ORIGIN,
            centroidLat = 0.0,
            centroidLon = degreesEastOfOriginFor(kilometres = 80.0),
            coverageRadiusKm = -1
        )
        assertFalse(outsideFloor.shouldAlarm)

        // And an absurdly large one does not become a global alarm: 500 km is past
        // the 300 km ceiling.
        val beyondCeiling = AlertGate.decide(
            userLocation = EQUATOR_ORIGIN,
            centroidLat = 0.0,
            centroidLon = degreesEastOfOriginFor(kilometres = 500.0),
            coverageRadiusKm = Int.MAX_VALUE
        )
        assertFalse(beyondCeiling.shouldAlarm)
    }

    @Test
    fun `shouldAlarm agrees with decide`() {
        listOf(BANDUNG, MEDAN, null).forEach { location ->
            val expected = AlertGate.decide(location, JAKARTA.latitude, JAKARTA.longitude, 150)
            assertEquals(
                expected.shouldAlarm,
                AlertGate.shouldAlarm(location, JAKARTA.latitude, JAKARTA.longitude, 150)
            )
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
