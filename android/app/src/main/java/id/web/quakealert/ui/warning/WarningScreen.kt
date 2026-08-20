package id.web.quakealert.ui.warning

import android.content.Intent
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
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.tooling.preview.Preview
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import id.web.quakealert.R
import id.web.quakealert.ui.common.QuakeAppBar
import id.web.quakealert.ui.common.QuakeEmptyState
import id.web.quakealert.ui.common.QuakeErrorState
import id.web.quakealert.ui.common.QuakeEventDetailModalDialog
import id.web.quakealert.ui.common.QuakeLoadingState
import id.web.quakealert.ui.common.fadingEdges
import id.web.quakealert.ui.history.MmiSeverity
import id.web.quakealert.ui.history.QuakeHistoryItem
import id.web.quakealert.ui.history.toShareText
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.QuakeAlertTheme
import id.web.quakealert.ui.theme.SectionTitle

/**
 * Stateful entry point that connects [WarningViewModel] to the stateless
 * [WarningScreen]. Kept thin so the presentation layer stays testable.
 *
 * Sharing lives here rather than in the ViewModel: launching a chooser needs the
 * composition-local [LocalContext], not app state — the same split HistoryRoute
 * makes for its detail overlay's "Share" action.
 */
@Composable
fun WarningRoute(
    modifier: Modifier = Modifier,
    listState: LazyListState = rememberLazyListState(),
    viewModel: WarningViewModel = viewModel()
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

    WarningScreen(
        uiState = uiState,
        onSeeDetails = viewModel::onSeeDetailsClicked,
        onEmergency = viewModel::onEmergencyClicked,
        onDetailDismissed = viewModel::onDetailDismissed,
        onPossibilityDismissed = viewModel::onPossibilityDismissed,
        onShareClicked = shareEvent,
        onRetry = viewModel::onRetry,
        listState = listState,
        modifier = modifier
    )
}

/**
 * Stateless Warning screen (Figma nodes 124:1297 / 124:1426 / 124:1605).
 * Structure, top → bottom, mirrors the Chat/History layout so all tabs share
 * behaviour:
 *  1. A static header [Column] pinned to the top: shared [QuakeAppBar] + the
 *     active [AlertBanner] + a short [WarningDivider].
 *  2. A weighted body rendering exactly one of [QuakeLoadingState],
 *     [QuakeErrorState], [QuakeEmptyState] or the [LazyColumn] carrying the
 *     state-driven section title and the tip rows, with the shared soft
 *     [fadingEdges] at the scroll bounds. The banner deliberately stays outside
 *     the branch: a failed refresh must never blank out an alert the user is
 *     already looking at.
 *  3. A pinned [EmergencyCta] at the bottom, which stays reachable in every state
 *     — the emergency route is the last thing to hide when something goes wrong.
 *
 * Two sibling overlays can be raised, mirroring [WarningUiState]:
 *  - the "Recent Earthquake" event detail (Figma 124:1192) from the active
 *    banner's action, and
 *  - the "Earthquake Possibility" card (Figma 124:1605) from the resting
 *    banner's action.
 *
 * All state and events are hoisted to the caller ([WarningRoute] /
 * [WarningViewModel]), including [listState] — hoisted to
 * [id.web.quakealert.ui.main.MainScreen] so the scroll position survives tab
 * switches, rotation and process death.
 */
