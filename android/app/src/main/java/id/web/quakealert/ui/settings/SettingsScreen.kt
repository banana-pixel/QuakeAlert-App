package id.web.quakealert.ui.settings

import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size

import androidx.compose.foundation.layout.wrapContentWidth
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListState
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import android.content.Context
import android.content.Intent
import android.net.Uri
import android.provider.Settings
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalUriHandler
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.tooling.preview.Preview
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import id.web.quakealert.data.UnitSystem
import id.web.quakealert.device.LOCATION_PERMISSIONS
import id.web.quakealert.device.hasLocationPermission
import id.web.quakealert.domain.SafetyPolicy
import id.web.quakealert.domain.ServerConnectionState
import id.web.quakealert.ui.common.QuakeAppBar
import id.web.quakealert.ui.common.QuakeCard
import id.web.quakealert.ui.common.QuakePill
import id.web.quakealert.ui.common.QuakeSwitch
import id.web.quakealert.ui.common.TestAlertSoundDialog
import id.web.quakealert.ui.common.fadingEdges
import id.web.quakealert.ui.sensors.SensorMapCard
import id.web.quakealert.ui.sensors.SensorMapOverview
import id.web.quakealert.ui.theme.CardBorder
import id.web.quakealert.ui.theme.CardSubtitle
import id.web.quakealert.ui.theme.CardSurface
import id.web.quakealert.ui.theme.CardTitle
import id.web.quakealert.ui.theme.ChipLabel
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.MmiRed
import id.web.quakealert.ui.theme.QuakeAlertTheme
import id.web.quakealert.ui.theme.SectionHeaderPillFill
import id.web.quakealert.ui.theme.TextPrimary
import id.web.quakealert.ui.theme.TextSecondary
import kotlinx.coroutines.delay



/**
 * Stateful entry point wiring [SettingsViewModel] to the stateless
 * [SettingsScreen]. Kept thin so the presentation layer stays testable.
 *
 * External-link navigation lives here rather than in the ViewModel: opening a URI
 * needs the composition-local [LocalUriHandler], not app state.
 *
 * @param listState settings-list scroll position, hoisted to
 *   [id.web.quakealert.ui.main.MainScreen] so it survives tab switches, rotation
 *   and process death.
 */
@Composable
fun SettingsRoute(
    connectionState: ServerConnectionState,
    modifier: Modifier = Modifier,
    listState: LazyListState = rememberLazyListState(),
    viewModel: SettingsViewModel = viewModel()
) {
    val uiState by viewModel.uiState.collectAsStateWithLifecycle()
    val context = LocalContext.current

    val uriHandler = LocalUriHandler.current
    val openLink: (String) -> Unit = remember(uriHandler) {
        { url ->
            // AndroidUriHandler throws when nothing on the device can handle the
            // URI (no browser, no mail client). Swallow it so a missing handler
            // leaves the overlay open instead of crashing the app.
            runCatching { uriHandler.openUri(url) }
        }
    }

    // The notification grant and the battery exemption are both changed in system
    // Settings, i.e. while this screen is stopped. Re-read them on every resume so
    // the rows never claim a state the OS has since revoked.
    val lifecycleOwner = LocalLifecycleOwner.current
    DisposableEffect(lifecycleOwner) {
        val observer = LifecycleEventObserver { _, event ->
            if (event == Lifecycle.Event.ON_RESUME) viewModel.refreshSystemState()
        }
        lifecycleOwner.lifecycle.addObserver(observer)
        onDispose { lifecycleOwner.lifecycle.removeObserver(observer) }
    }

    // "Sync Now" is the one place a user who declined at onboarding can recover:
    // without a prompt here the button could only ever report "permission is
    // needed", with nowhere in the app to grant it. A second decline is terminal
    // for the launcher, so the ViewModel's message then points at system Settings.
    val locationPermissionLauncher = rememberLauncherForActivityResult(
        ActivityResultContracts.RequestMultiplePermissions()
    ) { grants ->
        if (grants.values.any { it }) viewModel.onSyncLocationNow()
        else viewModel.onLocationPermissionDenied()
    }
    // Remembered so a recomposition does not hand SettingsScreen a new lambda
    // identity on every frame, the same reason `copy` below is remembered.
    val syncLocation: () -> Unit = remember(context, locationPermissionLauncher) {
        {
            if (context.hasLocationPermission()) viewModel.onSyncLocationNow()
            else locationPermissionLauncher.launch(LOCATION_PERMISSIONS)
        }
    }

    val clipboard = LocalClipboardManager.current
    val copy: (String) -> Unit = remember(clipboard) {
        { value -> clipboard.setText(AnnotatedString(value)) }
    }

    // Local to the Route rather than in SettingsUiState: the modal owns its own
    // playback and there is nothing for the ViewModel to decide or persist about it,
    // so putting it in the state would only widen what a Settings test has to know.
    var showTestAlertSound by remember { mutableStateOf(false) }
    if (showTestAlertSound) {
        TestAlertSoundDialog(onDismissRequest = { showTestAlertSound = false })
    }

    SettingsScreen(
        uiState = uiState,
        connectionState = connectionState,
        onAutoSyncToggled = viewModel::onAutoSyncToggled,
        onSyncLocationNow = syncLocation,
        onNotificationsToggled = viewModel::onNotificationsToggled,
        onTestAlertSound = { showTestAlertSound = true },
        onBatterySettings = { context.openBatteryOptimizationSettings() },
        onFixNotifications = { context.openNotificationSettings() },
        // The same launcher "Sync Now" uses: granting the permission and taking a
        // first fix are one action, and a grant that leaves the position unset would
        // still show the checklist's own consequence.
        onFixLocation = syncLocation,
        onCopyValue = copy,
        onRerollPseudonym = viewModel::onRerollPseudonym,
        onResetProfileRequested = viewModel::onResetProfileRequested,
        onResetProfileConfirmed = viewModel::onResetProfileConfirmed,
        onResetProfileDismissed = viewModel::onResetProfileDismissed,
        onStatusMessageShown = viewModel::onStatusMessageShown,
        onLightModeToggled = viewModel::onLightModeToggled,
        onLanguageSelected = viewModel::onLanguageSelected,
        onUnitSelected = viewModel::onUnitSelected,
        onMoreAboutUs = viewModel::onMoreAboutUs,
        onAboutDismissed = viewModel::onAboutDismissed,
        onGithubClick = { openLink(AboutLinks.GITHUB_PAGES) },
        onEmailClick = { openLink(AboutLinks.EMAIL) },
        onDonateClick = { openLink(AboutLinks.DONATE) },
        listState = listState,
        modifier = modifier
    )
}

