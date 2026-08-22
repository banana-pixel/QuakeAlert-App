package id.web.quakealert.ui.warning

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Text
import androidx.compose.material3.minimumInteractiveComponentSize
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import id.web.quakealert.R
import id.web.quakealert.domain.EmergencyContacts
import id.web.quakealert.domain.EmergencyNumber
import id.web.quakealert.ui.common.QuakeModalHeader
import id.web.quakealert.ui.theme.CardBorder
import id.web.quakealert.ui.theme.CardSubtitle
import id.web.quakealert.ui.theme.CardSurface
import id.web.quakealert.ui.theme.CardTitle
import id.web.quakealert.ui.theme.ChipLabel
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.MicroCaption
import id.web.quakealert.ui.theme.PossibilityModalGradient
import id.web.quakealert.ui.theme.QuakeAlertTheme
import id.web.quakealert.ui.theme.TextPrimary
import id.web.quakealert.ui.theme.TextSecondary

/**
 * "Emergency Steps & Contacts" as an overlay on the Warning screen, in its own
 * [Dialog] window. Every dismissal path — close button, back press, outside tap —
 * routes to [onDismiss], so it can never trap navigation during an alert.
 *
 * This replaces a button labelled "SHELTER & EMERGENCY INFO" whose handler was
 * empty. The label went with it: "shelter" promises evacuation points, which needs a
 * dataset the project does not have, and a button that promises one is worse than no
 * button. What is here instead is everything the app can answer honestly with no
 * network and no new backend — what to do in the next second, what to check once the
 * shaking stops, who to call where the phone actually is, and where the phone thinks
 * it is.
 *
 * @param info the resolved numbers and the position line, assembled by
 *   [WarningViewModel] when the overlay opens.
 * @param onDial invoked with a number to hand to the dialler.
 * @param onDismiss invoked by the close button, back press or an outside tap.
 */
@Composable
fun EmergencyInfoModalDialog(
    info: EmergencyInfoState,
    onDial: (String) -> Unit,
    onDismiss: () -> Unit
) {
    Dialog(
        onDismissRequest = onDismiss,
        properties = DialogProperties(usePlatformDefaultWidth = false)
    ) {
        EmergencyInfoModal(
            info = info,
            onDial = onDial,
            onDismiss = onDismiss,
            modifier = Modifier.padding(Dimens.ScreenHorizontalPadding)
        )
    }
}

/**
 * Stateless card behind [EmergencyInfoModalDialog]. Exposed separately so it can be
 * previewed without a window.
 *
 * Section order is the order the information is needed in, not the order it is easiest
 * to read: the three shaking steps come first because they matter within the second
 * the overlay opens, and the numbers come before the coordinates because a dispatcher
 * has to be reached before a position can be read to them.
 */
