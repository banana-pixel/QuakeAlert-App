package id.web.quakealert.ui.addsensor

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.platform.LocalSoftwareKeyboardController
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import id.web.quakealert.R
import id.web.quakealert.ui.common.ErrorCopy
import id.web.quakealert.ui.common.LocationPickerMap
import id.web.quakealert.ui.common.MapFocus
import id.web.quakealert.ui.common.QuakeConfirmDialog
import id.web.quakealert.ui.common.QuakeModalActionButton
import id.web.quakealert.ui.common.QuakeModalCard
import id.web.quakealert.ui.common.QuakeModalHairline
import id.web.quakealert.ui.common.QuakeModalHeader
import id.web.quakealert.ui.common.QuakeModalPanel
import id.web.quakealert.ui.common.QuakePageIndicator
import id.web.quakealert.ui.settings.SyncRefreshButton
import id.web.quakealert.ui.theme.AccentBlue
import id.web.quakealert.ui.theme.BackgroundGradientBottom
import id.web.quakealert.ui.theme.DestructiveActionFill
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.MetricLabel
import id.web.quakealert.ui.theme.MetricValue
import id.web.quakealert.ui.theme.ModalCardBorder
import id.web.quakealert.ui.theme.QuakeAlertTheme
import id.web.quakealert.ui.theme.SuccessGreen
import id.web.quakealert.ui.theme.TextPrimary
import id.web.quakealert.ui.theme.TextSecondary
import id.web.quakealert.ui.theme.WizardActionLabel
import id.web.quakealert.ui.theme.WizardAlertFill
import id.web.quakealert.ui.theme.WizardBadgeFill
import id.web.quakealert.ui.theme.WizardBodyText
import id.web.quakealert.ui.theme.WizardConfirmActionFill
import id.web.quakealert.ui.theme.WizardHeadline
import id.web.quakealert.ui.theme.WizardPanelStroke
import id.web.quakealert.ui.theme.WizardProcessingFill
import id.web.quakealert.ui.theme.WizardStatusText
import id.web.quakealert.ui.theme.WizardSyncChipFill
import kotlinx.coroutines.delay

/**
 * The add-a-sensor wizard as a modal over whatever screen launched it, drawn to
 * Figma 155:985 ... 155:1572.
 *
 * The card itself, the inset panels, the hairlines, the action capsules and the
 * discard question are the app's shared overlay vocabulary
 * ([QuakeModalCard], [QuakeModalPanel], [QuakeModalHairline],
 * [QuakeModalActionButton], [QuakeConfirmDialog]), so this file holds only what is
 * genuinely specific to provisioning a sensor. Nothing here computes copy either:
 * every sentence comes from AddSensorCopy.kt, which is what keeps transport text off
 * the screen.
 *
 * Structure top to bottom, exactly as drawn: header ("Add a Sensor" and close), the
 * "Step N" badge, the step headline, the step body, the failure panel when there is
 * one, the quiet helper paragraph, the animated page indicator, then the action row.
 * Only the middle scrolls; the header and the action row stay put, so the primary
 * action never walks off the bottom of a long step.
 */
