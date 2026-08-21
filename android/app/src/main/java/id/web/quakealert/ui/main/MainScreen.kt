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
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row

import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.consumeWindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold

import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.Saver
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.ColorFilter
import androidx.compose.ui.layout.ContentScale

import androidx.compose.ui.res.painterResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.selected
import androidx.compose.ui.semantics.semantics

import androidx.compose.ui.tooling.preview.Preview
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import id.web.quakealert.R
import id.web.quakealert.ui.app.ServerHealthViewModel
import id.web.quakealert.ui.chat.ChatRoute
import id.web.quakealert.ui.history.HistoryRoute
import id.web.quakealert.ui.sensors.SensorsRoute
import id.web.quakealert.ui.settings.SettingsRoute
import id.web.quakealert.ui.warning.WarningRoute


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
 * Saver for the selected [MainDestination]. Persisting the enum's stable [Enum.name]
 * (rather than its ordinal, which would silently re-map if the tab order ever
 * changed) keeps the selection valid across configuration changes and process
 * death.
 */
private val MainDestinationSaver: Saver<MainDestination, String> = Saver(
    save = { it.name },
    restore = { name -> runCatching { MainDestination.valueOf(name) }.getOrNull() }
)

/**
 * Main app scaffold hosting the custom [QuakeBottomNavigation] and swapping the
 * body content for the currently selected [MainDestination].
 *
 * State survival (rotation + process death) is owned here rather than by the tabs:
 *  - the selected tab is held in [rememberSaveable] via [MainDestinationSaver], so
 *    a rotation or a process restart reopens the tab the user was on;
 *  - each destination's scroll position lives in a [rememberLazyListState] created
 *    *outside* [AnimatedContent] and passed down. `rememberLazyListState` is itself
 *    saveable, so those positions survive a rotation and process death — and,
 *    because they are not created inside the swapped-out content, they also survive
 *    switching away to another tab and back.
 *
 * Screen UI state (filters, open overlays, loaded data) is owned by each tab's
 * ViewModel, scoped to the host Activity's store, so it likewise outlives both tab
 * switches and configuration changes.
 *
 * The [Scaffold] disables its default window insets so the custom bars can own
 * their own inset handling: each screen applies its status-bar padding internally,
 * while [QuakeBottomNavigation] applies navigation-bar padding.
 *
 * Server health is hoisted here for the same reason as the scroll positions: it is
 * shared by all five tabs. One [ServerHealthViewModel] is collected once and the
 * resulting [id.web.quakealert.domain.ServerConnectionState] is handed to every
 * route, so each tab's status badge is literally the same value rather than five
 * per-screen derivations that can disagree.
 */
@Composable
fun MainScreen(
    modifier: Modifier = Modifier,
    serverHealthViewModel: ServerHealthViewModel = viewModel()
) {
    var selected by rememberSaveable(stateSaver = MainDestinationSaver) {
        mutableStateOf(MainDestination.HISTORY)
    }

    // Collected at this single point: observing it is also what keeps the shared
    // alert socket open, so the connection the badge reports is the same one an
    // earthquake alert would arrive on, on whichever tab the user is sitting.
    val connectionState by serverHealthViewModel.connectionState.collectAsStateWithLifecycle()

    // One hoisted scroll state per scrolling destination. Declared here so a tab
    // swap disposes the content but not its scroll position.
    val historyListState = rememberLazyListState()
    val sensorsListState = rememberLazyListState()
    val warningListState = rememberLazyListState()
    val chatListState = rememberLazyListState()
    val settingsListState = rememberLazyListState()

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
                    // Reserve the bottom-bar space AND mark those insets consumed
                    // so a descendant's imePadding() subtracts the already-applied
                    // bottom offset instead of double-counting it (Rule A).
                    .padding(innerPadding)
                    .consumeWindowInsets(innerPadding)
            ) {
                when (destination) {
                    MainDestination.HISTORY -> HistoryRoute(
                        connectionState = connectionState,
                        listState = historyListState
                    )

                    MainDestination.SENSORS -> SensorsRoute(
                        connectionState = connectionState,
                        listState = sensorsListState
                    )

                    MainDestination.WARNING -> WarningRoute(
                        connectionState = connectionState,
                        listState = warningListState
                    )

                    MainDestination.CHAT -> ChatRoute(
                        connectionState = connectionState,
                        listState = chatListState
                    )

                    MainDestination.SETTINGS -> SettingsRoute(
                        connectionState = connectionState,
                        listState = settingsListState
                    )
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
 *
 * The 55dp square already exceeds the 48dp minimum touch target, and the tap is a
 * plain [clickable] carrying the standard ripple (previously suppressed with
 * `indication = null`). [Role.Tab] plus [selected] let TalkBack announce the entry
 * as a tab and say which one is current, and the icon's `contentDescription` is
 * dropped because the label beneath it already names the destination — keeping it
 * would make TalkBack read every tab's name twice.
 */
@Composable
private fun NavItem(
    destination: MainDestination,
    selected: Boolean,
    onClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    // Captured under a distinct name: inside the semantics lambda a bare
    // `selected` would resolve to SemanticsPropertyReceiver.selected (whose getter
    // throws) rather than to this parameter.
    val isSelected = selected
    val containerColor = if (isSelected) NavActiveFill else Color.Transparent
    val contentColor = if (isSelected) NavActiveText else NavLabel

    Column(
        modifier = modifier
            .size(Dimens.NavItemSize)
            .clip(RoundedCornerShape(Dimens.RadiusNavItem))
            .background(containerColor, RoundedCornerShape(Dimens.RadiusNavItem))
            .clickable(role = Role.Tab, onClick = onClick)
            .semantics { this.selected = isSelected },
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(Dimens.NavItemGap, Alignment.CenterVertically)
    ) {
        Image(
            painter = painterResource(id = destination.icon),
            contentDescription = null,
            contentScale = ContentScale.Fit,
            colorFilter = ColorFilter.tint(if (isSelected) NavActiveText else TextPrimary),
            modifier = Modifier.size(Dimens.NavIconSize)
        )

        Text(
            text = destination.label,
            style = MaterialTheme.typography.labelSmall,
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
