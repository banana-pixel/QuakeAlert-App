package id.web.quakealert.ui.history

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.tooling.preview.Preview
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.compose.runtime.getValue
import androidx.compose.runtime.collectAsState

import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.QuakeAlertTheme

/**
 * Stateful entry point that connects [HistoryViewModel] to the stateless
 * [HistoryScreen]. Kept thin so the presentation layer stays testable.
 */
@Composable
fun HistoryRoute(
    modifier: Modifier = Modifier,
    viewModel: HistoryViewModel = viewModel()
) {
    val uiState by viewModel.uiState.collectAsState()

    HistoryScreen(
        uiState = uiState,
        onFilterSelected = viewModel::onFilterSelected,
        onCalendarClicked = viewModel::onCalendarClicked,
        onShareClicked = viewModel::onShareClicked,
        onSeeMoreClicked = viewModel::onSeeMoreClicked,
        modifier = modifier
    )
}

/**
 * Stateless History screen (Figma node 1:701). Renders the header, filter row
 * and a scrolling list of [QuakeHistoryCard]s. All state and events are hoisted
 * to the caller ([HistoryRoute] / [HistoryViewModel]).
 */
@Composable
fun HistoryScreen(
    uiState: HistoryUiState,
    onFilterSelected: (HistoryFilter) -> Unit,
    onCalendarClicked: () -> Unit,
    onShareClicked: (QuakeHistoryItem) -> Unit,
    onSeeMoreClicked: (QuakeHistoryItem) -> Unit,
    modifier: Modifier = Modifier
) {
    Column(
        modifier = modifier
            .fillMaxSize()
            .padding(horizontal = Dimens.ScreenHorizontalPadding)
    ) {
        QuakeTopBar(isHealthy = uiState.isHealthy)

        QuakeFilterRow(
            selectedFilter = uiState.selectedFilter,
            nearRadiusKm = uiState.nearRadiusKm,
            onFilterSelected = onFilterSelected,
            onCalendarClicked = onCalendarClicked,
            modifier = Modifier.padding(top = Dimens.HeaderSectionGap)
        )

        LazyColumn(
            modifier = Modifier.fillMaxSize(),
            contentPadding = PaddingValues(vertical = Dimens.CardListVerticalPadding),
            verticalArrangement = Arrangement.spacedBy(Dimens.CardListSpacing)
        ) {
            items(items = uiState.items, key = { it.id }) { item ->
                QuakeHistoryCard(
                    item = item,
                    onShareClicked = { onShareClicked(item) },
                    onSeeMoreClicked = { onSeeMoreClicked(item) }
                )
            }
        }
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun HistoryScreenPreview() {
    QuakeAlertTheme {
        HistoryScreen(
            uiState = HistoryUiState(
                items = listOf(
                    QuakeHistoryItem(
                        id = "1",
                        intensity = "VII",
                        severity = MmiSeverity.MODERATE,
                        location = "Bandung, West Java, ID",
                        date = "20 Jun 2026",
                        time = "07:19:18 WIB",
                        distanceLabel = "20 km Away"
                    ),
                    QuakeHistoryItem(
                        id = "2",
                        intensity = "IX",
                        severity = MmiSeverity.SEVERE,
                        location = "Lembang, West Java, ID",
                        date = "16 Jun 2026",
                        time = "04:43:19 WIB",
                        distanceLabel = "60 km Away"
                    )
                )
            ),
            onFilterSelected = {},
            onCalendarClicked = {},
            onShareClicked = {},
            onSeeMoreClicked = {}
        )
    }
}
