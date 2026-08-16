package id.web.quakealert

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.ui.Modifier
import id.web.quakealert.ui.onboarding.OnboardingScreen
import id.web.quakealert.ui.theme.QuakeAlertTheme

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        setContent {
            QuakeAlertTheme {
                OnboardingScreen(modifier = Modifier.fillMaxSize())
            }
        }
    }
}
