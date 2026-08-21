package id.web.quakealert.ui.common

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.layout.wrapContentHeight
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.material3.minimumInteractiveComponentSize
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import id.web.quakealert.R
import id.web.quakealert.device.AlertOutput
import id.web.quakealert.device.DeviceAlertOutput
import id.web.quakealert.device.TestAlertPlayback
import id.web.quakealert.ui.theme.CardBorder
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.EmergencyAlertGradient
import id.web.quakealert.ui.theme.EmergencyAlertIconBadgeFill
import id.web.quakealert.ui.theme.EmergencyAlertTitle
import id.web.quakealert.ui.theme.EmergencyControlBorder
import id.web.quakealert.ui.theme.EmergencyControlFillIdle
import id.web.quakealert.ui.theme.EmergencyControlLabel
import id.web.quakealert.ui.theme.QuakeAlertTheme
import id.web.quakealert.ui.theme.TestAlertBodyText
import id.web.quakealert.ui.theme.TestAlertStartFill
import id.web.quakealert.ui.theme.TextPrimary

/**
 * The Test Alert Sound modal (Figma node 144:1025) — a dark-red variant of the
 * active-alert card that lets the user hear the siren and feel the vibration before
 * a real earthquake does it for them.
 *
 * Reusable rather than screen-owned because it is offered twice: during onboarding,
 * where "does this actually reach me?" is the whole point of the step, and from
 * Settings afterwards. Both entry points get identical copy and identical
 * behaviour, which matters — a user who tested it once should not meet a different
 * control the second time.
 *
 * Playback lives in [TestAlertPlayback] and stops on three paths: the STOP button,
 * its own [TestAlertPlayback.TEST_ALERT_DURATION_MS] timeout, and disposal. The
 * last is the one worth naming: a back press or an outside tap dismisses the dialog without touching
 * STOP, and a siren that outlived its own dialog would be unstoppable short of
 * killing the app.
 *
 * @param onDismissRequest invoked by a back press or a tap outside the card. STOP
 *   deliberately does *not* dismiss: the point of a separate STOP is to silence the
 *   phone while the warning about public places is still on screen.
 * @param output seam for the siren/vibrator pair. Defaults to the real device; a
 *   preview or test passes a no-op so nothing sounds.
 *
 * The card also carries a close X of its own ([QuakeModalHeader]): a back press is
 * not discoverable, and the two dark capsules read as the only way out otherwise —
 * neither of which closes the dialog.
 */
@Composable
fun TestAlertSoundDialog(
    onDismissRequest: () -> Unit,
    modifier: Modifier = Modifier,
    output: AlertOutput? = null
) {
    Dialog(
        onDismissRequest = onDismissRequest,
        properties = DialogProperties(usePlatformDefaultWidth = false)
    ) {
        val context = LocalContext.current
        val scope = rememberCoroutineScope()
        val playback = remember(output) {
            TestAlertPlayback(
                scope = scope,
                output = output ?: DeviceAlertOutput(context.applicationContext)
            )
        }
        // The stop-on-disposal guarantee. Registered here rather than inside the
        // card so it covers every way the Dialog can leave the composition,
        // including a configuration change mid-test.
        DisposableEffect(playback) {
            onDispose { playback.stop() }
        }

        val isPlaying by playback.isPlaying.collectAsStateWithLifecycle()
        val remainingSeconds by playback.remainingSeconds.collectAsStateWithLifecycle()

        TestAlertSoundCard(
            isPlaying = isPlaying,
            remainingSeconds = remainingSeconds,
            onStart = playback::start,
            onStop = playback::stop,
            onDismiss = onDismissRequest,
            modifier = modifier.padding(Dimens.ScreenHorizontalPadding)
        )
    }
}

/**
 * The card body (Figma node 144:1025), stateless so previews and any future
 * instrumented test can drive it directly.
 *
 * @param isPlaying raises START's fill while a test is sounding — the design has no
 *   engaged variant, and a button that looks the same whether or not it took effect
 *   is the worse guess.
 * @param remainingSeconds printed on START while a test runs ("START (5s)"), so the
 *   siren has a visible end. 0 leaves the plain label.
 * @param onDismiss closes the modal from the header X. Distinct from [onStop], which
 *   silences the phone and leaves the warning copy on screen.
 */
