package id.web.quakealert.ui.sensors

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListState
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.tooling.preview.Preview
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import id.web.quakealert.R
import id.web.quakealert.domain.ServerConnectionState
import id.web.quakealert.ui.common.QuakeAppBar
import id.web.quakealert.ui.common.QuakeEmptyState
import id.web.quakealert.ui.common.QuakeErrorState
import id.web.quakealert.ui.common.QuakeFilter
import id.web.quakealert.ui.common.QuakeFilterRow
import id.web.quakealert.ui.common.QuakeLoadingState
import id.web.quakealert.ui.common.fadingEdges
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.QuakeAlertTheme

/**
 * Stateful entry point that connects [SensorsViewModel] to the stateless
 * [SensorsScreen]. Kept thin so the presentation layer stays testable.
 *
 * @param onOpenSettings navigates to the Settings tab when the map's settings
 *   shortcut is tapped (hoisted to [id.web.quakealert.ui.main.MainScreen]).
 * @param listState station-list scroll position, hoisted to
 *   [id.web.quakealert.ui.main.MainScreen] so it survives tab switches, rotation
 *   and process death.
 */
@Composable
fun SensorsRoute(
    onOpenSettings: () -> Unit,
    connectionState: ServerConnectionState,
    modifier: Modifier = Modifier,
    listState: LazyListState = rememberLazyListState(),
    viewModel: SensorsViewModel = viewModel()
) {
    val uiState by viewModel.uiState.collectAsStateWithLifecycle()

    SensorsScreen(
        uiState = uiState,
        connectionState = connectionState,
        onFilterSelected = viewModel::onFilterSelected,
        onSensorClicked = viewModel::onSensorClicked,
        onOpenSettings = onOpenSettings,
        onRetry = viewModel::onRetry,
        listState = listState,
        modifier = modifier
    )
}

/**
 * Stateless Sensors screen (Figma node 1:1081). Structure, top → bottom, mirrors
 * the History screen so both tabs share layout behaviour:
 *  1. A static header [Column] pinned to the top: shared [QuakeAppBar] +
 *     [SensorMapCard] + shared [QuakeFilterRow].
 *  2. A weighted body filling the remaining space between the filter row and the
 *     bottom navigation bar, rendering exactly one of [QuakeLoadingState],
 *     [QuakeErrorState], [QuakeEmptyState] or the station [LazyColumn] — which
 *     carries the shared soft [fadingEdges] so cards dissolve in/out at the scroll
 *     bounds. The header stays outside the branch so the map and filters hold
 *     their place as the state changes.
 *
 * Keeping the map + filter static (rather than scrolling them) matches History's
 * fixed-header pattern and keeps the map anchored while only the station list
 * scrolls. All state and events are hoisted to the caller ([SensorsRoute] /
 * [SensorsViewModel]).
 */
@Composable
fun SensorsScreen(
    uiState: SensorsUiState,
    connectionState: ServerConnectionState = ServerConnectionState.CONNECTED,
    onFilterSelected: (QuakeFilter) -> Unit,
    onSensorClicked: (SensorStationItem) -> Unit,
    onOpenSettings: () -> Unit,
    onRetry: () -> Unit,
    modifier: Modifier = Modifier,
    listState: LazyListState = rememberLazyListState()
) {
    Column(
        modifier = modifier
            .fillMaxSize()
            .padding(horizontal = Dimens.ScreenHorizontalPadding)
    ) {
        // --- Static header: title + map preview + filter row -----------------
        QuakeAppBar(title = "Sensors", connectionState = connectionState)

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
            modifier = Modifier.padding(top = Dimens.SensorsHeaderBlockGap)
        )

        // --- Body: loading / error / empty / content -------------------------
        val bodyModifier = Modifier
            .weight(1f)
            .fillMaxWidth()

        when {
            uiState.isLoading -> QuakeLoadingState(
                modifier = bodyModifier,
                message = "Scanning the sensor network..."
            )

            uiState.isError -> QuakeErrorState(
                message = uiState.errorMessage ?: GENERIC_ERROR_MESSAGE,
                onRetry = onRetry,
                modifier = bodyModifier
            )

            uiState.sensors.isEmpty() -> QuakeEmptyState(
                icon = R.drawable.ic_nav_sensors,
                message = "No Sensors Found",
                subtitle = "No stations are reporting inside your coverage range yet.",
                modifier = bodyModifier
            )

            else -> LazyColumn(
                state = listState,
                modifier = bodyModifier.fadingEdges(),
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
}

/** Fallback shown when a failed load carried no message of its own. */
private const val GENERIC_ERROR_MESSAGE =
    "Could not reach the sensor network. Check your connection and try again."

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
            onSensorClicked = {},
            onOpenSettings = {},
            onRetry = {}
        )
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun SensorsScreenLoadingPreview() {
    QuakeAlertTheme {
        SensorsScreen(
            uiState = SensorsUiState(isLoading = true),
            onFilterSelected = {},
            onSensorClicked = {},
            onOpenSettings = {},
            onRetry = {}
        )
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun SensorsScreenEmptyPreview() {
    QuakeAlertTheme {
        SensorsScreen(
            uiState = SensorsUiState(),
            onFilterSelected = {},
            onSensorClicked = {},
            onOpenSettings = {},
            onRetry = {}
        )
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun SensorsScreenErrorPreview() {
    QuakeAlertTheme {
        SensorsScreen(
            uiState = SensorsUiState(
                isError = true,
                errorMessage = "Could not reach the sensor network. Check your connection and try again."
            ),
            onFilterSelected = {},
            onSensorClicked = {},
            onOpenSettings = {},
            onRetry = {}
        )
    }
}
