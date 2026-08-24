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
import androidx.core.net.toUri
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import id.web.quakealert.R
import id.web.quakealert.data.network.ServerHealth
import id.web.quakealert.ui.common.QuakeAppBar
import id.web.quakealert.ui.common.QuakeEmptyState
import id.web.quakealert.ui.common.GenericErrorCopy
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
    health: ServerHealth,
    onOpenUpdates: () -> Unit = {},
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
                    putExtra(Intent.EXTRA_SUBJECT, "QuakeAlert: ${item.location}")
                    putExtra(Intent.EXTRA_TEXT, item.toShareText(uiState.unitSystem))
                }
                context.startActivity(Intent.createChooser(send, "Share earthquake details"))
            }
        }
    }

    val dialNumber: (String) -> Unit = remember(context) {
        { number ->
            // ACTION_DIAL, never ACTION_CALL: it needs no CALL_PHONE permission and it
            // shows the number before it rings, which is what you want from a button
            // that can be hit by accident while the ground is moving. runCatching
            // because a device with no dialler at all must not crash the alert screen.
            runCatching {
                context.startActivity(Intent(Intent.ACTION_DIAL, "tel:$number".toUri()))
            }
        }
    }

    WarningScreen(
        uiState = uiState,
        health = health,
        onOpenUpdates = onOpenUpdates,
        onSeeDetails = viewModel::onSeeDetailsClicked,
        onEmergency = viewModel::onEmergencyClicked,
        onDetailDismissed = viewModel::onDetailDismissed,
        onActivityDismissed = viewModel::onActivityDismissed,
        onProtectionStatus = viewModel::onProtectionStatusClicked,
        onProtectionStatusDismissed = viewModel::onProtectionStatusDismissed,
        onEmergencyInfoDismissed = viewModel::onEmergencyInfoDismissed,
        onDial = dialNumber,
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
 *  1. the shared [QuakeAppBar], a [WarningOfflineNotice] while the link is down or a
 *     load failed, the summary [AlertBanner] and a short [WarningDivider];
 *  2. a weighted body rendering exactly one of [QuakeLoadingState], the [LazyColumn]
 *     of preparedness tips, [QuakeErrorState] or [QuakeEmptyState], with the shared
 *     soft [fadingEdges] at the scroll bounds. The banner stays outside the branch: a
 *     failed refresh must never blank out an alert the user is already looking at, and
 *     for the same reason the tips outrank the error card — they are held locally, so
 *     a dead network is precisely when they are the only thing left that works;
 *  3. a pinned [EmergencyCta], reachable in every state — the emergency route is the
 *     last thing to hide when something goes wrong.
 *
 * **ActiveAlert** (Figma node 1:1043): the header, then [ActiveAlertCard] filling
 * the rest of the column.
 *
 * Two sibling overlays can be raised over the idle state:
 *  - the "Recent Earthquake" event detail (Figma 124:1192) from the active banner's
 *    action, and
 *  - the "Recent Seismic Activity" card (Figma 124:1605) from the resting banner's,
 *    and
 *  - "Protection Status", from the info affordance beside the banner title.
 *
 * All state and events are hoisted to the caller ([WarningRoute] /
 * [WarningViewModel]), including [listState] — hoisted to
 * [id.web.quakealert.ui.main.MainScreen] so the scroll position survives tab
 * switches, rotation and process death.
 */
@Composable
fun WarningScreen(
    uiState: WarningUiState,
    health: ServerHealth = ServerHealth.HEALTHY,
    onOpenUpdates: () -> Unit = {},
    onSeeDetails: () -> Unit,
    onEmergency: () -> Unit,
    onDetailDismissed: () -> Unit,
    onActivityDismissed: () -> Unit,
    onProtectionStatus: () -> Unit,
    onProtectionStatusDismissed: () -> Unit,
    onEmergencyInfoDismissed: () -> Unit,
    onDial: (String) -> Unit,
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
        QuakeAppBar(title = "Warning", health = health, onUpdatesClicked = onOpenUpdates)

        when (uiState) {
            is WarningUiState.Idle -> IdleBody(
                uiState = uiState,
                health = health,
                onSeeDetails = onSeeDetails,
                onEmergency = onEmergency,
                onRetry = onRetry,
                onProtectionStatus = onProtectionStatus,
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

    // --- "Recent Seismic Activity" overlay (Figma node 124:1605) --------------
    idle?.selectedActivity?.let { activity ->
        RecentSeismicActivityModal(
            activity = activity,
            unitSystem = uiState.unitSystem,
            onDismiss = onActivityDismissed
        )
    }

    // --- "Protection Status" overlay ------------------------------------------
    // Raised here rather than from Settings: the question it answers ("will this
    // warn me?") is asked on this screen, and the radius it quotes is fixed policy,
    // so the flag carries no payload.
    if (idle?.isProtectionStatusOpen == true) {
        ProtectionStatusModalDialog(
            radiusLabel = uiState.alertRadiusLabel,
            onDismiss = onProtectionStatusDismissed
        )
    }

    // --- "Emergency Steps & Contacts" overlay ---------------------------------
    // Raised from the pinned CTA, which is reachable in every idle sub-state. Its
    // contents are resolved by the ViewModel at open time, so the numbers belong to
    // the network the phone is on right now.
    idle?.emergencyInfo?.let { info ->
        EmergencyInfoModalDialog(
            info = info,
            onDial = onDial,
            onDismiss = onEmergencyInfoDismissed
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
    health: ServerHealth,
    onSeeDetails: () -> Unit,
    onEmergency: () -> Unit,
    onRetry: () -> Unit,
    onProtectionStatus: () -> Unit,
    listState: LazyListState
) {
    // Reported at the top and nowhere else. A load that failed and a link that is
    // down are one thing to the user — "what you see below may be stale" — so they
    // share one strip; the load's own message wins when there is one, being the more
    // specific of the two. CONNECTING is left out: it is the normal first second of
    // every cold start, and a notice that always flashes on launch trains the user to
    // ignore the one that matters.
    val notice = when {
        // Suppressed during the first load, where the spinner is already saying
        // "we are asking" and a notice would only pre-announce a failure.
        uiState.isLoading -> null
        uiState.isError -> (uiState.errorCopy ?: GenericErrorCopy).message
        health == ServerHealth.OFFLINE -> OFFLINE_MESSAGE
        else -> null
    }
    if (notice != null) {
        WarningOfflineNotice(
            message = notice,
            onRetry = onRetry,
            modifier = Modifier.padding(top = Dimens.WarningHeaderGap)
        )
    }

    AlertBanner(
        banner = uiState.banner,
        onSeeDetails = onSeeDetails,
        onProtectionStatus = onProtectionStatus,
        // The header gap belongs to whichever element is first: with the notice above
        // it, the banner only needs to clear it, not the header a second time.
        modifier = Modifier.padding(
            top = if (notice != null) Dimens.StateTextGap else Dimens.WarningHeaderGap
        )
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

        // Tips outrank the error card deliberately, and this is the whole point of
        // plan item 5: the list defaults to a locally-held set, so it is real content
        // that owes nothing to the network. The failure is already stated in the notice
        // above; blanking the guidance to say it a second time would cost the user the
        // only part of this screen that still works.
        uiState.tips.isNotEmpty() -> LazyColumn(
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

        // Reached only when there is genuinely nothing to show, which is where the
        // full-body cards belong: the error variant when the failure is why the list
        // is empty, the empty variant when the list simply came back empty.
        uiState.isError -> QuakeErrorState(
            copy = uiState.errorCopy ?: GenericErrorCopy,
            onRetry = onRetry,
            modifier = bodyModifier
        )

        else -> QuakeEmptyState(
            icon = R.drawable.ic_nav_warning,
            message = "No Guidance Available",
            subtitle = "Preparedness guidance for your area will appear here.",
            modifier = bodyModifier
        )
    }

    EmergencyCta(
        onClick = onEmergency,
        modifier = Modifier.padding(bottom = Dimens.WarningListBottomPadding)
    )
}

/**
 * Shown while the backend link is down but nothing has failed outright. Names what
 * still works rather than only what does not: the guidance below this notice is held
 * locally, and a user reading it during a quake needs to know it is trustworthy.
 */
private const val OFFLINE_MESSAGE =
    "Offline: alerts are paused. The guidance below works without a connection."

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
private fun WarningScreenWithActivityPreview() {
    PreviewWarningScreen(
        WarningUiState.Idle(selectedActivity = previewSeismicActivity)
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
            errorCopy = GenericErrorCopy
        )
    )
}

/** Offline with the tips intact — the state plan item 5 exists to protect. */
@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun WarningScreenOfflinePreview() {
    PreviewWarningScreen(
        uiState = WarningUiState.Idle(),
        health = ServerHealth.OFFLINE
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
private fun PreviewWarningScreen(
    uiState: WarningUiState,
    health: ServerHealth = ServerHealth.HEALTHY
) {
    QuakeAlertTheme {
        WarningScreen(
            uiState = uiState,
            health = health,
            onSeeDetails = {},
            onEmergency = {},
            onDetailDismissed = {},
            onActivityDismissed = {},
            onProtectionStatus = {},
            onProtectionStatusDismissed = {},
            onEmergencyInfoDismissed = {},
            onDial = {},
            onShareClicked = {},
            onRetry = {},
            onMuteClick = {},
            onSosLightClick = {}
        )
    }
}

/** Shared preview fixture for the "Recent Seismic Activity" overlay preview. */
private val previewSeismicActivity = RecentSeismicActivity(
    locationLabel = "-6.91750, 107.61910",
    availability = ActivityAvailability.MEASURED,
    eventCount = 3,
    mostRecent = "IV (moderate), 2 days ago",
    strongest = "V (strong), 61.5 gal",
    latitude = -6.91750,
    longitude = 107.61910
)

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
    reportingNodesLabel = "3 stations",
    coordinates = "41.40338, 2.17403",
    latitude = 41.40338,
    longitude = 2.17403
)