@Composable
fun TestAlertSoundCard(
    isPlaying: Boolean,
    onStart: () -> Unit,
    onStop: () -> Unit,
    modifier: Modifier = Modifier,
    remainingSeconds: Int = 0,
    onDismiss: (() -> Unit)? = null
) {
    Column(
        modifier = modifier
            .fillMaxWidth()
            .wrapContentHeight()
            .background(
                brush = EmergencyAlertGradient,
                shape = RoundedCornerShape(Dimens.EmergencyCardRadius)
            )
            .border(
                width = Dimens.BorderThin,
                color = CardBorder,
                shape = RoundedCornerShape(Dimens.EmergencyCardRadius)
            )
            .padding(Dimens.EmergencyCardPadding),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(Dimens.EmergencyCardPadding)
    ) {
        // Title left empty: the card prints its own headline beneath the badge in the
        // design's larger style, so the shared header is here for the close X and its
        // alignment, not for a second title.
        if (onDismiss != null) {
            QuakeModalHeader(onDismiss = onDismiss, title = "")
        }

        Column(
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(Dimens.EmergencyBadgeTitleGap)
        ) {
            Column(
                modifier = Modifier
                    .size(Dimens.EmergencyIconBadgeSize)
                    .background(EmergencyAlertIconBadgeFill, CircleShape),
                verticalArrangement = Arrangement.Center,
                horizontalAlignment = Alignment.CenterHorizontally
            ) {
                Icon(
                    painter = painterResource(id = R.drawable.ic_alert_triangle_lg),
                    contentDescription = null,
                    tint = TextPrimary,
                    modifier = Modifier.size(Dimens.EmergencyIconGlyphSize)
                )
            }

            Text(text = TEST_ALERT_TITLE, style = EmergencyAlertTitle)
        }

        Text(text = TEST_ALERT_BODY, style = TestAlertBodyText)

        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.spacedBy(
                space = Dimens.TestAlertActionsGap,
                alignment = Alignment.CenterHorizontally
            ),
            verticalAlignment = Alignment.CenterVertically
        ) {
            TestAlertAction(
                label = if (isPlaying && remainingSeconds > 0) {
                    "START (${remainingSeconds}s)"
                } else {
                    "START"
                },
                engaged = isPlaying,
                onClick = onStart
            )
            TestAlertAction(
                label = "STOP",
                engaged = false,
                onClick = onStop
            )
        }
    }
}

/**
 * One of the two actions (Figma nodes 144:1050 / 144:1055).
 *
 * A `clickable` [Row] rather than a Material `Button` for the same reason as
 * [id.web.quakealert.ui.warning.EmergencyControls]: the design's flat 2px-stroked
 * capsule has neither elevation nor a tonal container, and a `Button` would fight
 * it on both. Accessibility comes back explicitly through [Role.Button],
 * `onClickLabel` and [minimumInteractiveComponentSize].
 */
@Composable
private fun TestAlertAction(
    label: String,
    engaged: Boolean,
    onClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    Row(
        modifier = modifier
            .minimumInteractiveComponentSize()
            .background(
                color = if (engaged) TestAlertStartFill else EmergencyControlFillIdle,
                shape = RoundedCornerShape(Dimens.EmergencyControlRadius)
            )
            .border(
                width = Dimens.EmergencyControlBorderWidth,
                color = EmergencyControlBorder,
                shape = RoundedCornerShape(Dimens.EmergencyControlRadius)
            )
            .clickable(role = Role.Button, onClickLabel = label, onClick = onClick)
            .padding(
                horizontal = Dimens.EmergencyMutePaddingHorizontal,
                vertical = Dimens.EmergencyMutePaddingVertical
            ),
        horizontalArrangement = Arrangement.Center,
        verticalAlignment = Alignment.CenterVertically
    ) {
        Text(
            text = label,
            style = EmergencyControlLabel,
            maxLines = 1,
            // A minimum rather than a fixed width: the label grows by four glyphs
            // while counting down, and the design's 90dp would clip it.
            modifier = Modifier
                .widthIn(min = Dimens.TestAlertActionWidth)
                .height(Dimens.EmergencyCtaHeight)
                .wrapContentHeight(Alignment.CenterVertically)
        )
    }
}

/** Title copy, node 144:1030. */
private const val TEST_ALERT_TITLE = "Test Alert Sound"

/**
 * Body copy, node 144:1033, verbatim from the design — including the typographic
 * apostrophe and the blank line between the two warnings.
 */
private const val TEST_ALERT_BODY =
    "Make sure you’re not playing this on public place to prevent panic. " +
        "It will play a loud noise.\n\n" +
        "If not working, check required permission and device volume settings."

@Preview(showBackground = true, backgroundColor = 0xFF0A0A0A)
@Composable
private fun TestAlertSoundCardPreview() {
    QuakeAlertTheme {
        TestAlertSoundCard(isPlaying = false, onStart = {}, onStop = {}, onDismiss = {})
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF0A0A0A)
@Composable
private fun TestAlertSoundCardPlayingPreview() {
    QuakeAlertTheme {
        TestAlertSoundCard(
            isPlaying = true,
            onStart = {},
            onStop = {},
            remainingSeconds = 3,
            onDismiss = {}
        )
    }
}
