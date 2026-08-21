package id.web.quakealert.ui.history

import android.content.Intent
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListState
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.tooling.preview.Preview
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import id.web.quakealert.R
import id.web.quakealert.domain.ServerConnectionState
import id.web.quakealert.ui.common.QuakeAppBar
import id.web.quakealert.ui.common.QuakeEmptyState
import id.web.quakealert.ui.common.QuakeErrorState
import id.web.quakealert.ui.common.QuakeEventDetailModalDialog
import id.web.quakealert.ui.common.QuakeFilter
import id.web.quakealert.ui.common.QuakeFilterRow
import id.web.quakealert.ui.common.QuakeLoadingState
import id.web.quakealert.ui.common.fadingEdges
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.TextPrimary
import id.web.quakealert.ui.theme.QuakeAlertTheme

/**
 * Stateful entry point that connects [HistoryViewModel] to the stateless
 * [HistoryScreen]. Kept thin so the presentation layer stays testable.
 *
 * Sharing lives here rather than in the ViewModel: launching a chooser needs the
 * composition-local [LocalContext], not app state. The same lambda serves the list
 * card's share button and the detail overlay's "Share" action, so both emit the
 * identical [toShareText] payload.
 */
@Composable
fun HistoryRoute(
    connectionState: ServerConnectionState,
    modifier: Modifier = Modifier,
    listState: LazyListState = rememberLazyListState(),
    viewModel: HistoryViewModel = viewModel()
) {
    val uiState by viewModel.uiState.collectAsStateWithLifecycle()

    val context = LocalContext.current
    val shareEvent: (QuakeHistoryItem) -> Unit = remember(context) {
        { item ->
            // startActivity throws when no app on the device can receive the
            // intent. Swallow it so a missing target leaves the screen (and any
            // open overlay) exactly as it was instead of crashing the app.
            runCatching {
                val send = Intent(Intent.ACTION_SEND).apply {
                    type = "text/plain"
                    putExtra(Intent.EXTRA_SUBJECT, "QuakeAlert — ${item.location}")
                    putExtra(Intent.EXTRA_TEXT, item.toShareText(uiState.unitSystem))
                }
                context.startActivity(Intent.createChooser(send, "Share earthquake details"))
            }
        }
    }

    HistoryScreen(
        uiState = uiState,
        connectionState = connectionState,
        onFilterSelected = viewModel::onFilterSelected,
        onShareClicked = shareEvent,
        onSeeMoreClicked = viewModel::onSeeMoreClicked,
        onDetailDismissed = viewModel::onDetailDismissed,
        onRetry = viewModel::onRetry,
        onLoadMore = viewModel::onLoadMore,
        listState = listState,
        modifier = modifier
    )
}

/**
 * Stateless History screen (Figma node 1:701). Structure, top → bottom:
 *  1. A static header [Column] pinned to the top: shared [QuakeAppBar] +
 *     [QuakeFilterRow].
 *  2. A weighted body filling the remaining space between the filter row and the
 *     bottom navigation bar. It renders exactly one of four states, driven by
 *     [HistoryUiState]: [QuakeLoadingState], [QuakeErrorState], [QuakeEmptyState]
 *     or the [LazyColumn] of cards, whose bounds carry a soft vertical fading edge
 *     (shared [fadingEdges]) so cards dissolve in/out at the scroll bounds.
 *     Keeping the header outside the branch means the title, badge and filters stay
 *     put as the state changes instead of the whole screen flashing.
 *  3. The [QuakeEventDetailModalDialog] overlay (Figma node 123:743), raised whenever
 *     [HistoryUiState.selectedEvent] is non-null. It is a sibling of the list
 *     rather than a child so the dialog window is never affected by the list's
 *     scroll state or fade.
 *
 * The header and fade are shared with the Sensors screen via [ui.common] so both
 * screens stay visually and behaviourally consistent. All state and events are
 * hoisted to the caller ([HistoryRoute] / [HistoryViewModel]), including
 * [listState] — hoisted to [id.web.quakealert.ui.main.MainScreen] so the scroll
 * position survives tab switches, rotation and process death.
 */
