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
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import id.web.quakealert.R
import id.web.quakealert.domain.ProvisionedNode
import id.web.quakealert.ui.common.MapFocus
import id.web.quakealert.ui.common.MapMarker
import id.web.quakealert.ui.common.MapMarkerKind
import id.web.quakealert.ui.common.QuakeModalHeader
import id.web.quakealert.ui.common.QuakePageIndicator
import id.web.quakealert.ui.common.QuakePrimaryButton
import id.web.quakealert.ui.common.QuakeSecondaryButton
import id.web.quakealert.ui.theme.AccentBlue
import id.web.quakealert.ui.theme.BackgroundGradientBottom
import id.web.quakealert.ui.theme.CardBorder
import id.web.quakealert.ui.theme.CardTitle
import id.web.quakealert.ui.theme.CardSubtitle
import id.web.quakealert.ui.theme.CardSurface
import id.web.quakealert.ui.theme.ChatInputFill
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.NunitoFontFamily
import id.web.quakealert.ui.theme.TextPrimary
import id.web.quakealert.ui.theme.TextSecondary

/**
 * The add-a-sensor wizard as a modal window over whatever screen launched it
 * (Figma language of nodes 1:845 / 1:1081; processing popup per node 152:970).
 *
 * Presentation rules:
 *  - One [Dialog] window, platform-width disabled so the card can breathe; the
 *    platform scrim keeps the launching screen dimmed behind it.
 *  - Header is the shared [QuakeModalHeader]; its close button is THE exit, and
 *    exiting from a non-terminal step asks once inside the card ("progress will
 *    be lost") instead of silently discarding typed data - or instead of closing
 *    mid-network-operation, which is refused outright ([state.isBusy]).
 *  - Steps advance under onboarding's animated [QuakePageIndicator], with the
 *    shared CTA capsules in the footer.
 *  - While any network step runs, an in-card [ProcessingCard] covers the body:
 *    the exact chrome of Figma 152:970 (black fill, white-10% stroke, cpu chip,
 *    "Processing, please hang tight…").
 */
