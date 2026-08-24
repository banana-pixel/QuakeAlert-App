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
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.material3.pulltorefresh.rememberPullToRefreshState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clipToBounds
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.tooling.preview.Preview
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import id.web.quakealert.data.network.ServerHealth
import id.web.quakealert.ui.common.QuakeAppBar
import id.web.quakealert.ui.common.GenericErrorCopy
import id.web.quakealert.ui.common.QuakeErrorState
import id.web.quakealert.ui.common.QuakeFilter
import id.web.quakealert.ui.common.FilterSection
import id.web.quakealert.ui.common.QuakeFilterDialog
import id.web.quakealert.ui.common.QuakeFilterRow
import id.web.quakealert.ui.common.QuakeFilterViewModel
import id.web.quakealert.ui.common.QuakeNoCoverageState
import id.web.quakealert.ui.common.QuakeNoPositionState
import id.web.quakealert.ui.common.QuakeNoStationsMatchState
import id.web.quakealert.ui.common.QuakeStationStatus
import id.web.quakealert.ui.common.QuakeSkeletonList
import id.web.quakealert.ui.common.fadingEdges
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.QuakeAlertTheme

/**
 * Stateful entry point that connects [SensorsViewModel] to the stateless
 * [SensorsScreen]. Kept thin so the presentation layer stays testable.
 *
 * @param listState station-list scroll position, hoisted to
 *   [id.web.quakealert.ui.main.MainScreen] so it survives tab switches, rotation
 *   and process death.
 */
@Composable
fun SensorsRoute(
    health: ServerHealth,
    onSyncLocation: () -> Unit,
    modifier: Modifier = Modifier,
    listState: LazyListState = rememberLazyListState(),
    viewModel: SensorsViewModel = viewModel(),
    filterViewModel: QuakeFilterViewModel = viewModel()
) {
    val uiState by viewModel.uiState.collectAsStateWithLifecycle()

    // Same Activity-scoped instance the History tab uses, so a filter set on either
    // tab is already in force on the other. Pushed in rather than read, which keeps
    // SensorsViewModel unaware that a second tab exists.
    val filter by filterViewModel.filter.collectAsStateWithLifecycle()
    val isSheetOpen by filterViewModel.isSheetOpen.collectAsStateWithLifecycle()
    LaunchedEffect(filter) { viewModel.applyFilter(filter) }

    SensorsScreen(
        uiState = uiState,
        health = health,
        onModeSelected = filterViewModel::onModeSelected,
        onFilterSheetClicked = filterViewModel::onSheetOpened,
        onWidenRadius = filterViewModel::onRadiusWidened,
        onFiltersReset = filterViewModel::onFiltersReset,
        onSensorClicked = viewModel::onSensorClicked,
        onSyncLocation = onSyncLocation,
        onRetry = viewModel::onRetry,
        onRefresh = viewModel::onRefresh,
        listState = listState,
        modifier = modifier
    )

    if (isSheetOpen) {
        QuakeFilterDialog(
            filter = filter,
            sections = FilterSection.SENSORS,
            unitSystem = uiState.unitSystem,
            onDismiss = filterViewModel::onSheetDismissed,
            onApply = filterViewModel::onCriteriaApplied,
            onReset = {
                filterViewModel.onFiltersReset()
                filterViewModel.onSheetDismissed()
            }
        )
    }
}