@Composable
fun HistoryScreen(
    uiState: HistoryUiState,
    connectionState: ServerConnectionState = ServerConnectionState.CONNECTED,
    onFilterSelected: (QuakeFilter) -> Unit,
    onShareClicked: (QuakeHistoryItem) -> Unit,
    onSeeMoreClicked: (QuakeHistoryItem) -> Unit,
    onDetailDismissed: () -> Unit,
    onRetry: () -> Unit,
    onLoadMore: () -> Unit = {},
    modifier: Modifier = Modifier,
    listState: LazyListState = rememberLazyListState()
) {
    Column(
        modifier = modifier
            .fillMaxSize()
            .padding(horizontal = Dimens.ScreenHorizontalPadding)
    ) {
        // --- Static header ---------------------------------------------------
        QuakeAppBar(title = "History", connectionState = connectionState)

        QuakeFilterRow(
            selectedFilter = uiState.selectedFilter,
            nearRadiusKm = uiState.nearRadiusKm,
            unitSystem = uiState.unitSystem,
            onFilterSelected = onFilterSelected,
            modifier = Modifier.padding(top = Dimens.HeaderSectionGap)
        )

        // Paginate from the scroll position rather than from the last item's
        // composition: derivedStateOf recomputes only when the trailing index
        // crosses the threshold, so onLoadMore is called once per page instead of
        // once per frame. The ViewModel guards the duplicate case regardless.
        val shouldLoadMore by remember(listState, uiState.items.size) {
            derivedStateOf {
                val lastVisible = listState.layoutInfo.visibleItemsInfo.lastOrNull()?.index ?: -1
                uiState.items.isNotEmpty() &&
                    lastVisible >= uiState.items.size - LOAD_MORE_THRESHOLD
            }
        }
        LaunchedEffect(shouldLoadMore) {
            if (shouldLoadMore) onLoadMore()
        }

        // --- Body: loading / error / empty / content -------------------------
        val bodyModifier = Modifier
            .weight(1f)
            .fillMaxWidth()

        when {
            uiState.isLoading -> QuakeLoadingState(
                modifier = bodyModifier,
                message = "Loading earthquake history..."
            )

            uiState.isError -> QuakeErrorState(
                message = uiState.errorMessage ?: GENERIC_ERROR_MESSAGE,
                onRetry = onRetry,
                modifier = bodyModifier
            )

            uiState.items.isEmpty() -> QuakeEmptyState(
                icon = R.drawable.ic_nav_history,
                message = "No Earthquake History",
                subtitle = "Events detected near your coverage area will appear here.",
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
                    items = uiState.items,
                    key = { it.id },
                    contentType = { "QuakeHistoryCard" }
                ) { item ->
                    QuakeHistoryCard(
                        item = item,
                        unitSystem = uiState.unitSystem,
                        onShareClicked = { onShareClicked(item) },
                        onSeeMoreClicked = { onSeeMoreClicked(item) }
                    )
                }

                // Spinner only while a page is actually in flight: a permanent
                // footer would read as "more exists" even on the last page.
                if (uiState.isLoadingMore) {
                    item(key = "history-load-more", contentType = "QuakeHistoryFooter") {
                        Box(
                            modifier = Modifier
                                .fillMaxWidth()
                                .padding(vertical = Dimens.CardListSpacing),
                            contentAlignment = Alignment.Center
                        ) {
                            CircularProgressIndicator(
                                modifier = Modifier.size(Dimens.SyncRefreshIconSize),
                                color = TextPrimary,
                                strokeWidth = Dimens.BorderMedium
                            )
                        }
                    }
                }
            }
        }
    }

    // --- Earthquake Details overlay (Figma node 123:743) ---------------------
    uiState.selectedEvent?.let { event ->
        QuakeEventDetailModalDialog(
            event = event,
            unitSystem = uiState.unitSystem,
            onDismiss = onDetailDismissed,
            onShare = { onShareClicked(event) }
        )
    }
}

/**
 * How close to the end of the loaded pages the scroll must come before the next
 * page is requested — far enough that the request usually completes before the
 * user reaches the bottom.
 */
private const val LOAD_MORE_THRESHOLD = 3

/** Fallback shown when a failed load carried no message of its own. */
private const val GENERIC_ERROR_MESSAGE =
    "Could not load earthquake history. Check your connection and try again."

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun HistoryScreenPreview() {
    QuakeAlertTheme {
        HistoryScreen(
            uiState = HistoryUiState(items = previewItems),
            onFilterSelected = {},
            onShareClicked = {},
            onSeeMoreClicked = {},
            onDetailDismissed = {},
            onRetry = {}
        )
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun HistoryScreenWithDetailPreview() {
    QuakeAlertTheme {
        HistoryScreen(
            uiState = HistoryUiState(
                items = previewItems,
                selectedEvent = previewItems.last()
            ),
            onFilterSelected = {},
            onShareClicked = {},
            onSeeMoreClicked = {},
            onDetailDismissed = {},
            onRetry = {}
        )
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun HistoryScreenLoadingPreview() {
    QuakeAlertTheme {
        HistoryScreen(
            uiState = HistoryUiState(isLoading = true),
            onFilterSelected = {},
            onShareClicked = {},
            onSeeMoreClicked = {},
            onDetailDismissed = {},
            onRetry = {}
        )
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun HistoryScreenEmptyPreview() {
    QuakeAlertTheme {
        HistoryScreen(
            uiState = HistoryUiState(),
            onFilterSelected = {},
            onShareClicked = {},
            onSeeMoreClicked = {},
            onDetailDismissed = {},
            onRetry = {}
        )
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun HistoryScreenErrorPreview() {
    QuakeAlertTheme {
        HistoryScreen(
            uiState = HistoryUiState(
                isError = true,
                errorMessage = "Could not load earthquake history. Check your connection and try again."
            ),
            onFilterSelected = {},
            onShareClicked = {},
            onSeeMoreClicked = {},
            onDetailDismissed = {},
            onRetry = {}
        )
    }
}

/** Shared preview fixture for both History previews. */
private val previewItems = listOf(
    QuakeHistoryItem(
        id = "1",
        intensity = "VII",
        severity = MmiSeverity.MODERATE,
        location = "Bandung, West Java, ID",
        date = "20 Jun 2026",
        time = "07:19:18 WIB",
        distanceKm = 20,
        relativeTime = "2 months ago",
        pgaLabel = "61.5 gal",
        durationLabel = "7 sec",
        coordinates = "-6.91750, 107.61910"
    ),
    QuakeHistoryItem(
        id = "2",
        intensity = "IX",
        severity = MmiSeverity.SEVERE,
        location = "Lembang, West Java, ID",
        date = "16 Jun 2026",
        time = "04:43:19 WIB",
        distanceKm = 60,
        relativeTime = "2 months ago",
        pgaLabel = "142.0 gal",
        durationLabel = "23 sec",
        coordinates = "-6.81180, 107.61760"
    )
)
