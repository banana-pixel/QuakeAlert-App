package id.web.quakealert.ui.settings

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding

import androidx.compose.foundation.layout.wrapContentWidth
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListState
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.LocalUriHandler
import androidx.compose.ui.tooling.preview.Preview
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import id.web.quakealert.data.UnitSystem
import id.web.quakealert.ui.common.QuakeAppBar
import id.web.quakealert.ui.common.QuakeCard
import id.web.quakealert.ui.common.QuakePill
import id.web.quakealert.ui.common.QuakeSwitch
import id.web.quakealert.ui.common.fadingEdges
import id.web.quakealert.ui.sensors.SensorMapCard
import id.web.quakealert.ui.sensors.SensorMapOverview
import id.web.quakealert.ui.theme.CardBorder
import id.web.quakealert.ui.theme.CardTitle
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.QuakeAlertTheme
import id.web.quakealert.ui.theme.SectionHeaderPillFill



/**
 * Stateful entry point wiring [SettingsViewModel] to the stateless
 * [SettingsScreen]. Kept thin so the presentation layer stays testable.
 *
 * External-link navigation lives here rather than in the ViewModel: opening a URI
 * needs the composition-local [LocalUriHandler], not app state.
 *
 * @param listState settings-list scroll position, hoisted to
 *   [id.web.quakealert.ui.main.MainScreen] so it survives tab switches, rotation
 *   and process death.
 */
@Composable
fun SettingsRoute(
    modifier: Modifier = Modifier,
    listState: LazyListState = rememberLazyListState(),
    viewModel: SettingsViewModel = viewModel()
) {
    val uiState by viewModel.uiState.collectAsStateWithLifecycle()

    val uriHandler = LocalUriHandler.current
    val openLink: (String) -> Unit = remember(uriHandler) {
        { url ->
            // AndroidUriHandler throws when nothing on the device can handle the
            // URI (no browser, no mail client). Swallow it so a missing handler
            // leaves the overlay open instead of crashing the app.
            runCatching { uriHandler.openUri(url) }
        }
    }

    SettingsScreen(
        uiState = uiState,
        onCoverageSelected = viewModel::onCoverageSelected,
        onAutoSyncToggled = viewModel::onAutoSyncToggled,
        onSyncLocationNow = viewModel::onSyncLocationNow,
        onTestAlertSound = viewModel::onTestAlertSound,
        onLightModeToggled = viewModel::onLightModeToggled,
        onLanguageSelected = viewModel::onLanguageSelected,
        onUnitSelected = viewModel::onUnitSelected,
        onMoreAboutUs = viewModel::onMoreAboutUs,
        onAboutDismissed = viewModel::onAboutDismissed,
        onGithubClick = { openLink(AboutLinks.GITHUB_PAGES) },
        onEmailClick = { openLink(AboutLinks.EMAIL) },
        onDonateClick = { openLink(AboutLinks.DONATE) },
        listState = listState,
        modifier = modifier
    )
}

/**
 * Stateless Settings screen ("Settings Page (Fix)", Figma node 1:845). Sections,
 * top → bottom:
 *  1. A static [QuakeAppBar] header ("Settings" + Healthy badge).
 *  2. "Location & Coverage": reactive [SensorMapCard] whose coverage geofence
 *     circle scales with the selected [CoverageRange], the Coverage segmented
 *     control, "Sync Location Now" action, and the "Auto Sync Location" switch.
 *  3. "Alert & Notification": "Test Alert Sound" action.
 *  4. "Appearance & Look": "Light Mode (Beta)" switch — disabled and badged
 *     "Coming Soon" while the app stays dark-theme only — plus the "Units" and
 *     "Language" segmented controls.
 *  5. "About": "More About Us" action card, which raises the [AboutModalDialog]
 *     overlay (Figma node 4:654) via `uiState.showAboutModal`.
 *
 * The scrolling body carries the shared soft [fadingEdges] so content dissolves
 * at the scroll bounds, matching the History / Sensors screens. All state and
 * events are hoisted to the caller ([SettingsRoute] / [SettingsViewModel]),
 * including [listState].
 */
