package id.web.quakealert.ui.settings

import androidx.annotation.DrawableRes
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.material3.minimumInteractiveComponentSize
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.style.TextAlign
import id.web.quakealert.R
import id.web.quakealert.ui.theme.AboutButtonFill
import id.web.quakealert.ui.theme.AboutCardBorder
import id.web.quakealert.ui.theme.AboutCardGradient
import id.web.quakealert.ui.theme.BorderFaint
import id.web.quakealert.ui.theme.BorderLight
import id.web.quakealert.ui.theme.CardBorder
import id.web.quakealert.ui.theme.CardSubtitle
import id.web.quakealert.ui.theme.CardTitle
import id.web.quakealert.ui.theme.ChipLabel
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.InfoPillFill
import id.web.quakealert.ui.theme.MmiOrange
import id.web.quakealert.ui.theme.MmiRed
import id.web.quakealert.ui.theme.PillLabel
import id.web.quakealert.ui.theme.SectionHeaderPillFill
import id.web.quakealert.ui.theme.SegmentActiveFill
import id.web.quakealert.ui.theme.SegmentInactiveFill
import id.web.quakealert.ui.theme.SuccessGreen
import id.web.quakealert.ui.theme.SuccessGreenTranslucent
import id.web.quakealert.ui.theme.TextPrimary
import id.web.quakealert.ui.theme.TextSecondary

/**
 * Full-width section header pill (Figma node 1:846 / EL-c963c95e) that labels a
 * group of setting cards, e.g. "Location & Coverage". A flat dark capsule with a
 * centered bold label.
 */
@Composable
fun SectionHeaderPill(
    title: String,
    modifier: Modifier = Modifier
) {
    val shape = RoundedCornerShape(Dimens.RadiusStadium)
    Box(modifier = modifier.fillMaxWidth(), contentAlignment = Alignment.Center) {
        Box(
            modifier = Modifier
                .height(Dimens.SectionHeaderPillHeight)
                .clip(shape)
                .background(SectionHeaderPillFill, shape)
                .border(Dimens.BorderThin, CardBorder, shape)
                .padding(horizontal = Dimens.SectionHeaderPillPaddingHorizontal),
            contentAlignment = Alignment.Center
        ) {
            Text(text = title, style = CardTitle)
        }
    }
}

/**
 * Small info pill used on setting cards to surface metadata such as
 * "Last Sync : 2 min. ago" (Figma node 1:880). Flat translucent-black capsule
 * with a dimmed centered label.
 */
@Composable
fun InfoPill(
    text: String,
    modifier: Modifier = Modifier
) {
    val shape = RoundedCornerShape(Dimens.InfoPillRadius)
    Box(
        modifier = modifier
            .height(Dimens.InfoPillHeight)
            .clip(shape)
            .background(InfoPillFill, shape)
            .padding(
                horizontal = Dimens.InfoPillPaddingHorizontal,
                vertical = Dimens.InfoPillPaddingVertical
            ),
        contentAlignment = Alignment.Center
    ) {
        Text(text = text, style = PillLabel, color = TextSecondary)
    }
}

/**
 * "Sync now" refresh icon button (Figma node 1:882, refresh-cw-02): the trailing
 * control on the "Sync Location Now" card. Rendered as the flat 32dp Figma vector
 * with no container fill, per the design.
 *
 * Accessibility: the glyph keeps its 32dp token while
 * [minimumInteractiveComponentSize] lifts the touch box to the 48dp minimum, and
 * the tap carries the standard ripple bounded to the circular clip.
 */
@Composable
fun SyncRefreshButton(
    onClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    Box(
        modifier = modifier
            .minimumInteractiveComponentSize()
            .size(Dimens.SyncRefreshIconSize)
            .clip(RoundedCornerShape(Dimens.SyncRefreshIconSize))
            .clickable(role = Role.Button, onClick = onClick),
        contentAlignment = Alignment.Center
    ) {
        Icon(
            painter = painterResource(id = R.drawable.ic_refresh_cw),
            contentDescription = "Sync location now",
            tint = TextPrimary,
            modifier = Modifier.size(Dimens.SyncRefreshIconSize)
        )
    }
}