@Composable
fun AddSensorWizardDialog(
    state: AddSensorState,
    onDismiss: () -> Unit,
    onStartClicked: () -> Unit,
    onSyncLocationClick: () -> Unit,
    onMapPinMoved: (Double, Double) -> Unit,
    onLocationNameChanged: (String) -> Unit,
    onLocationContinue: () -> Unit,
    onSecretRevealed: () -> Unit,
    onCopySecret: () -> Boolean,
    onCredentialsContinue: () -> Unit,
    onRescanNetworks: () -> Unit,
    onNetworkSelected: (String) -> Unit,
    onPasswordChanged: (String) -> Unit,
    onWlanContinue: () -> Unit,
    onCheckNow: () -> Unit,
    onBack: () -> Unit,
    onExitCancelled: () -> Unit,
    onRequestExit: () -> Unit,
    modifier: Modifier = Modifier
) {
    val busy = state.isBusy
    // A ceiling rather than a fixed height: the card grows with its step but always
    // leaves the launching screen visible at the edges, which is what tells the user
    // this is an overlay they can leave.
    val maxCardHeight = LocalConfiguration.current.screenHeightDp.dp *
        Dimens.WizardCardHeightFraction

    QuakeModalCard(
        onDismissRequest = onRequestExit,
        dismissOnBackPress = !busy,
        modifier = modifier
            .padding(horizontal = Dimens.ScreenHorizontalPadding)
            .heightIn(max = maxCardHeight)
    ) {
        QuakeModalHeader(title = "Add a Sensor", onDismiss = onRequestExit)

        Box(modifier = Modifier.weight(1f, fill = false)) {
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .verticalScroll(rememberScrollState()),
                horizontalAlignment = Alignment.CenterHorizontally
            ) {
                StepBadge(state.currentStep)

                // Welcome carries its own title in the body art, so no headline here.
                if (state.currentStep != AddSensorWizardStep.WELCOME) {
                    Text(
                        text = state.currentStep.headline(),
                        style = WizardHeadline,
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(top = Dimens.WizardSectionGap)
                    )
                }

                Spacer(Modifier.height(Dimens.WizardSectionGap))

                when (state.currentStep) {
                    AddSensorWizardStep.WELCOME -> WelcomeBody()

                    AddSensorWizardStep.LOCATION -> LocationBody(
                        state = state,
                        onSyncLocationClick = onSyncLocationClick,
                        onMapPinMoved = onMapPinMoved,
                        onLocationNameChanged = onLocationNameChanged
                    )

                    AddSensorWizardStep.CREDENTIALS -> CredentialsBody(
                        state = state,
                        onSecretRevealed = onSecretRevealed,
                        onCopySecret = onCopySecret
                    )

                    AddSensorWizardStep.WLAN -> WlanBody(
                        state = state,
                        onRescanNetworks = onRescanNetworks,
                        onNetworkSelected = onNetworkSelected,
                        onPasswordChanged = onPasswordChanged
                    )

                    AddSensorWizardStep.FINISHING -> FinishingBody(
                        state = state,
                        onCheckNow = onCheckNow
                    )

                    AddSensorWizardStep.RATE_LIMIT -> RateLimitBody()
                }

                // Everything the step is currently complaining about, in the order
                // the user meets it: the field rule first, then the situation.
                InlineNote(state.detailsError?.message())
                InlineNote(state.linkError?.message())
                state.failure?.let { FailurePanel(failureCopy(it)) }

                HelperText(state.currentStep.helperText())
            }

            if (busy) ProcessingCover(modifier = Modifier.matchParentSize())
        }

        PageIndicatorRow(state.currentStep)

        ActionRow(
            state = state,
            onStartClicked = onStartClicked,
            onLocationContinue = onLocationContinue,
            onCredentialsContinue = onCredentialsContinue,
            onWlanContinue = onWlanContinue,
            onDismiss = onDismiss,
            onBack = onBack
        )
    }

    // Asked in its own card on top of the wizard, not inside it: the question is
    // about the whole session, so it must not scroll away with one step's body.
    if (state.showingExitConfirm) {
        QuakeConfirmDialog(
            title = "Discard sensor setup?",
            message = "This setup is not finished. Leaving now discards it, and the " +
                "credentials on screen cannot be shown again.",
            confirmLabel = "Discard",
            dismissLabel = "Keep going",
            onConfirm = onDismiss,
            onDismiss = onExitCancelled
        )
    }
}

// ============================================================
// Shared chrome
// ============================================================

