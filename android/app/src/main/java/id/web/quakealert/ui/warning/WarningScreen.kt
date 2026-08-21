package id.web.quakealert.ui.warning

import android.content.Intent
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
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
import id.web.quakealert.domain.ServerConnectionState
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
    connectionState: ServerConnectionState,
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
        connectionState = connectionState,
        onSeeDetails = viewModel::onSeeDetailsClicked,
        onEmergency = viewModel::onEmergencyClicked,
        onDetailDismissed = viewModel::onDetailDismissed,
        onPossibilityDismissed = viewModel::onPossibilityDismissed,
        onShareClicked = shareEvent,
        onRetry = viewModel::onRetry,
        onMuteClick = viewModel::onMuteClick,
        onSosLightClick = viewModel::onSosLightClick,
        listState = listState,
        modifier = modifier
    )
}

/**
 * Stateless Warning screen, rendering whichever of the two [WarningUiState]
 * variants is current.
 *
 * The split is a whole-body swap rather than an overlay, which is the point: during
 * shaking the screen shows the emergency card and nothing else, so there is no
 * scrollable tip list behind it and no dismiss affordance that could put one back.
 * Only the header and the bottom navigation survive the switch — the header because
 * a live alert is the strongest possible evidence the server is healthy, and the
 * navigation because [id.web.quakealert.ui.main.MainScreen] owns it and the user
 * must stay free to reach Chat or Settings mid-quake.
 *
 * **Idle** (Figma 124:1297 / 124:1426), top → bottom, mirroring the Chat/History
 * layout so all tabs share behaviour:
 *  1. the shared [QuakeAppBar] + the summary [AlertBanner] + a short [WarningDivider];
 *  2. a weighted body rendering exactly one of [QuakeLoadingState],
 *     [QuakeErrorState], [QuakeEmptyState] or the [LazyColumn] of preparedness tips,
 *     with the shared soft [fadingEdges] at the scroll bounds. The banner stays
 *     outside the branch: a failed refresh must never blank out an alert the user is
 *     already looking at;
 *  3. a pinned [EmergencyCta], reachable in every state — the emergency route is the
 *     last thing to hide when something goes wrong.
 *
 * **ActiveAlert** (Figma node 1:1043): the header, then [ActiveAlertCard] filling
 * the rest of the column.
 *
 * Two sibling overlays can be raised over the idle state:
 *  - the "Recent Earthquake" event detail (Figma 124:1192) from the active banner's
 *    action, and
 *  - the "Earthquake Possibility" card (Figma 124:1605) from the resting banner's.
 *
 * All state and events are hoisted to the caller ([WarningRoute] /
 * [WarningViewModel]), including [listState] — hoisted to
 * [id.web.quakealert.ui.main.MainScreen] so the scroll position survives tab
 * switches, rotation and process death.
 */
@Composable
fun WarningScreen(
    uiState: WarningUiState,
    connectionState: ServerConnectionState = ServerConnectionState.CONNECTED,
    onSeeDetails: () -> Unit,
    onEmergency: () -> Unit,
    onDetailDismissed: () -> Unit,
    onPossibilityDismissed: () -> Unit,
    onShareClicked: (QuakeHistoryItem) -> Unit,
    onRetry: () -> Unit,
    onMuteClick: () -> Unit,
    onSosLightClick: () -> Unit,
    modifier: Modifier = Modifier,
    listState: LazyListState = rememberLazyListState()
) {
    Column(
        modifier = modifier
            .fillMaxSize()
            .padding(horizontal = Dimens.ScreenHorizontalPadding)
    ) {
        // Rendered once, outside the branch: the header is the one part of the screen
        // the emergency state keeps, unchanged from the resting screen.
        QuakeAppBar(title = "Warning", connectionState = connectionState)

        when (uiState) {
            is WarningUiState.Idle -> IdleBody(
                uiState = uiState,
                onSeeDetails = onSeeDetails,
                onEmergency = onEmergency,
                onRetry = onRetry,
                listState = listState
            )

            is WarningUiState.ActiveAlert -> ActiveAlertCard(
                state = uiState,
                onMuteClick = onMuteClick,
                onSosLightClick = onSosLightClick,
                modifier = Modifier
                    .weight(1f)
                    .fillMaxWidth()
                    .padding(
                        top = Dimens.EmergencyCardTopGap,
                        bottom = Dimens.WarningListBottomPadding
                    )
            )
        }
    }

    val idle = uiState as? WarningUiState.Idle

    // --- "Recent Earthquake" detail overlay (Figma node 124:1192) -------------
    idle?.selectedEventDetails?.let { event ->
        QuakeEventDetailModalDialog(
            event = event,
            unitSystem = uiState.unitSystem,
            onDismiss = onDetailDismissed,
            onShare = { onShareClicked(event) },
            title = "Recent Earthquake"
        )
    }

    // --- "Earthquake Possibility" overlay (Figma node 124:1605) ---------------
    idle?.selectedPossibility?.let { possibility ->
        EarthquakePossibilityModal(
            possibility = possibility,
            onDismiss = onPossibilityDismissed
        )
    }
}