/**
 * Opens the app's notification settings, the only place a revoked
 * `POST_NOTIFICATIONS` grant can be restored — a second runtime request is a no-op
 * once the user has denied it twice, so a launcher here would silently do nothing.
 * Falls back to the app's own detail page on a device without the direct screen.
 */
private fun Context.openNotificationSettings() {
    val intents = listOf(
        Intent(Settings.ACTION_APP_NOTIFICATION_SETTINGS)
            .putExtra(Settings.EXTRA_APP_PACKAGE, packageName),
        Intent(
            Settings.ACTION_APPLICATION_DETAILS_SETTINGS,
            Uri.fromParts("package", packageName, null)
        )
    )
    intents.firstNotNullOfOrNull { intent ->
        runCatching { startActivity(intent) }.getOrNull()
    }
}

/**
 * Opens the battery-optimisation screen so the user can exempt the app.
 *
 * Deliberately the *settings list* intent rather than
 * `ACTION_REQUEST_IGNORE_BATTERY_OPTIMIZATIONS`: the direct request dialog is a
 * Play-policy risk outside a narrow set of app categories, and the user reaching
 * this row has already been told why it matters. Falls back to the app's own detail
 * page on a device that ships neither screen.
 */
private fun Context.openBatteryOptimizationSettings() {
    val intents = listOf(
        Intent(Settings.ACTION_IGNORE_BATTERY_OPTIMIZATION_SETTINGS),
        Intent(
            Settings.ACTION_APPLICATION_DETAILS_SETTINGS,
            Uri.fromParts("package", packageName, null)
        )
    )
    intents.firstNotNullOfOrNull { intent ->
        runCatching { startActivity(intent) }.getOrNull()
    }
}

/**
 * Stateless Settings screen ("Settings Page (Fix)", Figma node 1:845). Sections,
 * top → bottom:
 *  1. A static [QuakeAppBar] header ("Settings" + connection badge), plus a status
 *     pill for the result of the last action.
 *  2. "Location & Coverage": the read-only
 *     [ProtectionStatusCardBody] stating the fixed safety rules, "Sync Location
 *     Now" — which now carries the [SensorMapCard] inline, confirming the position
 *     the control produces — and the "Auto Sync Location" switch. There is no radius control — see
 *     [id.web.quakealert.domain.SafetyPolicy] for why that decision is not the
 *     user's to make.
 *  3. "Alert & Notification": the alert switch (which also surfaces a revoked OS
 *     notification permission), "Test Alert Sound", and the "Delivery Checklist" —
 *     the three system prerequisites (notifications, location, Doze exemption)
 *     grouped in one panel because they fail as a set, not one at a time.
 *  4. "Account & Privacy": the anonymous pseudonym and `user_id`, both copyable,
 *     with a reroll and an irreversible profile reset behind a confirmation.
 *  5. "Appearance & Look": "Light Mode (Beta)" and the "Units" / "Language"
 *     controls. Light mode and language are persisted but inert, and badged so.
 *  6. "About": the "More About Us" card raising the [AboutModalDialog] overlay.
 *
 * All state and events are hoisted to the caller ([SettingsRoute] /
 * [SettingsViewModel]), [listState] included.
 */