/** "Step N" capsule; welcome shows none and the rate limit shows "Error". */
@Composable
private fun StepBadge(step: AddSensorWizardStep, modifier: Modifier = Modifier) {
    val label = when (step) {
        AddSensorWizardStep.WELCOME -> null
        AddSensorWizardStep.LOCATION -> "Step 1"
        AddSensorWizardStep.CREDENTIALS -> "Step 2"
        AddSensorWizardStep.WLAN -> "Step 3"
        AddSensorWizardStep.FINISHING -> "Step 4"
        AddSensorWizardStep.RATE_LIMIT -> "Error"
    } ?: return

    val shape = RoundedCornerShape(Dimens.SectionHeaderPillRadius)
    Box(
        modifier = modifier
            .padding(top = Dimens.WizardSectionGap)
            .clip(shape)
            .background(WizardBadgeFill, shape)
            .border(Dimens.BorderMedium, WizardPanelStroke, shape)
            .padding(
                horizontal = Dimens.WizardBadgePaddingHorizontal,
                vertical = Dimens.WizardFieldPaddingVertical / 2
            ),
        contentAlignment = Alignment.Center
    ) {
        Text(text = label, style = WizardActionLabel)
    }
}

@Composable
private fun PageIndicatorRow(step: AddSensorWizardStep) {
    Box(
        modifier = Modifier
            .fillMaxWidth()
            .padding(vertical = Dimens.WizardSectionGap),
        contentAlignment = Alignment.Center
    ) {
        QuakePageIndicator(pageCount = 5, currentPage = step.indicatorIndex())
    }
}

/** Bottom action row; contents follow the screen exactly as the design draws them. */
@Composable
private fun ActionRow(
    state: AddSensorState,
    onStartClicked: () -> Unit,
    onLocationContinue: () -> Unit,
    onCredentialsContinue: () -> Unit,
    onWlanContinue: () -> Unit,
    onDismiss: () -> Unit,
    onBack: () -> Unit
) {
    val busy = state.isBusy
    when (state.currentStep) {
        AddSensorWizardStep.WELCOME ->
            QuakeModalActionButton(
                label = "Start",
                filled = false,
                onClick = onStartClicked,
                enabled = !busy,
                modifier = Modifier.fillMaxWidth()
            )

        AddSensorWizardStep.LOCATION,
        AddSensorWizardStep.CREDENTIALS,
        AddSensorWizardStep.WLAN ->
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(Dimens.WizardActionGap)
            ) {
                QuakeModalActionButton(
                    label = "Back",
                    filled = false,
                    onClick = onBack,
                    enabled = !busy,
                    modifier = Modifier.weight(1f)
                )
                QuakeModalActionButton(
                    label = "Next",
                    enabled = !busy && when (state.currentStep) {
                        AddSensorWizardStep.LOCATION -> state.locationStepValid
                        AddSensorWizardStep.CREDENTIALS -> true
                        else -> state.linkValid
                    },
                    onClick = when (state.currentStep) {
                        AddSensorWizardStep.LOCATION -> onLocationContinue
                        AddSensorWizardStep.CREDENTIALS -> onCredentialsContinue
                        else -> onWlanContinue
                    },
                    modifier = Modifier.weight(1f)
                )
            }

        // The node is configured by the time this screen shows, so leaving is safe
        // whether or not it has checked in yet; it keeps trying on its own.
        AddSensorWizardStep.FINISHING ->
            QuakeModalActionButton(
                label = "Finish",
                onClick = onDismiss,
                modifier = Modifier.fillMaxWidth()
            )

        AddSensorWizardStep.RATE_LIMIT ->
            QuakeModalActionButton(
                label = "Exit",
                onClick = onDismiss,
                container = DestructiveActionFill,
                modifier = Modifier.fillMaxWidth()
            )
    }
}

/**
 * Cover shown while a step talks to the network. Over the body only, so the header
 * and the disabled action row still read as context rather than as breakage.
 */
