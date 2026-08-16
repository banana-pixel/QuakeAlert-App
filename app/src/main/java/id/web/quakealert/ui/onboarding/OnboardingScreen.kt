package id.web.quakealert.ui.onboarding

import android.Manifest
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import android.os.PowerManager
import android.provider.Settings
import android.widget.Toast
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.animation.core.animateDpAsState
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.Image
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.systemBarsPadding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.pager.HorizontalPager
import androidx.compose.foundation.pager.rememberPagerState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.core.content.ContextCompat
import id.web.quakealert.R
import id.web.quakealert.ui.theme.AccentBlueTranslucent
import id.web.quakealert.ui.theme.BorderLight
import id.web.quakealert.ui.theme.NunitoFontFamily
import id.web.quakealert.ui.theme.OnboardingBackgroundBrush
import id.web.quakealert.ui.theme.OverlayLight
import id.web.quakealert.ui.theme.QuakeAlertTheme
import id.web.quakealert.ui.theme.TextPrimary
import id.web.quakealert.ui.theme.TextSecondary
import kotlinx.coroutines.launch

/**
 * Horizontal inset applied to page content and the bottom controls. Kept out
 * of the pager itself so [HorizontalPager.pageSpacing] shows as a clean gap
 * between pages while swiping, without clipping the resting content.
 */
private val ScreenHorizontalPadding = 28.dp

/** Whether POST_NOTIFICATIONS is granted (always true below API 33). */
private fun isNotificationPermissionGranted(context: Context): Boolean {
    if (Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU) return true
    return ContextCompat.checkSelfPermission(
        context,
        Manifest.permission.POST_NOTIFICATIONS
    ) == PackageManager.PERMISSION_GRANTED
}

/** Whether at least coarse location is granted. */
private fun isLocationPermissionGranted(context: Context): Boolean {
    val fine = ContextCompat.checkSelfPermission(
        context, Manifest.permission.ACCESS_FINE_LOCATION
    ) == PackageManager.PERMISSION_GRANTED
    val coarse = ContextCompat.checkSelfPermission(
        context, Manifest.permission.ACCESS_COARSE_LOCATION
    ) == PackageManager.PERMISSION_GRANTED
    return fine || coarse
}

/** Whether the app is exempt from battery optimizations. */
private fun isIgnoringBatteryOptimizations(context: Context): Boolean {
    val pm = context.getSystemService(Context.POWER_SERVICE) as PowerManager
    return pm.isIgnoringBatteryOptimizations(context.packageName)
}

/**
 * Full onboarding flow — data-driven across seven Figma pages (nodes 1:470,
 * 1:337, 1:354, 1:378, 1:402, 1:426, 1:453). Pages are described by a list of
 * [OnboardingPage] and rendered through a [HorizontalPager]. The indicator and
 * bottom action row react to the pager's current page. Interactive pages
 * (notification, battery, location, test-alert) are wired to runtime
 * permission launchers and system intents.
 */
