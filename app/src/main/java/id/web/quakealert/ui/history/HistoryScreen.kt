package id.web.quakealert.ui.history

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
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.composed
import androidx.compose.ui.draw.drawWithContent
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalDensity

import androidx.compose.ui.tooling.preview.Preview
import androidx.lifecycle.viewmodel.compose.viewModel

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
 * Stateless History screen (Figma node 1:701). Structure, top → bottom:
 *  1. A static header [Column] pinned to the top: [QuakeTopBar] + [QuakeFilterRow].
 *  2. A weighted [LazyColumn] filling the remaining space between the filter row
 *     and the bottom navigation bar. Its bounds carry a soft vertical fading
 *     edge so cards dissolve in/out as they enter/leave the scroll area.
 *
 * All state and events are hoisted to the caller ([HistoryRoute] /
 * [HistoryViewModel]).
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
        // --- Static header ---------------------------------------------------
        QuakeTopBar(isHealthy = uiState.isHealthy)

        QuakeFilterRow(
            selectedFilter = uiState.selectedFilter,
            nearRadiusKm = uiState.nearRadiusKm,
            onFilterSelected = onFilterSelected,
            onCalendarClicked = onCalendarClicked,
            modifier = Modifier.padding(top = Dimens.HeaderSectionGap)
        )

        // --- Scrolling list, bounded by the filter row and the bottom nav ---
        LazyColumn(
            modifier = Modifier
                .weight(1f)
                .fillMaxWidth()
                .topFadingEdge(),
            contentPadding = PaddingValues(
                top = Dimens.CardListTopPadding,
                bottom = Dimens.CardListBottomPadding
            ),
            verticalArrangement = Arrangement.spacedBy(Dimens.CardListSpacing)
        ) {
            items(
                items = uiState.items,
                key = { it.id },
                contentType = { "QuakeHistoryCard" }
            ) { item ->
                QuakeHistoryCard(
                    item = item,
                    onShareClicked = { onShareClicked(item) },
                    onSeeMoreClicked = { onSeeMoreClicked(item) }
                )
            }
        }

    }
}

/**
 * Draws a thin soft fade at the top of the list so cards dissolve gently as
 * they scroll up under the header, without eating the last card at the bottom
 * (that edge sits flush against the bottom navigation bar).
 *
 * The gradient [Brush] and its height are hoisted with [remember] so no
 * `Brush`/`Color`/`List` objects are allocated during the draw phase — the
 * lambda passed to [drawWithContent] runs on every scroll frame, so keeping it
 * allocation-free avoids GC churn and frame drops.
 */
private fun Modifier.topFadingEdge(): Modifier = composed {
    val fadeHeightPx = with(LocalDensity.current) { Dimens.ListFadeHeight.toPx() }
    val topBrush = remember(fadeHeightPx) {
        Brush.verticalGradient(
            colors = listOf(Color.Black, Color.Transparent),
            startY = 0f,
            endY = fadeHeightPx
        )
    }
    drawWithContent {
        drawContent()
        drawRect(brush = topBrush, size = size.copy(height = fadeHeightPx))
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
