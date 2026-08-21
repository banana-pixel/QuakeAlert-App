package id.web.quakealert.data.network

import id.web.quakealert.domain.SafetyPolicy
import id.web.quakealert.domain.UserLocation
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Instant

/**
 * The query bounds belong to the contract, not to the client, and the server
 * answers 400 rather than clamping — so the clamping the client does is what keeps
 * a UI-supplied value from becoming a failed request. The spatial trio is the
 * sharpest edge: `range_km` without `latitude`/`longitude` is a 400
 * (contracts/openapi/openapi.yaml).
 */
class QuakeApiUrlTest {

    @Test
    fun `omits the spatial trio unless both radius and centre are present`() {
        val noneUrl = QuakeApiClient.eventsUrl(rangeKm = null, center = null)
        assertNull(noneUrl.queryParameter("range_km"))
        assertNull(noneUrl.queryParameter("latitude"))
        assertNull(noneUrl.queryParameter("longitude"))

        // A radius with no stored fix must degrade to an unfiltered page, not a 400.
        val radiusOnly = QuakeApiClient.eventsUrl(rangeKm = 150, center = null)
        assertNull(radiusOnly.queryParameter("range_km"))
        assertNull(radiusOnly.queryParameter("latitude"))

        // And a known position with no radius is not a filter either.
        val centreOnly = QuakeApiClient.eventsUrl(rangeKm = null, center = BANDUNG)
        assertNull(centreOnly.queryParameter("range_km"))
        assertNull(centreOnly.queryParameter("longitude"))
    }

    @Test
    fun `sends all three spatial parameters together`() {
        val url = QuakeApiClient.eventsUrl(rangeKm = 150, center = BANDUNG)

        assertEquals("150", url.queryParameter("range_km"))
        assertEquals("-6.9175", url.queryParameter("latitude"))
        assertEquals("107.6191", url.queryParameter("longitude"))
    }

    @Test
    fun `clamps limit offset and radius to the contract bounds`() {
        val tooSmall = QuakeApiClient.eventsUrl(
            limit = 0,
            offset = -5,
            rangeKm = 0,
            center = BANDUNG
        )
        assertEquals("1", tooSmall.queryParameter("limit"))
        assertEquals("0", tooSmall.queryParameter("offset"))
        assertEquals("1", tooSmall.queryParameter("range_km"))

        val tooLarge = QuakeApiClient.eventsUrl(
            limit = 500,
            offset = 90_000,
            rangeKm = 9_000,
            center = BANDUNG
        )
        assertEquals("100", tooLarge.queryParameter("limit"))
        // The documented ceiling; a larger offset is a 400, not a clamp, server-side.
        assertEquals("50000", tooLarge.queryParameter("offset"))
        assertEquals("2000", tooLarge.queryParameter("range_km"))
    }

    @Test
    fun `always sends limit and offset on events`() {
        val url = QuakeApiClient.eventsUrl()

        assertEquals(QuakeApiClient.DEFAULT_LIMIT.toString(), url.queryParameter("limit"))
        assertEquals("0", url.queryParameter("offset"))
        // Paths in QuakeApiConfig are relative to the base URL, hence the leading slash.
        assertEquals("/${QuakeApiConfig.PATH_EVENTS}", url.encodedPath)
    }

    @Test
    fun `sensors sends only a radius and honours its narrower ceiling`() {
        assertNull(QuakeApiClient.sensorsUrl(rangeKm = null).queryParameter("range_km"))

        val clamped = QuakeApiClient.sensorsUrl(rangeKm = 2_000)
        // 500, not /events' 2000: this endpoint's own limit.
        assertEquals(
            QuakeApiClient.MAX_SENSOR_RANGE_KM.toString(),
            clamped.queryParameter("range_km")
        )

        val exact = QuakeApiClient.sensorsUrl(rangeKm = 150)
        assertEquals("150", exact.queryParameter("range_km"))
        // No centre is ever sent: the server measures from the position it holds.
        assertNull(exact.queryParameter("latitude"))
        assertNull(exact.queryParameter("longitude"))
        assertEquals("/${QuakeApiConfig.PATH_SENSORS}", exact.encodedPath)
    }

    @Test
    fun `omits the intensity and time filters when they are unset`() {
        val url = QuakeApiClient.eventsUrl()

        // Absent, not neutral: `min_pga=0` is still a predicate the server evaluates.
        assertNull(url.queryParameter("min_pga"))
        assertNull(url.queryParameter("since"))
        assertNull(url.queryParameter("until"))
    }

    @Test
    fun `sends the intensity and time filters independently of the spatial trio`() {
        val url = QuakeApiClient.eventsUrl(
            minPgaGal = 90.45,
            since = Instant.parse("2026-08-01T00:00:00Z"),
            until = Instant.parse("2026-08-10T05:30:00Z")
        )

        assertEquals("90.45", url.queryParameter("min_pga"))
        // RFC3339 in UTC regardless of the device time zone.
        assertEquals("2026-08-01T00:00:00Z", url.queryParameter("since"))
        assertEquals("2026-08-10T05:30:00Z", url.queryParameter("until"))
        // "Strong quakes in the past week" is a valid question with no radius.
        assertNull(url.queryParameter("range_km"))
    }

    @Test
    fun `clamps min_pga and drops a reversed time range`() {
        val clamped = QuakeApiClient.eventsUrl(minPgaGal = 9_000.0)
        assertEquals(
            QuakeApiClient.MAX_MIN_PGA_GAL.toString(),
            clamped.queryParameter("min_pga")
        )
        assertEquals("0.0", QuakeApiClient.eventsUrl(minPgaGal = -5.0).queryParameter("min_pga"))

        // since > until is a 400; an unfiltered page beats a failed request.
        val reversed = QuakeApiClient.eventsUrl(
            since = Instant.parse("2026-08-10T00:00:00Z"),
            until = Instant.parse("2026-08-01T00:00:00Z")
        )
        assertNull(reversed.queryParameter("since"))
        assertNull(reversed.queryParameter("until"))
    }

    /**
     * The threshold a user's MMI choice becomes must be the inverse of the formula
     * the server labels events with, or the list contradicts its own chips: an
     * "MMI VI+" query that sent the band-table 137.2 gal would drop events the
     * server itself stamped VI.
     */
    @Test
    fun `maps an MMI bucket to the gal threshold that reproduces its label`() {
        for (mmi in listOf(4, 6, 7)) {
            val floor = SafetyPolicy.minPgaForMmi(mmi)

            assertEquals(mmi, Math.round(SafetyPolicy.mmiFromPga(floor)).toInt())
            assertEquals(mmi - 1, Math.round(SafetyPolicy.mmiFromPga(floor - 0.5)).toInt())

            val url = QuakeApiClient.eventsUrl(minPgaGal = floor)
            assertEquals(floor.toString(), url.queryParameter("min_pga"))
        }

        // Sanity on the magnitudes so a sign flip in the regression cannot pass.
        assertTrue(SafetyPolicy.minPgaForMmi(4) in 25.0..26.0)
        assertTrue(SafetyPolicy.minPgaForMmi(6) in 90.0..91.0)
        assertTrue(SafetyPolicy.minPgaForMmi(7) in 169.0..170.0)
        // "All intensities" is no filter at all, not a floor of 0 gal.
        assertEquals(0.0, SafetyPolicy.minPgaForMmi(1), 0.0)
    }

    private companion object {
        val BANDUNG = UserLocation(latitude = -6.9175, longitude = 107.6191)
    }
}