@Composable
private fun ProcessingCover(modifier: Modifier = Modifier) {
    val shape = RoundedCornerShape(Dimens.RadiusCard)
    Column(
        modifier = modifier
            .clip(shape)
            .background(WizardProcessingFill, shape)
            .border(Dimens.BorderThin, ModalCardBorder, shape)
            .padding(Dimens.WizardEmphasisPadding),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.Center
    ) {
        GlyphDisc(R.drawable.ic_loading_spinner)
        Spacer(Modifier.height(Dimens.WizardPanelGap))
        Text(text = "Processing, please hang tight...", style = WizardStatusText)
    }
}

/** Dark disc behind a wizard glyph, as the design draws every icon pair. */
@Composable
private fun GlyphDisc(iconRes: Int, tint: Color = TextPrimary) {
    val shape = RoundedCornerShape(Dimens.RadiusStadium)
    Box(
        modifier = Modifier
            .size(Dimens.WizardGlyphDisc)
            .clip(shape)
            .background(Color.Black, shape),
        contentAlignment = Alignment.Center
    ) {
        Icon(
            painter = painterResource(id = iconRes),
            contentDescription = null,
            tint = tint,
            modifier = Modifier.size(Dimens.WizardGlyphIcon)
        )
    }
}

/** The design's two-glyph art row, used by Welcome, Finishing and the rate limit. */
@Composable
private fun GlyphPair(secondIcon: Int, secondTint: Color = TextPrimary) {
    Row(horizontalArrangement = Arrangement.spacedBy(Dimens.WizardGlyphGap)) {
        GlyphDisc(R.drawable.ic_cpu_chip)
        GlyphDisc(secondIcon, tint = secondTint)
    }
}

// ============================================================
// Step 0 - WELCOME (155:985)
// ============================================================

@Composable
private fun WelcomeBody() {
    Column(horizontalAlignment = Alignment.CenterHorizontally) {
        GlyphPair(R.drawable.ic_wifi_signal)
        Text(
            text = "Welcome to QuakeAlert Sensor Wizard!",
            style = WizardHeadline,
            modifier = Modifier
                .fillMaxWidth()
                .padding(top = Dimens.WizardSectionGap)
        )
        Text(
            text = "You are going to add a new device to the QuakeAlert Network, for " +
                "further info you can visit Sensor Guide.\n\nWhen you are ready, start " +
                "the sensor addition process with the button below.",
            style = WizardBodyText,
            modifier = Modifier
                .fillMaxWidth()
                .padding(top = Dimens.WizardPanelGap)
        )
    }
}

// ============================================================
// Step 1 - LOCATION (155:1123)
// ============================================================

/** Street-level framing so a pan moves the pin by blocks, not by provinces. */
private const val ZOOM_PICK = 13.0

@Composable
private fun LocationBody(
    state: AddSensorState,
    onSyncLocationClick: () -> Unit,
    onMapPinMoved: (Double, Double) -> Unit,
    onLocationNameChanged: (String) -> Unit
) {
    Column(
        verticalArrangement = Arrangement.spacedBy(Dimens.WizardSectionGap),
        horizontalAlignment = Alignment.CenterHorizontally
    ) {
        val latitude = state.latitude
        val longitude = state.longitude
        val shape = RoundedCornerShape(Dimens.RadiusCard)

        Box(
            modifier = Modifier
                .fillMaxWidth()
                .height(Dimens.WizardMapHeight)
                .clip(shape)
                .border(Dimens.BorderThin, ModalCardBorder, shape)
        ) {
            if (latitude != null && longitude != null) {
                LocationPickerMap(
                    focus = MapFocus(latitude = latitude, longitude = longitude, zoom = ZOOM_PICK),
                    onCenterSettled = onMapPinMoved,
                    modifier = Modifier.matchParentSize()
                )
                // The pin is the centre of the frame, not a marker on the map: what
                // the user reads as "here" and what the camera reports can then
                // never disagree, however the map is panned or zoomed.
                CentrePin(modifier = Modifier.align(Alignment.Center))
            } else {
                // No position anywhere yet, so there is nothing honest to centre on.
                Box(
                    modifier = Modifier
                        .matchParentSize()
                        .background(WizardProcessingFill),
                    contentAlignment = Alignment.Center
                ) {
                    Text(
                        text = if (state.isSyncingLocation) {
                            "Finding your location..."
                        } else {
                            "Tap sync to put your location on the map."
                        },
                        style = WizardBodyText.copy(color = TextSecondary),
                        modifier = Modifier.padding(horizontal = Dimens.WizardEmphasisPadding)
                    )
                }
            }

            // Bottom right inside the map frame, as the design draws it, and out of
            // the way of the thumb that is panning.
            SyncChip(
                isSyncing = state.isSyncingLocation,
                onClick = onSyncLocationClick,
                modifier = Modifier
                    .align(Alignment.BottomEnd)
                    .padding(Dimens.WizardMapChipInset)
            )
        }

        PlaceAndCoordinatesPanel(
            detectedName = state.detectedLocationName,
            editedName = state.locationName,
            latitude = latitude,
            longitude = longitude,
            onNameChanged = onLocationNameChanged
        )
    }
}

