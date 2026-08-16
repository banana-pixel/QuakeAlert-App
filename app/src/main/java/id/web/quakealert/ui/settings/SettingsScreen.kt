package id.web.quakealert.ui.settings

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.tooling.preview.Preview
import androidx.lifecycle.viewmodel.compose.viewModel
import id.web.quakealert.ui.common.QuakeAppBar
import id.web.quakealert.ui.common.fadingEdges
import id.web.quakealert.ui.theme.AboutButtonFill
import id.web.quakealert.ui.theme.AboutCardGradient
import id.web.quakealert.ui.theme.CardBorder
import id.web.quakealert.ui.theme.CardSubtitle
import id.web.quakealert.ui.theme.CardTitle
import id.web.quakealert.ui.theme.ChipLabel
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.QuakeAlertTheme
import id.web.quakealert.ui.theme.TextPrimary

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
        onLanguageSelected = viewModel::onLanguageSelected,
        onAutoSyncToggled = viewModel::onAutoSyncToggled,
        onKeepAlertingToggled = viewModel::onKeepAlertingToggled,
        onLightModeToggled = viewModel::onLightModeToggled,
        onSyncLocationNow = viewModel::onSyncLocationNow,
        onTestAlertSound = viewModel::onTestAlertSound,
        onMoreAboutUs = viewModel::onMoreAboutUs,
        modifier = modifier
    )
}

/**
 * Stateless Settings screen (Figma node 1:845). Structure, top → bottom:
 *  1. A static [QuakeAppBar] header pinned to the top.
 *  2. A weighted [LazyColumn] carrying the shared soft [fadingEdges] so the
 *     grouped setting sections dissolve at the scroll bounds, matching the
 *     History / Sensors screens.
 *
 * Content is grouped into "Location & Coverage", "Alerts", "Appearance &
 * Language" and "About" sections. All state and events are hoisted to the caller
 * ([SettingsRoute] / [SettingsViewModel]).
 */
@Composable
fun SettingsScreen(
    uiState: SettingsUiState,
    onCoverageSelected: (CoverageRange) -> Unit,
    onLanguageSelected: (AppLanguage) -> Unit,
    onAutoSyncToggled: (Boolean) -> Unit,
    onKeepAlertingToggled: (Boolean) -> Unit,
    onLightModeToggled: (Boolean) -> Unit,
    onSyncLocationNow: () -> Unit,
    onTestAlertSound: () -> Unit,
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
                SectionHeaderPill(title = "Location & Coverage")
            }
            item(key = "card_location") {
                SettingCard(
                    title = uiState.locationLabel,
                    subtitle = "Detected Location"
                ) {
                    InfoPill(text = uiState.lastSyncPillLabel)
                    SyncRefreshButton(onClick = onSyncLocationNow)
                }
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
            item(key = "card_autosync") {
                SettingCard(
                    title = "Intelligent Location Sync",
                    subtitle = "Auto-update coverage as you move"
                ) {
                    QuakeSwitch(
                        checked = uiState.autoSyncLocation,
                        onCheckedChange = onAutoSyncToggled
                    )
                }
            }

            // --- Alerts ---------------------------------------------------
            item(key = "header_alerts") {
                SectionHeaderPill(title = "Alerts")
            }
            item(key = "card_keep_alerting") {
                SettingCard(
                    title = "Keep Alerting",
                    subtitle = "Repeat siren until acknowledged"
                ) {
                    QuakeSwitch(
                        checked = uiState.keepAlerting,
                        onCheckedChange = onKeepAlertingToggled
                    )
                }
            }
            item(key = "card_test_sound") {
                SettingCard(
                    title = "Test Alert Sound",
                    subtitle = "Preview the earthquake siren",
                    onClick = onTestAlertSound
                )
            }

            // --- Appearance & Language ------------------------------------
            item(key = "header_appearance") {
                SectionHeaderPill(title = "Appearance & Language")
            }
            item(key = "card_light_mode") {
                SettingCard(
                    title = "Light Mode (Beta)",
                    subtitle = "Switch to a bright theme"
                ) {
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
                SectionHeaderPill(title = "About")
            }
            item(key = "card_about") {
                AboutCard(
                    appVersion = uiState.appVersion,
                    onMoreAboutUs = onMoreAboutUs
                )
            }
        }
    }
}

/**
 * "About" call-to-action card (Figma node 1:934): a gradient-filled rounded card
 * with the app version, a short blurb and a "More About Us" pill button.
 */
@Composable
private fun AboutCard(
    appVersion: String,
    onMoreAboutUs: () -> Unit,
    modifier: Modifier = Modifier
) {
    val shape = RoundedCornerShape(Dimens.SettingCardRadius)
    Column(
        modifier = modifier
            .fillMaxWidth()
            .clip(shape)
            .background(AboutCardGradient, shape)
            .border(Dimens.BorderThin, CardBorder, shape)
            .padding(Dimens.SettingCardPadding),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(Dimens.SettingCardTitleGap)
    ) {
        Text(text = "QuakeAlert", style = CardTitle)
        Text(
            text = appVersion,
            style = CardSubtitle,
            textAlign = TextAlign.Center
        )
        Text(
            text = "Community-powered earthquake early warning.",
            style = CardSubtitle,
            textAlign = TextAlign.Center
        )

        AboutButton(onClick = onMoreAboutUs)
    }
}

/** Stadium "More About Us" pill button on the About card. */
@Composable
private fun AboutButton(
    onClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    val shape = RoundedCornerShape(Dimens.AboutButtonRadius)
    Row(
        modifier = modifier
            .clip(shape)
            .background(AboutButtonFill, shape)
            .border(Dimens.BorderThin, CardBorder, shape)
            .clickable(onClick = onClick)
            .padding(
                horizontal = Dimens.AboutButtonPaddingHorizontal,
                vertical = Dimens.AboutButtonPaddingVertical
            ),
        verticalAlignment = Alignment.CenterVertically
    ) {
        Text(text = "More About Us", style = ChipLabel, color = TextPrimary)
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun SettingsScreenPreview() {
    QuakeAlertTheme {
        SettingsScreen(
            uiState = SettingsUiState(),
            onCoverageSelected = {},
            onLanguageSelected = {},
            onAutoSyncToggled = {},
            onKeepAlertingToggled = {},
            onLightModeToggled = {},
            onSyncLocationNow = {},
            onTestAlertSound = {},
            onMoreAboutUs = {}
        )
    }
}