@Composable
fun SettingsScreen(
    uiState: SettingsUiState,
    connectionState: ServerConnectionState = ServerConnectionState.CONNECTED,
    onAutoSyncToggled: (Boolean) -> Unit,
    onSyncLocationNow: () -> Unit,
    onNotificationsToggled: (Boolean) -> Unit,
    onTestAlertSound: () -> Unit,
    onBatterySettings: () -> Unit,
    onFixNotifications: () -> Unit,
    onFixLocation: () -> Unit,
    onCopyValue: (String) -> Unit,
    onRerollPseudonym: () -> Unit,
    onResetProfileRequested: () -> Unit,
    onResetProfileConfirmed: () -> Unit,
    onResetProfileDismissed: () -> Unit,
    onStatusMessageShown: () -> Unit,
    onLightModeToggled: (Boolean) -> Unit,
    onLanguageSelected: (AppLanguage) -> Unit,
    onUnitSelected: (UnitSystem) -> Unit,
    onMoreAboutUs: () -> Unit,
    onAboutDismissed: () -> Unit,
    onGithubClick: () -> Unit,
    onEmailClick: () -> Unit,
    onDonateClick: () -> Unit,
    modifier: Modifier = Modifier,
    listState: LazyListState = rememberLazyListState()
) {
    Column(
        modifier = modifier
            .fillMaxSize()
            .padding(horizontal = Dimens.ScreenHorizontalPadding)
    ) {
        QuakeAppBar(title = "Settings", connectionState = connectionState)

        LazyColumn(
            state = listState,
            modifier = Modifier
                .weight(1f)
                .fillMaxWidth()
                .fadingEdges(),
            contentPadding = PaddingValues(
                top = Dimens.SettingsHeaderGap,
                bottom = Dimens.SettingsListBottomPadding
            ),
            verticalArrangement = Arrangement.spacedBy(Dimens.SettingsSectionSpacing)
        ) {
            // The outcome of the last action (a sync, a reroll, a reset). Auto-clears
            // so a stale "Location updated" cannot outlive the action it describes.
            uiState.statusMessage?.let { message ->
                item(key = "status") {
                    LaunchedEffect(message) {
                        delay(STATUS_MESSAGE_MS)
                        onStatusMessageShown()
                    }
                    Box(modifier = Modifier.fillMaxWidth(), contentAlignment = Alignment.Center) {
                        QuakePill(text = message)
                    }
                }
            }

            // --- Location & Coverage --------------------------------------
            item(key = "header_location") {
                CenteredSectionBadge(title = "Location & Coverage")
            }

            item(key = "card_protection") {
                QuakeCard(
                    title = "Protection Status",
                    // Read-only on purpose. This card used to hold the radius
                    // slider; what it holds now is the explanation of why there is
                    // nothing to adjust.
                    detail = {
                        ProtectionStatusCardBody(radiusLabel = uiState.alertRadiusLabel)
                    }
                )
            }

            item(key = "card_location") {
                QuakeCard(
                    title = "Sync Location Now",
                    detail = {
                        QuakePill(text = uiState.lastSyncPillLabel)
                        // The map moved inside this card (plan item 9): what a sync
                        // produces is a position, so the confirmation of it belongs
                        // next to the control that asks for one rather than in a
                        // separate card the user has to connect it to. Short, and
                        // without the coverage circle — at 130dp the circle would
                        // fill the frame and say nothing about coverage.
                        SensorMapCard(
                            overview = SensorMapOverview(
                                locationLabel = uiState.locationPillLabel,
                                rangeKm = SafetyPolicy.ALERT_RADIUS_KM,
                                sensorCount = uiState.sensorCount,
                                geofenceFraction = uiState.geofenceFraction,
                                latitude = uiState.latitude,
                                longitude = uiState.longitude
                            ),
                            unitSystem = uiState.unitSystem,
                            showGeofence = false,
                            height = Dimens.MapCardInlineHeight,
                            modifier = Modifier.padding(top = Dimens.SettingCardTitleGap)
                        )
                    }
                ) {
                    if (uiState.isSyncing) {
                        CircularProgressIndicator(
                            color = TextPrimary,
                            strokeWidth = Dimens.BorderMedium,
                            modifier = Modifier.size(Dimens.SyncRefreshIconSize)
                        )
                    } else {
                        SyncRefreshButton(onClick = onSyncLocationNow)
                    }
                }
            }

            item(key = "card_autosync") {
                QuakeCard(title = "Auto Sync Location") {
                    QuakeSwitch(
                        checked = uiState.autoSyncLocation,
                        onCheckedChange = onAutoSyncToggled
                    )
                }
            }


            // --- Alert & Notification -------------------------------------
            item(key = "header_alert") {
                CenteredSectionBadge(title = "Alert & Notification")
            }

            item(key = "card_notifications") {
                QuakeCard(
                    title = "Earthquake Alerts",
                    detail = {
                        // Only shown when the two disagree: the user asked for
                        // alerts and the OS is dropping them.
                        if (uiState.notificationsEnabled && !uiState.notificationPermissionGranted) {
                            QuakePill(text = "Blocked by system settings")
                        }
                    }
                ) {
                    QuakeSwitch(
                        checked = uiState.notificationsEnabled,
                        onCheckedChange = onNotificationsToggled
                    )
                }
            }

            item(key = "card_test_alert") {
                QuakeCard(
                    title = "Test Alert Sound",
                    onClick = onTestAlertSound
                )
            }

            item(key = "card_permissions") {
                QuakeCard(
                    title = "Delivery Checklist",
                    detail = {
                        QuakePill(
                            text = if (uiState.allPermissionsReady) {
                                "All set — alerts can reach you"
                            } else {
                                "${uiState.permissionsReadyCount} of " +
                                    "${uiState.permissionsTotal} ready"
                            }
                        )
                        PermissionsHubCardBody(
                            notificationGranted = uiState.notificationPermissionGranted,
                            locationGranted = uiState.locationPermissionGranted,
                            batteryUnrestricted = uiState.batteryUnrestricted,
                            onFixNotifications = onFixNotifications,
                            onFixLocation = onFixLocation,
                            onFixBattery = onBatterySettings,
                            modifier = Modifier.padding(top = Dimens.SettingCardTitleGap)
                        )
                    }
                )
            }

            // --- Account & Privacy ----------------------------------------
            item(key = "header_account") {
                CenteredSectionBadge(title = "Account & Privacy")
            }

            item(key = "card_identity") {
                QuakeCard(
                    title = "Anonymous Profile",
                    detail = {
                        IdentityRow(
                            label = "Pseudonym",
                            value = uiState.pseudonym,
                            onCopy = onCopyValue
                        )
                        IdentityRow(
                            label = "User ID",
                            value = uiState.userId,
                            onCopy = onCopyValue
                        )
                        SettingsActionButton(
                            label = if (uiState.isRerolling) "Rerolling…" else "Reroll Pseudonym",
                            onClick = onRerollPseudonym,
                            enabled = !uiState.isRerolling
                        )
                        SettingsActionButton(
                            label = if (uiState.isResetting) "Resetting…" else "Reset Profile",
                            onClick = onResetProfileRequested,
                            enabled = !uiState.isResetting,
                            destructive = true
                        )
                    }
                )
            }


            // --- Appearance & Look ----------------------------------------
            item(key = "header_appearance") {
                CenteredSectionBadge(title = "Appearance & Look")
            }

            item(key = "card_light_mode") {
                // Disabled while the app ships dark-theme only: the switch is
                // greyed out and the card carries a "Coming Soon" badge so the
                // control reads as deliberately unavailable rather than broken.
                QuakeCard(
                    title = "Light Mode (Beta)",
                    detail = { QuakePill(text = "Coming Soon") }
                ) {
                    QuakeSwitch(
                        checked = uiState.lightMode,
                        onCheckedChange = onLightModeToggled,
                        enabled = false
                    )
                }
            }

            item(key = "card_units") {
                QuakeCard(title = "Units") {
                    QuakeSegmentedControl(
                        options = UnitSystem.entries,
                        selected = uiState.unitSystem,
                        labelOf = { it.label },
                        onSelect = onUnitSelected
                    )
                }
            }


            item(key = "card_language") {
                QuakeCard(
                    title = "Language",
                    // Persisted but not applied: the strings ship in English only.
                    detail = { QuakePill(text = "Coming Soon") }
                ) {
                    QuakeSegmentedControl(
                        options = AppLanguage.entries,
                        selected = uiState.language,
                        labelOf = { it.label },
                        onSelect = onLanguageSelected
                    )
                }
            }

            // --- About ----------------------------------------------------
            item(key = "header_about") {
                CenteredSectionBadge(title = "About")
            }

            item(key = "card_about") {
                AboutCard(
                    credit = uiState.appCredit,
                    version = uiState.appVersion,
                    onMoreAboutUs = onMoreAboutUs
                )
            }


        }
    }

    // Hosted in its own dialog window, so it overlays the whole Settings screen
    // (nav bar included) without displacing any of the layout above.
    if (uiState.showAboutModal) {
        AboutModalDialog(
            onDismiss = onAboutDismissed,
            onGithubClick = onGithubClick,
            onEmailClick = onEmailClick,
            onDonateClick = onDonateClick
        )
    }

    if (uiState.showResetDialog) {
        ResetProfileDialog(
            onConfirm = onResetProfileConfirmed,
            onDismiss = onResetProfileDismissed
        )
    }
}