/** The fixed centre marker: a white ring with a coloured core, drawn in Compose. */
@Composable
private fun CentrePin(modifier: Modifier = Modifier) {
    val shape = RoundedCornerShape(Dimens.RadiusStadium)
    Box(
        modifier = modifier
            .size(Dimens.WizardPinSize)
            .clip(shape)
            .background(AccentBlue.copy(alpha = 0.35f), shape)
            .border(Dimens.BorderMedium, TextPrimary, shape),
        contentAlignment = Alignment.Center
    ) {
        Box(
            modifier = Modifier
                .size(Dimens.WizardPinCoreSize)
                .clip(shape)
                .background(TextPrimary, shape)
        )
    }
}

@Composable
private fun SyncChip(
    isSyncing: Boolean,
    onClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    val shape = RoundedCornerShape(Dimens.SegmentPillRadius)
    Box(
        modifier = modifier
            .width(Dimens.WizardSyncChipWidth)
            .height(Dimens.WizardSyncChipHeight)
            .clip(shape)
            .background(WizardSyncChipFill, shape)
            .border(Dimens.BorderMedium, WizardPanelStroke, shape),
        contentAlignment = Alignment.Center
    ) {
        // The button owns the tap; the chip is only its container, so there is one
        // clickable here and not two competing for the same pixels.
        SyncRefreshButton(onClick = onClick, isSyncing = isSyncing)
    }
}

/**
 * "Detected City Name" and "Your Current Coordinates".
 *
 * The name is one always-present text field rather than a label that swaps into an
 * editor: an editor that only exists after a tap cannot be focused by that same tap
 * reliably, which is how the keyboard ended up never opening. Now the whole row,
 * pencil included, simply asks the field for focus and asks for the keyboard, so the
 * first tap anywhere on the row starts typing.
 *
 * The geocoder's name is the field's placeholder, not its value, so the label keeps
 * following the pin until the user types something of their own, and clearing the
 * field means an empty name instead of a silent revert.
 */
