package id.web.quakealert.ui.common

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.core.tween
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.slideInVertically
import androidx.compose.animation.slideOutVertically
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import id.web.quakealert.ui.theme.CardBorder
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.PillLabel
import id.web.quakealert.ui.theme.PossibilityModalGradient
import id.web.quakealert.ui.theme.QuakeAlertTheme
import id.web.quakealert.ui.theme.TextPrimary
import kotlinx.coroutines.delay

/**
 * How long a posted message stays up before it clears itself. Long enough to read a
 * one-line outcome twice, short enough that it cannot be mistaken for part of the
 * screen underneath.
 */
private const val TOAST_DURATION_MS = 4_000L

/**
 * Holder for the single floating message the app shows at a time.
 *
 * One slot rather than a queue on purpose: every message here is the outcome of an
 * action the user just took, so a second one means the first is already stale —
 * replacing it is the honest behaviour, and a queue would make the user wait several
 * seconds to read the result of what they did last.
 *
 * Owned by [id.web.quakealert.ui.main.MainScreen] and reached through
 * [LocalQuakeToast], so a screen can report an outcome without the message becoming
 * part of that screen's layout — which is what let the old Settings pill reflow the
 * list and scroll out of view.
 */
class QuakeToastState {

    /** The message currently on screen, or null when nothing is showing. */
    var message: String? by mutableStateOf(null)
        private set

    /** Posts [text], replacing whatever was showing. Blank text is ignored. */
    fun show(text: String) {
        if (text.isNotBlank()) message = text
    }

    /** Clears the current message, from the auto-dismiss timer. */
    fun dismiss() {
        message = null
    }
}

/**
 * The app-wide toast slot. Defaults to a state nothing renders, so a composable or a
 * preview outside [id.web.quakealert.ui.main.MainScreen] can post without crashing —
 * the message is simply never drawn, which is better than a `error("no host")` in a
 * path that only ever reports non-critical outcomes.
 */
val LocalQuakeToast = staticCompositionLocalOf { QuakeToastState() }

/** Remembers a [QuakeToastState] for the host to own. */
@Composable
fun rememberQuakeToastState(): QuakeToastState = remember { QuakeToastState() }

/**
 * Renders whatever [state] holds as a card that slides up over the content and
 * auto-dismisses after [TOAST_DURATION_MS].
 *
 * Placed once, above the bottom navigation, by
 * [id.web.quakealert.ui.main.MainScreen]. It deliberately does not intercept touches:
 * an outcome report must never sit between the user and a control, least of all on
 * this app's screens.
 */
@Composable
fun QuakeToastHost(
    state: QuakeToastState,
    modifier: Modifier = Modifier
) {
    val message = state.message

    // Retained so the card keeps its text through the exit animation, after the
    // slot has already been cleared. Written before it is read in the same
    // composition, so there is no extra recomposition and no stale frame.
    val retained = remember { mutableStateOf("") }
    if (message != null) retained.value = message

    // Keyed on the text: a second outcome restarts the clock rather than inheriting
    // the remains of the first one's.
    LaunchedEffect(message) {
        if (message != null) {
            delay(TOAST_DURATION_MS)
            state.dismiss()
        }
    }

    AnimatedVisibility(
        visible = message != null,
        enter = slideInVertically(tween(220)) { it / 2 } + fadeIn(tween(220)),
        exit = slideOutVertically(tween(180)) { it / 2 } + fadeOut(tween(180)),
        modifier = modifier
    ) {
        val shape = RoundedCornerShape(Dimens.RadiusCard)
        Box(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = Dimens.ScreenHorizontalPadding)
                .clip(shape)
                .background(PossibilityModalGradient, shape)
                .border(Dimens.BorderThin, CardBorder, shape)
                .padding(horizontal = Dimens.ModalPadding, vertical = 12.dp)
        ) {
            Text(
                text = retained.value,
                style = PillLabel,
                color = TextPrimary,
                textAlign = TextAlign.Center,
                modifier = Modifier.fillMaxWidth()
            )
        }
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun QuakeToastHostPreview() {
    QuakeAlertTheme {
        QuakeToastHost(
            state = rememberQuakeToastState().apply { show("Location updated") }
        )
    }
}
