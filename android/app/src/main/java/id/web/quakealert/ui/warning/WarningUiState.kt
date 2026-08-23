package id.web.quakealert.ui.warning

import androidx.annotation.DrawableRes
import androidx.compose.runtime.Immutable
import id.web.quakealert.R
import id.web.quakealert.data.UnitSystem
import id.web.quakealert.domain.EmergencyNumber
import id.web.quakealert.domain.SafetyPolicy
import id.web.quakealert.ui.common.ErrorCopy
import id.web.quakealert.ui.history.QuakeHistoryItem

/**
 * The banner at the top of the Warning screen's resting state (Figma nodes
 * 124:1297 / 124:1426).
 *
 * Sealed so the two states can never mix their fields: a recently-detected quake
 * carries an intensity + relative time, while the calm state carries a read of how
 * much seismic activity the network has actually recorded nearby. The variant also drives the banner's gradient and glyph in the component
 * layer, mirroring how [id.web.quakealert.ui.history.MmiSeverity] picks the detail
 * modal's gradient — variant is identity, rendering is the component's job.
 *
 * Note the scope: this is the *summary* banner shown while monitoring, inside
 * [WarningUiState.Idle]. A quake that is happening right now replaces the whole
 * screen with [WarningUiState.ActiveAlert] instead, so there is no banner variant
 * for it.
 */
@Immutable
sealed interface WarningBanner {
    /** Banner headline (e.g. "Recent Earthquake Alert"). */
    val title: String
}

/**
 * Recent-quake state (Figma 124:1297): a quake was detected recently, so the banner
 * leads with the intensity and how long ago it hit.
 */
@Immutable
data class ActiveQuakeBanner(
    override val title: String,
    /** Roman-numeral intensity line (e.g. "Intensity : IV (moderate)"). */
    val intensityLabel: String,
    /** Relative detection time (e.g. "20 minutes ago"). */
    val timeAgo: String
) : WarningBanner

/**
 * Calm state (Figma 124:1426): no recent quake, so the banner reads what the network
 * *has* recorded nearby lately.
 *
 * It used to read "Possibility : High Risk". That was a prediction, and this system
 * cannot make one — it reports shaking its stations have already felt. A risk read
 * the data cannot support is worse than no read at all: a user who is told "high
 * risk" every calm day learns to discount the screen, and the one thing this screen
 * must never be is ignorable.
 */
@Immutable
data class SeismicActivityBanner(
    override val title: String,
    /** Recorded-activity read (e.g. "3 events nearby in the past 30 days"). */
    val activityLabel: String
) : WarningBanner

/**
 * Content of the "Recent Seismic Activity" overlay (the design's Figma 124:1605
 * frame, re-pointed), opened from the resting banner's "SEE DETAILS" action.
 *
 * This replaces the design's Earthquake Possibility card, and the replacement is the
 * honest one: the card's placeholders promised a risk forecast
 * (`"Possibility : High Risk"`), a radius the app did not know (`"(--- km radius)"`)
 * and a "count" that was actually a pair of coordinates. A MEMS network measures
 * shaking that has already happened; it does not forecast. So every field here is a
 * count, a time or an intensity the server has recorded — and when the server has
 * recorded nothing, the card says so rather than filling the space.
 *
 * Every string is composed by [WarningViewModel] from real events, except the two
 * that are constants of the query itself ([radiusKm], [windowDays]) and are printed
 * by the card in the user's unit.
 *
 * @param locationLabel the point the query was measured from, in prose or
 *   coordinates; names the absence when no fix has ever been synced.
 * @param radiusKm the radius the counts cover. Defaults to the fixed alert radius,
 *   because "near you" on this screen means the area alerts are issued for — not the
 *   browsable radius of the History filter, which the user can change.
 * @param windowDays how far back the counts reach.
 * @param eventCount confirmed events inside that radius and window.
 * @param isCountCapped true when the page hit its limit, so [eventCount] is a floor
 *   and must be printed as "N+" rather than as an exact tally.
 * @param mostRecent intensity + relative time of the newest event ("IV (moderate),
 *   2 days ago"), or null when the window holds none.
 * @param strongest intensity + PGA of the hardest shaking in the window, or null as
 *   above. Distinct from [mostRecent]: "the last one" and "the worst one" are
 *   different questions and are rarely the same event.
 * @param latitude device latitude the card's basemap is centred on, or null when no
 *   position has ever been synced — this card is about activity *where the user is*,
 *   so with no fix there is nothing honest to centre on.
 * @param longitude device longitude; see [latitude].
 */
