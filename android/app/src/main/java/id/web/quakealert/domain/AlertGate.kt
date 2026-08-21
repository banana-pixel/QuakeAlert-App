package id.web.quakealert.domain

/**
 * Why the gate reached its verdict, so the screen can say something honest about
 * what it knows.
 */
enum class AlertGateReason {

    /** The centroid is inside the user's coverage radius. */
    WITHIN_RADIUS,

    /** The centroid is outside it — no alarm, banner only. */
    OUTSIDE_RADIUS,

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
 * The distance check that must run before anything wakes the user.
 *
 * .clinerules/20 rule 2 and docs/CLIENT_SPEC.md §7 both make this mandatory and
 * non-negotiable: the server's realtime channels are broadcast, so *every* device
 * receives *every* event, and without this gate a tremor in Bandung sounds an
 * alarm in Medan. Both alarm paths go through here — [AlertGate] is called before
 * `AlertSiren.start()` in the foreground and before `startActivity()` in the
 * push handler.
 *
 * Distances use [haversineKm] with the project-fixed `R = 6371.0 km`, which is
 * what keeps this agreeing with the PostGIS radius filters on the server.
 */
object AlertGate {

    /**
     * Decides whether [centroidLat] / [centroidLon] is close enough to
     * [userLocation] to sound the alarm.
     *
     * **Fails open when the position is unknown.** A user who has not yet synced a
     * position would otherwise be silently excluded from every warning, which is
     * the one failure mode this app cannot have: a distant alarm is an annoyance, a
     * missing one is the thing the product exists to prevent. The decision carries
     * [AlertDecision.isDistanceUnknown] so the UI can say the distance is unknown
     * rather than invent one.
     */
    fun decide(
        userLocation: UserLocation?,
        centroidLat: Double,
        centroidLon: Double,
        coverageRadiusKm: Int
    ): AlertDecision {
        val distanceKm = userLocation.distanceKmTo(centroidLat, centroidLon)
            ?: return AlertDecision(
                shouldAlarm = true,
                distanceKm = null,
                reason = AlertGateReason.LOCATION_UNKNOWN
            )

        // A non-positive or absurd radius means a corrupt preference, not "alert on
        // nothing" — clamp to the same bounds the Settings slider offers.
        val radiusKm = coverageRadiusKm.coerceIn(MIN_RADIUS_KM, MAX_RADIUS_KM)
        val within = distanceKm <= radiusKm

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
        coverageRadiusKm: Int
    ): Boolean = decide(userLocation, centroidLat, centroidLon, coverageRadiusKm).shouldAlarm

    /** Mirrors `AppSettingsRepository.RADIUS_RANGE`, duplicated to keep domain pure. */
    const val MIN_RADIUS_KM = 50
    const val MAX_RADIUS_KM = 300
}
