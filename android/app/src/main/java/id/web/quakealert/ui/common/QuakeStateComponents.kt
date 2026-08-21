package id.web.quakealert.ui.common

import androidx.annotation.DrawableRes
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.tooling.preview.Preview
import id.web.quakealert.R
import id.web.quakealert.ui.theme.BorderLight
import id.web.quakealert.ui.theme.CardBorder
import id.web.quakealert.ui.theme.CardSubtitle
import id.web.quakealert.ui.theme.ChipLabel
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.MmiRed
import id.web.quakealert.ui.theme.QuakeAlertTheme
import id.web.quakealert.ui.theme.SectionTitle
import id.web.quakealert.ui.theme.StateCardActionBorder
import id.web.quakealert.ui.theme.StateCardActionFill
import id.web.quakealert.ui.theme.StateCardFill
import id.web.quakealert.ui.theme.StateCardMessageGlow
import id.web.quakealert.ui.theme.StateMessage
import id.web.quakealert.ui.theme.TextPrimary

/**
 * Generic, dark-theme state placeholders shared by every list-bearing screen
 * (History, Sensors, Warning). Each is a self-contained centred block sized to
 * fill whatever region the caller hands it — screens swap their scrolling body
 * for one of these while keeping their header and filter row in place, so the
 * chrome never flickers as the state changes.
 *
 * Every non-loading state is the same card (Figma node 148:1066) with different
 * content, so a network failure, an empty filter result and an out-of-coverage
 * area all read as the app speaking rather than as three degrees of brokenness.
 * What differs is only the honest part: an error means the question could not be
 * asked, so it offers "Retry"; an empty result is a valid answer, so it offers a
 * way to widen the question — and offers nothing at all when no filter is
 * narrowing it, since there would be nothing to widen.
 *
 * [QuakeLoadingState] stays a bare centred spinner: a card would imply the screen
 * has settled on an outcome when it has not.
 */

/**
 * Centred indeterminate spinner shown while a screen's data is in flight.
 *
 * @param message optional dimmed line beneath the spinner (e.g. "Loading
 *   earthquake history..."). Omitted by default so the spinner can stand alone in
 *   tight regions.
 */
@Composable
fun QuakeLoadingState(
    modifier: Modifier = Modifier,
    message: String? = null
) {
    Column(
        modifier = modifier
            .fillMaxSize()
            .padding(horizontal = Dimens.StateBlockPadding),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(
            space = Dimens.StateContentGap,
            alignment = Alignment.CenterVertically
        )
    ) {
        CircularProgressIndicator(
            modifier = Modifier.size(Dimens.StateSpinnerSize),
            color = TextPrimary,
            trackColor = BorderLight,
            strokeWidth = Dimens.StateSpinnerStroke
        )

        if (message != null) {
            Text(
                text = message,
                style = CardSubtitle,
                textAlign = TextAlign.Center,
                modifier = Modifier.fillMaxWidth()
            )
        }
    }
}

/**
 * Zero-data placeholder: the [StateBlock] card carrying a glyph, a headline and an
 * explanation, with an optional action.
 *
 * An empty result is a valid answer rather than a failure, so it never offers
 * "Retry" — but it may offer a way to widen the question, which is what
 * [QuakeNoDataState] and [QuakeNoCoverageState] do.
 *
 * @param icon 50dp glyph; pass the drawable of the screen's own navigation tab so
 *   the card reads as belonging to it.
 * @param message bold headline naming what is missing.
 * @param subtitle line explaining why, or what to do next.
 * @param actionLabel label of the trailing capsule; omit (with [onAction]) for a
 *   purely informational card.
 */
