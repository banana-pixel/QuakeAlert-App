package id.web.quakealert.ui.warning

import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.material3.minimumInteractiveComponentSize
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.ColorFilter
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import id.web.quakealert.R
import id.web.quakealert.data.UnitSystem
import id.web.quakealert.ui.common.MapFocus
import id.web.quakealert.ui.common.QuakeMap
import id.web.quakealert.ui.common.QuakeModalHeader
import id.web.quakealert.ui.theme.AlertActionBorder
import id.web.quakealert.ui.theme.AlertBannerGradient
import id.web.quakealert.ui.theme.BannerMeta
import id.web.quakealert.ui.theme.BannerTitle
import id.web.quakealert.ui.theme.BannerValue
import id.web.quakealert.ui.theme.BorderLight
import id.web.quakealert.ui.theme.CardBorder
import id.web.quakealert.ui.theme.CardSubtitle
import id.web.quakealert.ui.theme.ChipLabel
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.EmergencyCtaBorder
import id.web.quakealert.ui.theme.EmergencyCtaFill
import id.web.quakealert.ui.theme.EventDetailDividerColor
import id.web.quakealert.ui.theme.EventDetailLocation
import id.web.quakealert.ui.theme.EventDetailMeta
import id.web.quakealert.ui.theme.MetricLabel
import id.web.quakealert.ui.theme.MmiOrange
import id.web.quakealert.ui.theme.MetricPanelFill
import id.web.quakealert.ui.theme.MetricValue
import id.web.quakealert.ui.theme.NunitoFontFamily
import id.web.quakealert.ui.theme.OfflineNoticeBorder
import id.web.quakealert.ui.theme.OfflineNoticeFill
import id.web.quakealert.ui.theme.PossibilityBannerGradient
import id.web.quakealert.ui.theme.PossibilityDisclaimer
import id.web.quakealert.ui.theme.PossibilityModalGradient
import id.web.quakealert.ui.theme.PrepIconBorder
import id.web.quakealert.ui.theme.QuakeAlertTheme
import id.web.quakealert.ui.theme.TextPrimary
import id.web.quakealert.ui.theme.TextSecondary
import id.web.quakealert.ui.theme.WarningDividerColor

/**
 * Warning banner (Figma nodes 124:1297 / 124:1426): a fixed-height stadium card
 * whose gradient, glyph and text block all follow the [WarningBanner] variant —
 * an active alert is crimson with a waveform glyph and an intensity read, the
 * resting state is amber with a globe glyph and a possibility read. The variant
 * is identity; rendering is this component's job, mirroring how the event detail
 * modal keys its gradient off severity.
 *
 * The title line carries a small info affordance, which is where "will this app
 * actually warn me?" gets answered: the rules live one tap from the screen that does
 * the warning rather than in a settings list the user has to go looking through.
 *
 * @param banner alert summary content for the current state.
 * @param onSeeDetails invoked when the "SEE DETAILS" capsule is tapped.
 * @param onProtectionStatus invoked by the info affordance beside the title.
 */
