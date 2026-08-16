package id.web.quakealert.ui.main

import androidx.annotation.DrawableRes
import androidx.compose.animation.AnimatedContent
import androidx.compose.animation.core.FastOutLinearInEasing
import androidx.compose.animation.core.LinearOutSlowInEasing
import androidx.compose.animation.core.tween
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.scaleIn
import androidx.compose.animation.togetherWith
import androidx.compose.foundation.Image
import androidx.compose.foundation.background

import androidx.compose.foundation.border

import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row

import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Scaffold

import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.ColorFilter
import androidx.compose.ui.layout.ContentScale

import androidx.compose.ui.res.painterResource

import androidx.compose.ui.tooling.preview.Preview
import id.web.quakealert.R
import id.web.quakealert.ui.history.HistoryRoute
import id.web.quakealert.ui.theme.BackgroundGradientBottom
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.NavActiveFill
import id.web.quakealert.ui.theme.NavActiveText
import id.web.quakealert.ui.theme.NavBarBorder
import id.web.quakealert.ui.theme.NavBarFill
import id.web.quakealert.ui.theme.NavLabel
import id.web.quakealert.ui.theme.QuakeAlertTheme
import id.web.quakealert.ui.theme.TextPrimary

/**
 * The five primary destinations shown in the bottom navigation (Figma bottom
 * bar). Each carries its own icon drawable.
 */
enum class MainDestination(
    @param:DrawableRes val icon: Int,
    val label: String
) {
    HISTORY(R.drawable.ic_nav_history, "History"),
    SENSORS(R.drawable.ic_nav_sensors, "Sensors"),
    WARNING(R.drawable.ic_nav_warning, "Warning"),
    CHAT(R.drawable.ic_nav_chat, "Chat"),
    SETTINGS(R.drawable.ic_nav_settings, "Settings")
}

/**
 * Main app scaffold hosting the custom [QuakeBottomNavigation] and swapping the
 * body content for the currently selected [MainDestination]. Only History is
 * implemented for now; other tabs render a placeholder.
 *
 * The [Scaffold] disables its default window insets so the custom bars can own
 * their own inset handling: the History content applies its status-bar padding
 * internally, while [QuakeBottomNavigation] applies navigation-bar padding.
 */
@Composable
fun MainScreen(modifier: Modifier = Modifier) {
    var selected by remember { mutableStateOf(MainDestination.HISTORY) }

    Scaffold(
        modifier = modifier.fillMaxSize(),
        containerColor = BackgroundGradientBottom,
        contentWindowInsets = WindowInsets(0, 0, 0, 0),
        bottomBar = {
            QuakeBottomNavigation(
                selected = selected,
                onSelect = { selected = it }
            )
        }
    ) { innerPadding ->
        // Smooth, non-blocking destination transition (Rule D). Enter fades and
        // subtly scales in; exit fades out faster so the incoming screen leads.
        AnimatedContent(
            targetState = selected,
            transitionSpec = {
                (fadeIn(tween(200, easing = LinearOutSlowInEasing)) +
                    scaleIn(initialScale = 0.98f, animationSpec = tween(200)))
                    .togetherWith(fadeOut(tween(150, easing = FastOutLinearInEasing)))
            },
            label = "MainDestinationTransition"
        ) { destination ->
            Box(
                modifier = Modifier
                    .fillMaxSize()
                    .padding(innerPadding)
            ) {
                when (destination) {
                    MainDestination.HISTORY -> HistoryRoute()
                    else -> Unit // Placeholder until other flows are implemented.
                }
            }
        }
    }
}


/**
 * Custom flat, dark bottom navigation bar (Figma node 1:843). Each entry is a
 * 55×55 column pill containing a 24dp icon and a Nunito label. The active tab
 * uses a highlight fill with tinted icon/label, per the Figma design tokens.
 * Applies [navigationBarsPadding] so it clears the Android gesture bar.
 *
 * Only the *bottom* breathing-room padding is inside this composable's measured
 * box; the top margin is intentionally omitted so it is NOT folded into the
 * Scaffold's `innerPadding.bottom`. This keeps the content region's bottom edge
 * flush with the visible pill top, letting the History list own the gap via
 * [Dimens.CardListBottomPadding] as the single source of truth.
 */
@Composable
fun QuakeBottomNavigation(
    selected: MainDestination,
    onSelect: (MainDestination) -> Unit,
    modifier: Modifier = Modifier
) {
    Row(
        modifier = modifier
            .fillMaxWidth()
            .navigationBarsPadding()
            .padding(
                start = Dimens.ScreenHorizontalPadding,
                end = Dimens.ScreenHorizontalPadding,
                bottom = Dimens.NavBarPaddingVertical
            )


            .height(Dimens.NavBarHeight)
            .clip(RoundedCornerShape(Dimens.RadiusNavBar))
            .background(NavBarFill, RoundedCornerShape(Dimens.RadiusNavBar))
            .border(Dimens.BorderThin, NavBarBorder, RoundedCornerShape(Dimens.RadiusNavBar))
            .padding(horizontal = Dimens.NavBarPaddingHorizontal),
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalAlignment = Alignment.CenterVertically
    ) {
        MainDestination.entries.forEach { destination ->
            NavItem(
                destination = destination,
                selected = destination == selected,
                onClick = { onSelect(destination) }
            )
        }
    }
}

/**
 * A single navigation entry: a 55×55 column pill with a 24dp icon above its
 * label. The container fill, icon tint and label colour highlight when active.
 */
@Composable
private fun NavItem(
    destination: MainDestination,
    selected: Boolean,
    onClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    val containerColor = if (selected) NavActiveFill else Color.Transparent
    val contentColor = if (selected) NavActiveText else NavLabel
    val interactionSource = remember { MutableInteractionSource() }

    Column(
        modifier = modifier
            .size(Dimens.NavItemSize)
            .clip(RoundedCornerShape(Dimens.RadiusNavItem))
            .background(containerColor, RoundedCornerShape(Dimens.RadiusNavItem))
            .clickable(
                interactionSource = interactionSource,
                indication = null,
                onClick = onClick
            ),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(Dimens.NavItemGap, Alignment.CenterVertically)
    ) {
        Image(
            painter = painterResource(id = destination.icon),
            contentDescription = destination.label,
            contentScale = ContentScale.Fit,
            colorFilter = ColorFilter.tint(if (selected) NavActiveText else TextPrimary),
            modifier = Modifier.size(Dimens.NavIconSize)
        )

        Text(
            text = destination.label,
            style = androidx.compose.material3.MaterialTheme.typography.labelSmall,
            color = contentColor
        )
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun MainScreenPreview() {
    QuakeAlertTheme {
        MainScreen()
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun QuakeBottomNavigationPreview() {
    QuakeAlertTheme {
        QuakeBottomNavigation(
            selected = MainDestination.HISTORY,
            onSelect = {}
        )
    }
}
