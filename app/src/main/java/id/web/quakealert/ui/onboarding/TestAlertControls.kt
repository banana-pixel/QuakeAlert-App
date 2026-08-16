package id.web.quakealert.ui.onboarding

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Switch
import androidx.compose.material3.SwitchDefaults
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import id.web.quakealert.ui.theme.AccentBlue
import id.web.quakealert.ui.theme.BorderLight
import id.web.quakealert.ui.theme.CardDark
import id.web.quakealert.ui.theme.NunitoFontFamily
import id.web.quakealert.ui.theme.TextPrimary

/**
 * Controls hosted by Onboarding Page 6 (Figma node 1:426):
 *  - a tappable "Test Alert" card that fires a local notification, and
 *  - a "Keep Alerting" row with a toggle that makes the alert insistent.
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
        verticalArrangement = Arrangement.spacedBy(20.dp)
    ) {
        // "Test Alert" action card — whole surface is clickable.
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .clip(RoundedCornerShape(16.dp))
                .background(CardDark)
                .border(2.dp, BorderLight, RoundedCornerShape(16.dp))
                .clickable(onClick = onTestAlert)
                .padding(10.dp),
            verticalAlignment = Alignment.CenterVertically
        ) {
            Text(
                text = "Test Alert",
                color = TextPrimary,
                fontFamily = NunitoFontFamily,
                fontWeight = FontWeight.Bold,
                fontSize = 15.sp,
                lineHeight = 20.sp
            )
        }

        // "Keep Alerting" row with a toggle switch. Uses the same 10.dp padding
        // as the Test Alert / permission cards so all cards share one height.
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .clip(RoundedCornerShape(16.dp))
                .background(CardDark)
                .border(2.dp, BorderLight, RoundedCornerShape(16.dp))
                .padding(10.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.SpaceBetween
        ) {
            Text(
                text = "Keep Alerting",
                color = TextPrimary,
                fontFamily = NunitoFontFamily,
                fontWeight = FontWeight.Bold,
                fontSize = 15.sp,
                lineHeight = 20.sp
            )
            Switch(
                checked = keepAlerting,
                onCheckedChange = onKeepAlertingChange,
                colors = SwitchDefaults.colors(
                    checkedThumbColor = TextPrimary,
                    checkedTrackColor = AccentBlue,
                    uncheckedThumbColor = TextPrimary,
                    uncheckedTrackColor = CardDark,
                    uncheckedBorderColor = BorderLight
                )
            )
        }
    }
}