@Composable
fun SettingsScreen(
    uiState: SettingsUiState,
    onCoverageSelected: (CoverageRange) -> Unit,
    onAutoSyncToggled: (Boolean) -> Unit,
    onSyncLocationNow: () -> Unit,
    onTestAlertSound: () -> Unit,
    onLightModeToggled: (Boolean) -> Unit,
    onLanguageSelected: (AppLanguage) -> Unit,
    onUnitSelected: (UnitSystem) -> Unit,
    onMoreAboutUs: () -> Unit,
    onAboutDismissed: () -> Unit,
    onGithubClick: () -> Unit,
    onEmailClick: () -> Unit,
    onDonateClick: () -> Unit,
    modifier: Modifier = Modifier,
    listState: LazyListState = rememberLazyListState()
) {
    Column(
        modifier = modifier
            .fillMaxSize()
            .padding(horizontal = Dimens.ScreenHorizontalPadding)
    ) {
        QuakeAppBar(title = "Settings", isHealthy = uiState.isHealthy)

        LazyColumn(
            state = listState,
            modifier = Modifier
                .weight(1f)
                .fillMaxWidth()
                .fadingEdges(),
            contentPadding = PaddingValues(
                top = Dimens.SettingsHeaderGap,
                bottom = Dimens.SettingsListBottomPadding
            ),
            verticalArrangement = Arrangement.spacedBy(Dimens.SettingsSectionSpacing)
        ) {
            // --- Location & Coverage --------------------------------------
            item(key = "header_location") {
                CenteredSectionBadge(title = "Location & Coverage")
            }

            item(key = "map_coverage") {
                // Same linked map as the Sensors screen, minus the settings
                // shortcut (Settings passes no onSettingsShortcut). The coverage
                // radius drives the shared geofence circle via the overview.
                SensorMapCard(
                    overview = SensorMapOverview(
                        locationLabel = uiState.locationLabel,
                        rangeKm = uiState.coverageRange.km,
                        sensorCount = uiState.sensorCount,
                        geofenceFraction = uiState.coverageRange.geofenceFraction
                    ),
                    unitSystem = uiState.unitSystem
                )
            }

            item(key = "card_coverage") {
                QuakeCard(title = "Coverage Range") {
                    QuakeSegmentedControl(
                        options = CoverageRange.entries,
                        selected = uiState.coverageRange,
                        labelOf = { it.label(uiState.unitSystem) },
                        onSelect = onCoverageSelected
                    )
                }
            }

            item(key = "card_location") {
                // Static "Sync Location Now" title (Figma node 1:876). The
                // detected location name lives only on the map card header above
                // to avoid duplicating location text across the section.
                QuakeCard(
                    title = "Sync Location Now",
                    detail = { QuakePill(text = uiState.lastSyncPillLabel) }
                ) {
                    SyncRefreshButton(onClick = onSyncLocationNow)
                }
            }

            item(key = "card_autosync") {
                QuakeCard(title = "Auto Sync Location") {
                    QuakeSwitch(
                        checked = uiState.autoSyncLocation,
                        onCheckedChange = onAutoSyncToggled
                    )
                }
            }


            // --- Alert & Notification -------------------------------------
            item(key = "header_alert") {
                CenteredSectionBadge(title = "Alert & Notification")
            }

            item(key = "card_test_alert") {
                QuakeCard(
                    title = "Test Alert Sound",
                    onClick = onTestAlertSound
                )
            }


            // --- Appearance & Look ----------------------------------------
            item(key = "header_appearance") {
                CenteredSectionBadge(title = "Appearance & Look")
            }

            item(key = "card_light_mode") {
                // Disabled while the app ships dark-theme only: the switch is
                // greyed out and the card carries a "Coming Soon" badge so the
                // control reads as deliberately unavailable rather than broken.
                QuakeCard(
                    title = "Light Mode (Beta)",
                    detail = { QuakePill(text = "Coming Soon") }
                ) {
                    QuakeSwitch(
                        checked = uiState.lightMode,
                        onCheckedChange = onLightModeToggled,
                        enabled = false
                    )
                }
            }

            item(key = "card_units") {
                QuakeCard(title = "Units") {
                    QuakeSegmentedControl(
                        options = UnitSystem.entries,
                        selected = uiState.unitSystem,
                        labelOf = { it.label },
                        onSelect = onUnitSelected
                    )
                }
            }


            item(key = "card_language") {
                QuakeCard(title = "Language") {
                    QuakeSegmentedControl(
                        options = AppLanguage.entries,
                        selected = uiState.language,
                        labelOf = { it.label },
                        onSelect = onLanguageSelected
                    )
                }
            }

            // --- About ----------------------------------------------------
            item(key = "header_about") {
                CenteredSectionBadge(title = "About")
            }

            item(key = "card_about") {
                AboutCard(
                    credit = uiState.appCredit,
                    version = uiState.appVersion,
                    onMoreAboutUs = onMoreAboutUs
                )
            }


        }
    }

    // Hosted in its own dialog window, so it overlays the whole Settings screen
    // (nav bar included) without displacing any of the layout above.
    if (uiState.showAboutModal) {
        AboutModalDialog(
            onDismiss = onAboutDismissed,
            onGithubClick = onGithubClick,
            onEmailClick = onEmailClick,
            onDonateClick = onDonateClick
        )
    }
}

/**
 * Centered section badge ("Location & Coverage", Figma node 1:856). A hug-width
 * #2D2D2D slim stadium capsule, fixed 23dp tall with 14dp horizontal padding and
 * a 1px white-10% stroke, horizontally centered within the list.
 */
@Composable
private fun CenteredSectionBadge(
    title: String,
    modifier: Modifier = Modifier
) {
    val shape = RoundedCornerShape(Dimens.RadiusStadium)
    Box(modifier = modifier.fillMaxWidth(), contentAlignment = Alignment.Center) {
        Box(
            modifier = Modifier
                .wrapContentWidth()
                .height(Dimens.SectionHeaderPillHeight)
                .clip(shape)
                .background(SectionHeaderPillFill, shape)
                .border(Dimens.BorderThin, CardBorder, shape)
                .padding(horizontal = Dimens.SectionHeaderPillPaddingHorizontal),
            contentAlignment = Alignment.Center
        ) {
            Text(text = title, style = CardTitle)
        }
    }
}



@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun SettingsScreenPreview() {
    QuakeAlertTheme {
        SettingsScreen(
            uiState = SettingsUiState(),
            onCoverageSelected = {},
            onAutoSyncToggled = {},
            onSyncLocationNow = {},
            onTestAlertSound = {},
            onLightModeToggled = {},
            onLanguageSelected = {},
            onUnitSelected = {},
            onMoreAboutUs = {},
            onAboutDismissed = {},
            onGithubClick = {},
            onEmailClick = {},
            onDonateClick = {}
        )
    }
}