@Composable
private fun PlaceAndCoordinatesPanel(
    detectedName: String?,
    editedName: String,
    latitude: Double?,
    longitude: Double?,
    onNameChanged: (String) -> Unit
) {
    val focusRequester = remember { FocusRequester() }
    val focusManager = LocalFocusManager.current
    val keyboard = LocalSoftwareKeyboardController.current
    val beginEditing: () -> Unit = {
        focusRequester.requestFocus()
        // Asked for explicitly: a focus request alone does not reopen the keyboard
        // once it has been dismissed while the field kept focus.
        keyboard?.show()
    }

    QuakeModalPanel {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .clip(RoundedCornerShape(Dimens.RadiusSmall))
                .clickable(role = Role.Button, onClick = beginEditing)
        ) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically
            ) {
                Text(
                    text = "Detected City Name :",
                    style = MetricLabel,
                    modifier = Modifier.weight(1f)
                )
                Icon(
                    painter = painterResource(R.drawable.ic_edit),
                    contentDescription = null,
                    tint = TextSecondary,
                    modifier = Modifier.size(Dimens.WizardEditGlyphSize)
                )
            }
            BasicTextField(
                value = editedName,
                onValueChange = onNameChanged,
                singleLine = true,
                textStyle = MetricValue,
                cursorBrush = SolidColor(TextPrimary),
                keyboardOptions = KeyboardOptions(imeAction = ImeAction.Done),
                keyboardActions = KeyboardActions(onDone = { focusManager.clearFocus() }),
                modifier = Modifier
                    .fillMaxWidth()
                    .focusRequester(focusRequester),
                decorationBox = { field ->
                    if (editedName.isEmpty()) {
                        Text(
                            text = detectedName ?: "Tap to enter a place name",
                            style = MetricValue,
                            color = TextSecondary,
                            maxLines = 1
                        )
                    }
                    field()
                }
            )
        }

        QuakeModalHairline()

        Column(modifier = Modifier.fillMaxWidth()) {
            Text(text = "Your Current Coordinates :", style = MetricLabel)
            Text(
                text = latitude?.let { lat ->
                    longitude?.let { lon -> "%.5f, %.5f".format(lat, lon) }
                } ?: "Not available",
                style = MetricValue.copy(fontWeight = FontWeight.Black)
            )
        }
    }
}

// ============================================================
// Step 2 - CREDENTIALS (155:1219)
// ============================================================

/** How long the Copy chip reads "Copied" before flipping back. */
private const val COPIED_RESET_MS = 2_000L

@Composable
private fun CredentialsBody(
    state: AddSensorState,
    onSecretRevealed: () -> Unit,
    onCopySecret: () -> Boolean
) {
    val node = state.provisioned ?: return
    var copied by remember { mutableStateOf(false) }
    LaunchedEffect(copied) {
        if (copied) {
            delay(COPIED_RESET_MS)
            copied = false
        }
    }

    QuakeModalPanel {
        Column(modifier = Modifier.fillMaxWidth()) {
            Text(text = "Station ID", style = MetricLabel)
            Text(text = node.stationId, style = MetricValue, maxLines = 1)
        }

        QuakeModalHairline()

        Row(
            modifier = Modifier.fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically
        ) {
            Column(modifier = Modifier.weight(1f)) {
                Text(text = "Provisioning Secrets", style = MetricLabel)
                if (state.secretRevealed) {
                    // Display once: the server keeps only ciphertext of this.
                    Text(text = node.provisioningSecret, style = MetricValue, maxLines = 2)
                } else {
                    Text(
                        text = "Show secret",
                        style = MetricValue.copy(textDecoration = TextDecoration.Underline),
                        modifier = Modifier.clickable(role = Role.Button, onClick = onSecretRevealed)
                    )
                }
            }
            if (state.secretRevealed) {
                ChipButton(
                    label = if (copied) "Copied" else "Copy",
                    onClick = { copied = onCopySecret() }
                )
            }
        }
    }
}

/** The design's small "Choose" / "Copy" micro-button. */
@Composable
private fun ChipButton(label: String, onClick: () -> Unit) {
    val shape = RoundedCornerShape(Dimens.RadiusSmall)
    Box(
        modifier = Modifier
            .width(Dimens.WizardChipWidth)
            .height(Dimens.WizardChipHeight)
            .clip(shape)
            .background(WizardBadgeFill, shape)
            .border(Dimens.BorderMedium, WizardPanelStroke, shape)
            .clickable(role = Role.Button, onClick = onClick),
        contentAlignment = Alignment.Center
    ) {
        Text(text = label, style = MetricValue)
    }
}

// ============================================================
// Step 3 - WLAN (155:1287)
// ============================================================