/**
 * Stateless Sensors screen (Figma node 1:1081). Structure, top → bottom, mirrors
 * the History screen so both tabs share layout behaviour:
 *  1. A static header [Column] pinned to the top: shared [QuakeAppBar] +
 *     [SensorMapCard] + shared [QuakeFilterRow].
 *  2. A weighted body filling the remaining space between the filter row and the
 *     bottom navigation bar, inside a [PullToRefreshBox] and rendering exactly one
 *     of [QuakeSkeletonList],
 *     [QuakeErrorState], [QuakeNoStationsMatchState], [QuakeNoCoverageState] or
 *     the station [LazyColumn] — which
 *     carries the shared soft [fadingEdges] so cards dissolve in/out at the scroll
 *     bounds. The header stays outside the branch so the map and filters hold
 *     their place as the state changes.
 *
 * Keeping the map + filter static (rather than scrolling them) matches History's
 * fixed-header pattern and keeps the map anchored while only the station list
 * scrolls. All state and events are hoisted to the caller ([SensorsRoute] /
 * [SensorsViewModel]).
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SensorsScreen(
    uiState: SensorsUiState,
    health: ServerHealth = ServerHealth.HEALTHY,
    onModeSelected: (QuakeFilter) -> Unit,
    onSensorClicked: (SensorStationItem) -> Unit,
    onRetry: () -> Unit,
    onSyncLocation: () -> Unit = {},
    onFilterSheetClicked: (() -> Unit)? = null,
    onWidenRadius: () -> Unit = {},
    onFiltersReset: () -> Unit = {},
    onRefresh: () -> Unit = {},
    modifier: Modifier = Modifier,
    listState: LazyListState = rememberLazyListState()
) {
    Column(
        modifier = modifier
            .fillMaxSize()
            .padding(horizontal = Dimens.ScreenHorizontalPadding)
    ) {
        // --- Static header: title + map preview + filter row -----------------
        QuakeAppBar(title = "Sensors", health = health)

        SensorMapCard(
            overview = uiState.overview,
            unitSystem = uiState.unitSystem,
            // Both derived from the same state as the list below, so the dot the
            // camera moves to is the row the user tapped.
            markers = uiState.mapMarkers(),
            focus = uiState.mapFocus(),
            pillLabel = uiState.mapPillLabel(),
            modifier = Modifier.padding(top = Dimens.HeaderSectionGap)
        )

        QuakeFilterRow(
            filter = uiState.filter,
            sections = FilterSection.SENSORS,
            unitSystem = uiState.unitSystem,
            onModeSelected = onModeSelected,
            onFilterSheetClicked = onFilterSheetClicked,
            modifier = Modifier.padding(top = Dimens.SensorsHeaderBlockGap)
        )

        // --- Body: pull-to-refresh over loading / error / empty / content ----
        // The refresh gesture wraps *all four* states, not just the list: a user
        // looking at an error or an empty roll is exactly the user most likely to
        // pull, and sending them to the Retry button instead would be arbitrary.
        val pullState = rememberPullToRefreshState()
        PullToRefreshBox(
            isRefreshing = uiState.isRefreshing,
            onRefresh = onRefresh,
            state = pullState,
            modifier = Modifier
                .weight(1f)
                .fillMaxWidth()
                // PullToRefreshBox does not clip, and the body below is translated
                // during a pull — without this the roll would paint over the bottom
                // navigation bar as the finger moves.
                .clipToBounds()
        ) {
            // Elastic follow-through, identical to History: read inside
            // graphicsLayer so a pull invalidates only the draw phase, and applied
            // outside fadingEdges() so the fade travels with the content.
            val bodyModifier = Modifier
                .fillMaxSize()
                .graphicsLayer {
                    translationY = pullState.distanceFraction * Dimens.PullElasticDistance.toPx()
                }

            when {
                uiState.isLoading -> QuakeSkeletonList(
                    loadingLabel = LOADING_MESSAGE,
                    modifier = bodyModifier.padding(top = Dimens.CardListTopPadding)
                )

                uiState.isError -> QuakeErrorState(
                    copy = uiState.errorCopy ?: GenericErrorCopy,
                    onRetry = onRetry,
                    modifier = bodyModifier,
                    // Offered only for a rejected query, and only by the copy: a
                    // filter the server refused is the one failure the user can
                    // resolve themselves.
                    onResetFilters = onFiltersReset
                )

                // Before every empty branch: with no synced position there was no
                // query, so neither "nothing matches" nor "we do not watch there"
                // is a true thing to say.
                uiState.needsPosition -> QuakeNoPositionState(
                    onSyncLocation = onSyncLocation,
                    modifier = bodyModifier
                )

                // An empty *slice* of a non-empty roll is a different fact from an
                // empty roll: the stations exist and the status criterion hid them,
                // so widening the radius would be the wrong offer.
                uiState.sensors.isEmpty() &&
                    uiState.filter.stationStatus != QuakeStationStatus.ALL ->
                    QuakeNoStationsMatchState(
                        status = uiState.filter.stationStatus,
                        onResetFilters = onFiltersReset,
                        modifier = bodyModifier
                    )

                // Worth the separate copy: the browse radius reaches far beyond
                // the network, so an empty roll usually means "we do not watch
                // there", not "there is nothing to watch". The widen action appears
                // only when a radius is actually narrowing the query.
                uiState.sensors.isEmpty() -> QuakeNoCoverageState(
                    onWidenRadius = onWidenRadius.takeIf {
                        uiState.filter.mode == QuakeFilter.NEAR
                    },
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
                            onClick = { onSensorClicked(item) },
                            isSelected = item.id == uiState.selectedStationId
                        )
                    }
                }
            }
        }
    }
}

/**
 * What a screen reader announces while the skeleton is up. A skeleton conveys
 * "loading" visually and nothing at all otherwise, so the copy the spinner used to
 * show is spoken instead.
 */
private const val LOADING_MESSAGE = "Scanning the sensor network..."

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
            onModeSelected = {},
            onSensorClicked = {},
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
            onModeSelected = {},
            onSensorClicked = {},
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
            onModeSelected = {},
            onSensorClicked = {},
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
                errorCopy = GenericErrorCopy
            ),
            onModeSelected = {},
            onSensorClicked = {},
            onRetry = {}
        )
    }
}