@Composable
fun AlertBanner(
    banner: WarningBanner,
    onSeeDetails: () -> Unit,
    onProtectionStatus: () -> Unit,
    modifier: Modifier = Modifier
) {
    val shape = RoundedCornerShape(Dimens.AlertBannerRadius)
    val (gradient, glyph) = when (banner) {
        is ActiveQuakeBanner -> AlertBannerGradient to R.drawable.ic_recording_02
        is SeismicActivityBanner -> PossibilityBannerGradient to R.drawable.ic_globe_04
    }

    Row(
        modifier = modifier
            .fillMaxWidth()
            .heightIn(min = Dimens.AlertBannerHeight)
            .clip(shape)
            .background(gradient, shape)
            .border(Dimens.BorderThin, CardBorder, shape)
            .padding(Dimens.AlertBannerPadding),
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalAlignment = Alignment.CenterVertically
    ) {
        Column(
            modifier = Modifier.weight(1f),
            verticalArrangement = Arrangement.spacedBy(Dimens.AlertBannerTitleGap)
        ) {
            Row(
                horizontalArrangement = Arrangement.spacedBy(Dimens.AlertBannerInfoGap),
                verticalAlignment = Alignment.CenterVertically
            ) {
                Text(
                    text = banner.title,
                    style = BannerTitle,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.weight(1f, fill = false)
                )
                // minimumInteractiveComponentSize rather than a padded box: the glyph
                // is drawn at its design size and only its touch target grows to the
                // 48dp minimum, so the banner's text block keeps its rhythm.
                Icon(
                    painter = painterResource(id = R.drawable.ic_info_circle),
                    contentDescription = "How alerts work",
                    tint = TextPrimary,
                    modifier = Modifier
                        .minimumInteractiveComponentSize()
                        .size(Dimens.AlertBannerInfoIconSize)
                        .clickable(role = Role.Button, onClick = onProtectionStatus)
                )
            }
            when (banner) {
                is ActiveQuakeBanner -> {
                    Text(
                        text = banner.timeAgo,
                        style = BannerMeta,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis
                    )
                    Text(
                        text = banner.intensityLabel,
                        style = BannerValue,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis
                    )
                }
                // Two lines, unlike its sibling: this variant's line is a sentence
                // ("Sync your location to see nearby activity"), not a label/value
                // pair, and clipping it to one line truncated exactly the half that
                // said what to do. The banner's height is a minimum, so it grows.
                is SeismicActivityBanner -> Text(
                    text = banner.activityLabel,
                    style = BannerValue,
                    maxLines = 2,
                    overflow = TextOverflow.Ellipsis
                )
            }

            // "SEE DETAILS" capsule (Figma fill_a7745cf9): translucent stroke only.
            // minimumInteractiveComponentSize lifts the ~32dp capsule's touch box to
            // the 48dp minimum without touching its drawn geometry, and the tap
            // carries the standard ripple (previously suppressed).
            Box(
                modifier = Modifier
                    .padding(top = 6.dp)
                    .minimumInteractiveComponentSize()
                    .clip(RoundedCornerShape(Dimens.AlertActionRadius))
                    .border(
                        Dimens.BorderMedium,
                        AlertActionBorder,
                        RoundedCornerShape(Dimens.AlertActionRadius)
                    )
                    .clickable(role = Role.Button, onClick = onSeeDetails)
                    .padding(
                        horizontal = Dimens.AlertActionPaddingHorizontal,
                        vertical = Dimens.AlertActionPaddingVertical
                    )
            ) {
                Text(text = "SEE DETAILS", style = ChipLabel)
            }
        }

        Image(
            painter = painterResource(id = glyph),
            contentDescription = null,
            colorFilter = ColorFilter.tint(TextPrimary),
            modifier = Modifier.size(Dimens.AlertWaveIconSize)
        )
    }
}

/**
 * Offline / failed-load notice, pinned above the alert banner.
 *
 * It sits at the *top* and takes only the height of its own copy because of what is
 * beneath it: the preparedness guidance and the Emergency call-to-action, both of
 * which work with no network at all. A disaster is exactly when the cell network
 * fails, and replacing that guidance with a full-body error card would take the
 * screen's only offline-capable content away at the moment it matters most. So the
 * bad news is reported here, in one strip, and the rest of the screen stays where
 * the user left it.
 *
 * No design node: the [id.web.quakealert.ui.common.QuakeErrorState] card it replaces
 * in this position owns the "something failed" language, so this borrows that card's
 * `alert-triangle` glyph and its "Retry" affordance rather than inventing a third.
 *
 * @param message what failed, in one line — a ViewModel `errorMessage`, or the
 *   screen's offline copy.
 * @param onRetry re-runs the load. Kept even while the device is offline: the user
 *   knows better than we do when their signal is back.
 */
@Composable
fun WarningOfflineNotice(
    message: String,
    onRetry: () -> Unit,
    modifier: Modifier = Modifier
) {
    val shape = RoundedCornerShape(Dimens.RadiusSmall)

    Row(
        modifier = modifier
            .fillMaxWidth()
            .clip(shape)
            .background(OfflineNoticeFill, shape)
            .border(Dimens.BorderThin, OfflineNoticeBorder, shape)
            .padding(Dimens.OfflineNoticePadding),
        horizontalArrangement = Arrangement.spacedBy(Dimens.OfflineNoticeGap),
        verticalAlignment = Alignment.CenterVertically
    ) {
        Image(
            painter = painterResource(id = R.drawable.ic_alert_triangle),
            contentDescription = null,
            colorFilter = ColorFilter.tint(MmiOrange),
            modifier = Modifier.size(Dimens.OfflineNoticeGlyphSize)
        )

        Text(
            text = message,
            style = CardSubtitle.copy(color = TextPrimary),
            modifier = Modifier.weight(1f)
        )

        Box(
            modifier = Modifier
                .minimumInteractiveComponentSize()
                .clip(RoundedCornerShape(Dimens.AlertActionRadius))
                .border(
                    Dimens.BorderMedium,
                    AlertActionBorder,
                    RoundedCornerShape(Dimens.AlertActionRadius)
                )
                .clickable(role = Role.Button, onClick = onRetry)
                .padding(
                    horizontal = Dimens.AlertActionPaddingHorizontal,
                    vertical = Dimens.AlertActionPaddingVertical
                ),
            contentAlignment = Alignment.Center
        ) {
            Text(text = "RETRY", style = ChipLabel)
        }
    }
}