/**
 * Horizontal segmented toggle control (Figma node 1:872 Coverage / 1:912
 * Language): a row of standalone bordered pills (there is no outer container in
 * the design) where the selected [options] entry is highlighted with
 * [SegmentActiveFill] and the rest use [SegmentInactiveFill]. Every pill carries
 * the same 2px white-30% stroke and 12dp radius.
 *
 * The row fills its width and the pills share it equally (via [RowScope.weight]),
 * so every control on the screen has the same geometry regardless of how long its
 * labels are. Sizing the row to its widest label instead — which is what it used to
 * do — made "EN / ID" collapse to two cramped boxes beside a full-width
 * "Metric / Imperial", as though the two settings were different kinds of control.
 *
 * @param options the selectable values.
 * @param selected the currently selected value.
 * @param labelOf maps an option to its pill label.
 * @param onSelect invoked with the tapped option.
 */
@Composable
fun <T> QuakeSegmentedControl(
    options: List<T>,
    selected: T,
    labelOf: (T) -> String,
    onSelect: (T) -> Unit,
    modifier: Modifier = Modifier
) {
    Row(
        modifier = modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.spacedBy(Dimens.SegmentRowGap),
        verticalAlignment = Alignment.CenterVertically
    ) {
        options.forEach { option ->
            SegmentPill(
                label = labelOf(option),
                selected = option == selected,
                onClick = { onSelect(option) },
                modifier = Modifier.weight(1f)
            )
        }
    }
}

/** A single standalone pill within [QuakeSegmentedControl]. */
@Composable
private fun SegmentPill(
    label: String,
    selected: Boolean,
    onClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    val shape = RoundedCornerShape(Dimens.SegmentPillRadius)
    val fill = if (selected) SegmentActiveFill else SegmentInactiveFill
    Box(
        modifier = modifier
            .clip(shape)
            .background(fill, shape)
            .border(Dimens.BorderMedium, BorderLight, shape)
            .clickable(onClick = onClick)
            .padding(
                horizontal = Dimens.SegmentPillPaddingHorizontal,
                vertical = Dimens.SegmentPillPaddingVertical
            ),
        contentAlignment = Alignment.Center
    ) {
        Text(
            text = label,
            style = ChipLabel,
            color = TextPrimary,
            textAlign = TextAlign.Center
        )
    }
}

/**
 * "About" card (Figma node 1:918): a gradient (khaki → green, 28% alpha) rounded
 * card with a soft green stroke that stacks a two-line credit block (bold app
 * credit + dimmed version) above a full-width caramel "More About Us"
 * call-to-action button (node 1:922).
 *
 * @param credit primary credit line ("QuakeAlert App by @banana-pixel").
 * @param version secondary version line ("v 1.0.1 (Beta)"), dimmed.
 * @param onMoreAboutUs invoked when the CTA button is tapped.
 */
@Composable
fun AboutCard(
    credit: String,
    version: String,
    onMoreAboutUs: () -> Unit,
    modifier: Modifier = Modifier
) {
    val shape = RoundedCornerShape(Dimens.SettingCardRadius)
    Column(
        modifier = modifier
            .fillMaxWidth()
            .clip(shape)
            .background(AboutCardGradient, shape)
            .border(Dimens.BorderThin, AboutCardBorder, shape)
            .padding(
                horizontal = Dimens.SettingCardPaddingHorizontal,
                vertical = Dimens.SettingCardPaddingVertical
            ),
        verticalArrangement = Arrangement.spacedBy(Dimens.SettingCardContentGap)
    ) {
        Column(verticalArrangement = Arrangement.spacedBy(Dimens.SettingCardTitleGap)) {
            Text(text = credit, style = CardTitle)
            Text(text = version, style = CardSubtitle)
        }

        val buttonShape = RoundedCornerShape(Dimens.AboutButtonRadius)

        Box(
            modifier = Modifier
                .fillMaxWidth()
                .clip(buttonShape)
                .background(AboutButtonFill, buttonShape)
                .border(Dimens.BorderMedium, BorderLight, buttonShape)
                .clickable(onClick = onMoreAboutUs)
                .padding(
                    horizontal = Dimens.AboutButtonPaddingHorizontal,
                    vertical = Dimens.AboutButtonPaddingVertical
                ),
            contentAlignment = Alignment.Center
        ) {
            Text(text = "More About Us", style = ChipLabel, color = TextPrimary)
        }
    }
}

