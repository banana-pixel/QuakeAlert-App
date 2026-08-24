package id.web.quakealert.ui.app

import androidx.compose.animation.AnimatedContent
import androidx.compose.animation.core.tween
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.togetherWith
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.lifecycle.compose.LifecycleStartEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import id.web.quakealert.ui.addsensor.AddSensorScreen
import id.web.quakealert.ui.addsensor.AddSensorViewModel
import id.web.quakealert.ui.main.MainScreen
import id.web.quakealert.ui.onboarding.OnboardingScreen
import id.web.quakealert.ui.theme.BackgroundGradientBottom

/**
 * Application entry point that gates the UI behind the onboarding flag. It reads
 * the persisted state from [AppViewModel] and shows either the onboarding flow
 * or the main app.
 *
 * The two destinations cross-fade via [AnimatedContent] (a fade-through, per
 * Rule D) so the transition from onboarding to [MainScreen] is smooth. Both
 * branches sit on the shared dark [BackgroundGradientBottom] surface and own
 * their own window insets, so there is no status-bar/inset flicker across the
 * swap. While the flag is still loading, a neutral background is held to avoid a
 * flash of the wrong screen.
 */
@Composable
fun AppRoot(
    modifier: Modifier = Modifier,
    viewModel: AppViewModel = viewModel()
) {
    val uiState by viewModel.uiState.collectAsStateWithLifecycle()

    // Every foreground is a chance for the stored position to have gone stale — the
    // process-start check alone misses an app that was simply left in the background.
    // Placed on the root rather than in a tab so it covers onboarding too, and nothing
    // is needed on the way out.
    LifecycleStartEffect(Unit) {
        viewModel.onAppForegrounded()
        onStopOrDispose { }
    }

    Box(
        modifier = modifier
            .fillMaxSize()
            .background(BackgroundGradientBottom)
    ) {
        AnimatedContent(
            targetState = uiState,
            transitionSpec = {
                fadeIn(tween(300)).togetherWith(fadeOut(tween(300)))
            },
            label = "AppRootTransition"
        ) { state ->
            when (state) {
                is AppUiState.Loading -> Box(modifier = Modifier.fillMaxSize())

                is AppUiState.Ready -> {
                    if (state.onboardingCompleted) {
                        // The wizard overlays everything, exactly like onboarding:
                        // a full-screen flow that owns the back gesture until it is
                        // dismissed, so no tab sits half-visible behind a step.
                        var showAddSensor by remember { mutableStateOf(false) }
                        if (showAddSensor) {
                            val wizard: AddSensorViewModel = viewModel()
                            val wizardState by wizard.state.collectAsStateWithLifecycle()
                            AddSensorScreen(
                                state = wizardState,
                                onDismiss = {
                                    showAddSensor = false
                                    wizard.onDismissed()
                                },
                                onNameChanged = wizard::onNameChanged,
                                onModelChanged = wizard::onModelChanged,
                                onUseCurrentPosition = wizard::onUseCurrentPosition,
                                onManualLatitudeChanged = wizard::onManualLatitudeChanged,
                                onManualLongitudeChanged = wizard::onManualLongitudeChanged,
                                onDetailsContinue = wizard::onDetailsContinue,
                                onRetryFromDetails = wizard::retryFromDetails,
                                onSecretRevealed = wizard::onSecretRevealed,
                                onCredentialsContinue = wizard::onCredentialsContinue,
                                onRescanClicked = wizard::onRescanClicked,
                                onSsidSelected = wizard::onSsidSelected,
                                onPasswordChanged = wizard::onPasswordChanged,
                                onConfigureNode = wizard::onConfigureNode,
                                onRefreshConfirm = wizard::onRefreshConfirm,
                                modifier = Modifier.fillMaxSize()
                            )
                        } else {
                            MainScreen(
                                modifier = Modifier.fillMaxSize(),
                                onAddSensor = { showAddSensor = true }
                            )
                        }
                    } else {
                        OnboardingScreen(
                            modifier = Modifier.fillMaxSize(),
                            onFinish = viewModel::completeOnboarding
                        )
                    }
                }
            }
        }
    }
}
