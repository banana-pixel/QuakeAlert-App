package id.web.quakealert.domain

/**
 * Why the gate reached its verdict, so the screen can say something honest about
 * what it knows.
 */
enum class AlertGateReason {

    /** The centroid is inside [SafetyPolicy.ALERT_RADIUS_KM]. */
    WITHIN_RADIUS,

    /** The centroid is outside it — no alarm, banner only. */
    OUTSIDE_RADIUS,

    /**
     * The quake is severe enough that distance was not considered at all
     * (`SafetyPolicy.isSevere`). Distinct from [WITHIN_RADIUS] because the user may
     * be hundreds of kilometres away and still hear the siren; the UI should be able
     * to explain that rather than look broken.
     */
    SEVERE_OVERRIDE,

    /**
     * The user's position is unknown, so the distance could not be computed and the
     * gate failed open.
     */
    LOCATION_UNKNOWN
}

/**
 * The gate's verdict on one alert.
 *
 * @param shouldAlarm whether the siren and the full-screen alert may fire.
 * @param distanceKm great-circle distance from the user to the centroid, or null
 *   when the user's position is unknown. Null rather than 0.0 — see
 *   [distanceKmTo].
 */
data class AlertDecision(
    val shouldAlarm: Boolean,
    val distanceKm: Double?,
    val reason: AlertGateReason
) {
    /** True when the alarm fired without knowing how far away the quake is. */
    val isDistanceUnknown: Boolean get() = distanceKm == null
}

/**
 * The check that must run before anything wakes the user.
 *
 * .clinerules/20 rule 2 and docs/CLIENT_SPEC.md §7 both make this mandatory and
 * non-negotiable: the server's realtime channels are broadcast, so a device can
 * receive an event it is nowhere near, and without this gate a tremor in Bandung
 * sounds an alarm in Medan. Both alarm paths go through here — [AlertGate] is
 * called before `AlertSiren.start()` in the foreground and before `startActivity()`
 * in the push handler.
 *
 * Distances use [haversineKm] with the project-fixed `R = 6371.0 km`, against the
 * fixed [SafetyPolicy.ALERT_RADIUS_KM], which is what keeps this agreeing with the
 * PostGIS radius filter the server used to choose recipients in the first place.
 */
object AlertGate {

    /**
     * Decides whether [centroidLat] / [centroidLon] should sound the alarm for a
     * user at [userLocation], given the event's intensity.
     *
     * Three ways to end up alarming, in the order they are checked:
     *
     * 1. **Intensity override.** MMI ≥ VII or PGA ≥ 250 gal alarms unconditionally,
     *    before the distance is even computed. The server did not filter this event
     *    by distance either, so neither may the client.
     * 2. **Unknown position — fails open.** A user who has not synced a position
     *    would otherwise be silently excluded from every warning, the one failure
     *    mode this app cannot have: a distant alarm is an annoyance, a missing one is
     *    the thing the product exists to prevent. The decision carries
     *    [AlertDecision.isDistanceUnknown] so the UI can say the distance is unknown
     *    rather than invent one.
     * 3. **Within [SafetyPolicy.ALERT_RADIUS_KM]** of the centroid.
     *
     * @param mmi Roman-numeral intensity from the payload; null or unrecognised
     *   falls through to [pgaGal], which is why both are accepted.
     */
    fun decide(
        userLocation: UserLocation?,
        centroidLat: Double,
        centroidLon: Double,
        mmi: String? = null,
        pgaGal: Double = 0.0
    ): AlertDecision {
        val distanceKm = userLocation.distanceKmTo(centroidLat, centroidLon)

        // Checked first, and independently of the distance, so a severe quake alarms
        // even for a user whose position was never synced.
        if (SafetyPolicy.isSevere(mmi, pgaGal)) {
            return AlertDecision(
                shouldAlarm = true,
                distanceKm = distanceKm,
                reason = AlertGateReason.SEVERE_OVERRIDE
            )
        }

        if (distanceKm == null) {
            return AlertDecision(
                shouldAlarm = true,
                distanceKm = null,
                reason = AlertGateReason.LOCATION_UNKNOWN
            )
        }

        val within = distanceKm <= SafetyPolicy.ALERT_RADIUS_KM
        return AlertDecision(
            shouldAlarm = within,
            distanceKm = distanceKm,
            reason = if (within) AlertGateReason.WITHIN_RADIUS else AlertGateReason.OUTSIDE_RADIUS
        )
    }

    /** [decide]'s verdict alone, for callers that need nothing else. */
    fun shouldAlarm(
        userLocation: UserLocation?,
        centroidLat: Double,
        centroidLon: Double,
        mmi: String? = null,
        pgaGal: Double = 0.0
    ): Boolean = decide(userLocation, centroidLat, centroidLon, mmi, pgaGal).shouldAlarm
}
