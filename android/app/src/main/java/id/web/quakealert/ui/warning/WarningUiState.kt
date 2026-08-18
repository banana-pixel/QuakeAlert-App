package id.web.quakealert.ui.warning

import androidx.annotation.DrawableRes
import androidx.compose.runtime.Immutable
import id.web.quakealert.R

/**
 * The active alert banner shown at the top of the Warning screen (Figma node
 * 1:1035). Mirrors an incoming quake alert: intensity headline, when it was
 * detected and a short descriptive line.
 *
 * @param title alert headline (e.g. "Earthquake Detected").
 * @param intensityLabel Roman-numeral MMI + short qualifier (e.g. "MMI VII · Strong").
 * @param timeAgo relative detection time (e.g. "20 minutes ago").
 * @param description short guidance line under the headline.
 */
@Immutable
data class AlertBannerInfo(
    val title: String,
    val intensityLabel: String,
    val timeAgo: String,
    val description: String
)

/**
 * A single preparedness tip row (Figma node 1:1038): a circular white glyph on
 * the left with a bold title + dimmed description on the right.
 *
 * @param id stable identity for list keys.
 * @param icon circular tip glyph drawable.
 * @param title bold tip title (e.g. "Prepare an Emergency Kit").
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
 * Immutable UI state for the Warning screen (Figma node 1:1024). Hoisted into
 * [WarningViewModel] and consumed by the stateless [WarningScreen].
 *
 * @param isHealthy drives the shared [id.web.quakealert.ui.common.QuakeAppBar]
 *   network-status badge.
 * @param banner the active alert banner summary.
 * @param tips ordered preparedness tips to render.
 */
@Immutable
data class WarningUiState(
    val isHealthy: Boolean = true,
    val banner: AlertBannerInfo = AlertBannerInfo(
        title = "Earthquake Detected",
        intensityLabel = "MMI VII · Strong",
        timeAgo = "20 minutes ago",
        description = "Strong shaking expected in your area. Stay calm and take cover."
    ),
    val tips: List<PreparednessTip> = defaultTips()
)

/**
 * Static preparedness tips mirroring the Figma design (node 1:1038). Kept as a
 * top-level default so the state has meaningful content for both the ViewModel
 * seed and the @Preview.
 */
private fun defaultTips(): List<PreparednessTip> = listOf(
    PreparednessTip(
        id = "kit",
        icon = R.drawable.ic_prep_kit,
        title = "Prepare an Emergency Kit",
        description = "Water, food, flashlight, and a first-aid kit for at least 3 days."
    ),
    PreparednessTip(
        id = "home",
        icon = R.drawable.ic_prep_home,
        title = "Secure Your Home",
        description = "Anchor heavy furniture and know where your gas and water shutoffs are."
    ),
    PreparednessTip(
        id = "comms",
        icon = R.drawable.ic_prep_comms,
        title = "Plan Your Communication",
        description = "Agree on a meeting point and an out-of-area contact with your family."
    )
)