@Composable
fun AddSensorWizardDialog(
    state: AddSensorState,
    onDismiss: () -> Unit,
    onNameChanged: (String) -> Unit,
    onModelChanged: (String) -> Unit,
    onUseCurrentPosition: (Double, Double) -> Unit,
    onManualLatitudeChanged: (String) -> Unit,
    onManualLongitudeChanged: (String) -> Unit,
    onDetailsContinue: () -> Unit,
    onRetryFromDetails: () -> Unit,
    onSecretRevealed: () -> Unit,
    onCredentialsContinue: () -> Unit,
    onRescanClicked: () -> Unit,
    onSsidSelected: (String) -> Unit,
    onPasswordChanged: (String) -> Unit,
    onConfigureNode: () -> Unit,
    onRefreshConfirm: () -> Unit,
    modifier: Modifier = Modifier
) {
    // Mid-flow exit asks once; DETAILS has nothing to lose and ONLINE is done.
    var confirmingExit by remember { mutableStateOf(false) }
    val busy = state.isBusy

    fun requestExit() {
        if (busy) return // never strand a half-written node behind a mis-tap
        if (state.step == WizardStep.DETAILS || state.confirmState == ConfirmState.ONLINE) {
            onDismiss()
        } else {
            confirmingExit = true
        }
    }

    Dialog(
        onDismissRequest = ::requestExit,
        properties = DialogProperties(
            usePlatformDefaultWidth = false,
            dismissOnClickOutside = false,
            dismissOnBackPress = !busy
        )
    ) {
        val shape = RoundedCornerShape(Dimens.SettingCardRadius)
        Column(
            modifier = modifier
                .fillMaxWidth()
                .padding(horizontal = Dimens.ScreenHorizontalPadding)
                .clip(shape)
                .background(CardSurface)
                .border(Dimens.BorderThin, CardBorder, shape)
                .imePadding()
                .padding(vertical = Dimens.SettingCardPaddingVertical)
        ) {
            Box(modifier = Modifier.weight(1f, fill = false)) {
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .verticalScroll(rememberScrollState())
                        .padding(horizontal = Dimens.ScreenHorizontalPadding),
                    horizontalAlignment = Alignment.CenterHorizontally
                ) {
                    QuakeModalHeader(
                        title = "Add a Sensor",
                        onDismiss = ::requestExit,
                        modifier = Modifier.padding(top = Dimens.HeaderSectionGap)
                    )

                    QuakePageIndicator(
                        pageCount = WizardStep.entries.size,
                        currentPage = WizardStep.entries.indexOf(state.step),
                        modifier = Modifier.padding(top = Dimens.HeaderSectionGap)
                    )

                    if (confirmingExit) {
                        ExitConfirmStrip(
                            onStay = { confirmingExit = false },
                            onExit = onDismiss,
                            modifier = Modifier.padding(top = Dimens.HeaderSectionGap)
                        )
                    }

                    when (state.step) {
                        WizardStep.DETAILS -> DetailsStep(
                            state = state,
                            onNameChanged = onNameChanged,
                            onModelChanged = onModelChanged,
                            onUseCurrentPosition = onUseCurrentPosition,
                            onManualLatitudeChanged = onManualLatitudeChanged,
                            onManualLongitudeChanged = onManualLongitudeChanged
                        )

                        WizardStep.CREDENTIALS -> CredentialsStep(state, onSecretRevealed)

                        WizardStep.LINK -> LinkStep(
                            state = state,
                            onSsidSelected = onSsidSelected,
                            onPasswordChanged = onPasswordChanged,
                            onRescanClicked = onRescanClicked
                        )

                        WizardStep.CONFIRM -> ConfirmStep(state, onRefreshConfirm)
                    }

                    state.errorMessage?.let { message ->
                        ErrorNote(message)
                    }

                    Spacer(Modifier.height(Dimens.HeaderSectionGap))
                }

                if (busy) {
                    ProcessingOverlay(modifier = Modifier.matchParentSize())
                }
            }

            WizardFooter(
                state = state,
                onDismiss = onDismiss,
                onDetailsContinue = onDetailsContinue,
                onCredentialsContinue = onCredentialsContinue,
                onConfigureNode = onConfigureNode,
                modifier = Modifier.padding(horizontal = Dimens.ScreenHorizontalPadding)
            )
        }
    }
}

// ============================================================
// Processing popup (Figma 152:970)
// ============================================================

/**
 * In-card busy cover: black fill, white-10% stroke, the cpu-chip glyph and the
 * one line the design gives it. Covers only the wizard's body so the footer's
 * disabled state stays visible context for why nothing is tappable.
 */