@Composable
fun WarningScreen(
    uiState: WarningUiState,
    onSeeDetails: () -> Unit,
    onEmergency: () -> Unit,
    onDetailDismissed: () -> Unit,
    onPossibilityDismissed: () -> Unit,
    onShareClicked: (QuakeHistoryItem) -> Unit,
    onRetry: () -> Unit,
    modifier: Modifier = Modifier,
    listState: LazyListState = rememberLazyListState()
) {
    Column(
        modifier = modifier
            .fillMaxSize()
            .padding(horizontal = Dimens.ScreenHorizontalPadding)
    ) {
        // --- Static header: title + alert banner + divider -------------------
        QuakeAppBar(title = "Warning", isHealthy = uiState.isHealthy)

        AlertBanner(
            banner = uiState.banner,
            onSeeDetails = onSeeDetails,
            modifier = Modifier.padding(top = Dimens.WarningHeaderGap)
        )

        WarningDivider()

        // --- Body: loading / error / empty / preparedness tips ---------------
        val bodyModifier = Modifier
            .weight(1f)
            .fillMaxWidth()

        when {
            uiState.isLoading -> QuakeLoadingState(
                modifier = bodyModifier,
                message = "Checking the alert network..."
            )

            uiState.isError -> QuakeErrorState(
                message = uiState.errorMessage ?: GENERIC_ERROR_MESSAGE,
                onRetry = onRetry,
                modifier = bodyModifier
            )

            uiState.tips.isEmpty() -> QuakeEmptyState(
                icon = R.drawable.ic_nav_warning,
                message = "No Guidance Available",
                subtitle = "Preparedness guidance for your area will appear here.",
                modifier = bodyModifier
            )

            else -> LazyColumn(
                state = listState,
                modifier = bodyModifier.fadingEdges(),
                contentPadding = PaddingValues(
                    top = Dimens.PrepSectionGap,
                    bottom = Dimens.WarningListBottomPadding
                ),
                verticalArrangement = Arrangement.spacedBy(Dimens.PrepTipSpacing)
            ) {
                item(key = "prep-title") {
                    Text(
                        text = uiState.sectionTitle,
                        style = SectionTitle,
                        modifier = Modifier.fillMaxWidth()
                    )
                }
                items(
                    items = uiState.tips,
                    key = { it.id }
                ) { tip ->
                    PrepTipRow(tip = tip)
                }
            }
        }

        // --- Pinned emergency CTA --------------------------------------------
        EmergencyCta(
            onClick = onEmergency,
            modifier = Modifier.padding(bottom = Dimens.WarningListBottomPadding)
        )
    }

    // --- "Recent Earthquake" detail overlay (Figma node 124:1192) -------------
    uiState.selectedEventDetails?.let { event ->
        QuakeEventDetailModalDialog(
            event = event,
            unitSystem = uiState.unitSystem,
            onDismiss = onDetailDismissed,
            onShare = { onShareClicked(event) },
            title = "Recent Earthquake"
        )
    }

    // --- "Earthquake Possibility" overlay (Figma node 124:1605) ---------------
    uiState.selectedPossibility?.let { possibility ->
        EarthquakePossibilityModal(
            possibility = possibility,
            onDismiss = onPossibilityDismissed
        )
    }
}

/** Fallback shown when a failed load carried no message of its own. */
private const val GENERIC_ERROR_MESSAGE =
    "Could not reach the alert network. Check your connection and try again."

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun WarningScreenPreview() {
    QuakeAlertTheme {
        WarningScreen(
            uiState = WarningUiState(),
            onSeeDetails = {},
            onEmergency = {},
            onDetailDismissed = {},
            onPossibilityDismissed = {},
            onShareClicked = {},
            onRetry = {}
        )
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun WarningScreenActiveQuakePreview() {
    QuakeAlertTheme {
        WarningScreen(
            uiState = WarningUiState(
                banner = ActiveQuakeBanner(
                    title = "Recent Earthquake Alert",
                    timeAgo = "20 minutes ago",
                    intensityLabel = "Intensity : IV (moderate)"
                ),
                sectionTitle = "Stay alert for aftershocks",
                tips = activeQuakeTips()
            ),
            onSeeDetails = {},
            onEmergency = {},
            onDetailDismissed = {},
            onPossibilityDismissed = {},
            onShareClicked = {},
            onRetry = {}
        )
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun WarningScreenWithDetailPreview() {
    QuakeAlertTheme {
        WarningScreen(
            uiState = WarningUiState(
                selectedEventDetails = previewActiveAlertDetails
            ),
            onSeeDetails = {},
            onEmergency = {},
            onDetailDismissed = {},
            onPossibilityDismissed = {},
            onShareClicked = {},
            onRetry = {}
        )
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun WarningScreenWithPossibilityPreview() {
    QuakeAlertTheme {
        WarningScreen(
            uiState = WarningUiState(
                selectedPossibility = EarthquakePossibility()
            ),
            onSeeDetails = {},
            onEmergency = {},
            onDetailDismissed = {},
            onPossibilityDismissed = {},
            onShareClicked = {},
            onRetry = {}
        )
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun WarningScreenLoadingPreview() {
    QuakeAlertTheme {
        WarningScreen(
            uiState = WarningUiState(isLoading = true),
            onSeeDetails = {},
            onEmergency = {},
            onDetailDismissed = {},
            onPossibilityDismissed = {},
            onShareClicked = {},
            onRetry = {}
        )
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun WarningScreenErrorPreview() {
    QuakeAlertTheme {
        WarningScreen(
            uiState = WarningUiState(
                isError = true,
                errorMessage = "Could not reach the alert network. Check your connection and try again."
            ),
            onSeeDetails = {},
            onEmergency = {},
            onDetailDismissed = {},
            onPossibilityDismissed = {},
            onShareClicked = {},
            onRetry = {}
        )
    }
}

/** Shared preview fixture for the Warning detail overlay previews. */
private val previewActiveAlertDetails = QuakeHistoryItem(
    id = "active-alert",
    intensity = "XI",
    severity = MmiSeverity.SEVERE,
    location = "Lembang, West Java, ID",
    date = "20 Jun 2026",
    time = "07:19:18 WIB",
    distanceKm = 60,
    relativeTime = "2 months ago",
    pgaLabel = "61.5 gal",
    durationLabel = "7 sec",
    coordinates = "41.40338, 2.17403"
)