@Composable
fun QuakeEmptyState(
    @DrawableRes icon: Int,
    message: String,
    modifier: Modifier = Modifier,
    subtitle: String? = null,
    actionLabel: String? = null,
    onAction: (() -> Unit)? = null
) {
    StateBlock(
        icon = icon,
        iconTint = TextPrimary,
        message = message,
        subtitle = subtitle,
        modifier = modifier,
        action = if (actionLabel != null && onAction != null) {
            { StateAction(label = actionLabel, onClick = onAction) }
        } else {
            null
        }
    )
}

/**
 * Failure placeholder: the alert triangle over the failure copy with a "Retry"
 * action. The glyph is tinted the severe MMI red so a failure is distinguishable
 * from an empty result before either line is read.
 *
 * @param message the failure copy (a ViewModel's `errorMessage`, or a generic
 *   fallback the caller supplies).
 * @param onRetry invoked by "Retry"; wire this to the owning ViewModel's retry
 *   hook so the state machine re-enters its loading branch.
 */
@Composable
fun QuakeErrorState(
    message: String,
    onRetry: () -> Unit,
    modifier: Modifier = Modifier
) {
    StateBlock(
        icon = R.drawable.ic_alert_triangle,
        iconTint = MmiRed,
        message = "Something Went Wrong",
        subtitle = message,
        modifier = modifier
    ) {
        StateAction(label = "Retry", onClick = onRetry)
    }
}

/**
 * "No Data Available" — the query succeeded and returned nothing *because of the
 * active filter*. The copy names that filter, because a bare "no results" invites
 * the wrong conclusion ("there were no earthquakes") when the truth is "not the
 * ones you asked for".
 *
 * @param filterSummary the active criteria in prose, from
 *   [QuakeFilterState.summary] — e.g. "at MMI VI+ within 250 km in the past 7
 *   days". Pass null when nothing is narrowing the query; the card then states the
 *   plain emptiness and offers no action, since resetting would change nothing.
 * @param onResetFilters clears the filter back to the unfiltered feed.
 */
@Composable
fun QuakeNoDataState(
    filterSummary: String?,
    onResetFilters: () -> Unit,
    modifier: Modifier = Modifier
) {
    QuakeEmptyState(
        icon = R.drawable.ic_nav_history,
        message = if (filterSummary == null) "No Earthquake History" else "No Data Available",
        modifier = modifier,
        subtitle = if (filterSummary == null) {
            "Events detected by the sensor network will appear here."
        } else {
            "No events $filterSummary. Try a wider filter."
        },
        actionLabel = "Reset Filters".takeIf { filterSummary != null },
        onAction = onResetFilters.takeIf { filterSummary != null }
    )
}

/**
 * "No Sensors In This Area" — the area asked about lies outside the network's
 * coverage. Distinct from [QuakeNoDataState] on purpose: the browse radius reaches
 * 1000 km while the network is regional, so a perfectly valid query will return
 * nothing, and the honest reading of that is a limit of our coverage, not an
 * absence of earthquakes.
 *
 * @param onWidenRadius widens the browse radius one step; pass null when the query
 *   is not narrowed by a radius at all (nothing to widen — the area simply has no
 *   stations), and the card states that without offering an action.
 */
@Composable
fun QuakeNoCoverageState(
    onWidenRadius: (() -> Unit)?,
    modifier: Modifier = Modifier
) {
    QuakeEmptyState(
        icon = R.drawable.ic_nav_sensors,
        message = "No Sensors In This Area",
        modifier = modifier,
        subtitle = "QuakeAlert's sensor network does not cover this area yet.",
        actionLabel = "Widen Search Radius".takeIf { onWidenRadius != null },
        onAction = onWidenRadius
    )
}

/**
 * Shared card behind every non-loading state (Figma node 148:1066): a 50dp glyph,
 * a bold title, an explanation on a soft glow, and an optional full-width action,
 * spaced apart inside a black card with a white-10% hairline.
 *
 * Private so the public states above stay the only entry points: the chrome is
 * defined once here, which is what keeps a network failure and an empty result
 * reading as the same visual language instead of drifting into two designs.
 *
 * The card is sized 346x322 in the design; the width is a cap and the height a
 * floor, so the longest copy grows the card rather than clipping.
 */