@Composable
private fun WlanBody(
    state: AddSensorState,
    onRescanNetworks: () -> Unit,
    onNetworkSelected: (String) -> Unit,
    onPasswordChanged: (String) -> Unit
) {
    Column(verticalArrangement = Arrangement.spacedBy(Dimens.WizardSectionGap)) {
        QuakeModalPanel {
            Text(text = "Networks Detected by Sensor :", style = MetricLabel)

            if (state.scannedSsids.isEmpty()) {
                Text(
                    text = "None found yet. Rescan once the sensor has finished starting up.",
                    style = WizardBodyText.copy(color = TextSecondary)
                )
            } else {
                state.scannedSsids.forEachIndexed { index, ssid ->
                    SsidRow(
                        ssid = ssid,
                        selected = ssid == state.selectedSsid,
                        onSelected = { onNetworkSelected(ssid) }
                    )
                    if (index < state.scannedSsids.lastIndex) QuakeModalHairline()
                }
            }

            QuakeModalActionButton(
                label = "Rescan",
                container = WizardConfirmActionFill,
                onClick = onRescanNetworks,
                enabled = !state.isBusy,
                modifier = Modifier.fillMaxWidth()
            )
        }

        QuakeModalPanel {
            Text(text = "WLAN Password (empty if open network) :", style = MetricLabel)
            PasswordField(password = state.wifiPassword, onPasswordChanged = onPasswordChanged)
        }
    }
}

@Composable
private fun SsidRow(ssid: String, selected: Boolean, onSelected: () -> Unit) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .height(Dimens.WizardSsidRowHeight)
            // The row is the choice, so the whole row is the target; the chip is
            // the label for what a tap will do.
            .clickable(role = Role.Button, onClick = onSelected),
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalAlignment = Alignment.CenterVertically
    ) {
        Text(
            text = ssid,
            style = MetricValue.copy(fontWeight = FontWeight.Black),
            maxLines = 1,
            modifier = Modifier.weight(1f)
        )
        ChipButton(label = if (selected) "Chosen" else "Choose", onClick = onSelected)
    }
}

@Composable
private fun PasswordField(password: String, onPasswordChanged: (String) -> Unit) {
    val shape = RoundedCornerShape(Dimens.RadiusSmall)
    BasicTextField(
        value = password,
        onValueChange = onPasswordChanged,
        singleLine = true,
        textStyle = MetricValue,
        cursorBrush = SolidColor(TextPrimary),
        keyboardOptions = KeyboardOptions(
            keyboardType = KeyboardType.Password,
            imeAction = ImeAction.Done
        ),
        decorationBox = { inner ->
            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .clip(shape)
                    .background(WizardBadgeFill, shape)
                    .border(Dimens.BorderMedium, WizardPanelStroke, shape)
                    .padding(
                        horizontal = Dimens.WizardFieldPaddingHorizontal,
                        vertical = Dimens.WizardFieldPaddingVertical
                    )
            ) {
                if (password.isEmpty()) {
                    Text(
                        text = "Enter password...",
                        style = MetricValue.copy(color = TextSecondary)
                    )
                }
                inner()
            }
        },
        modifier = Modifier.fillMaxWidth()
    )
}

// ============================================================
// Step 4 - FINISHING (155:1397 / 155:1518)
// ============================================================

@Composable
private fun FinishingBody(
    state: AddSensorState,
    onCheckNow: () -> Unit
) {
    val stationId = state.effectiveStationId ?: state.provisioned?.stationId ?: ""
    val shape = RoundedCornerShape(Dimens.RadiusCard)

    Column(verticalArrangement = Arrangement.spacedBy(Dimens.WizardSectionGap)) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .clip(shape)
                .background(WizardProcessingFill, shape)
                .border(Dimens.BorderThin, ModalCardBorder, shape)
                .padding(Dimens.WizardEmphasisPadding),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(Dimens.WizardGlyphGap)
        ) {
            when (state.confirmState) {
                ConfirmState.WAITING -> GlyphPair(R.drawable.ic_loading_spinner)
                ConfirmState.PENDING,
                ConfirmState.ONLINE ->
                    GlyphPair(R.drawable.ic_check_circle, secondTint = SuccessGreen)
            }
            Text(
                text = when (state.confirmState) {
                    ConfirmState.WAITING -> "Processing, please hang tight..."
                    ConfirmState.PENDING -> "Configured. Your sensor is awaiting verification."
                    ConfirmState.ONLINE -> "Your sensor is online."
                },
                style = WizardStatusText
            )
        }

        QuakeModalPanel {
            Text(text = "Station ID", style = MetricLabel)
            Text(text = stationId, style = MetricValue, maxLines = 1)
        }

        if (state.confirmState == ConfirmState.WAITING) {
            QuakeModalActionButton(
                label = "Check Now",
                container = WizardConfirmActionFill,
                onClick = onCheckNow,
                modifier = Modifier.fillMaxWidth()
            )
        }
    }
}

