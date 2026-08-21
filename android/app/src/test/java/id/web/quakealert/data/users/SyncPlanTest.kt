package id.web.quakealert.data.users

import id.web.quakealert.device.Coordinates
import id.web.quakealert.domain.UserLocation
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Rules for [planSync] — the one piece of judgement in the position-sync path.
 *
 * Worth its own test because both branches are cheap to get backwards and neither
 * is visible when wrong: uploading too eagerly spends a request per launch, and
 * uploading too rarely leaves the server holding a position the user has left,
 * which silently mis-aims every alert gate and radius filter downstream.
 */
class SyncPlanTest {

    /** Alun-alun Bandung. */
    private val bandung = UserLocation(
        latitude = -6.9175,
        longitude = 107.6191,
        locationName = "Bandung"
    )

    /** ~117 km away: unambiguously a different place. */
    private val jakartaFix = Coordinates(latitude = -6.2088, longitude = 106.8456)

    /** ~0.1 km from [bandung]: the same spot, GPS jitter aside. */
    private val jitterFix = Coordinates(latitude = -6.9184, longitude = 107.6191)

    @Test
    fun `uploads when nothing is stored yet`() {
        val plan = planSync(stored = null, fix = jakartaFix, force = false)

        assertTrue("a server with no position must be told one", plan.upload)
        // There is no stored label to fall back on, so a failed geocode must send
        // none rather than reuse a name for a different place.
        assertFalse(plan.reuseStoredLabel)
    }

    @Test
    fun `skips the upload when the device has barely moved`() {
        val plan = planSync(stored = bandung, fix = jitterFix, force = false)

        assertFalse("GPS jitter must not spend a request", plan.upload)
        assertTrue("the stored label still describes this spot", plan.reuseStoredLabel)
    }

    @Test
    fun `uploads when the device has moved beyond the threshold`() {
        val plan = planSync(stored = bandung, fix = jakartaFix, force = false)

        assertTrue(plan.upload)
        assertFalse("\"Bandung\" must not be reattached to a Jakarta fix", plan.reuseStoredLabel)
    }

    @Test
    fun `a forced sync uploads even when the device has not moved`() {
        val plan = planSync(stored = bandung, fix = jitterFix, force = true)

        // Sync Now must produce a round trip: a silent no-op reads as a broken button.
        assertTrue(plan.upload)
        // Still the same spot, so the stored label remains valid as a geocode fallback.
        assertTrue(plan.reuseStoredLabel)
    }

    @Test
    fun `an unsynced coverage radius uploads even when the device has not moved`() {
        val plan = planSync(stored = bandung, fix = jitterFix, force = false, radiusChanged = true)

        // The slider is moved at a desk, so the 1 km shortcut would otherwise hold
        // the new radius back indefinitely — and the server aims alerts by it.
        assertTrue(plan.upload)
        assertTrue("still the same spot, so the label is still valid", plan.reuseStoredLabel)
    }

    @Test
    fun `the threshold is what decides, not the distance itself`() {
        // Same pair of positions either side of a threshold placed between them:
        // proves the comparison is against minMoveKm rather than a baked-in constant.
        val movedKm = 0.1

        assertFalse(planSync(bandung, jitterFix, force = false, minMoveKm = movedKm * 2).upload)
        assertTrue(planSync(bandung, jitterFix, force = false, minMoveKm = movedKm / 2).upload)
    }

    @Test
    fun `the default threshold is one kilometre`() {
        // Pinned because docs/CLIENT_SPEC.md 4.2 states it and the server has no say:
        // a change here silently alters how often every client reports in.
        assertTrue(MIN_MOVE_KM == 1.0)
    }
}
