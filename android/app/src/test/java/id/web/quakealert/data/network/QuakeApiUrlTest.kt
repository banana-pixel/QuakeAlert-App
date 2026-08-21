package id.web.quakealert.data.network

import id.web.quakealert.domain.UserLocation
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

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

    private companion object {
        val BANDUNG = UserLocation(latitude = -6.9175, longitude = 107.6191)
    }
}