/**
 * The permissions hub's body: the three system prerequisites an alert has to clear
 * before it can reach the user, each with its live grant state and a tap that fixes
 * it (Settings "Alert & Notification" section).
 *
 * Grouped into one panel rather than left as three unrelated rows because they fail
 * as a set: notifications allowed but location denied means every alert is discarded
 * by distance before it is shown, and a user reading a single green row has no way to
 * know that. The summary line above them counts what is ready, so a partial state
 * cannot pass for a working one.
 *
 * Each row is a glyph, a name and its state, the same shape the onboarding
 * [id.web.quakealert.ui.onboarding.PermissionCard] uses. The reasons each permission
 * matters are not repeated here: onboarding already makes that case at the moment the
 * user is deciding, and three paragraphs of consequence turned a checklist meant to
 * be scanned in a second into the longest card on the screen.
 *
 * @param notificationGranted the OS `POST_NOTIFICATIONS` grant.
 * @param locationGranted whether a position can be read at all.
 * @param batteryUnrestricted whether Doze is exempted for this app.
 * @param onFixNotifications opens the app's notification settings.
 * @param onFixLocation requests (or, after a terminal decline, points at) the
 *   location permission.
 * @param onFixBattery opens the battery-optimisation screen.
 */
@Composable
fun PermissionsHubCardBody(
    notificationGranted: Boolean,
    locationGranted: Boolean,
    batteryUnrestricted: Boolean,
    onFixNotifications: () -> Unit,
    onFixLocation: () -> Unit,
    onFixBattery: () -> Unit,
    modifier: Modifier = Modifier
) {
    Column(
        modifier = modifier.fillMaxWidth(),
        verticalArrangement = Arrangement.spacedBy(Dimens.SettingCardContentGap)
    ) {
        PermissionHubRow(
            iconRes = R.drawable.ic_notification_permission,
            title = "Notifications",
            granted = notificationGranted,
            grantedLabel = "Allowed",
            onFix = onFixNotifications
        )
        PermissionHubRow(
            iconRes = R.drawable.ic_pin_location,
            title = "Precise Location",
            granted = locationGranted,
            grantedLabel = "Allowed",
            onFix = onFixLocation
        )
        PermissionHubRow(
            iconRes = R.drawable.ic_battery_optimization,
            title = "Background Delivery",
            granted = batteryUnrestricted,
            grantedLabel = "Unrestricted",
            onFix = onFixBattery
        )
    }
}

/**
 * One prerequisite in [PermissionsHubCardBody].
 *
 * Clickable only while unsatisfied: a granted row has nothing left to do, and a tap
 * that opens system Settings to show the user what they already did is noise. The
 * missing state is tinted [MmiOrange] rather than red — it is a gap the user can
 * close in one tap, not a failure.
 *
 * One line tall, like the onboarding card it mirrors: what the row has to say is the
 * name and the state, and the label on the unsatisfied side says what the tap does
 * ("Tap to allow") rather than naming the row a problem ("Fix").
 */
