package id.web.quakealert.ui.common

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.SideEffect
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import androidx.compose.ui.window.DialogWindowProvider
import androidx.core.view.WindowCompat
import id.web.quakealert.ui.theme.DestructiveActionFill
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.ModalCardBorder
import id.web.quakealert.ui.theme.ModalScrim
import id.web.quakealert.ui.theme.ModalTitle
import id.web.quakealert.ui.theme.WizardBodyText
import id.web.quakealert.ui.theme.WizardDividerColor
import id.web.quakealert.ui.theme.WizardModalGradient
import id.web.quakealert.ui.theme.WizardPanelFill
import id.web.quakealert.ui.theme.WizardPanelStroke

/**
 * The overlay vocabulary every modal in the app draws: the dialog card itself, the
 * inset panel inside it, the hairline between two panel rows, and the yes/no card.
 *
 * Extracted because each overlay used to rebuild all four privately, which is how
 * the same panel ended up with three different paddings and the same question
 * ("discard this?") ended up as a 12sp text strip in one place and a card in
 * another. Header chrome lives next door in [QuakeModalHeader], and the action
 * capsule in `QuakeButtons.kt`.
 */

/**
 * Makes the dialog's own window keyboard-aware.
 *
 * A dialog gets a separate window from the activity, and that window keeps
 * `decorFitsSystemWindows = true` no matter what the activity did with
 * [androidx.activity.enableEdgeToEdge]. Compose therefore sees no IME inset inside a
 * dialog, and an `imePadding()` on the card silently does nothing: the keyboard opens
 * over the field being edited. Opting the dialog window out of decor fitting is what
 * makes the inset arrive.
 *
 * The soft-input mode is deliberately NOT set to ADJUST_RESIZE here: that would let
 * the system resize the whole dialog surface while `imePadding()` on the card also
 * reacts to the same keyboard, and the two mechanisms animating at different rates
 * made the card visibly lurch whenever it opened. One compensation only — the inset —
 * keeps the lift a single smooth motion.
 *
 * A [SideEffect] rather than a `LaunchedEffect`: the window must be configured before
 * the first layout that could read the inset, and re-applying it is free.
 */
@Composable
private fun ImeAwareDialogWindow() {
    val dialogWindow = (LocalView.current.parent as? DialogWindowProvider)?.window
    SideEffect {
        val window = dialogWindow ?: return@SideEffect
        WindowCompat.setDecorFitsSystemWindows(window, false)
    }
}

/**
 * A modal card centred over the screen.
 *
 * The platform width is switched off so the card can size itself to the design's
 * bound instead of the system dialog width, and [imePadding] is applied to the card
 * rather than to its content so an opening keyboard lifts the whole card, never
 * covers the field inside it.
 *
 * @param onDismissRequest back press and, when [dismissOnClickOutside] is set, a
 *   tap outside the card. The card never dismisses itself.
 * @param gradient card fill; pass the palette belonging to the flow.
 * @param maxWidth ceiling on the card width, from the flow's own design frame.
 */
@Composable
fun QuakeModalCard(
    onDismissRequest: () -> Unit,
    modifier: Modifier = Modifier,
    gradient: Brush = WizardModalGradient,
    maxWidth: androidx.compose.ui.unit.Dp = Dimens.WizardCardMaxWidth,
    dismissOnBackPress: Boolean = true,
    dismissOnClickOutside: Boolean = false,
    content: @Composable ColumnScope.() -> Unit
) {
    Dialog(
        onDismissRequest = onDismissRequest,
        properties = DialogProperties(
            usePlatformDefaultWidth = false,
            dismissOnBackPress = dismissOnBackPress,
            dismissOnClickOutside = dismissOnClickOutside
        )
    ) {
        ImeAwareDialogWindow()
        val shape = remember { RoundedCornerShape(Dimens.RadiusCard) }
        Column(
            modifier = modifier
                .fillMaxWidth()
                .widthIn(max = maxWidth)
                .imePadding()
                .clip(shape)
                .background(gradient, shape)
                .border(Dimens.BorderThin, ModalCardBorder, shape)
                .padding(Dimens.ModalPadding),
            content = content
        )
    }
}

