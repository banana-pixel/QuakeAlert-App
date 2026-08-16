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
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.tooling.preview.Preview
import androidx.lifecycle.viewmodel.compose.viewModel
import id.web.quakealert.ui.common.QuakeAppBar
import id.web.quakealert.ui.common.fadingEdges
import id.web.quakealert.ui.sensors.SensorMapCard
import id.web.quakealert.ui.sensors.SensorMapOverview
import id.web.quakealert.ui.theme.BorderLight
import id.web.quakealert.ui.theme.CardTitle
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.QuakeAlertTheme
import id.web.quakealert.ui.theme.SectionHeaderPillFill


/**
 * Stateful entry point wiring [SettingsViewModel] to the stateless
 * [SettingsScreen]. Kept thin so the presentation layer stays testable.
 */
@Composable
fun SettingsRoute(
    modifier: Modifier = Modifier,
    viewModel: SettingsViewModel = viewModel()
) {
    val uiState by viewModel.uiState.collectAsState()

    SettingsScreen(
        uiState = uiState,
        onCoverageSelected = viewModel::onCoverageSelected,
        onAutoSyncToggled = viewModel::onAutoSyncToggled,
        onSyncLocationNow = viewModel::onSyncLocationNow,
        onKeepAlertingToggled = viewModel::onKeepAlertingToggled,
        onTestAlertSound = viewModel::onTestAlertSound,
        onLightModeToggled = viewModel::onLightModeToggled,
        onLanguageSelected = viewModel::onLanguageSelected,
        onMoreAboutUs = viewModel::onMoreAboutUs,
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
 *  3. "Alert & Notification": "Keep Alerting" switch + "Test Alert Sound" action.
 *  4. "Appearance & Look": "Light Mode (Beta)" switch + "Language" segmented
 *     control.
 *  5. "About": "More About Us" action card.
 *
 * The scrolling body carries the shared soft [fadingEdges] so content dissolves
 * at the scroll bounds, matching the History / Sensors screens. All state and
 * events are hoisted to the caller ([SettingsRoute] / [SettingsViewModel]).
 */
@Composable
fun SettingsScreen(
    uiState: SettingsUiState,
    onCoverageSelected: (CoverageRange) -> Unit,
    onAutoSyncToggled: (Boolean) -> Unit,
    onSyncLocationNow: () -> Unit,
    onKeepAlertingToggled: (Boolean) -> Unit,
    onTestAlertSound: () -> Unit,
    onLightModeToggled: (Boolean) -> Unit,
    onLanguageSelected: (AppLanguage) -> Unit,
    onMoreAboutUs: () -> Unit,
    modifier: Modifier = Modifier
) {
    Column(
        modifier = modifier
            .fillMaxSize()
            .padding(horizontal = Dimens.ScreenHorizontalPadding)
    ) {
        QuakeAppBar(title = "Settings", isHealthy = uiState.isHealthy)

        LazyColumn(
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
                    )
                )
            }

            item(key = "card_coverage") {
                SettingCard(title = "Coverage Range") {
                    QuakeSegmentedControl(
                        options = CoverageRange.entries,
                        selected = uiState.coverageRange,
                        labelOf = { it.label },
                        onSelect = onCoverageSelected
                    )
                }
            }

            item(key = "card_location") {
                SettingCard(
                    title = uiState.locationLabel,
                    detail = { InfoPill(text = uiState.lastSyncPillLabel) }
                ) {
                    SyncRefreshButton(onClick = onSyncLocationNow)
                }
            }

            item(key = "card_autosync") {
                SettingCard(title = "Auto Sync Location") {
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

            item(key = "card_keep_alerting") {
                SettingCard(title = "Keep Alerting") {
                    QuakeSwitch(
                        checked = uiState.keepAlerting,
                        onCheckedChange = onKeepAlertingToggled
                    )
                }
            }

            item(key = "card_test_alert") {
                SettingCard(
                    title = "Test Alert Sound",
                    onClick = onTestAlertSound
                )
            }


            // --- Appearance & Look ----------------------------------------
            item(key = "header_appearance") {
                CenteredSectionBadge(title = "Appearance & Look")
            }

            item(key = "card_light_mode") {
                SettingCard(title = "Light Mode (Beta)") {
                    QuakeSwitch(
                        checked = uiState.lightMode,
                        onCheckedChange = onLightModeToggled
                    )
                }
            }


            item(key = "card_language") {
                SettingCard(title = "Language") {
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
                    onMoreAboutUs = onMoreAboutUs
                )
            }

        }
    }
}

/**
 * Centered section badge ("Location & Coverage", Figma node 1:856). A hug-width
 * #2D2D2D capsule, fixed 23dp tall with 12dp horizontal padding, a 2px white-30%
 * stroke and a 10dp radius, horizontally centered within the list.
 */
@Composable
private fun CenteredSectionBadge(
    title: String,
    modifier: Modifier = Modifier
) {
    val shape = RoundedCornerShape(Dimens.SectionHeaderPillRadius)
    Box(modifier = modifier.fillMaxWidth(), contentAlignment = Alignment.Center) {
        Box(
            modifier = Modifier
                .wrapContentWidth()
                .height(Dimens.SectionHeaderPillHeight)
                .clip(shape)
                .background(SectionHeaderPillFill, shape)
                .border(Dimens.BorderMedium, BorderLight, shape)
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
            onKeepAlertingToggled = {},
            onTestAlertSound = {},
            onLightModeToggled = {},
            onLanguageSelected = {},
            onMoreAboutUs = {}
        )
    }
}
