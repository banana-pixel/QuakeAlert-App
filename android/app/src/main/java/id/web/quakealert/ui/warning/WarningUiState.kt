package id.web.quakealert.ui.warning

import androidx.annotation.DrawableRes
import androidx.compose.runtime.Immutable
import id.web.quakealert.R
import id.web.quakealert.data.UnitSystem
import id.web.quakealert.ui.history.QuakeHistoryItem

/**
 * The banner at the top of the Warning screen's resting state (Figma nodes
 * 124:1297 / 124:1426).
 *
 * Sealed so the two states can never mix their fields: a recently-detected quake
 * carries an intensity + relative time, while the calm state carries a possibility
 * read. The variant also drives the banner's gradient and glyph in the component
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
 * Calm state (Figma 124:1426): no recent quake, so the banner reads the
 * possibility of one instead.
 */
@Immutable
data class PossibilityBanner(
    override val title: String,
    /** Possibility read (e.g. "Possibility : High Risk"). */
    val possibilityLabel: String
) : WarningBanner

/**
 * Content of the Earthquake Possibility overlay (Figma 124:1605), opened from the
 * resting banner's "SEE DETAILS" action: where the risk is, the most recent quake
 * and the local count within the coverage radius, plus the accuracy disclaimer.
 *
 * The `(--- km radius)` markers are placeholders from the design — they become
 * real radius values once the sensor data feed is wired in.
 */
@Immutable
data class EarthquakePossibility(
    val location: String = "Lembang, West Java, ID",
    val possibilityLabel: String = "Possibility : High Risk",
    val recentEarthquakeLabel: String = "Recent Earthquake (--- km radius)",
    val recentEarthquakeValue: String = "2 days ago",
    val earthquakeCountLabel: String = "Earthquake Count (--- km radius)",
    val earthquakeCountValue: String = "41.40338, 2.17403",
    val disclaimer: String = "Data may not accurate and should not used for reference. Sensor count in your area affect data accuracy."
)

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
     * Calm / monitoring state (Figma 124:1297 / 124:1426).
     *
     * The banner/tips/section title are a single package so the sub-states stay
     * coherent: a recent quake pairs the crimson banner with aftershock tips, and
     * a quiet network pairs the possibility banner with preparedness tips.
     *
     * [isLoading], [isError] and [errorMessage] form the body's state machine: the
     * tips region renders exactly one of loading / error / empty / content.
     *
     * @param isLoading true while the alert feed is in flight.
     * @param isError true when the last load failed; pairs with [errorMessage].
     * @param errorMessage failure copy shown by
     *   [id.web.quakealert.ui.common.QuakeErrorState], or null when there is no error.
     * @param banner the summary [WarningBanner] variant.
     * @param sectionTitle headline above the tip list, driven by the banner state.
     * @param tips ordered preparedness tips to render.
     * @param selectedEventDetails the event whose "Recent Earthquake" detail overlay
     *   (Figma node 124:1192) is open — raised from the active banner's action — or
     *   null when no overlay is showing.
     * @param selectedPossibility the [EarthquakePossibility] overlay (Figma 124:1605)
     *   raised from the resting banner's action, or null when it is closed.
     */
    @Immutable
    data class Idle(
        val isLoading: Boolean = false,
        val isError: Boolean = false,
        val errorMessage: String? = null,
        val banner: WarningBanner = PossibilityBanner(
            title = "No Recent Earthquake",
            possibilityLabel = "Possibility : High Risk"
        ),
        val sectionTitle: String = "Stay prepared for an earthquake",
        val tips: List<PreparednessTip> = noActiveQuakeTips(),
        override val unitSystem: UnitSystem = UnitSystem.METRIC,
        val selectedEventDetails: QuakeHistoryItem? = null,
        val selectedPossibility: EarthquakePossibility? = null
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
     */
    @Immutable
    data class ActiveAlert(
        val eventId: String,
        val intensityValue: String,
        val distanceKm: Int?,
        val locationName: String,
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