@Composable
fun OnboardingScreen(
    modifier: Modifier = Modifier,
    onFinish: () -> Unit = {}
) {
    val pages = rememberOnboardingPages()
    val pagerState = rememberPagerState(pageCount = { pages.size })
    val coroutineScope = rememberCoroutineScope()
    val context = LocalContext.current

    // --- Permission / requirement state -----------------------------------
    var notificationGranted by remember {
        mutableStateOf(isNotificationPermissionGranted(context))
    }
    var locationGranted by remember {
        mutableStateOf(isLocationPermissionGranted(context))
    }
    var batteryUnrestricted by remember {
        mutableStateOf(isIgnoringBatteryOptimizations(context))
    }
    var keepAlerting by remember { mutableStateOf(false) }

    val notificationLauncher = rememberLauncherForActivityResult(
        contract = ActivityResultContracts.RequestPermission()
    ) { granted -> notificationGranted = granted }

    val locationLauncher = rememberLauncherForActivityResult(
        contract = ActivityResultContracts.RequestMultiplePermissions()
    ) { result ->
        locationGranted = result.values.any { it }
    }

    // Re-check requirements that are resolved outside the app (Settings screens).
    val settingsLauncher = rememberLauncherForActivityResult(
        contract = ActivityResultContracts.StartActivityForResult()
    ) {
        batteryUnrestricted = isIgnoringBatteryOptimizations(context)
    }

    val requestNotification: () -> Unit = {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            notificationLauncher.launch(Manifest.permission.POST_NOTIFICATIONS)
        } else {
            notificationGranted = true
        }
    }

    val requestLocation: () -> Unit = {
        locationLauncher.launch(
            arrayOf(
                Manifest.permission.ACCESS_FINE_LOCATION,
                Manifest.permission.ACCESS_COARSE_LOCATION
            )
        )
    }

    val requestBattery: () -> Unit = {
        openBatteryOptimizationSettings(context, settingsLauncher::launch)
    }

    val fireTestAlert: () -> Unit = {
        val shown = TestAlertNotifier.showTestAlert(context, keepAlerting)
        if (!shown) {
            Toast.makeText(
                context,
                "Enable notifications first to test alerts.",
                Toast.LENGTH_SHORT
            ).show()
            requestNotification()
        }
    }

    Box(
        modifier = modifier
            .fillMaxSize()
            .background(OnboardingBackgroundBrush)
    ) {
        Column(
            modifier = Modifier
                .fillMaxSize()
                .systemBarsPadding()
                .padding(vertical = 20.dp)
        ) {
            // The pager itself spans full width (no horizontal inset) so that
            // pageSpacing reads as a clean gap between pages while swiping.
            // Per-page horizontal padding lives inside OnboardingPageItem so the
            // resting content stays aligned with the bottom controls below.
            HorizontalPager(
                state = pagerState,
                pageSpacing = 32.dp,
                modifier = Modifier
                    .fillMaxWidth()
                    .weight(1f)
            ) { pageIndex ->
                OnboardingPageItem(
                    page = pages[pageIndex],
                    notificationGranted = notificationGranted,
                    locationGranted = locationGranted,
                    batteryUnrestricted = batteryUnrestricted,
                    keepAlerting = keepAlerting,
                    onRequestNotification = requestNotification,
                    onRequestLocation = requestLocation,
                    onRequestBattery = requestBattery,
                    onKeepAlertingChange = { keepAlerting = it },
                    onTestAlert = fireTestAlert,
                    modifier = Modifier.padding(horizontal = ScreenHorizontalPadding)
                )
            }

            Spacer(modifier = Modifier.height(20.dp))

            // Bottom controls share the same horizontal padding as the page
            // content so everything lines up when a page rests in place.
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = ScreenHorizontalPadding)
            ) {
                PageIndicator(
                    pageCount = pages.size,
                    currentPage = pagerState.currentPage,
                    modifier = Modifier.align(Alignment.CenterHorizontally)
                )

                Spacer(modifier = Modifier.height(20.dp))

                // Bottom actions: single CTA on the first page, Back/Next otherwise.
                if (pagerState.currentPage == 0) {
                    PrimaryButton(
                        text = pages[0].actionText ?: "Start",
                        onClick = {
                            if (pages.size > 1) {
                                coroutineScope.launch { pagerState.animateScrollToPage(1) }
                            } else {
                                onFinish()
                            }
                        },
                        modifier = Modifier.fillMaxWidth()
                    )
                } else {
                    val isLast = pagerState.currentPage == pages.lastIndex
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.spacedBy(20.dp),
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        SecondaryButton(
                            text = "Back",
                            onClick = {
                                coroutineScope.launch {
                                    pagerState.animateScrollToPage(pagerState.currentPage - 1)
                                }
                            },
                            modifier = Modifier.weight(1f)
                        )
                        PrimaryButton(
                            text = if (isLast) "Get Started" else "Next",
                            onClick = {
                                if (isLast) {
                                    onFinish()
                                } else {
                                    coroutineScope.launch {
                                        pagerState.animateScrollToPage(pagerState.currentPage + 1)
                                    }
                                }
                            },
                            modifier = Modifier.weight(1f)
                        )
                    }
                }
            }
        }
    }
}

/**
 * Renders a single page: centered illustration, title, description and any
 * page-specific interactive control selected by [OnboardingPage.kind].
 */
