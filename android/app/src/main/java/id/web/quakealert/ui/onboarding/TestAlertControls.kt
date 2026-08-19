package id.web.quakealert.ui.onboarding

import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import id.web.quakealert.ui.common.QuakeCard

/**
 * Controls hosted by Onboarding Page 6 (Figma node 1:426): a tappable "Test
 * Alert" action card that fires a local notification so the user can confirm
 * alerts reach them.
 */
@Composable
fun TestAlertControls(
    onTestAlert: () -> Unit,
    modifier: Modifier = Modifier
) {
    QuakeCard(
        title = "Test Alert",
        onClick = onTestAlert,
        modifier = modifier.fillMaxWidth()
    )
}