package id.web.quakealert.ui.sensors

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.tooling.preview.Preview
import androidx.lifecycle.viewmodel.compose.viewModel
import id.web.quakealert.ui.common.QuakeAppBar
import id.web.quakealert.ui.common.QuakeFilter
import id.web.quakealert.ui.common.QuakeFilterRow
import id.web.quakealert.ui.common.fadingEdges
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.QuakeAlertTheme

/**
 * Stateful entry point that connects [SensorsViewModel] to the stateless
 * [SensorsScreen]. Kept thin so the presentation layer stays testable.
 *
 * @param onOpenSettings navigates to the Settings tab when the map's settings
 *   shortcut is tapped (hoisted to [id.web.quakealert.ui.main.MainScreen]).
 */
@Composable
fun SensorsRoute(
    onOpenSettings: () -> Unit,
    modifier: Modifier = Modifier,
    viewModel: SensorsViewModel = viewModel()
) {
    val uiState by viewModel.uiState.collectAsState()

    SensorsScreen(
        uiState = uiState,
        onFilterSelected = viewModel::onFilterSelected,
        onCalendarClicked = viewModel::onCalendarClicked,
        onSensorClicked = viewModel::onSensorClicked,
        onOpenSettings = onOpenSettings,
        modifier = modifier
    )
}

/**
 * Stateless Sensors screen (Figma node 1:1081). Structure, top → bottom, mirrors
 * the History screen so both tabs share layout behaviour:
 *  1. A static header [Column] pinned to the top: shared [QuakeAppBar] +
 *     [SensorMapCard] + shared [QuakeFilterRow].
 *  2. A weighted [LazyColumn] filling the remaining space between the filter row
 *     and the bottom navigation bar, carrying the shared soft [fadingEdges] so
 *     station cards dissolve in/out at the scroll bounds.
 *
 * Keeping the map + filter static (rather than scrolling them) matches History's
 * fixed-header pattern and keeps the map anchored while only the station list
 * scrolls. All state and events are hoisted to the caller ([SensorsRoute] /
 * [SensorsViewModel]).
 */
@Composable
fun SensorsScreen(
    uiState: SensorsUiState,
    onFilterSelected: (QuakeFilter) -> Unit,
    onCalendarClicked: () -> Unit,
    onSensorClicked: (SensorStationItem) -> Unit,
    onOpenSettings: () -> Unit,
    modifier: Modifier = Modifier
) {
    Column(
        modifier = modifier
            .fillMaxSize()
            .padding(horizontal = Dimens.ScreenHorizontalPadding)
    ) {
        // --- Static header: title + map preview + filter row -----------------
        QuakeAppBar(title = "Sensors", isHealthy = uiState.isHealthy)

        SensorMapCard(
            overview = uiState.overview,
            unitSystem = uiState.unitSystem,
            onSettingsShortcut = onOpenSettings,
            modifier = Modifier.padding(top = Dimens.HeaderSectionGap)
        )

        QuakeFilterRow(
            selectedFilter = uiState.selectedFilter,
            nearRadiusKm = uiState.nearRadiusKm,
            unitSystem = uiState.unitSystem,
            onFilterSelected = onFilterSelected,
            onCalendarClicked = onCalendarClicked,
            modifier = Modifier.padding(top = Dimens.SensorsHeaderBlockGap)
        )

        // --- Scrolling station list, bounded by the filter row and bottom nav
        LazyColumn(
            modifier = Modifier
                .weight(1f)
                .fillMaxWidth()
                .fadingEdges(),
            contentPadding = PaddingValues(
                top = Dimens.CardListTopPadding,
                bottom = Dimens.CardListBottomPadding
            ),
            verticalArrangement = Arrangement.spacedBy(Dimens.CardListSpacing)
        ) {
            items(
                items = uiState.sensors,
                key = { it.id },
                contentType = { "SensorItemCard" }
            ) { item ->
                SensorItemCard(
                    item = item,
                    onClick = { onSensorClicked(item) }
                )
            }
        }
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun SensorsScreenPreview() {
    QuakeAlertTheme {
        SensorsScreen(
            uiState = SensorsUiState(
                sensors = listOf(
                    SensorStationItem(
                        id = "1",
                        stationId = "NODE-163A149F",
                        location = "Cimahi, West Java, ID",
                        chipLabel = "MPU 6050",
                        status = SensorStatus.ONLINE,
                        telemetry = SensorTelemetry(
                            lastPing = "Last Ping : 33s ago",
                            rssi = "RSSI : -61 dBm",
                            latency = "Latency : 2 ms"
                        )
                    ),
                    SensorStationItem(
                        id = "2",
                        stationId = "NODE-53FC66GH",
                        location = "Bandung, West Java, ID",
                        chipLabel = "MPU 6050",
                        status = SensorStatus.OFFLINE,
                        telemetry = SensorTelemetry(
                            lastPing = "Last Ping : - s ago",
                            rssi = "RSSI : - dBm",
                            latency = "Latency : - ms"
                        )
                    )
                )
            ),
            onFilterSelected = {},
            onCalendarClicked = {},
            onSensorClicked = {},
            onOpenSettings = {}
        )
    }
}