@Composable
fun OnboardingPageItem(
    page: OnboardingPage,
    notificationGranted: Boolean,
    locationGranted: Boolean,
    batteryUnrestricted: Boolean,
    keepAlerting: Boolean,
    onRequestNotification: () -> Unit,
    onRequestLocation: () -> Unit,
    onRequestBattery: () -> Unit,
    onKeepAlertingChange: (Boolean) -> Unit,
    onTestAlert: () -> Unit,
    modifier: Modifier = Modifier
) {
    Column(
        modifier = modifier.fillMaxSize(),
        verticalArrangement = Arrangement.SpaceBetween,
        horizontalAlignment = Alignment.Start
    ) {
        Box(
            modifier = Modifier
                .fillMaxWidth()
                .weight(1f),
            contentAlignment = Alignment.Center
        ) {
            // Illustrations are full-colour vector art; render them untinted
            // (Image, not Icon) so every page shows its original artwork
            // consistently instead of a single flat tint.
            Image(
                painter = painterResource(id = page.iconRes),
                contentDescription = page.title,
                modifier = Modifier.size(150.dp)
            )
        }

        Column(
            modifier = Modifier.fillMaxWidth(),
            verticalArrangement = Arrangement.spacedBy(20.dp)
        ) {
            Text(
                text = page.title,
                color = TextPrimary,
                fontFamily = NunitoFontFamily,
                fontWeight = FontWeight.ExtraBold,
                fontSize = if (page.largeTitle) 32.sp else 24.sp,
                lineHeight = if (page.largeTitle) 36.sp else 26.sp,
                textAlign = TextAlign.Start,
                modifier = Modifier.fillMaxWidth()
            )

            when (page.kind) {
                OnboardingPageKind.READY -> ReadyText(modifier = Modifier.fillMaxWidth())
                else -> Text(
                    text = page.description,
                    color = TextSecondary,
                    fontFamily = NunitoFontFamily,
                    fontWeight = FontWeight.Normal,
                    fontSize = 14.sp,
                    lineHeight = 24.sp,
                    textAlign = TextAlign.Start,
                    modifier = Modifier.fillMaxWidth()
                )
            }

            when (page.kind) {
                OnboardingPageKind.NOTIFICATION_PERMISSION -> PermissionCard(
                    title = page.cardTitle,
                    isGranted = notificationGranted,
                    grantedLabel = page.grantedLabel,
                    onClick = onRequestNotification
                )

                OnboardingPageKind.BATTERY_OPTIMIZATION -> PermissionCard(
                    title = page.cardTitle,
                    isGranted = batteryUnrestricted,
                    grantedLabel = page.grantedLabel,
                    onClick = onRequestBattery
                )

                OnboardingPageKind.LOCATION_PERMISSION -> PermissionCard(
                    title = page.cardTitle,
                    isGranted = locationGranted,
                    grantedLabel = page.grantedLabel,
                    onClick = onRequestLocation
                )

                OnboardingPageKind.TEST_ALERT -> TestAlertControls(
                    keepAlerting = keepAlerting,
                    onKeepAlertingChange = onKeepAlertingChange,
                    onTestAlert = onTestAlert
                )

                else -> Unit
            }
        }
    }
}

/**
 * Determinate page indicator: a rounded track whose active white segment
 * animates to sit under the current page (anchored left).
 */
@Composable
private fun PageIndicator(
    pageCount: Int,
    currentPage: Int,
    modifier: Modifier = Modifier
) {
    val totalWidth = 100.dp
    val segment = if (pageCount > 0) totalWidth / pageCount else totalWidth
    val activeWidth by animateDpAsState(targetValue = segment, label = "indicatorWidth")
    val offset by animateDpAsState(targetValue = segment * currentPage, label = "indicatorOffset")

    Box(
        modifier = modifier
            .width(totalWidth)
            .height(6.dp)
            .clip(RoundedCornerShape(100.dp))
            .background(OverlayLight)
    ) {
        Box(
            modifier = Modifier
                .padding(start = offset)
                .width(activeWidth)
                .height(6.dp)
                .clip(RoundedCornerShape(100.dp))
                .background(TextPrimary)
        )
    }
}

/** Filled cyan/dark-blue CTA button (Start / Next / Get Started). */
@Composable
private fun PrimaryButton(
    text: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    Button(
        onClick = onClick,
        shape = RoundedCornerShape(40.dp),
        colors = ButtonDefaults.buttonColors(
            containerColor = AccentBlueTranslucent,
            contentColor = TextPrimary
        ),
        modifier = modifier
            .height(51.dp)
            .border(width = 3.dp, color = BorderLight, shape = RoundedCornerShape(40.dp))
    ) {
        ButtonLabel(text)
    }
}

