package id.web.quakealert.ui.warning

import androidx.annotation.DrawableRes
import androidx.compose.runtime.Immutable
import id.web.quakealert.R
import id.web.quakealert.data.UnitSystem
import id.web.quakealert.ui.history.QuakeHistoryItem

/**
 * The banner at the top of the Warning screen (Figma nodes 124:1297 / 124:1426).
 *
 * Sealed so the two states can never mix their fields: an active alert carries an
 * intensity + relative time, while the resting state carries a possibility read.
 * The variant also drives the banner's gradient and glyph in the component layer,
 * mirroring how [id.web.quakealert.ui.history.MmiSeverity] picks the detail
 * modal's gradient — variant is identity, rendering is the component's job.
 */
@Immutable
sealed interface WarningBanner {
    /** Banner headline (e.g. "Recent Earthquake Alert"). */
    val title: String
}

/**
 * Alert state (Figma 124:1297): a quake was detected recently, so the banner
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
 * Resting state (Figma 124:1426): no recent quake, so the banner reads the
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
 * Immutable UI state for the Warning screen (Figma nodes 124:1297 / 124:1426 /
 * 124:1605). Hoisted into [WarningViewModel] and consumed by the stateless
 * [WarningScreen].
 *
 * The banner/tips/section title are a single package so the two states stay
 * coherent: an active alert pairs the crimson banner with aftershock tips, and
 * the resting state pairs the possibility banner with preparedness tips.
 *
 * @param isHealthy drives the shared [id.web.quakealert.ui.common.QuakeAppBar]
 *   network-status badge.
 * @param banner the active [WarningBanner] variant.
 * @param sectionTitle headline above the tip list, driven by the banner state.
 * @param tips ordered preparedness tips to render.
 * @param unitSystem distance unit system (Metric / Imperial), persisted via
 *   [id.web.quakealert.data.AppSettingsRepository] and shared with the History
 *   and Sensors screens.
 * @param selectedEventDetails the event whose "Recent Earthquake" detail overlay
 *   (Figma node 124:1192) is open — raised from the active banner's action — or
 *   null when no overlay is showing.
 * @param selectedPossibility the [EarthquakePossibility] overlay (Figma 124:1605)
 *   raised from the resting banner's action, or null when it is closed.
 */
@Immutable
data class WarningUiState(
    val isHealthy: Boolean = true,
    val banner: WarningBanner = PossibilityBanner(
        title = "No Recent Earthquake",
        possibilityLabel = "Possibility : High Risk"
    ),
    val sectionTitle: String = "Stay prepared for an earthquake",
    val tips: List<PreparednessTip> = noActiveQuakeTips(),
    val unitSystem: UnitSystem = UnitSystem.METRIC,
    val selectedEventDetails: QuakeHistoryItem? = null,
    val selectedPossibility: EarthquakePossibility? = null
)

/**
 * Tips for the active-alert state (Figma 124:1297): post-quake guidance for
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
 * Tips for the resting state (Figma 124:1426): pre-quake preparedness guidance.
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