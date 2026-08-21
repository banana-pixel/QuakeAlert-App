package id.web.quakealert.domain

/**
 * The distance and intensity rules that decide who gets woken up.
 *
 * These were user preferences once — a slider in Settings that shipped with a
 * default nobody understood the consequences of. That was the wrong place for the
 * decision. Someone who narrows their radius to cut down on notifications has made
 * a safety choice without knowing they were making one, and the only person who
 * finds out they were wrong is them, after the earthquake. Every operational EEW
 * system fixes these thresholds for the same reason.
 *
 * So they live here, in one object, as constants. Two of them are contracts with
 * code that is not in this repository:
 *
 * - [ALERT_RADIUS_KM] must equal `dispatch.AlertRadiusKm` on the server. The server
 *   uses it to pick which FCM tokens to send to; this app uses it as the final gate
 *   before the siren. If they ever disagree, the device and the server disagree
 *   about who is in danger.
 * - [OVERRIDE_MMI] / [OVERRIDE_PGA_GAL] must equal `dispatch.SevereMMI` and
 *   `dispatch.SeverePGAGal`. For a quake this large the server stops filtering by
 *   distance entirely and broadcasts to every device; the client has to stop
 *   filtering too, or it would discard exactly the alerts that matter most.
 */
object SafetyPolicy {

    /**
     * Fixed alert radius, in kilometres, from the centroid. Wide enough to cover the
     * damage distance of a destructive Indonesian quake, narrow enough not to train
     * people to ignore the siren.
     */
    const val ALERT_RADIUS_KM = 200

    /**
     * Modified Mercalli intensity at or above which distance stops mattering. VII is
     * where non-structural damage becomes widespread, so there is no distance at
     * which "you did not need to know" is the right answer.
     */
    const val OVERRIDE_MMI = 7

    /**
     * Peak ground acceleration, in gal, that triggers the same override.
     *
     * Independent of [OVERRIDE_MMI] rather than derived from it: MMI arrives as a
     * Roman-numeral string and an unrecognised one parses to zero, so this is the
     * path that still fires when the label is missing or malformed.
     */
    const val OVERRIDE_PGA_GAL = 250.0

    /**
     * Radius for the History screen's "NEAR" filter. Wider than [ALERT_RADIUS_KM] on
     * purpose: browsing past quakes is not a life-safety path, and someone looking
     * back at what happened wants the regional picture, including the events that
     * were never loud enough to alarm them.
     */
    const val HISTORY_NEAR_RADIUS_KM = 250

    /**
     * Radius for the Sensors screen's "NEAR" filter. Narrower than
     * [ALERT_RADIUS_KM]: this list answers "what is watching my area", and a station
     * 200 km away is not meaningfully watching it.
     */
    const val SENSORS_NEAR_RADIUS_KM = 150

    /**
     * Whether an event's intensity alone is enough to alarm, whatever the distance.
     *
     * Either signal is sufficient. They are two estimates of the same measurement and
     * either can be absent, so requiring both would mean a malformed [mmi] silences a
     * severe quake.
     */
    fun isSevere(mmi: String?, pgaGal: Double): Boolean =
        romanToMmi(mmi) >= OVERRIDE_MMI || pgaGal >= OVERRIDE_PGA_GAL

    /**
     * Parses a Roman-numeral MMI (`"I"`..`"XII"`, as carried end to end by the
     * contract) into an integer.
     *
     * Returns 0 for anything unrecognised — deliberately the *low* end. A parse
     * failure that produced a high number would turn every minor tremor into a
     * national siren; the [OVERRIDE_PGA_GAL] path is the safety net instead.
     */
    fun romanToMmi(mmi: String?): Int = when (mmi?.trim()?.uppercase()) {
        "I" -> 1
        "II" -> 2
        "III" -> 3
        "IV" -> 4
        "V" -> 5
        "VI" -> 6
        "VII" -> 7
        "VIII" -> 8
        "IX" -> 9
        "X" -> 10
        "XI" -> 11
        "XII" -> 12
        else -> 0
    }
}