/**
 * Confirmation for the one irreversible action in the app.
 *
 * There is no refresh endpoint and no account recovery: resetting mints a *new*
 * `user_id` and pseudonym, and the old identity — including anything posted under it
 * — is unreachable afterwards. The dialog names that consequence instead of asking
 * a generic "are you sure?".
 */
@Composable
private fun ResetProfileDialog(
    onConfirm: () -> Unit,
    onDismiss: () -> Unit
) {
    AlertDialog(
        onDismissRequest = onDismiss,
        containerColor = CardSurface,
        title = { Text(text = "Reset profile?", style = CardTitle) },
        text = {
            Text(
                text = "This creates a brand-new anonymous identity. Your current " +
                    "pseudonym and user ID are discarded and cannot be restored. " +
                    "Your location and alert registration are sent again under the " +
                    "new identity.",
                style = CardSubtitle,
                color = TextSecondary
            )
        },
        confirmButton = {
            TextButton(onClick = onConfirm) {
                Text(text = "Reset", style = ChipLabel, color = MmiRed)
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) {
                Text(text = "Cancel", style = ChipLabel, color = TextPrimary)
            }
        }
    )
}

/** How long a status pill stays up before clearing itself. */
private const val STATUS_MESSAGE_MS = 4_000L

/**
 * Centered section badge ("Location & Coverage", Figma node 1:856). A hug-width
 * #2D2D2D slim stadium capsule, fixed 23dp tall with 14dp horizontal padding and
 * a 1px white-10% stroke, horizontally centered within the list.
 */