@Composable
private fun ProcessingOverlay(modifier: Modifier = Modifier) {
    val shape = RoundedCornerShape(14.dp)
    Column(
        modifier = modifier
            .padding(Dimens.SettingCardPaddingHorizontal)
            .clip(shape)
            .background(androidx.compose.ui.graphics.Color.Black)
            .border(Dimens.BorderThin, CardBorder, shape)
            .padding(25.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.Center
    ) {
        Icon(
            painter = painterResource(R.drawable.ic_cpu_chip),
            contentDescription = null,
            tint = TextPrimary,
            modifier = Modifier.size(50.dp)
        )
        Spacer(Modifier.height(12.dp))
        Text(
            text = "Processing, please hang tight...",
            color = TextPrimary,
            fontFamily = NunitoFontFamily,
            fontWeight = FontWeight.Bold,
            fontSize = 16.sp,
            lineHeight = 24.sp
        )
    }
}

// ============================================================
// Exit confirmation strip
// ============================================================

@Composable
private fun ExitConfirmStrip(
    onStay: () -> Unit,
    onExit: () -> Unit,
    modifier: Modifier = Modifier
) {
    val shape = RoundedCornerShape(Dimens.ChatInputFieldRadius)
    Row(
        modifier = modifier
            .fillMaxWidth()
            .clip(shape)
            .background(ChatInputFill)
            .border(Dimens.BorderThin, CardBorder, shape)
            .padding(
                horizontal = Dimens.ChatInputFieldPaddingHorizontal,
                vertical = Dimens.ChatInputFieldPaddingVertical
            ),
        verticalAlignment = Alignment.CenterVertically
    ) {
        Text(
            text = "Exit? Progress will be lost.",
            style = CardSubtitle,
            color = TextPrimary,
            modifier = Modifier.weight(1f)
        )
        Text(
            text = "Exit",
            style = CardSubtitle,
            color = TextPrimary,
            fontWeight = FontWeight.Bold,
            modifier = Modifier
                .clip(RoundedCornerShape(Dimens.RadiusSmall))
                .clickable(role = Role.Button, onClick = onExit)
                .padding(6.dp)
        )
        Text(
            text = "Stay",
            style = CardSubtitle,
            color = TextPrimary,
            fontWeight = FontWeight.Bold,
            modifier = Modifier
                .clip(RoundedCornerShape(Dimens.RadiusSmall))
                .clickable(role = Role.Button, onClick = onStay)
                .padding(6.dp)
        )
    }
}

// ============================================================
// Step 1 - DETAILS
// ============================================================

@Composable
private fun DetailsStep(
    state: AddSensorState,
    onNameChanged: (String) -> Unit,
    onModelChanged: (String) -> Unit,
    onUseCurrentPosition: (Double, Double) -> Unit,
    onManualLatitudeChanged: (String) -> Unit,
    onManualLongitudeChanged: (String) -> Unit
) {
    SectionLabel("Where is this sensor?")
    WizardCard {
        Column(verticalArrangement = Arrangement.spacedBy(Dimens.SettingCardTitleGap)) {
            state.latitude?.let { lat ->
                state.longitude?.let { lon ->
                    id.web.quakealert.ui.common.QuakeMap(
                        focus = MapFocus(latitude = lat, longitude = lon),
                        markers = listOf(
                            MapMarker("wizard-pin", lat, lon, MapMarkerKind.STATION_ONLINE)
                        ),
                        modifier = Modifier
                            .fillMaxWidth()
                            .height(140.dp)
                            .clip(RoundedCornerShape(Dimens.SettingCardRadius))
                    )
                }
            }

            WizardTextField(
                value = state.locationName,
                onValueChange = onNameChanged,
                placeholder = PLACE_NAME_PLACEHOLDER,
                singleLine = true
            )

            state.detailsError?.let { error ->
                Text(text = error.label(), style = CardSubtitle, color = TextPrimary)
            }

            if (state.latitude == null || state.longitude == null) {
                SecondaryHint("No position yet - type coordinates below.")
            } else {
                Row(horizontalArrangement = Arrangement.spacedBy(Dimens.SensorChipRowGap)) {
                    WizardTextField(
                        value = state.latitude.toString(),
                        onValueChange = onManualLatitudeChanged,
                        placeholder = "Latitude",
                        singleLine = true,
                        modifier = Modifier.weight(1f)
                    )
                    WizardTextField(
                        value = state.longitude.toString(),
                        onValueChange = onManualLongitudeChanged,
                        placeholder = "Longitude",
                        singleLine = true,
                        modifier = Modifier.weight(1f)
                    )
                }
            }
        }
    }
    SectionLabel("Sensor model")
    ModelChoices(selected = state.sensorModel, onSelected = onModelChanged)
}

@Composable
private fun ModelChoices(selected: String, onSelected: (String) -> Unit) {
    Row(horizontalArrangement = Arrangement.spacedBy(Dimens.SensorChipRowGap)) {
        SensorNameRules.MODEL_CHOICES.forEach { model ->
            val isSelected = model == selected
            val shape = RoundedCornerShape(Dimens.RadiusSmall)
            Box(
                modifier = Modifier
                    .clip(shape)
                    .background(if (isSelected) AccentBlue else ChatInputFill)
                    .border(Dimens.BorderThin, CardBorder, shape)
                    .clickable(role = Role.Button) { onSelected(model) }
                    .padding(
                        horizontal = Dimens.ChatInputFieldPaddingHorizontal,
                        vertical = Dimens.ChatInputFieldPaddingVertical
                    )
            ) {
                Text(text = model, style = CardTitle, color = TextPrimary)
            }
        }
    }
}

// ============================================================
// Step 2 - CREDENTIALS
// ============================================================

@Composable
private fun CredentialsStep(state: AddSensorState, onSecretRevealed: () -> Unit) {
    val node = state.provisioned ?: return
    SectionLabel("This sensor's identity")
    WizardCard {
        Column(verticalArrangement = Arrangement.spacedBy(Dimens.SettingCardTitleGap)) {
            CredentialRow(label = "Station ID", value = node.stationId)

            // Display-once: the server stores only ciphertext, so this secret can
            // never be shown again after this session.
            Row(horizontalArrangement = Arrangement.spacedBy(Dimens.SensorChipRowGap)) {
                Text(
                    text = "Provisioning secret",
                    style = CardSubtitle,
                    color = TextPrimary,
                    modifier = Modifier.weight(1f)
                )
                Text(
                    text = if (state.secretRevealed) node.provisioningSecret else "\u2022".repeat(12),
                    style = CardTitle,
                    color = TextPrimary
                )
            }
            if (!state.secretRevealed) {
                QuakeSecondaryButton(text = "Show secret once", onClick = onSecretRevealed)
            } else {
                CopySecretButton(node)
            }
            SecondaryHint("It is embedded into the sensor during the next step and cannot be recovered later.")
        }
    }
    SecondaryHint("Next: connect this phone to the sensor's setup network.")
}

@Composable
private fun CredentialRow(label: String, value: String) {
    Row(
        horizontalArrangement = Arrangement.spacedBy(Dimens.SensorChipRowGap),
        verticalAlignment = Alignment.CenterVertically
    ) {
        Text(text = label, style = CardSubtitle, color = TextPrimary, modifier = Modifier.weight(1f))
        Text(text = value, style = CardTitle, color = TextPrimary)
    }
}

@Composable
private fun CopySecretButton(node: ProvisionedNode) {
    val clipboard = LocalClipboardManager.current
    QuakeSecondaryButton(text = "Copy secret") {
        clipboard.setText(AnnotatedString(node.provisioningSecret))
    }
}

// ============================================================
// Step 3 - LINK
// ============================================================

@Composable
private fun LinkStep(
    state: AddSensorState,
    onSsidSelected: (String) -> Unit,
    onPasswordChanged: (String) -> Unit,
    onRescanClicked: () -> Unit
) {
    SectionLabel("The sensor's home Wi-Fi")
    WizardCard {
        Column(verticalArrangement = Arrangement.spacedBy(Dimens.SettingCardTitleGap)) {
            Text(text = "Networks seen by the sensor:", style = CardSubtitle, color = TextPrimary)

            if (state.scannedSsids.isEmpty() && !state.isBusy) {
                Text(
                    text = "None found yet - rescan after the sensor finishes booting.",
                    style = CardSubtitle,
                    color = TextSecondary
                )
            }
            state.scannedSsids.forEach { ssid ->
                SsidChoice(
                    ssid = ssid,
                    selected = ssid == state.selectedSsid,
                    onSelected = { onSsidSelected(ssid) }
                )
            }
            QuakeSecondaryButton(text = "Rescan", onClick = onRescanClicked)

            if (state.selectedSsid.isNotEmpty()) {
                WizardTextField(
                    value = state.wifiPassword,
                    onValueChange = onPasswordChanged,
                    placeholder = "Wi-Fi password (empty for open networks)",
                    singleLine = true
                )
            }
            state.linkError?.let { error ->
                Text(
                    text = when (error) {
                        LinkError.SSID_REQUIRED -> "Choose a network first."
                        LinkError.PASSWORD_TOO_SHORT -> "That password is too long to be valid."
                    },
                    style = CardSubtitle,
                    color = TextPrimary
                )
            }
        }
    }
}

@Composable
private fun SsidChoice(ssid: String, selected: Boolean, onSelected: () -> Unit) {
    val shape = RoundedCornerShape(Dimens.RadiusSmall)
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clip(shape)
            .background(if (selected) AccentBlue else ChatInputFill)
            .border(Dimens.BorderThin, CardBorder, shape)
            // Selectable - this row IS the choice; without it the list renders
            // beautifully and does absolutely nothing.
            .clickable(role = Role.Button, onClick = onSelected)
            .padding(
                horizontal = Dimens.ChatInputFieldPaddingHorizontal,
                vertical = Dimens.ChatInputFieldPaddingVertical
            ),
        horizontalArrangement = Arrangement.spacedBy(Dimens.SensorChipRowGap),
        verticalAlignment = Alignment.CenterVertically
    ) {
        Icon(
            painter = painterResource(R.drawable.ic_globe),
            contentDescription = null,
            tint = TextPrimary
        )
        Text(text = ssid, style = CardTitle, color = TextPrimary)
    }
}