/** Bordered, transparent-fill button (Back). */
@Composable
private fun SecondaryButton(
    text: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    OutlinedButton(
        onClick = onClick,
        shape = RoundedCornerShape(40.dp),
        colors = ButtonDefaults.outlinedButtonColors(
            containerColor = Color.Transparent,
            contentColor = TextPrimary
        ),
        border = androidx.compose.foundation.BorderStroke(3.dp, BorderLight),
        modifier = modifier.height(51.dp)
    ) {
        ButtonLabel(text)
    }
}

@Composable
private fun ButtonLabel(text: String) {
    Text(
        text = text,
        color = TextPrimary,
        fontFamily = NunitoFontFamily,
        fontWeight = FontWeight.Bold,
        fontSize = 15.sp
    )
}

/**
 * Opens the per-app battery-optimization exemption dialog, falling back to the
 * general battery-optimization settings list if the direct request is
 * unavailable on the device.
 */
private fun openBatteryOptimizationSettings(
    context: Context,
    launch: (Intent) -> Unit
) {
    val direct = Intent(
        Settings.ACTION_REQUEST_IGNORE_BATTERY_OPTIMIZATIONS,
        Uri.parse("package:${context.packageName}")
    )
    if (direct.resolveActivity(context.packageManager) != null) {
        launch(direct)
    } else {
        launch(Intent(Settings.ACTION_IGNORE_BATTERY_OPTIMIZATION_SETTINGS))
    }
}

@Composable
private fun rememberOnboardingPages(): List<OnboardingPage> = listOf(
    OnboardingPage(
        iconRes = R.drawable.ic_puzzle_piece,
        title = "Welcome to QuakeAlert App.",
        description = "QuakeAlert is community based earthquake early warning " +
            "system (platform). Keep safe with intelligent real time EWS " +
            "alert that can be notified through this app!",
        actionText = "Start",
        largeTitle = true
    ),
    OnboardingPage(
        iconRes = R.drawable.ic_sensor_chip,
        title = "Based on low cost Sensors that can be placed all over the world.",
        description = "This is a community supported early warning system. You can " +
            "place your own low cost sensors on your home. Just need a stable WiFi " +
            "network and you\u2019re good to go! Read disclaimer and guides here on " +
            "our GitHub pages."
    ),
    OnboardingPage(
        iconRes = R.drawable.ic_notification_permission,
        title = "Please allow notification permission.",
        description = "To receive earthquake alerts, QuakeAlert App needs " +
            "permission to send you notifications.",
        kind = OnboardingPageKind.NOTIFICATION_PERMISSION,
        cardTitle = "Allow Notification",
        grantedLabel = "Allowed"
    ),
    OnboardingPage(
        iconRes = R.drawable.ic_battery_optimization,
        title = "Please set battery optimization settings.",
        description = "To ensure alerts are never delayed, QuakeAlert App needs " +
            "to run witout battery restrictions.",
        kind = OnboardingPageKind.BATTERY_OPTIMIZATION,
        cardTitle = "Disable Restrictions",
        grantedLabel = "Disabled"
    ),
    OnboardingPage(
        iconRes = R.drawable.ic_location_permission,
        title = "Please allow precise location access.",
        description = "To calculate your location from the earthquake center and " +
            "give relevant and accurate earthquake alerts.",
        kind = OnboardingPageKind.LOCATION_PERMISSION,
        cardTitle = "Allow Precise Location Access",
        grantedLabel = "Allowed"
    ),
    OnboardingPage(
        iconRes = R.drawable.ic_alert_test,
        title = "Test alert.",
        description = "Send a test notification to make sure the notification " +
            "service is working. You can make it just ring once, or make it keep " +
            "alerting until you make it stop.",
        kind = OnboardingPageKind.TEST_ALERT
    ),
    OnboardingPage(
        iconRes = R.drawable.ic_ready_smiley,
        title = "You\u2019re ready.",
        description = "",
        kind = OnboardingPageKind.READY
    )
)

@Preview(showBackground = true, widthDp = 402, heightDp = 874)
@Composable
private fun OnboardingScreenPreview() {
    QuakeAlertTheme {
        OnboardingScreen()
    }
}
