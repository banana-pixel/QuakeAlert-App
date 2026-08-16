package id.web.quakealert.ui.sensors

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.viewmodel.compose.viewModel
import id.web.quakealert.R
import id.web.quakealert.ui.theme.CardBorder
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.FilterActiveFill
import id.web.quakealert.ui.theme.FilterInactiveFill
import id.web.quakealert.ui.theme.HealthyBadgeFill
import id.web.quakealert.ui.theme.NunitoFontFamily
import id.web.quakealert.ui.theme.QuakeAlertTheme
import id.web.quakealert.ui.theme.TextPrimary

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
 * Stateless Sensors screen (Figma node 1:1081). Structure, top → bottom:
 *  1. A static header [Column] pinned to the top: title + "Healthy" badge,
 *     a filter row, and the [SensorMapCard].
 *  2. A weighted [LazyColumn] of [SensorItemCard]s filling the remaining space
 *     between the map and the bottom navigation bar.
 *
 * All state and events are hoisted to the caller ([SensorsRoute] /
 * [SensorsViewModel]).
 */
@Composable
fun SensorsScreen(
    uiState: SensorsUiState,
    onFilterSelected: (SensorFilter) -> Unit,
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
        // --- Static header ---------------------------------------------------
        SensorsTopBar(isHealthy = uiState.isHealthy)

        SensorsFilterRow(
            selectedFilter = uiState.selectedFilter,
            nearRadiusKm = uiState.nearRadiusKm,
            onFilterSelected = onFilterSelected,
            onCalendarClicked = onCalendarClicked,
            modifier = Modifier.padding(top = Dimens.HeaderSectionGap)
        )

        SensorMapCard(
            overview = uiState.overview,
            onSettingsShortcut = onOpenSettings,
            modifier = Modifier.padding(top = Dimens.HeaderSectionGap)
        )

        // --- Scrolling list of stations -------------------------------------
        LazyColumn(
            modifier = Modifier
                .weight(1f)
                .fillMaxWidth(),
            contentPadding = PaddingValues(
                top = Dimens.SensorsHeaderBlockGap,
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

/** Sensors header: "Sensors" title + optional "Healthy" status badge. */
@Composable
private fun SensorsTopBar(isHealthy: Boolean, modifier: Modifier = Modifier) {
    Row(
        modifier = modifier
            .fillMaxWidth()
            .statusBarsPadding(),
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalAlignment = Alignment.CenterVertically
    ) {
        Text(
            text = "Sensors",
            color = TextPrimary,
            fontFamily = NunitoFontFamily,
            fontWeight = FontWeight.ExtraBold,
            fontSize = 24.sp,
            lineHeight = 26.sp
        )
        if (isHealthy) {
            Row(
                modifier = Modifier
                    .background(HealthyBadgeFill, RoundedCornerShape(Dimens.RadiusSmall))
                    .border(Dimens.BorderThin, CardBorder, RoundedCornerShape(Dimens.RadiusSmall))
                    .padding(
                        horizontal = Dimens.BadgePaddingHorizontal,
                        vertical = Dimens.BadgePaddingVertical
                    ),
                horizontalArrangement = Arrangement.spacedBy(Dimens.BadgeIconGap),
                verticalAlignment = Alignment.CenterVertically
            ) {
                Icon(
                    painter = painterResource(id = R.drawable.ic_globe),
                    contentDescription = null,
                    tint = TextPrimary,
                    modifier = Modifier.size(16.dp)
                )
                Text(
                    text = "Healthy",
                    color = TextPrimary,
                    fontFamily = NunitoFontFamily,
                    fontWeight = FontWeight.Bold,
                    fontSize = 15.sp
                )
            }
        }
    }
}

/** Filter row mirroring the History layout: "All", "Near - {radius}km", calendar. */
@Composable
private fun SensorsFilterRow(
    selectedFilter: SensorFilter,
    nearRadiusKm: Int,
    onFilterSelected: (SensorFilter) -> Unit,
    onCalendarClicked: () -> Unit,
    modifier: Modifier = Modifier
) {
    Row(
        modifier = modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.spacedBy(Dimens.FilterRowGap),
        verticalAlignment = Alignment.CenterVertically
    ) {
        FilterPill(
            label = "All",
            selected = selectedFilter == SensorFilter.ALL,
            onClick = { onFilterSelected(SensorFilter.ALL) }
        )
        FilterPill(
            label = "Near - ${nearRadiusKm}km",
            selected = selectedFilter == SensorFilter.NEAR,
            onClick = { onFilterSelected(SensorFilter.NEAR) }
        )
        Spacer(modifier = Modifier.weight(1f))
        CalendarButton(onClick = onCalendarClicked)
    }
}

/** Stadium/pill toggle used for the "All" and "Near" filters. */
@Composable
private fun FilterPill(
    label: String,
    selected: Boolean,
    onClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    val shape = RoundedCornerShape(Dimens.RadiusSmall)
    val fill = if (selected) FilterActiveFill else FilterInactiveFill
    Box(
        modifier = modifier
            .height(Dimens.FilterPillHeight)
            .clip(shape)
            .background(fill, shape)
            .border(Dimens.BorderThin, CardBorder, shape)
            .clickable(onClick = onClick)
            .padding(
                horizontal = Dimens.FilterPillPaddingHorizontal,
                vertical = Dimens.FilterPillPaddingVertical
            ),
        contentAlignment = Alignment.Center
    ) {
        Text(
            text = label,
            color = TextPrimary,
            fontFamily = NunitoFontFamily,
            fontWeight = FontWeight.Bold,
            fontSize = 13.sp
        )
    }
}

/** Rounded-square calendar icon button aligned with the filter pills. */
@Composable
private fun CalendarButton(onClick: () -> Unit, modifier: Modifier = Modifier) {
    val shape = RoundedCornerShape(Dimens.RadiusSmall)
    Box(
        modifier = modifier
            .size(Dimens.FilterPillHeight)
            .clip(shape)
            .background(FilterInactiveFill, shape)
            .border(Dimens.BorderThin, CardBorder, shape)
            .clickable(onClick = onClick)
            .padding(Dimens.CalendarButtonPadding),
        contentAlignment = Alignment.Center
    ) {
        Icon(
            painter = painterResource(id = R.drawable.ic_calendar),
            contentDescription = "Filter by date",
            tint = TextPrimary,
            modifier = Modifier.size(16.dp)
        )
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
