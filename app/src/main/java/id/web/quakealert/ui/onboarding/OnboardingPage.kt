package id.web.quakealert.ui.onboarding

import androidx.annotation.DrawableRes

/**
 * Distinguishes the interactive behaviour hosted by a given onboarding page.
 * Keeps [OnboardingScreen] data-driven while letting each page opt into the
 * right control (permission card, action row, closing links, …).
 */
enum class OnboardingPageKind {
    /** Plain informational page (illustration + copy). */
    INFO,

    /** Notification runtime permission card. */
    NOTIFICATION_PERMISSION,

    /** Battery-optimization exemption card. */
    BATTERY_OPTIMIZATION,

    /** Precise-location runtime permission card. */
    LOCATION_PERMISSION,

    /** "Test Alert" action + "Keep Alerting" switch. */
    TEST_ALERT,

    /** Closing page with clickable GitHub/credit links. */
    READY
}

/**
 * Data model for a single onboarding page. Driving the UI from a list of
 * these keeps [OnboardingScreen] data-driven and easy to extend.
 *
 * @param iconRes drawable resource for the page illustration.
 * @param title headline text (rendered with Nunito ExtraBold).
 * @param description supporting body text.
 * @param actionText optional single-CTA label used for the first page ("Start").
 * @param kind the interactive behaviour hosted by this page.
 * @param cardTitle title shown inside the [PermissionCard] (when applicable).
 * @param grantedLabel badge label displayed once the requirement is satisfied.
 * @param largeTitle when true, renders the title with the 32/36 hero style used
 *   by the welcome page (Figma node 1:470); all other pages use the 24/26 style
 *   (e.g. node 1:453), keeping headline sizing consistent across the flow.
 */
data class OnboardingPage(
    @param:DrawableRes val iconRes: Int,
    val title: String,
    val description: String,
    val actionText: String? = null,
    val kind: OnboardingPageKind = OnboardingPageKind.INFO,
    val cardTitle: String = "",
    val grantedLabel: String = "Allowed",
    val largeTitle: Boolean = false
)