@Composable
private fun CenteredSectionBadge(
    title: String,
    modifier: Modifier = Modifier
) {
    val shape = RoundedCornerShape(Dimens.RadiusStadium)
    Box(modifier = modifier.fillMaxWidth(), contentAlignment = Alignment.Center) {
        Box(
            modifier = Modifier
                .wrapContentWidth()
                .height(Dimens.SectionHeaderPillHeight)
                .clip(shape)
                .background(SectionHeaderPillFill, shape)
                .border(Dimens.BorderThin, CardBorder, shape)
                .padding(horizontal = Dimens.SectionHeaderPillPaddingHorizontal),
            contentAlignment = Alignment.Center
        ) {
            Text(text = title, style = CardTitle)
        }
    }
}



@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun SettingsScreenPreview() {
    QuakeAlertTheme {
        SettingsScreen(
            uiState = SettingsUiState(
                locationLabel = "Bandung, West Java, ID",
                sensorCount = 2,
                lastSyncLabel = "2 minutes ago",
                pseudonym = "Quakezen-7B9A",
                userId = "1f0c3a52-9e1d-4a77-8f2b-6d0a1c4e5b90"
            ),
            onAutoSyncToggled = {},
            onSyncLocationNow = {},
            onFixNotifications = {},
            onFixLocation = {},
            onNotificationsToggled = {},
            onTestAlertSound = {},
            onBatterySettings = {},
            onCopyValue = {},
            onRerollPseudonym = {},
            onResetProfileRequested = {},
            onResetProfileConfirmed = {},
            onResetProfileDismissed = {},
            onStatusMessageShown = {},
            onLightModeToggled = {},
            onLanguageSelected = {},
            onUnitSelected = {},
            onMoreAboutUs = {},
            onAboutDismissed = {},
            onGithubClick = {},
            onEmailClick = {},
            onDonateClick = {}
        )
    }
}
