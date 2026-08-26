package id.web.quakealert.ui.warning

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import id.web.quakealert.domain.SafetyPolicy
import id.web.quakealert.ui.common.QuakeModalHeader
import id.web.quakealert.ui.settings.InfoPill
import id.web.quakealert.ui.theme.CardBorder
import id.web.quakealert.ui.theme.CardSubtitle
import id.web.quakealert.ui.theme.CardTitle
import id.web.quakealert.ui.theme.ChipLabel
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.PossibilityModalGradient
import id.web.quakealert.ui.theme.QuakeAlertTheme
import id.web.quakealert.ui.theme.TextPrimary
import id.web.quakealert.ui.theme.TextSecondary
import kotlin.math.roundToInt

/**
 * "Protection Status" as an overlay on the Warning screen, in its own [Dialog]
 * window. Back press and outside taps route to [onDismiss], so it can never trap
 * navigation during an alert.
 *
 * This used to be a permanent card in Settings, and moving it is the point: it
 * answers one question — "will this app actually warn me?" — and the place a user
 * asks it is the screen that does the warning, not a settings list they opened to
 * change something. As a card it also had to be read past on every visit; as an
 * overlay it is there when asked for and gone otherwise.
 *
 * @param radiusLabel the fixed alert radius in the user's unit system.
 * @param alertsEnabled the user's earthquake-warnings switch (Settings).
 * @param notificationsPermitted the OS `POST_NOTIFICATIONS` grant; false means every
 *   warning is dropped no matter what either switch says.
 * @param onDismiss invoked by the close button, back press or an outside tap.
 */
@Composable
fun ProtectionStatusModalDialog(
    radiusLabel: String,
    alertsEnabled: Boolean,
    notificationsPermitted: Boolean,
    onDismiss: () -> Unit
) {
    Dialog(
        onDismissRequest = onDismiss,
        properties = DialogProperties(usePlatformDefaultWidth = false)
    ) {
        ProtectionStatusModal(
            radiusLabel = radiusLabel,
            alertsEnabled = alertsEnabled,
            notificationsPermitted = notificationsPermitted,
            onDismiss = onDismiss,
            modifier = Modifier.padding(Dimens.ScreenHorizontalPadding)
        )
    }
}

/**
 * Stateless card behind [ProtectionStatusModalDialog]: the shared
 * [QuakeModalHeader], the mode with a badge that states the actual state, then the
 * two rules in force ([SafetyPolicy]).
 *
 * The badge is the honesty surface: it mirrors [ProtectionStatus.headline]'s ranking
 * (permission blocked > user switch off > active) rather than ever claiming "always".
 *
 * Nothing here is tappable and it does not pretend to be — no switch shape, no
 * chevron. The coverage radius was a slider once; the setting itself was the
 * mistake, because it let someone trade away their own warning to get fewer
 * notifications and the only person who would ever learn that was the wrong trade
 * is them, after an earthquake. Operational EEW systems make this call centrally
 * for exactly that reason, so what is left is the explanation: a user who notices
 * there is nothing to adjust can see what replaced it, and that it is wider than
 * anything they could have chosen.
 *
 * Exposed separately from the dialog so it can be previewed without a window.
 */
@Composable
fun ProtectionStatusModal(
    radiusLabel: String,
    alertsEnabled: Boolean,
    notificationsPermitted: Boolean,
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
        QuakeModalHeader(onDismiss = onDismiss, title = "Protection Status")

        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically
        ) {
            Text(text = "Automatic", style = CardTitle, color = TextPrimary)
            InfoPill(text = statusBadge(alertsEnabled, notificationsPermitted))
        }

        ProtectionRule(
            title = "Alerts within $radiusLabel",
            detail = "Any earthquake whose estimated centroid falls inside this " +
                "distance sounds the alarm. The radius is set by the system, the " +
                "same value the server uses to choose who to notify."
        )

        ProtectionRule(
            title = "Severe quakes ignore distance",
            detail = "MMI VII and above, or peak ground acceleration of " +
                "${SafetyPolicy.OVERRIDE_PGA_GAL.roundToInt()} gal or more, alarms " +
                "wherever you are. At that size there is no distance at which you " +
                "did not need to know."
        )

        if (!alertsEnabled || !notificationsPermitted) {
            ProtectionRule(
                title = if (alertsEnabled) {
                    "Warnings cannot be delivered"
                } else {
                    // The badge already says "Turned off"; the row is the fix, not an
                    // echo of the fact.
                    "Re-enable earthquake warnings in Settings."
                },
                detail = if (alertsEnabled) {
                    // Permission blocked: the app wants to warn, the OS will not post.
                    "Notifications are blocked in system settings. Allow them for " +
                        "QuakeAlert so warnings can reach your screen."
                } else {
                    // User switch off: point at where it lives.
                    "The earthquake-warnings switch is on the Settings screen. " +
                        "Turning it back on restores protection immediately."
                }
            )
        }
    }
}

/**
 * The badge text next to "Automatic", mirroring [ProtectionStatus.headline]'s
 * ranking: a revoked grant is the loudest fact, then the user's own switch, then
 * active protection. One word each — the rules below carry the explanation.
 */
private fun statusBadge(alertsEnabled: Boolean, notificationsPermitted: Boolean): String =
    when {
        !notificationsPermitted -> "Blocked"
        !alertsEnabled -> "Turned off"
        else -> "Active"
    }

/**
 * One rule inside [ProtectionStatusModal]: a bold claim and the reason for it.
 *
 * Split out so both rules are laid out identically — the point of the overlay is
 * that these are two facts of equal standing, not a headline and a footnote.
 */
@Composable
private fun ProtectionRule(title: String, detail: String) {
    Column(verticalArrangement = Arrangement.spacedBy(Dimens.SettingCardTitleGap)) {
        Text(text = title, style = ChipLabel, color = TextPrimary)
        Text(text = detail, style = CardSubtitle, color = TextSecondary)
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun ProtectionStatusModalPreview() {
    QuakeAlertTheme {
        ProtectionStatusModal(
            radiusLabel = "200 km",
            alertsEnabled = true,
            notificationsPermitted = true,
            onDismiss = {}
        )
    }
}
