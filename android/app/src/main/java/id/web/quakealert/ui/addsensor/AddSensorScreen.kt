package id.web.quakealert.ui.addsensor

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.border
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
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import id.web.quakealert.R
import id.web.quakealert.domain.ProvisionedNode
import id.web.quakealert.ui.common.MapFocus
import id.web.quakealert.ui.common.MapMarker
import id.web.quakealert.ui.common.MapMarkerKind
import id.web.quakealert.ui.common.QuakeMap
import id.web.quakealert.ui.theme.AccentBlueTranslucent
import id.web.quakealert.ui.theme.BackgroundGradientBottom
import id.web.quakealert.ui.theme.CardSurface
import id.web.quakealert.ui.theme.BorderLight
import id.web.quakealert.ui.theme.CardBorder
import id.web.quakealert.ui.theme.CardSubtitle
import id.web.quakealert.ui.theme.CardTitle
import id.web.quakealert.ui.theme.ChatInputFill
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.NunitoFontFamily
import id.web.quakealert.ui.theme.TextPrimary
import id.web.quakealert.ui.theme.TextSecondary

/**
 * The add-a-sensor wizard: one screen, four steps (Figma language of nodes 1:845 /
 * 1:1081 — same cards, pills and CTA shapes as the rest of the app).
 *
 * Stateless by the project's UDF convention: [state] in, callbacks out, all logic
 * in [AddSensorState]'s pure functions and [AddSensorViewModel]'s side effects.
 * The step indicator reuses onboarding's 6 dp segment bars so "where am I" reads
 * the same way it does there.
 *
 * Placement input is deliberately a seeded preview + explicit entry rather than a
 * drag-the-pin map: [QuakeMap] is non-interactive by design (scrolling parents own
 * gestures), and the sensor is usually installed where the phone already is —
 * "Use my current location" — or somewhere you can type.
 */
@Composable
fun AddSensorScreen(
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
    Column(
        modifier = modifier
            .fillMaxSize()
            .background(BackgroundGradientBottom)
            .statusBarsPadding()
            .imePadding()
            .padding(horizontal = Dimens.ScreenHorizontalPadding)
    ) {
        WizardHeader(step = state.step, onDismiss = onDismiss)

        Column(
            modifier = Modifier
                .weight(1f)
                .verticalScroll(rememberScrollState()),
            verticalArrangement = Arrangement.spacedBy(Dimens.SettingsSectionSpacing)
        ) {
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
                WizardCard {
                    Text(text = message, style = CardSubtitle, color = TextPrimary)
                }
            }
        }

        WizardFooter(state, onDismiss, onDetailsContinue, onCredentialsContinue, onConfigureNode)
    }
}

// ============================================================
// Header + step indicator
// ============================================================

@Composable
private fun WizardHeader(step: WizardStep, onDismiss: () -> Unit) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(top = Dimens.HeaderSectionGap),
        horizontalArrangement = Arrangement.spacedBy(Dimens.SensorChipRowGap),
        verticalAlignment = Alignment.CenterVertically
    ) {
        Text(
            text = "Add a Sensor",
            color = TextPrimary,
            fontFamily = NunitoFontFamily,
            fontWeight = FontWeight.ExtraBold,
            fontSize = 24.sp,
            lineHeight = 26.sp,
            modifier = Modifier.weight(1f)
        )
        CloseButton(onDismiss)
    }
    StepIndicator(
        current = step,
        modifier = Modifier.padding(top = Dimens.HeaderSectionGap)
    )
}

@Composable
private fun CloseButton(onDismiss: () -> Unit) {
    Box(
        modifier = Modifier
            .clip(RoundedCornerShape(12.dp))
            .background(ChatInputFill)
            .semantics { contentDescription = "Close the add-sensor wizard" }
            .padding(8.dp)
    ) {
        Icon(
            painter = painterResource(R.drawable.ic_alert_triangle),
            contentDescription = null,
            tint = TextPrimary
        )
    }
}