/**
 * Short centered drag-handle divider between the banner and the preparedness
 * tips (Figma node 1:1037): a 100dp rounded bar in white 40%.
 */
@Composable
fun WarningDivider(modifier: Modifier = Modifier) {
    Box(
        modifier = modifier
            .fillMaxWidth()
            .padding(vertical = Dimens.WarningDividerPaddingVertical),
        contentAlignment = Alignment.Center
    ) {
        Box(
            modifier = Modifier
                .size(width = Dimens.WarningDividerWidth, height = Dimens.WarningDividerThickness)
                .clip(RoundedCornerShape(Dimens.WarningDividerThickness))
                .background(WarningDividerColor)
        )
    }
}

/**
 * A single preparedness tip row (Figma node 1:1038): a circular white-outline
 * glyph on the left and a bold title + dimmed description in the text column.
 */
@Composable
fun PrepTipRow(
    tip: PreparednessTip,
    modifier: Modifier = Modifier
) {
    Row(
        modifier = modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.spacedBy(Dimens.PrepTipContentGap),
        verticalAlignment = Alignment.CenterVertically
    ) {
        Box(
            modifier = Modifier
                .size(Dimens.PrepIconCircleSize)
                .clip(RoundedCornerShape(Dimens.RadiusStadium))
                .border(Dimens.BorderThin, PrepIconBorder, RoundedCornerShape(Dimens.RadiusStadium)),
            contentAlignment = Alignment.Center
        ) {
            Image(
                painter = painterResource(id = tip.icon),
                contentDescription = null,
                modifier = Modifier.size(Dimens.PrepIconGlyphSize)
            )
        }

        Column(
            modifier = Modifier.weight(1f),
            verticalArrangement = Arrangement.spacedBy(Dimens.PrepTipTextGap)
        ) {
            Text(
                text = tip.title,
                color = TextPrimary,
                fontFamily = NunitoFontFamily,
                fontWeight = FontWeight.Bold,
                fontSize = 16.sp,
                lineHeight = 20.sp
            )
            Text(text = tip.description, style = CardSubtitle)
        }
    }
}

/**
 * Bottom emergency call-to-action (Figma nodes 124:1312 / 124:1441): a fixed-height
 * stadium button with a translucent wine fill and a white 30% stroke. Label
 * follows the current design's "SHELTER & EMERGENCY INFO"; the action is a stub
 * until the emergency-resource flow is defined.
 *
 * Full-width, so its touch area already clears the 48dp minimum on the horizontal
 * axis; [minimumInteractiveComponentSize] lifts the 34dp height to 48dp too while
 * the drawn capsule keeps its token. This is the app's most safety-critical tap
 * target, so it gets the same treatment as the small icon buttons.
 */
@Composable
fun EmergencyCta(
    onClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    val shape = RoundedCornerShape(Dimens.EmergencyCtaRadius)

    Box(
        modifier = modifier
            .fillMaxWidth()
            .minimumInteractiveComponentSize()
            .height(Dimens.EmergencyCtaHeight)
            .clip(shape)
            .background(EmergencyCtaFill, shape)
            .border(Dimens.BorderMedium, EmergencyCtaBorder, shape)
            .clickable(role = Role.Button, onClick = onClick),
        contentAlignment = Alignment.Center
    ) {
        Text(
            text = "SHELTER & EMERGENCY INFO",
            style = ChipLabel
        )
    }
}

/**
 * The "Recent Seismic Activity" overlay hosted in its own [Dialog] window (the
 * design's Figma 124:1605 frame), opened from the resting banner's "SEE DETAILS"
 * action. Same chrome as [id.web.quakealert.ui.common.QuakeEventDetailModalDialog] —
 * platform width disabled so the card spans the screens' content column, and every
 * dismissal path (close button, back press, outside tap) routes to [onDismiss].
 */
