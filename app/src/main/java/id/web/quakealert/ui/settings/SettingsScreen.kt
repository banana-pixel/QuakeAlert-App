package id.web.quakealert.ui.settings

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
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
import id.web.quakealert.ui.theme.CardBorder
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
        modifier = modifier
    )
}

/**
 * Stateless Settings screen ("Settings Page (Fix)", Figma node 1:845). Focused
 * purely on the "Location & Coverage" section, top → bottom:
 *  1. A static [QuakeAppBar] header ("Settings" + Healthy badge).
 *  2. A centered "Location & Coverage" section badge.
 *  3. A reactive [SensorMapCard] whose coverage geofence circle scales with the
 *     selected [CoverageRange].
 *  4. Three setting cards: Coverage range segmented control, "Sync Location Now"
 *     action, and the "Auto Sync Location" switch.
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
            item(key = "header_location") {
                CenteredSectionBadge(title = "Location & Coverage")
            }

            item(key = "map_coverage") {
                SensorMapCard(
                    overview = SensorMapOverview(
                        locationLabel = uiState.locationLabel,
                        rangeKm = uiState.coverageRange.km,
                        sensorCount = uiState.sensorCount
                    ),
                    geofenceFraction = uiState.coverageRange.geofenceFraction
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
                    subtitle = "Sync Location Now"
                ) {
                    InfoPill(text = uiState.lastSyncPillLabel)
                    SyncRefreshButton(onClick = onSyncLocationNow)
                }
            }

            item(key = "card_autosync") {
                SettingCard(
                    title = "Auto Sync Location",
                    subtitle = "Auto-update coverage as you move"
                ) {
                    QuakeSwitch(
                        checked = uiState.autoSyncLocation,
                        onCheckedChange = onAutoSyncToggled
                    )
                }
            }
        }
    }
}

/**
 * Centered section badge ("Location & Coverage", Figma node 1:846). Unlike the
 * full-width left-aligned [SectionHeaderPill], this is a wrap-content capsule
 * horizontally centered within the list.
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
                .clip(shape)
                .background(SectionHeaderPillFill, shape)
                .border(Dimens.BorderThin, CardBorder, shape)
                .padding(
                    horizontal = Dimens.SectionHeaderPillPaddingHorizontal,
                    vertical = Dimens.SectionHeaderPillPaddingVertical
                ),
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
            onSyncLocationNow = {}
        )
    }
}