/**
 * Why [RecentSeismicActivity]'s numbers may be missing. Three cases rather than a
 * nullable count, because they need three different sentences: an area the network
 * genuinely recorded nothing in is not the same as an area we could not ask about,
 * and neither is the same as a device that has never told us where it is. Collapsing
 * them would print "No events nearby" to a user we simply failed to measure.
 */
enum class ActivityAvailability { MEASURED, NO_POSITION, UNAVAILABLE }

@Immutable
data class RecentSeismicActivity(
    val locationLabel: String = "Location not synced",
    val availability: ActivityAvailability = ActivityAvailability.NO_POSITION,
    val radiusKm: Int = SafetyPolicy.ALERT_RADIUS_KM,
    val windowDays: Int = ACTIVITY_WINDOW_DAYS,
    val eventCount: Int = 0,
    val isCountCapped: Boolean = false,
    val mostRecent: String? = null,
    val strongest: String? = null,
    val latitude: Double? = null,
    val longitude: Double? = null
) {

    /** "3 events" / "20+ events" / "1 event" / "No events" — measured cases only. */
    private val countText: String
        get() = when {
            eventCount == 0 -> "No events"
            isCountCapped -> "$eventCount+ events"
            eventCount == 1 -> "1 event"
            else -> "$eventCount events"
        }

    /** The count row's value, or the reason there is no count. */
    val countValue: String get() = measured(countText)

    /** The newest event, "None recorded" for a quiet window, or the reason. */
    val mostRecentValue: String get() = measured(mostRecent ?: NONE_RECORDED)

    /** The hardest shaking, on the same three-way terms as [mostRecentValue]. */
    val strongestValue: String get() = measured(strongest ?: NONE_RECORDED)

    /** The count with no noun, for the banner's second clause: "9" / "100+". */
    private val countShort: String
        get() = if (isCountCapped) "$eventCount+" else "$eventCount"

    /**
     * The resting banner's headline.
     *
     * "No Recent Earthquake" is only said in the one case where it is true: the
     * network measured the neighbourhood and recorded nothing. With events on the
     * books it would contradict the line printed directly beneath it, and with no
     * measurement at all it would be a claim of quiet the app has not earned. Both
     * of those say "No Active Earthquake" instead, which is the narrower thing this
     * screen actually knows — no alert is running right now.
     */
    val bannerTitle: String
        get() = if (availability == ActivityAvailability.MEASURED && eventCount == 0) {
            TITLE_NONE_RECENT
        } else {
            TITLE_NONE_ACTIVE
        }

    /**
     * The resting banner's one line. Radius is left out on purpose — the banner has
     * a single line to spend and the card states the radius exactly, in the user's
     * own unit, which the banner is built too early to know.
     *
     * The newest event leads it, because "when did the ground last move near me" is
     * the question a user opens this screen with; the count follows as context. The
     * banner renders two lines, so both clauses fit.
     */
    val bannerLabel: String
        get() = when (availability) {
            ActivityAvailability.MEASURED -> when {
                eventCount == 0 ->
                    "No quakes recorded near you in the past $windowDays days"
                mostRecent != null ->
                    "Latest: $mostRecent \u00b7 $countShort nearby in $windowDays days"
                // A count with no newest event should not happen (both are read from
                // the same page), so this keeps the old wording rather than inventing
                // copy for a state that has no meaning.
                else -> "$countText nearby in the past $windowDays days"
            }
            ActivityAvailability.NO_POSITION -> "Sync your location to see nearby activity"
            ActivityAvailability.UNAVAILABLE -> "Recent activity unavailable"
        }

    private fun measured(value: String): String = when (availability) {
        ActivityAvailability.MEASURED -> value
        ActivityAvailability.NO_POSITION -> NEEDS_POSITION
        ActivityAvailability.UNAVAILABLE -> UNAVAILABLE_VALUE
    }

    companion object {
        /** Copy for the idle banner variants, matching the design (Figma 124:1426). */
        const val TITLE_NONE_RECENT = "No Recent Earthquake"
        const val TITLE_NONE_ACTIVE = "No Active Earthquake"

        private const val NONE_RECORDED = "None recorded"
        private const val NEEDS_POSITION = "Needs your location"
        private const val UNAVAILABLE_VALUE = "Unavailable offline"
    }
}