@Composable
private fun PermissionHubRow(
    @DrawableRes iconRes: Int,
    title: String,
    granted: Boolean,
    grantedLabel: String,
    onFix: () -> Unit
) {
    val shape = RoundedCornerShape(Dimens.RadiusSmall)
    val base = Modifier
        .fillMaxWidth()
        .clip(shape)
        .background(if (granted) SuccessGreenTranslucent else InfoPillFill, shape)
        .border(Dimens.BorderThin, if (granted) BorderFaint else BorderLight, shape)
    Row(
        modifier = (if (granted) base else base.clickable(role = Role.Button, onClick = onFix))
            .padding(Dimens.SettingCardContentGap),
        horizontalArrangement = Arrangement.spacedBy(Dimens.SettingCardContentGap),
        verticalAlignment = Alignment.CenterVertically
    ) {
        Icon(
            painter = painterResource(id = iconRes),
            contentDescription = null,
            tint = if (granted) SuccessGreen else MmiOrange,
            modifier = Modifier.size(Dimens.PermissionHubGlyphSize)
        )
        Text(
            text = title,
            style = ChipLabel,
            color = TextPrimary,
            modifier = Modifier.weight(1f)
        )
        Row(
            horizontalArrangement = Arrangement.spacedBy(Dimens.SettingCardTitleGap),
            verticalAlignment = Alignment.CenterVertically
        ) {
            if (granted) {
                Icon(
                    painter = painterResource(id = R.drawable.ic_check_circle),
                    contentDescription = null,
                    tint = SuccessGreen,
                    modifier = Modifier.size(Dimens.PermissionHubBadgeGlyphSize)
                )
            }
            Text(
                text = if (granted) grantedLabel else "Tap to allow",
                style = ChipLabel,
                color = if (granted) TextPrimary else MmiOrange
            )
        }
    }
}

/**
 * Two-line read-only row for an identity value — the pseudonym or the `user_id` —
 * with a copy affordance.
 *
 * The id is monospace-adjacent in intent: it is a UUID the user may have to quote
 * in a bug report, so it is shown in full and copyable rather than truncated.
 *
 * @param label what the value is ("Pseudonym").
 * @param value the value itself, or null while the identity is still bootstrapping.
 * @param onCopy invoked with the value when the row is tapped; absent for a value
 *   not worth copying.
 */
@Composable
fun IdentityRow(
    label: String,
    value: String?,
    onCopy: ((String) -> Unit)? = null,
    modifier: Modifier = Modifier
) {
    val shown = value?.takeIf { it.isNotBlank() }
    val rowModifier = if (shown != null && onCopy != null) {
        modifier.clickable(role = Role.Button) { onCopy(shown) }
    } else {
        modifier
    }
    Column(
        modifier = rowModifier.fillMaxWidth(),
        verticalArrangement = Arrangement.spacedBy(Dimens.SettingCardTitleGap)
    ) {
        Text(text = label, style = CardSubtitle, color = TextSecondary)
        Text(
            // "Not signed in yet" rather than an empty line: the bootstrap is a
            // network call, so this state is reachable on a cold start offline.
            text = shown ?: "Not signed in yet",
            style = CardTitle,
            color = TextPrimary
        )
    }
}

/**
 * Full-width secondary action button used by the Account & Privacy section
 * ("Reroll Pseudonym", "Reset Profile").
 *
 * @param label the action.
 * @param enabled false while the action is in flight, so a destructive request
 *   cannot be fired twice.
 * @param destructive tints the stroke and label red for the irreversible action,
 *   so "Reset Profile" does not look like "Reroll Pseudonym".
 */
@Composable
fun SettingsActionButton(
    label: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    enabled: Boolean = true,
    destructive: Boolean = false
) {
    val shape = RoundedCornerShape(Dimens.SegmentPillRadius)
    val stroke = if (destructive) MmiRed else BorderLight
    val content = when {
        !enabled -> TextSecondary
        destructive -> MmiRed
        else -> TextPrimary
    }
    Box(
        modifier = modifier
            .fillMaxWidth()
            .clip(shape)
            .background(SegmentInactiveFill, shape)
            .border(Dimens.BorderMedium, stroke, shape)
            .clickable(enabled = enabled, role = Role.Button, onClick = onClick)
            .padding(
                horizontal = Dimens.SegmentPillPaddingHorizontal,
                vertical = Dimens.SegmentPillPaddingVertical
            ),
        contentAlignment = Alignment.Center
    ) {
        Text(text = label, style = ChipLabel, color = content, textAlign = TextAlign.Center)
    }
}
