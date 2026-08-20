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
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.tooling.preview.Preview
import id.web.quakealert.R
import id.web.quakealert.ui.theme.BorderLight
import id.web.quakealert.ui.theme.CardSubtitle
import id.web.quakealert.ui.theme.CardTitle
import id.web.quakealert.ui.theme.ChipLabel
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.EmergencyCtaBorder
import id.web.quakealert.ui.theme.EmergencyCtaFill
import id.web.quakealert.ui.theme.MmiRed
import id.web.quakealert.ui.theme.PrepIconBorder
import id.web.quakealert.ui.theme.QuakeAlertTheme
import id.web.quakealert.ui.theme.TextPrimary

/**
 * Generic, dark-theme state placeholders shared by every list-bearing screen
 * (History, Sensors, Warning). Each is a self-contained centred block sized to
 * fill whatever region the caller hands it — screens swap their scrolling body
 * for one of these while keeping their header and filter row in place, so the
 * chrome never flickers as the state changes.
 *
 * The three states deliberately share one visual skeleton — a circular glyph
 * above a bold message and a dimmed subtitle — so a user reading "No Earthquake
 * History" and a user reading a network error see the same shape carrying
 * different content. Only [QuakeErrorState] adds an action, and only
 * [QuakeLoadingState] replaces the glyph with a spinner.
 *
 * All typography reuses the shared [CardTitle] / [CardSubtitle] / [ChipLabel]
 * tokens rather than introducing state-only styles, so these placeholders track
 * the card typography they sit among.
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
 * Centred zero-data placeholder, e.g. "No Earthquake History" / "No Sensors
 * Found". Purely informational — an empty result is a valid outcome, not a
 * failure, so it carries no retry action.
 *
 * @param icon glyph rendered inside the outlined circle; pass the same drawable
 *   the screen's navigation tab uses so the placeholder reads as belonging to it.
 * @param message bold headline naming what is missing.
 * @param subtitle optional dimmed line explaining why, or what to do next.
 */
@Composable
fun QuakeEmptyState(
    @DrawableRes icon: Int,
    message: String,
    modifier: Modifier = Modifier,
    subtitle: String? = null
) {
    StateBlock(
        icon = icon,
        iconTint = TextPrimary,
        iconBorder = PrepIconBorder,
        message = message,
        subtitle = subtitle,
        modifier = modifier
    )
}

/**
 * Centred failure placeholder: an alert glyph over the failure copy, with a
 * "Retry" action beneath. The glyph circle is stroked in the severe MMI red so an
 * error is distinguishable from an empty result at a glance.
 *
 * @param message the failure copy (a ViewModel's `errorMessage`, or a generic
 *   fallback the caller supplies).
 * @param onRetry invoked by the "Retry" action; wire this to the owning
 *   ViewModel's retry hook so the state machine re-enters its loading branch.
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
        iconBorder = MmiRed,
        message = "Something went wrong",
        subtitle = message,
        modifier = modifier
    ) {
        RetryAction(onClick = onRetry)
    }
}

/**
 * Shared skeleton behind [QuakeEmptyState] and [QuakeErrorState]: an outlined
 * circular glyph, a bold message, an optional dimmed subtitle and an optional
 * trailing [action]. Private so the two public states stay the only entry points
 * and can never drift apart in spacing or alignment.
 */
@Composable
private fun StateBlock(
    @DrawableRes icon: Int,
    iconTint: Color,
    iconBorder: Color,
    message: String,
    subtitle: String?,
    modifier: Modifier = Modifier,
    action: (@Composable () -> Unit)? = null
) {
    val circleShape = RoundedCornerShape(Dimens.RadiusStadium)

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
        Box(
            modifier = Modifier
                .size(Dimens.StateIconCircleSize)
                .clip(circleShape)
                .border(Dimens.BorderThin, iconBorder, circleShape),
            contentAlignment = Alignment.Center
        ) {
            Icon(
                painter = painterResource(id = icon),
                contentDescription = null,
                tint = iconTint,
                modifier = Modifier.size(Dimens.StateIconGlyphSize)
            )
        }

        Column(
            modifier = Modifier.fillMaxWidth(),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(Dimens.StateTextGap)
        ) {
            Text(
                text = message,
                style = CardTitle,
                textAlign = TextAlign.Center,
                modifier = Modifier.fillMaxWidth()
            )
            if (subtitle != null) {
                Text(
                    text = subtitle,
                    style = CardSubtitle,
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

/**
 * "Retry" capsule for [QuakeErrorState]. Reuses the shared overlay-action chrome
 * ([Dimens.ModalActionHeight] + 2dp stroke) and the Emergency CTA's wine wash, so
 * it is recognisably the same button family as the app's other actions rather
 * than a one-off. Standard ripple feedback is left in place.
 */
@Composable
private fun RetryAction(
    onClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    val shape = RoundedCornerShape(Dimens.RadiusSmall)

    Box(
        modifier = modifier
            .height(Dimens.ModalActionHeight)
            .clip(shape)
            .background(EmergencyCtaFill, shape)
            .border(Dimens.BorderMedium, EmergencyCtaBorder, shape)
            .clickable(role = Role.Button, onClick = onClick)
            .padding(horizontal = Dimens.StateRetryPaddingHorizontal),
        contentAlignment = Alignment.Center
    ) {
        Text(text = "Retry", style = ChipLabel)
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
private fun QuakeEmptyStatePreview() {
    QuakeAlertTheme {
        QuakeEmptyState(
            icon = R.drawable.ic_nav_history,
            message = "No Earthquake History",
            subtitle = "Events detected near you will appear here."
        )
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