@Composable
private fun StateBlock(
    @DrawableRes icon: Int,
    iconTint: Color,
    message: String,
    subtitle: String?,
    modifier: Modifier = Modifier,
    action: (@Composable () -> Unit)? = null
) {
    val cardShape = RoundedCornerShape(Dimens.RadiusCard)

    Box(
        modifier = modifier
            .fillMaxSize()
            .padding(horizontal = Dimens.StateBlockPadding),
        contentAlignment = Alignment.Center
    ) {
        Column(
            modifier = Modifier
                .widthIn(max = Dimens.StateCardMaxWidth)
                .fillMaxWidth()
                .heightIn(min = Dimens.StateCardMinHeight)
                .clip(cardShape)
                .background(StateCardFill, cardShape)
                .border(Dimens.BorderThin, CardBorder, cardShape)
                .padding(Dimens.StateCardPadding),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.SpaceBetween
        ) {
            Icon(
                painter = painterResource(id = icon),
                contentDescription = null,
                tint = iconTint,
                modifier = Modifier.size(Dimens.StateCardGlyphSize)
            )

            Text(
                text = message,
                style = SectionTitle,
                textAlign = TextAlign.Center,
                modifier = Modifier.widthIn(max = Dimens.StateCardTitleWidth)
            )

            if (subtitle != null) {
                // The design puts a 30px white glow behind this frame. Compose cannot
                // blur a shadow behind transparent content, so the glow is painted as
                // a radial wash that fades out well before the card's edges.
                Box(
                    modifier = Modifier
                        .fillMaxWidth()
                        .background(
                            Brush.radialGradient(
                                colors = listOf(StateCardMessageGlow, Color.Transparent)
                            )
                        )
                        .padding(vertical = Dimens.StateTextGap),
                    contentAlignment = Alignment.Center
                ) {
                    Text(
                        text = subtitle,
                        style = StateMessage,
                        textAlign = TextAlign.Center,
                        modifier = Modifier.fillMaxWidth()
                    )
                }
            }

            if (action != null) {
                action()
            }
        }
    }
}

/**
 * Full-width action capsule of the state card: white-31% fill behind a 2dp
 * white-30% stroke, matching Figma 148:1076. Shared by every variant so "Retry",
 * "Reset Filters" and "Widen Search Radius" are visibly the same control doing
 * different work. Standard ripple feedback is left in place.
 */
@Composable
private fun StateAction(
    label: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    val shape = RoundedCornerShape(Dimens.RadiusSmall)

    Box(
        modifier = modifier
            .fillMaxWidth()
            .height(Dimens.ModalActionHeight)
            .clip(shape)
            .background(StateCardActionFill, shape)
            .border(Dimens.BorderMedium, StateCardActionBorder, shape)
            .clickable(role = Role.Button, onClick = onClick),
        contentAlignment = Alignment.Center
    ) {
        Text(text = label, style = ChipLabel)
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun QuakeLoadingStatePreview() {
    QuakeAlertTheme {
        QuakeLoadingState(message = "Loading earthquake history...")
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun QuakeNoDataStatePreview() {
    QuakeAlertTheme {
        QuakeNoDataState(
            filterSummary = "at MMI VI+ within 250 km in the past 7 days",
            onResetFilters = {}
        )
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun QuakeEmptyStatePreview() {
    QuakeAlertTheme {
        QuakeNoDataState(filterSummary = null, onResetFilters = {})
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun QuakeNoCoverageStatePreview() {
    QuakeAlertTheme {
        QuakeNoCoverageState(onWidenRadius = {})
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun QuakeErrorStatePreview() {
    QuakeAlertTheme {
        QuakeErrorState(
            message = "Could not reach the QuakeAlert network. Check your connection and try again.",
            onRetry = {}
        )
    }
}