// ============================================================
// Step 4 - CONFIRM
// ============================================================

@Composable
private fun ConfirmStep(state: AddSensorState, onRefreshConfirm: () -> Unit) {
    SectionLabel("Waiting for the sensor")
    WizardCard {
        Column(verticalArrangement = Arrangement.spacedBy(Dimens.SettingCardTitleGap)) {
            when (state.confirmState) {
                ConfirmState.WAITING -> {
                    Text(
                        text = "The sensor is rebooting and joining your Wi-Fi…",
                        style = CardTitle,
                        color = TextPrimary
                    )
                    Text(
                        text = "Checking again in ${state.attemptsLeft} chances.",
                        style = CardSubtitle,
                        color = TextSecondary
                    )
                }

                ConfirmState.PENDING -> {
                    Text(
                        text = "Configured — awaiting operator confirmation.",
                        style = CardTitle,
                        color = TextPrimary
                    )
                    Text(
                        text = "The station shows as Pending until it is verified. It will not count toward alerts until then.",
                        style = CardSubtitle,
                        color = TextSecondary
                    )
                }

                ConfirmState.ONLINE -> Text(
                    text = "${state.effectiveStationId ?: "The sensor"} is online.",
                    style = CardTitle,
                    color = TextPrimary
                )
            }
            if (state.confirmState != ConfirmState.ONLINE) {
                QuakeSecondaryButton(text = "Check now", onClick = onRefreshConfirm)
            }
        }
    }
}

