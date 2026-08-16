package id.web.quakealert.ui.main

import androidx.annotation.DrawableRes
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Icon
import androidx.compose.material3.Scaffold
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
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
        Box(
            modifier = Modifier
                .fillMaxSize()
                .padding(innerPadding)
        ) {
            when (selected) {
                MainDestination.HISTORY -> HistoryRoute()
                else -> Unit // Placeholder until other flows are implemented.
            }
        }
    }
}

/**
 * Custom flat, dark bottom navigation bar. The active tab is rendered inside a
 * rounded pill container with a highlight fill and tinted icon, per the Figma
 * design tokens.
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
            .padding(
                horizontal = Dimens.ScreenHorizontalPadding,
                vertical = Dimens.NavBarPaddingVertical
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

/** A single navigation entry; highlighted with a pill container when active. */
@Composable
private fun NavItem(
    destination: MainDestination,
    selected: Boolean,
    onClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    val containerColor = if (selected) NavActiveFill else Color.Transparent
    val tint = if (selected) NavActiveText else TextPrimary
    val interactionSource = remember { MutableInteractionSource() }

    Box(
        modifier = modifier
            .size(Dimens.NavItemSize)
            .clip(RoundedCornerShape(Dimens.RadiusNavItem))
            .background(containerColor, RoundedCornerShape(Dimens.RadiusNavItem))
            .clickable(
                interactionSource = interactionSource,
                indication = null,
                onClick = onClick
            ),
        contentAlignment = Alignment.Center
    ) {
        Icon(
            painter = painterResource(id = destination.icon),
            contentDescription = destination.label,
            tint = tint,
            modifier = Modifier.size(Dimens.NavIconSize)
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
