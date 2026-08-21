package id.web.quakealert.ui.onboarding

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import id.web.quakealert.ui.common.QuakeCard
import id.web.quakealert.ui.theme.Dimens

/**
 * Controls hosted by Onboarding Page 6 (Figma node 1:426): two tappable action
 * cards that let the user confirm an alert would actually reach them.
 *
 * Two rather than one because "an alert reaches me" has two independent failure
 * modes and the page is the only chance to catch either. "Test Alert" fires a local
 * notification, which proves the notification channel and the OS grant; "Test Alert
 * Sound" opens [id.web.quakealert.ui.common.TestAlertSoundDialog], which proves the
 * siren is audible on this device's alarm volume. A phone with notifications granted
 * and its alarm stream turned down passes the first and fails the second, and only
 * the second failure is silent when a real quake arrives.
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
            title = "Test Alert",
            onClick = onTestAlert,
            modifier = Modifier.fillMaxWidth()
        )
        QuakeCard(
            title = "Test Alert Sound",
            onClick = onTestAlertSound,
            modifier = Modifier.fillMaxWidth()
        )
    }
}