// ============================================================
// Footer + shared atoms
// ============================================================

@Composable
private fun WizardFooter(
    state: AddSensorState,
    onDismiss: () -> Unit,
    onDetailsContinue: () -> Unit,
    onCredentialsContinue: () -> Unit,
    onConfigureNode: () -> Unit,
    modifier: Modifier = Modifier
) {
    val (label, enabled, action) = when (state.step) {
        WizardStep.DETAILS ->
            Triple(
                if (state.isBusy) "Registering…" else "Register sensor",
                state.detailsValid && !state.isBusy,
                onDetailsContinue
            )

        WizardStep.CREDENTIALS ->
            Triple("Continue", !state.isBusy, onCredentialsContinue)

        WizardStep.LINK ->
            Triple(
                if (state.isBusy) "Configuring…" else "Configure sensor",
                state.linkValid && !state.isBusy,
                onConfigureNode
            )

        WizardStep.CONFIRM ->
            Triple(
                if (state.confirmState == ConfirmState.ONLINE) "Done" else "Finish later",
                true,
                onDismiss
            )
    }

    Column(modifier = modifier.padding(vertical = Dimens.HeaderSectionGap)) {
        QuakePrimaryButton(text = label, enabled = enabled, onClick = action, modifier = Modifier.fillMaxWidth())
        if (state.step == WizardStep.CONFIRM && state.confirmState != ConfirmState.ONLINE) {
            Spacer(Modifier.height(Dimens.SettingsSectionSpacing))
            QuakeSecondaryButton(text = "Close", onClick = onDismiss, modifier = Modifier.fillMaxWidth())
        }
    }
}

@Composable
private fun SectionLabel(text: String) {
    Text(
        text = text,
        color = TextPrimary,
        fontFamily = NunitoFontFamily,
        fontWeight = FontWeight.Bold,
        fontSize = 16.sp,
        modifier = Modifier
            .fillMaxWidth()
            .padding(top = Dimens.HeaderSectionGap)
    )
}

/** Quiet one-liner under a section; never carries anything actionable. */
@Composable
private fun SecondaryHint(text: String) {
    Text(text = text, style = CardSubtitle, color = TextSecondary)
}