/**
 * Inset panel inside a modal card: a black wash so the card's gradient still shows
 * through, plus the white 30% stroke the design draws around every grouped block.
 */
@Composable
fun QuakeModalPanel(
    modifier: Modifier = Modifier,
    content: @Composable ColumnScope.() -> Unit
) {
    val shape = remember { RoundedCornerShape(Dimens.RadiusSmall) }
    Column(
        modifier = modifier
            .fillMaxWidth()
            .clip(shape)
            .background(WizardPanelFill, shape)
            .border(Dimens.BorderMedium, WizardPanelStroke, shape)
            .padding(
                horizontal = Dimens.WizardPanelPaddingHorizontal,
                vertical = Dimens.WizardPanelPaddingVertical
            ),
        verticalArrangement = Arrangement.spacedBy(Dimens.WizardPanelGap),
        content = content
    )
}

/** Hairline rule between two rows of a [QuakeModalPanel]. */
@Composable
fun QuakeModalHairline(modifier: Modifier = Modifier) {
    Box(
        modifier = modifier
            .fillMaxWidth()
            .height(Dimens.BorderThin)
            .background(WizardDividerColor)
    )
}

/**
 * A yes/no question asked on top of the work it is about: title, one short
 * paragraph, then the dismiss and confirm capsules side by side.
 *
 * The confirm action is the destructive one and carries the filled red treatment,
 * while dismissing is the outlined default, so a mis-tap keeps the work rather than
 * throwing it away and the colour says which is which before the label is read. A scrim behind the card parks whatever is underneath, and
 * the card is deliberately not dismissible by tapping outside: the answer to
 * "should I throw this away" must be given, not guessed from a stray tap.
 */
@Composable
fun QuakeConfirmDialog(
    title: String,
    message: String,
    confirmLabel: String,
    dismissLabel: String,
    onConfirm: () -> Unit,
    onDismiss: () -> Unit,
    modifier: Modifier = Modifier
) {
    Dialog(
        onDismissRequest = onDismiss,
        properties = DialogProperties(
            usePlatformDefaultWidth = false,
            dismissOnBackPress = true,
            dismissOnClickOutside = false
        )
    ) {
        ImeAwareDialogWindow()
        val shape = remember { RoundedCornerShape(Dimens.RadiusCard) }
        Box(
            modifier = Modifier
                .fillMaxSize()
                .background(ModalScrim)
                // Swallows taps on the scrim so they never reach the wizard behind
                // it; no ripple, because the scrim is not a control.
                .clickable(
                    indication = null,
                    interactionSource = remember { MutableInteractionSource() }
                ) {},
            contentAlignment = Alignment.Center
        ) {
            Column(
                modifier = modifier
                    .fillMaxWidth()
                    .widthIn(max = Dimens.WizardCardMaxWidth)
                    .padding(horizontal = Dimens.ScreenHorizontalPadding)
                    .clip(shape)
                    .background(WizardModalGradient, shape)
                    .border(Dimens.BorderThin, ModalCardBorder, shape)
                    .padding(Dimens.ModalPadding),
                horizontalAlignment = Alignment.CenterHorizontally,
                verticalArrangement = Arrangement.spacedBy(Dimens.WizardConfirmContentGap)
            ) {
                Text(text = title, style = ModalTitle)
                Text(
                    text = message,
                    style = WizardBodyText,
                    modifier = Modifier.fillMaxWidth()
                )
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.spacedBy(Dimens.WizardActionGap)
                ) {
                    QuakeModalActionButton(
                        label = dismissLabel,
                        onClick = onDismiss,
                        filled = false,
                        modifier = Modifier.weight(1f)
                    )
                    QuakeModalActionButton(
                        label = confirmLabel,
                        onClick = onConfirm,
                        container = DestructiveActionFill,
                        modifier = Modifier.weight(1f)
                    )
                }
            }
        }
    }
}