@Composable
fun RecentSeismicActivityModal(
    activity: RecentSeismicActivity,
    unitSystem: UnitSystem,
    onDismiss: () -> Unit,
    modifier: Modifier = Modifier
) {
    Dialog(
        onDismissRequest = onDismiss,
        properties = DialogProperties(usePlatformDefaultWidth = false)
    ) {
        RecentSeismicActivityCard(
            activity = activity,
            unitSystem = unitSystem,
            onDismiss = onDismiss,
            modifier = modifier.padding(Dimens.ScreenHorizontalPadding)
        )
    }
}

/**
 * Stateless "Recent Seismic Activity" card (the design's Figma node 124:1605): a
 * near-flat dark surface (so the overlay reads as a lifted panel, not an alert)
 * carrying five sections [Dimens.EventDetailSectionGap] apart:
 *  1. Header — centered "Recent Seismic Activity" with the shared circular close.
 *  2. The point the counts were measured from, and the query in one line.
 *  3. Basemap ([QuakeMap]) centred on the device's own position, since the read is
 *     about activity *where the user is*. With no position ever synced there is
 *     nothing honest to centre on, so the card keeps [QuakeMap]'s dark ground and
 *     section 2's copy carries the news.
 *  4. Stats panel — three recorded facts: how many events, the newest, the hardest.
 *  5. Accuracy disclaimer in light italic.
 *
 * Every row prints a fact or says plainly that there is none — an em dash and a "None
 * recorded" instead of a plausible-looking placeholder. This card is the one place the
 * app volunteers numbers nobody asked for, so a wrong one here would be believed.
 *
 * The card scrolls internally so every section stays reachable on short
 * viewports. Exposed separately from [RecentSeismicActivityModal] so it can be
 * previewed without a dialog window.
 *
 * @param unitSystem renders the query radius in the unit chosen in Settings.
 */
@Composable
fun RecentSeismicActivityCard(
    activity: RecentSeismicActivity,
    unitSystem: UnitSystem,
    onDismiss: () -> Unit,
    modifier: Modifier = Modifier
) {
    val shape = remember { RoundedCornerShape(Dimens.RadiusCard) }
    val statsShape = remember { RoundedCornerShape(Dimens.RadiusSmall) }
    val focus = remember(activity.latitude, activity.longitude) {
        val latitude = activity.latitude
        val longitude = activity.longitude
        if (latitude != null && longitude != null) {
            MapFocus(latitude = latitude, longitude = longitude, zoom = MapFocus.ZOOM_REGION)
        } else {
            null
        }
    }
    val radiusLabel = unitSystem.formatDistance(activity.radiusKm)

    Column(
        modifier = modifier
            .fillMaxWidth()
            .clip(shape)
            .background(PossibilityModalGradient, shape)
            .border(Dimens.BorderThin, CardBorder, shape)
            .verticalScroll(rememberScrollState())
            .padding(Dimens.ModalPadding),
        verticalArrangement = Arrangement.spacedBy(Dimens.EventDetailSectionGap)
    ) {
        QuakeModalHeader(onDismiss = onDismiss, title = "Recent Seismic Activity")

        Column(
            modifier = Modifier.fillMaxWidth(),
            verticalArrangement = Arrangement.spacedBy(Dimens.EventDetailMmiColumnGap)
        ) {
            Text(
                text = activity.locationLabel,
                style = EventDetailLocation,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis
            )
            // The query, stated rather than implied. Every number below is scoped by
            // it, and a count without its radius and window is not a fact.
            Text(
                text = "Within $radiusLabel, past ${activity.windowDays} days",
                style = EventDetailMeta,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis
            )
        }

        QuakeMap(
            focus = focus,
            // Top-start: this card draws no overlays of its own, and the top edge
            // sits furthest from the stats panel below.
            attributionAlignment = Alignment.TopStart,
            // Clipped, not outlined, for the same reason as the event detail map:
            // the tiles are the card's edge.
            modifier = Modifier
                .fillMaxWidth()
                .height(Dimens.EventDetailMapHeight)
                .clip(shape)
        )

        Column(
            modifier = Modifier
                .fillMaxWidth()
                .clip(statsShape)
                .background(MetricPanelFill, statsShape)
                .border(Dimens.BorderMedium, BorderLight, statsShape)
                .padding(Dimens.EventDetailInfoPadding),
            verticalArrangement = Arrangement.spacedBy(Dimens.EventDetailInfoGap)
        ) {
            ActivityStatRow(
                label = "Confirmed Events",
                value = activity.countValue
            )

            ActivityStatDivider()

            ActivityStatRow(
                label = "Most Recent",
                value = activity.mostRecentValue
            )

            ActivityStatDivider()

            // "The last one" and "the worst one" are different questions, and in a
            // month of records they are usually different events.
            ActivityStatRow(
                label = "Strongest Shaking",
                value = activity.strongestValue
            )
        }

        Text(
            text = ACTIVITY_DISCLAIMER,
            style = PossibilityDisclaimer,
            modifier = Modifier.fillMaxWidth()
        )
    }
}

