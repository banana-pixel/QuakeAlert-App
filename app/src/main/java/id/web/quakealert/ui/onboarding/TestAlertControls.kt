package id.web.quakealert.ui.onboarding

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import id.web.quakealert.ui.common.QuakeCard
import id.web.quakealert.ui.common.QuakeSwitch
import id.web.quakealert.ui.theme.Dimens

/**
 * Controls hosted by Onboarding Page 6 (Figma node 1:426). Both rows are built
 * on the shared [QuakeCard] so they match the Settings setting rows exactly:
 *  - a tappable "Test Alert" action card that fires a local notification, and
 *  - a "Keep Alerting" row with a [QuakeSwitch] that makes the alert insistent.
 */
@Composable
fun TestAlertControls(
    keepAlerting: Boolean,
    onKeepAlertingChange: (Boolean) -> Unit,
    onTestAlert: () -> Unit,
    modifier: Modifier = Modifier
) {
    Column(
        modifier = modifier.fillMaxWidth(),
        verticalArrangement = Arrangement.spacedBy(Dimens.SettingsSectionSpacing)
    ) {
        // "Test Alert" action card — whole surface is clickable.
        QuakeCard(title = "Test Alert", onClick = onTestAlert)

        // "Keep Alerting" row with the shared custom toggle switch.
        QuakeCard(title = "Keep Alerting") {
            QuakeSwitch(
                checked = keepAlerting,
                onCheckedChange = onKeepAlertingChange
            )
        }
    }
}