// ============================================================
// Rate limit screen (155:1572)
// ============================================================

@Composable
private fun RateLimitBody() {
    val shape = RoundedCornerShape(Dimens.RadiusCard)
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .clip(shape)
            .background(WizardAlertFill, shape)
            .border(Dimens.BorderThin, ModalCardBorder, shape)
            .padding(Dimens.WizardEmphasisPadding),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(Dimens.WizardGlyphGap)
    ) {
        GlyphPair(R.drawable.ic_alert_hexagon)
        Text(
            text = "You have added sensors as often as the network allows for now. " +
                "Try again in a few hours.",
            style = WizardStatusText
        )
    }
}

// ============================================================
// Notes and helper copy
// ============================================================

/** Quiet paragraph under the step body; never carries anything actionable. */
@Composable
private fun HelperText(text: String) {
    if (text.isEmpty()) return
    Text(
        text = text,
        style = WizardBodyText.copy(color = TextSecondary),
        modifier = Modifier
            .fillMaxWidth()
            .padding(top = Dimens.WizardSectionGap)
    )
}

/** One-line rule feedback under the body, for a field the user can fix in place. */
@Composable
private fun InlineNote(message: String?) {
    if (message == null) return
    Text(
        text = message,
        style = WizardBodyText,
        modifier = Modifier
            .fillMaxWidth()
            .padding(top = Dimens.WizardPanelGap)
    )
}

/**
 * The failure panel from 155:1518: a red wash, what happened, and what to do about
 * it. The text comes from [failureCopy], so this composable has no way to render a
 * server sentence or an exception even if one reached the ViewModel.
 */
@Composable
private fun FailurePanel(copy: ErrorCopy) {
    val shape = RoundedCornerShape(Dimens.RadiusSmall)
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .padding(top = Dimens.WizardSectionGap)
            .clip(shape)
            .background(WizardAlertFill, shape)
            .border(Dimens.BorderMedium, WizardPanelStroke, shape)
            .padding(
                horizontal = Dimens.WizardPanelPaddingHorizontal,
                vertical = Dimens.WizardPanelPaddingVertical
            ),
        verticalArrangement = Arrangement.spacedBy(Dimens.WizardFieldPaddingVertical / 2)
    ) {
        Text(text = copy.title, style = WizardActionLabel)
        Text(text = copy.message, style = WizardBodyText)
    }
}

// ============================================================
// Preview
// ============================================================

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun AddSensorWizardWelcomePreview() {
    QuakeAlertTheme {
        Box(Modifier.fillMaxSize().background(BackgroundGradientBottom))
        AddSensorWizardDialog(
            state = AddSensorState(),
            onDismiss = {},
            onStartClicked = {},
            onSyncLocationClick = {},
            onMapPinMoved = { _, _ -> },
            onLocationNameChanged = {},
            onLocationContinue = {},
            onSecretRevealed = {},
            onCopySecret = { true },
            onCredentialsContinue = {},
            onRescanNetworks = {},
            onNetworkSelected = {},
            onPasswordChanged = {},
            onWlanContinue = {},
            onCheckNow = {},
            onBack = {},
            onExitCancelled = {},
            onRequestExit = {}
        )
    }
}
