package id.web.quakealert.ui.onboarding

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import id.web.quakealert.ui.common.QuakeCard
import id.web.quakealert.ui.theme.CardSubtitle
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.TextSecondary

/**
 * Controls hosted by Onboarding Page 6 (Figma node 1:426): two tappable action
 * cards that let the user confirm an alert would actually reach them.
 *
 * Two rather than one because "an alert reaches me" has two independent failure
 * modes and the page is the only chance to catch either. "Test Notification" fires a
 * local notification, which proves the notification channel and the OS grant; "Test
 * Alert Sound" opens [id.web.quakealert.ui.common.TestAlertSoundDialog], which proves
 * the siren is audible on this device's alarm volume. A phone with notifications
 * granted and its alarm stream turned down passes the first and fails the second, and
 * only the second failure is silent when a real quake arrives.
 *
 * Each card says what it proves, in a line under its title. The pair used to read
 * "Test Alert" and "Test Alert Sound" — one word apart, which left the user to guess
 * whether the second was the first with audio or a different thing entirely. The
 * distinction is worth spelling out precisely because both can pass while an alert
 * still arrives silently.
 */
@Composable
fun TestAlertControls(
    onTestAlert: () -> Unit,
    onTestAlertSound: () -> Unit,
    modifier: Modifier = Modifier
) {
    Column(
        modifier = modifier.fillMaxWidth(),
        verticalArrangement = Arrangement.spacedBy(Dimens.CardListSpacing)
    ) {
        QuakeCard(
            title = "Test Notification",
            onClick = onTestAlert,
            modifier = Modifier.fillMaxWidth(),
            detail = { CardHint("Sends one now, to check alerts reach your screen") }
        )
        QuakeCard(
            title = "Test Alert Sound",
            onClick = onTestAlertSound,
            modifier = Modifier.fillMaxWidth(),
            detail = { CardHint("Plays the siren, to check it is loud enough to wake you") }
        )
    }
}

/** One line under a card title saying what tapping it proves. */
@Composable
private fun CardHint(text: String) {
    Text(text = text, style = CardSubtitle, color = TextSecondary)
}