/**
 * Accuracy note under the stats. Reworded from the design's copy to drop its
 * "possibility" framing and to name the real limit: these are counts from a community
 * network whose density varies, so an area with two stations under-reports compared to
 * one with twenty. It is not, and must not read as, a forecast.
 */
private const val ACTIVITY_DISCLAIMER =
    "Counts come from QuakeAlert's own stations and depend on how many are near you. " +
        "They describe shaking already recorded. They are not a forecast of what comes next."

/** Hairline between two [ActivityStatRow]s. */
@Composable
private fun ActivityStatDivider(modifier: Modifier = Modifier) {
    Box(
        modifier = modifier
            .fillMaxWidth()
            .height(Dimens.BorderThin)
            .background(EventDetailDividerColor)
    )
}

/**
 * One stat row of the activity card (Figma node 124:1652): a fixed 44dp block
 * that Figma splits into two 22dp halves. [Arrangement.SpaceEvenly] reproduces
 * that split from the two line boxes — the same treatment the Earthquake Details
 * overlay's spatial rows use.
 */
@Composable
private fun ActivityStatRow(
    label: String,
    value: String,
    modifier: Modifier = Modifier
) {
    Column(
        modifier = modifier
            .fillMaxWidth()
            .height(Dimens.EventDetailInfoRowHeight),
        verticalArrangement = Arrangement.SpaceEvenly
    ) {
        Text(
            text = label,
            style = MetricLabel,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis
        )
        Text(
            text = value,
            style = MetricValue,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis
        )
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun SeismicActivityBannerPreview() {
    QuakeAlertTheme {
        AlertBanner(
            banner = SeismicActivityBanner(
                title = "No Recent Earthquake",
                activityLabel = previewActivity.bannerLabel
            ),
            onSeeDetails = {},
            onProtectionStatus = {},
            modifier = Modifier.padding(16.dp)
        )
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun ActiveQuakeBannerPreview() {
    QuakeAlertTheme {
        AlertBanner(
            banner = ActiveQuakeBanner(
                title = "Recent Earthquake Alert",
                timeAgo = "20 minutes ago",
                intensityLabel = "Intensity : IV (moderate)"
            ),
            onSeeDetails = {},
            onProtectionStatus = {},
            modifier = Modifier.padding(16.dp)
        )
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun PrepTipRowPreview() {
    QuakeAlertTheme {
        PrepTipRow(
            tip = PreparednessTip(
                id = "kit",
                icon = R.drawable.ic_prep_kit,
                title = "Build a 72-Hour Kit",
                description = "Pack water, non-perishable food, flashlights, extra batteries, and a first-aid kit in an easy-to-reach bag."
            ),
            modifier = Modifier.padding(16.dp)
        )
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun RecentSeismicActivityCardPreview() {
    QuakeAlertTheme {
        RecentSeismicActivityCard(
            activity = previewActivity,
            unitSystem = UnitSystem.METRIC,
            onDismiss = {},
            modifier = Modifier.padding(Dimens.ScreenHorizontalPadding)
        )
    }
}

/** A populated month, so the previews show the card's fullest state. */
private val previewActivity = RecentSeismicActivity(
    locationLabel = "-6.91750, 107.61910",
    availability = ActivityAvailability.MEASURED,
    eventCount = 3,
    mostRecent = "IV (moderate), 2 days ago",
    strongest = "V (strong), 61.5 gal",
    latitude = -6.91750,
    longitude = 107.61910
)

/** The quiet month — the state the card must state plainly rather than pad. */
@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun RecentSeismicActivityCardEmptyPreview() {
    QuakeAlertTheme {
        RecentSeismicActivityCard(
            activity = RecentSeismicActivity(
                locationLabel = "-6.91750, 107.61910",
                availability = ActivityAvailability.MEASURED
            ),
            unitSystem = UnitSystem.METRIC,
            onDismiss = {},
            modifier = Modifier.padding(Dimens.ScreenHorizontalPadding)
        )
    }
}