@Composable
private fun ErrorNote(message: String) {
    val shape = RoundedCornerShape(Dimens.ChatInputFieldRadius)
    Text(
        text = message,
        style = CardSubtitle,
        color = TextPrimary,
        modifier = Modifier
            .fillMaxWidth()
            .clip(shape)
            .background(ChatInputFill)
            .border(Dimens.BorderThin, CardBorder, shape)
            .padding(
                horizontal = Dimens.ChatInputFieldPaddingHorizontal,
                vertical = Dimens.ChatInputFieldPaddingVertical
            )
    )
}

/**
 * The wizard's content card. Same chrome as the settings rows (CardSurface fill,
 * thin CardBorder, SettingCardRadius) but a plain Column body: steps are forms
 * and stacks, not title+trailing rows.
 */
@Composable
private fun WizardCard(
    modifier: Modifier = Modifier,
    content: @Composable androidx.compose.foundation.layout.ColumnScope.() -> Unit
) {
    val shape = RoundedCornerShape(Dimens.SettingCardRadius)
    Column(
        modifier = modifier
            .fillMaxWidth()
            .clip(shape)
            .background(CardSurface, shape)
            .border(Dimens.BorderThin, CardBorder, shape)
            .padding(
                horizontal = Dimens.SettingCardPaddingHorizontal,
                vertical = Dimens.SettingCardPaddingVertical
            ),
        verticalArrangement = Arrangement.spacedBy(Dimens.SettingCardTitleGap),
        content = content
    )
}

@Composable
private fun WizardTextField(
    value: String,
    onValueChange: (String) -> Unit,
    placeholder: String,
    singleLine: Boolean,
    modifier: Modifier = Modifier
) {
    val shape = RoundedCornerShape(Dimens.ChatInputFieldRadius)
    BasicTextField(
        value = value,
        onValueChange = onValueChange,
        modifier = modifier
            .fillMaxWidth()
            .clip(shape)
            .background(ChatInputFill, shape)
            .border(Dimens.BorderThin, CardBorder, shape)
            .padding(
                horizontal = Dimens.ChatInputFieldPaddingHorizontal,
                vertical = Dimens.ChatInputFieldPaddingVertical
            ),
        textStyle = CardTitle.copy(color = TextPrimary),
        singleLine = singleLine,
        cursorBrush = SolidColor(TextPrimary),
        decorationBox = { innerTextField ->
            if (value.isEmpty()) {
                Text(text = placeholder, style = CardTitle, color = TextSecondary)
            }
            innerTextField()
        }
    )
}

private const val PLACE_NAME_PLACEHOLDER = "Place name, e.g. \"Lembang, Kab. Bandung Barat\""

/** User-facing wording for each rule the DETAILS gate can refuse on. */
private fun DetailsError.label(): String = when (this) {
    DetailsError.NAME_REQUIRED -> "Give this sensor a place name."
    DetailsError.NAME_TOO_LONG -> "That place name is too long (150 characters max)."
    DetailsError.NAME_HAS_NODE_ID -> "A station id is not a place name."
    DetailsError.NAME_NOT_PLACE_LIKE -> "That does not look like a place name."
    DetailsError.POSITION_MISSING -> "Set where the sensor will live - use your location or type coordinates."
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun AddSensorWizardPreview() {
    id.web.quakealert.ui.theme.QuakeAlertTheme {
        Box(Modifier.fillMaxSize().background(BackgroundGradientBottom))
        AddSensorWizardDialog(
            state = AddSensorState(locationName = "Cimahi", latitude = -6.87, longitude = 107.54),
            onDismiss = {},
            onNameChanged = {},
            onModelChanged = {},
            onUseCurrentPosition = { _, _ -> },
            onManualLatitudeChanged = {},
            onManualLongitudeChanged = {},
            onDetailsContinue = {},
            onRetryFromDetails = {},
            onSecretRevealed = {},
            onCredentialsContinue = {},
            onRescanClicked = {},
            onSsidSelected = {},
            onPasswordChanged = {},
            onConfigureNode = {},
            onRefreshConfirm = {}
        )
    }
}