/**
 * How far back [RecentSeismicActivity] counts. A month: long enough that a quiet
 * fortnight does not read as a dead network, short enough that the count still
 * describes now rather than the region's history.
 */
const val ACTIVITY_WINDOW_DAYS = 30

/**
 * A single preparedness tip row (Figma node 1:1038): a circular white glyph on
 * the left with a bold title + dimmed description on the right.
 *
 * @param id stable identity for list keys.
 * @param icon circular tip glyph drawable.
 * @param title bold tip title (e.g. "Build a 72-Hour Kit").
 * @param description supporting guidance line.
 */
@Immutable
data class PreparednessTip(
    val id: String,
    @param:DrawableRes val icon: Int,
    val title: String,
    val description: String
)

/**
 * One of the three official Drop-Cover-Hold-On pictograms shown inside the active
 * alert's "Suggested Actions :" box (Figma node 1:1071).
 *
 * Figma ships the trio as a single raster; it is sliced into three panels here so
 * the box is three real cards, which is what lets each one carry its own
 * `contentDescription`. The label is baked into each panel's artwork (it is part of
 * the official IAEM/earthquakecountry graphic and must not be re-typeset), so
 * [label] exists for accessibility, not for drawing.
 */
@Immutable
data class SuggestedAction(
    val id: String,
    @param:DrawableRes val pictogram: Int,
    val label: String
)

/**
 * The three actions, in the order the official graphic sequences them — the order
 * is the instruction, so it is fixed rather than data-driven.
 */
fun suggestedActions(): List<SuggestedAction> = listOf(
    SuggestedAction("drop", R.drawable.ic_action_drop, "Drop!"),
    SuggestedAction("cover", R.drawable.ic_action_cover, "Cover!"),
    SuggestedAction("hold-on", R.drawable.ic_action_hold_on, "Hold on!")
)

/**
 * Immutable UI state for the Warning screen, as a two-state hierarchy rather than
 * one flag-bearing class:
 *
 *  - [Idle] — the calm/monitoring screen (Figma 124:1297 / 124:1426): header,
 *    summary banner, preparedness tips, emergency CTA.
 *  - [ActiveAlert] — the seismic emergency screen (Figma node 1:1043): the
 *    crimson card, the intensity and proximity read-out, the Drop-Cover-Hold-On
 *    actions and the two hardware controls.
 *
 * Sealed, and a whole-screen swap rather than a dialog over [Idle], for one
 * reason: during shaking the screen must show *only* what is actionable. A modal
 * leaves a scrollable list of preparedness tips and a stale banner alive behind it,
 * with a dismiss affordance that returns the user to them; separate states make
 * "there is nothing else on this screen right now" a property of the type instead
 * of something every composable has to remember to honour.
 *
 * [unitSystem] is common to both because a distance read-out has to respect the
 * user's unit choice on the emergency screen just as much as in the History feed.
 * Server health is deliberately absent: the header's badge reads the global
 * [id.web.quakealert.domain.ServerConnectionState], so it cannot disagree with the
 * other tabs.
 */
@Immutable
sealed interface WarningUiState {

    /**
     * Distance unit system (Metric / Imperial), persisted via
     * [id.web.quakealert.data.AppSettingsRepository] and shared with the History
     * and Sensors screens.
     */
    val unitSystem: UnitSystem

    /**
     * Returns this state with [unitSystem] applied. Exists because the persisted
     * unit arrives on its own flow and has to be folded into whichever variant is
     * current — the alternative, a `copy` per variant at every call site, is where
     * a new variant silently starts ignoring the setting.
     */
    fun withUnitSystem(unitSystem: UnitSystem): WarningUiState

    /**
     * The fixed alert radius in the selected unit, for the "Protection Status"
     * overlay's first rule.
     *
     * Derived here rather than passed in because it is not state: the radius is a
     * constant of [id.web.quakealert.domain.SafetyPolicy], and the only thing that
     * varies is the unit it is printed in.
     */
    val alertRadiusLabel: String
        get() = unitSystem.formatDistance(SafetyPolicy.ALERT_RADIUS_KM)