/**
 * The calm/monitoring body: summary banner, divider, the loading/error/empty/tips
 * region and the pinned emergency CTA.
 *
 * A `ColumnScope` extension because the body claims the column's remaining height
 * with `weight(1f)` — keeping that inside the branch is what lets the emergency card
 * claim the same space without either state knowing about the other.
 */
@Composable
private fun ColumnScope.IdleBody(
    uiState: WarningUiState.Idle,
    onSeeDetails: () -> Unit,
    onEmergency: () -> Unit,
    onRetry: () -> Unit,
    listState: LazyListState
) {
    AlertBanner(
        banner = uiState.banner,
        onSeeDetails = onSeeDetails,
        modifier = Modifier.padding(top = Dimens.WarningHeaderGap)
    )

    WarningDivider()

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

    EmergencyCta(
        onClick = onEmergency,
        modifier = Modifier.padding(bottom = Dimens.WarningListBottomPadding)
    )
}

/** Fallback shown when a failed load carried no message of its own. */
private const val GENERIC_ERROR_MESSAGE =
    "Could not reach the alert network. Check your connection and try again."

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun WarningScreenPreview() {
    PreviewWarningScreen(WarningUiState.Idle())
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun WarningScreenActiveQuakePreview() {
    PreviewWarningScreen(
        WarningUiState.Idle(
            banner = ActiveQuakeBanner(
                title = "Recent Earthquake Alert",
                timeAgo = "20 minutes ago",
                intensityLabel = "Intensity : IV (moderate)"
            ),
            sectionTitle = "Stay alert for aftershocks",
            tips = activeQuakeTips()
        )
    )
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun WarningScreenWithDetailPreview() {
    PreviewWarningScreen(
        WarningUiState.Idle(selectedEventDetails = previewActiveAlertDetails)
    )
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun WarningScreenWithPossibilityPreview() {
    PreviewWarningScreen(
        WarningUiState.Idle(selectedPossibility = EarthquakePossibility())
    )
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun WarningScreenLoadingPreview() {
    PreviewWarningScreen(WarningUiState.Idle(isLoading = true))
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun WarningScreenErrorPreview() {
    PreviewWarningScreen(
        WarningUiState.Idle(
            isError = true,
            errorMessage = "Could not reach the alert network. Check your connection and try again."
        )
    )
}

/** The emergency state (Figma node 1:1043) as the whole screen renders it. */
@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun WarningScreenActiveAlertPreview() {
    PreviewWarningScreen(previewActiveAlert)
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun WarningScreenActiveAlertMutedPreview() {
    PreviewWarningScreen(previewActiveAlert.copy(isMuted = true, isSosLightOn = true))
}

/**
 * Shared preview host, so a new callback on [WarningScreen] does not have to be
 * threaded through eight identical preview bodies.
 */
@Composable
private fun PreviewWarningScreen(uiState: WarningUiState) {
    QuakeAlertTheme {
        WarningScreen(
            uiState = uiState,
            onSeeDetails = {},
            onEmergency = {},
            onDetailDismissed = {},
            onPossibilityDismissed = {},
            onShareClicked = {},
            onRetry = {},
            onMuteClick = {},
            onSosLightClick = {}
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