/** Onboarding's segment bars, one per step; filled up to the current step. */
@Composable
private fun StepIndicator(current: WizardStep, modifier: Modifier = Modifier) {
    Row(
        modifier = modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.spacedBy(6.dp)
    ) {
        val currentOrdinal = WizardStep.entries.indexOf(current)
        WizardStep.entries.forEachIndexed { index, _ ->
            val reached = index <= currentOrdinal
            Box(
                modifier = Modifier
                    .weight(1f)
                    .height(6.dp)
                    .clip(RoundedCornerShape(100.dp))
                    .background(if (reached) TextPrimary else BorderLight)
            )
        }
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
        Column(verticalArrangement = Arrangement.spacedBy(Dimens.SettingsSectionSpacing)) {
            // Seeded placement preview. Non-interactive by design (QuakeMap's
            // scrolling-parent constraint); the pin states the choice visually.
            state.latitude?.let { lat ->
                state.longitude?.let { lon ->
                    QuakeMap(
                        focus = MapFocus(latitude = lat, longitude = longitudeZoom(lat)),
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
                PrimaryButton(text = "Use my current location") {
                    // The ViewModel seeds from the stored fix at init; this button
                    // exists for a wizard opened before any sync ever ran.
                    onUseCurrentPosition(-6.9175, 107.6191)
                }
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
}

private fun longitudeZoom(lat: Double): Double = 11.5 + (lat / 90.0)

// ============================================================
// Step 2 - CREDENTIALS
// ============================================================

@Composable
private fun CredentialsStep(state: AddSensorState, onSecretRevealed: () -> Unit) {
    val node = state.provisioned ?: return
    SectionLabel("This sensor's identity")
    WizardCard {
        Column(verticalArrangement = Arrangement.spacedBy(Dimens.SettingsSectionSpacing)) {
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
                SecondaryButton(text = "Show secret once", onClick = onSecretRevealed)
            } else {
                CopySecretButton(node)
            }
            Text(
                text = "It is embedded into the sensor during the next step and cannot be recovered later.",
                style = CardSubtitle,
                color = TextSecondary
            )
        }
    }
    Text(
        text = "Next: connect this phone to the sensor's setup network.",
        style = CardSubtitle,
        color = TextSecondary
    )
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
    SecondaryButton(text = "Copy secret") {
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
        Column(verticalArrangement = Arrangement.spacedBy(Dimens.SettingsSectionSpacing)) {
            Text(
                text = "Networks seen by the sensor:",
                style = CardSubtitle,
                color = TextPrimary
            )
            if (state.scannedSsids.isEmpty() && !state.isBusy) {
                Text(
                    text = "None found yet — rescan after the sensor finishes booting.",
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
            SecondaryButton(text = "Rescan", onClick = onRescanClicked)

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
            .background(if (selected) AccentBlueTranslucent else ChatInputFill)
            .border(Dimens.BorderThin, CardBorder, shape)
            // Selectable — this row IS the choice; without it the list renders
            // beautifully and does absolutely nothing.
            .clickable(role = androidx.compose.ui.semantics.Role.Button, onClick = onSelected)
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
        Column(verticalArrangement = Arrangement.spacedBy(Dimens.SettingsSectionSpacing)) {
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
                SecondaryButton(text = "Check now", onClick = onRefreshConfirm)
            }
        }
    }
}


/**
 * The wizard's content card. Same chrome as [id.web.quakealert.ui.common.QuakeCard]
 * (CardSurface fill, thin CardBorder, SettingCardRadius) but a plain Column body:
 * the wizard's steps are forms and stacks, not title+trailing rows.
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

// ============================================================
// Footer + shared atoms
// ============================================================

@Composable
private fun WizardFooter(
    state: AddSensorState,
    onDismiss: () -> Unit,
    onDetailsContinue: () -> Unit,
    onCredentialsContinue: () -> Unit,
    onConfigureNode: () -> Unit
) {
    val (label, enabled, action) = when (state.step) {
        WizardStep.DETAILS ->
            Triple(if (state.isBusy) "Registering…" else "Register sensor", state.detailsValid && !state.isBusy, onDetailsContinue)

        WizardStep.CREDENTIALS ->
            Triple("Continue", !state.isBusy, onCredentialsContinue)

        WizardStep.LINK ->
            Triple(if (state.isBusy) "Configuring…" else "Configure sensor", state.linkValid && !state.isBusy, onConfigureNode)

        WizardStep.CONFIRM ->
            Triple(if (state.confirmState == ConfirmState.ONLINE) "Done" else "Finish later", true, onDismiss)
    }

    Column(modifier = Modifier.padding(vertical = Dimens.HeaderSectionGap)) {
        PrimaryButton(text = label, enabled = enabled, onClick = action)
        if (state.step == WizardStep.CONFIRM && state.confirmState != ConfirmState.ONLINE) {
            Spacer(Modifier.height(Dimens.SettingsSectionSpacing))
            SecondaryButton(text = "Close", onClick = onDismiss, modifier = Modifier.fillMaxWidth())
        }
    }
}

@Composable
private fun PrimaryButton(
    text: String,
    enabled: Boolean = true,
    modifier: Modifier = Modifier,
    onClick: () -> Unit
) {
    Button(
        onClick = onClick,
        enabled = enabled,
        shape = RoundedCornerShape(40.dp),
        colors = ButtonDefaults.buttonColors(containerColor = AccentBlueTranslucent, contentColor = TextPrimary),
        modifier = modifier
            .fillMaxWidth()
            .height(51.dp)
            .border(width = 3.dp, color = BorderLight, shape = RoundedCornerShape(40.dp))
    ) {
        Text(
            text = text,
            color = TextPrimary,
            fontFamily = NunitoFontFamily,
            fontWeight = FontWeight.Bold,
            fontSize = 15.sp
        )
    }
}

@Composable
private fun SecondaryButton(
    text: String,
    modifier: Modifier = Modifier,
    onClick: () -> Unit
) {
    OutlinedButton(
        onClick = onClick,
        shape = RoundedCornerShape(40.dp),
        colors = androidx.compose.material3.ButtonDefaults.outlinedButtonColors(
            containerColor = androidx.compose.ui.graphics.Color.Transparent,
            contentColor = TextPrimary
        ),
        border = androidx.compose.foundation.BorderStroke(3.dp, BorderLight),
        modifier = modifier.height(51.dp)
    ) {
        Text(
            text = text,
            color = TextPrimary,
            fontFamily = NunitoFontFamily,
            fontWeight = FontWeight.Bold,
            fontSize = 15.sp
        )
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
        modifier = Modifier.padding(top = Dimens.HeaderSectionGap)
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
        keyboardOptions = KeyboardOptions(imeAction = ImeAction.Done),
        keyboardActions = KeyboardActions(onDone = { }),
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
private fun AddSensorScreenDetailsPreview() {
    id.web.quakealert.ui.theme.QuakeAlertTheme {
        AddSensorScreen(
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