    /**
     * Calm / monitoring state (Figma 124:1297 / 124:1426).
     *
     * The banner/tips/section title are a single package so the sub-states stay
     * coherent: a recent quake pairs the crimson banner with aftershock tips, and
     * a quiet network pairs the possibility banner with preparedness tips.
     *
     * [isLoading], [isError] and [errorCopy] form the body's state machine: the
     * tips region renders exactly one of loading / error / empty / content.
     *
     * @param isLoading true while the alert feed is in flight.
     * @param isError true when the last load failed; pairs with [errorCopy].
     * @param errorCopy classified failure copy from
     *   [id.web.quakealert.ui.common.errorCopy], rendered by
     *   [id.web.quakealert.ui.common.QuakeErrorState], or null when there is no error.
     * @param banner the summary [WarningBanner] variant.
     * @param sectionTitle headline above the tip list, driven by the banner state.
     * @param tips ordered preparedness tips to render.
     * @param selectedEventDetails the event whose "Recent Earthquake" detail overlay
     *   (Figma node 124:1192) is open — raised from the active banner's action — or
     *   null when no overlay is showing.
     * @param selectedActivity the [RecentSeismicActivity] overlay raised from the
     *   resting banner's action, or null when it is closed.
     * @param emergencyInfo the resolved contents of the "Emergency Steps & Contacts"
     *   overlay, or null when it is closed. A payload rather than a flag, unlike
     *   [isProtectionStatusOpen]: the numbers depend on the network the phone is
     *   attached to and the position line on the last sync, so both are resolved when
     *   the overlay opens and the card stays a pure function of what it is handed.
     * @param isProtectionStatusOpen whether the "Protection Status" overlay is up,
     *   raised by the banner's info affordance. A flag rather than a payload,
     *   unlike its two siblings: the overlay states fixed policy, so there is
     *   nothing about the current alert for it to carry.
     */
    @Immutable
    data class Idle(
        val isLoading: Boolean = false,
        val isError: Boolean = false,
        val errorCopy: ErrorCopy? = null,
        val banner: WarningBanner = SeismicActivityBanner(
            title = RecentSeismicActivity.TITLE_NONE_RECENT,
            // The pre-load read, and deliberately not a number: until the query
            // returns, the app does not know how many events are nearby, and printing
            // a placeholder count would be the same lie the old "High Risk" told.
            activityLabel = "Checking recent activity nearby"
        ),
        val sectionTitle: String = "Stay prepared for an earthquake",
        val tips: List<PreparednessTip> = noActiveQuakeTips(),
        override val unitSystem: UnitSystem = UnitSystem.METRIC,
        val selectedEventDetails: QuakeHistoryItem? = null,
        val selectedActivity: RecentSeismicActivity? = null,
        val isProtectionStatusOpen: Boolean = false,
        val emergencyInfo: EmergencyInfoState? = null
    ) : WarningUiState {

        override fun withUnitSystem(unitSystem: UnitSystem): Idle =
            copy(unitSystem = unitSystem)
    }