@Composable
fun EmergencyInfoModal(
    info: EmergencyInfoState,
    onDial: (String) -> Unit,
    onDismiss: () -> Unit,
    modifier: Modifier = Modifier
) {
    val shape = RoundedCornerShape(Dimens.RadiusCard)

    Column(
        modifier = modifier
            .fillMaxWidth()
            .clip(shape)
            .background(PossibilityModalGradient, shape)
            .border(Dimens.BorderThin, CardBorder, shape)
            .verticalScroll(rememberScrollState())
            .padding(Dimens.ModalPadding),
        verticalArrangement = Arrangement.spacedBy(Dimens.SettingCardContentGap)
    ) {
        QuakeModalHeader(onDismiss = onDismiss, title = stringResource(R.string.emergency_title))

        SectionTitle(stringResource(R.string.emergency_during_title))
        EmergencyStep(
            title = stringResource(R.string.emergency_step_drop_title),
            detail = stringResource(R.string.emergency_step_drop_detail)
        )
        EmergencyStep(
            title = stringResource(R.string.emergency_step_cover_title),
            detail = stringResource(R.string.emergency_step_cover_detail)
        )
        EmergencyStep(
            title = stringResource(R.string.emergency_step_hold_title),
            detail = stringResource(R.string.emergency_step_hold_detail)
        )

        SectionTitle(stringResource(R.string.emergency_after_title))
        Text(text = stringResource(R.string.emergency_after_aftershocks), style = CardSubtitle, color = TextSecondary)
        Text(text = stringResource(R.string.emergency_after_gas), style = CardSubtitle, color = TextSecondary)
        Text(text = stringResource(R.string.emergency_after_exit), style = CardSubtitle, color = TextSecondary)
        Text(text = stringResource(R.string.emergency_after_injuries), style = CardSubtitle, color = TextSecondary)

        SectionTitle(stringResource(R.string.emergency_contacts_title))
        info.numbers.forEach { number ->
            EmergencyNumberRow(number = number, onDial = onDial)
        }
        Text(text = stringResource(R.string.emergency_contacts_note), style = MicroCaption, color = TextSecondary)

        SectionTitle(stringResource(R.string.emergency_position_title))
        if (info.coordinatesLabel != null) {
            // Selectable so the coordinates can be copied into a message when they
            // cannot be read out loud; the rest of the card is not.
            SelectionContainer {
                Text(text = info.coordinatesLabel, style = CardTitle, color = TextPrimary)
            }
            Text(text = stringResource(R.string.emergency_position_note), style = MicroCaption, color = TextSecondary)
        } else {
            Text(text = stringResource(R.string.emergency_position_unknown), style = CardSubtitle, color = TextSecondary)
        }

        Text(text = stringResource(R.string.emergency_offline_note), style = MicroCaption, color = TextSecondary)
    }
}

/** Section heading inside the card, in the same weight as a card title. */
@Composable
private fun SectionTitle(text: String) {
    Text(text = text, style = CardTitle, color = TextPrimary)
}

/** One numbered shaking step: the instruction, then why it is the instruction. */
@Composable
private fun EmergencyStep(title: String, detail: String) {
    Column(verticalArrangement = Arrangement.spacedBy(Dimens.SettingCardTitleGap)) {
        Text(text = title, style = ChipLabel, color = TextPrimary)
        Text(text = detail, style = CardSubtitle, color = TextSecondary)
    }
}

/**
 * A tappable number row.
 *
 * The whole row is the target, and [minimumInteractiveComponentSize] guarantees the
 * 48dp minimum: this is a control aimed at someone whose hands are shaking, and a
 * digit-sized hit area is the wrong thing to hand them. It is announced as a button
 * with "Dial <number>" so a screen reader says what will happen rather than reading
 * two labels in a row.
 */
@Composable
private fun EmergencyNumberRow(number: EmergencyNumber, onDial: (String) -> Unit) {
    val shape = RoundedCornerShape(Dimens.RadiusSmall)
    val action = stringResource(R.string.emergency_dial_action, number.number)

    Row(
        modifier = Modifier
            .fillMaxWidth()
            .minimumInteractiveComponentSize()
            .clip(shape)
            .background(CardSurface, shape)
            .border(Dimens.BorderThin, CardBorder, shape)
            .clickable(role = Role.Button, onClickLabel = action) { onDial(number.number) }
            .padding(
                horizontal = Dimens.MapRangeBadgePaddingHorizontal,
                vertical = Dimens.SettingCardContentGap
            ),
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalAlignment = Alignment.CenterVertically
    ) {
        Text(text = number.label, style = CardSubtitle, color = TextSecondary)
        Text(text = number.number, style = CardTitle, color = TextPrimary)
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun EmergencyInfoModalPreview() {
    QuakeAlertTheme {
        EmergencyInfoModal(
            info = EmergencyInfoState(
                numbers = EmergencyContacts.forCountry("ID"),
                coordinatesLabel = "-6.91750, 107.61910"
            ),
            onDial = {},
            onDismiss = {}
        )
    }
}