    /**
     * Seismic emergency state (Figma node 1:1043), raised by a `HAPPENING` event
     * from the REST seed or an `EARTHQUAKE_ALERT` frame from the WebSocket/FCM
     * channel, and stood down by `EVENT_RESOLVED`.
     *
     * @param eventId de-duplication key. Blank for an advisory-shaped payload, so
     *   it must not be used as a map key without checking.
     * @param intensityValue the bare intensity read, e.g. "IV (moderate)" — no
     *   "Intensity :" prefix, which the card renders as its own label line.
     * @param distanceKm distance from the user to the centroid, or null when the
     *   device position is unknown. Null rather than 0 because "0 km away" reads as
     *   *at the epicentre*, the most alarming possible value to show for missing data.
     * @param locationName geocoded centroid name, e.g. "Bandung, West Java, ID".
     * @param isMuted true once the user has silenced the siren via "MUTE ALERT".
     *   The visual alert deliberately stays up regardless.
     * @param isSosLightOn true while the torch is strobing SOS.
     * @param isSosLightUnavailable true when the torch could not be turned on at
     *   all — no flash unit, or another app holds the camera. Surfaced so the
     *   control can say so instead of looking engaged over a dark LED.
     * @param isTest true when this screen was raised by a drill
     *   (`POST /api/v1/admin/test-alert`), which renders a "TEST" badge over the
     *   card. Only ever true on a debug build — a release build drops a drill frame
     *   before it becomes an alert
     *   (id.web.quakealert.data.network.mapper.toDomainOrNull) — so the badge is not
     *   what protects the public; it is what stops the *tester* mistaking a drill for
     *   the real thing, which is the whole reason a drill is safe to run at all.
     */
    @Immutable
    data class ActiveAlert(
        val eventId: String,
        val intensityValue: String,
        val distanceKm: Int?,
        val locationName: String,
        val isTest: Boolean = false,
        val isMuted: Boolean = false,
        val isSosLightOn: Boolean = false,
        val isSosLightUnavailable: Boolean = false,
        override val unitSystem: UnitSystem = UnitSystem.METRIC
    ) : WarningUiState {

        override fun withUnitSystem(unitSystem: UnitSystem): ActiveAlert =
            copy(unitSystem = unitSystem)

        /**
         * The card's single proximity line (Figma node 1:1068), e.g.
         * "3 km away (Bandung, West Java, ID)".
         *
         * Both halves degrade independently, because either can be missing on a
         * real payload: an unknown position drops to "Distance unknown" rather than
         * a fabricated number, and a centroid the server could not name drops the
         * parenthetical entirely rather than rendering an empty "()".
         */
        val proximityLabel: String
            get() {
                val distance = distanceKm
                    ?.let { "${unitSystem.formatDistance(it)} away" }
                    ?: "Distance unknown"
                return locationName
                    .takeIf { it.isNotBlank() }
                    ?.let { "$distance ($it)" }
                    ?: distance
            }
    }
}

/**
 * Tips for the recent-quake state (Figma 124:1297): post-quake guidance for
 * aftershocks. Mirrors the design's copy and glyphs verbatim.
 */
fun activeQuakeTips(): List<PreparednessTip> = listOf(
    PreparednessTip(
        id = "inspect",
        icon = R.drawable.ic_prep_inspect,
        title = "Inspect Your Home",
        description = "Check walls, ceilings, and foundation for cracks or damage before re-entering."
    ),
    PreparednessTip(
        id = "review",
        icon = R.drawable.ic_prep_review,
        title = "Review Your Safety Plan",
        description = "Confirm family members are safe and update your emergency contacts if needed."
    ),
    PreparednessTip(
        id = "hazard",
        icon = R.drawable.ic_prep_hazard,
        title = "Avoid Hazards",
        description = "Stay clear of broken glass, spilled chemicals, and damaged electrical wiring."
    )
)

/**
 * Tips for the calm state (Figma 124:1426): pre-quake preparedness guidance.
 * Mirrors the design's copy, reusing the existing kit/comms/home glyphs.
 */
fun noActiveQuakeTips(): List<PreparednessTip> = listOf(
    PreparednessTip(
        id = "kit",
        icon = R.drawable.ic_prep_kit,
        title = "Build a 72-Hour Kit",
        description = "Pack water, non-perishable food, flashlights, extra batteries, and a first-aid kit in an easy-to-reach bag."
    ),
    PreparednessTip(
        id = "comms",
        icon = R.drawable.ic_prep_comms,
        title = "Create a Communication Plan",
        description = "Choose a safe family meeting spot and designate an out-of-town emergency contact in case local cell networks fail."
    ),
    PreparednessTip(
        id = "home",
        icon = R.drawable.ic_prep_home,
        title = "Secure Heavy Items",
        description = "Anchor tall furniture, TVs, and large appliances to wall studs so they do not fall."
    )
)

/**
 * What the "Emergency Steps & Contacts" overlay shows that is not fixed copy.
 *
 * Assembled in [WarningViewModel] when the overlay opens rather than held all the
 * time: the country a phone is attached to changes while the app is running, and a
 * list resolved at startup would be the wrong country's by the time it mattered.
 *
 * @param numbers dialable numbers, [id.web.quakealert.domain.EmergencyContacts]'
 *   universal 112 first.
 * @param coordinatesLabel the last synced position, already formatted, or null when
 *   nothing has ever synced — in which case the overlay says so instead of printing
 *   0,0.
 */
@Immutable
data class EmergencyInfoState(
    val numbers: List<EmergencyNumber>,
    val coordinatesLabel: String?
)
