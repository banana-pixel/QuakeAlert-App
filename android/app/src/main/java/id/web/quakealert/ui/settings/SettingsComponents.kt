package id.web.quakealert.ui.settings

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.IntrinsicSize
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
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
import id.web.quakealert.domain.SafetyPolicy
import id.web.quakealert.ui.theme.AboutButtonFill
import id.web.quakealert.ui.theme.AboutCardBorder
import id.web.quakealert.ui.theme.AboutCardGradient
import id.web.quakealert.ui.theme.BorderLight
import id.web.quakealert.ui.theme.CardBorder
import id.web.quakealert.ui.theme.CardSubtitle
import id.web.quakealert.ui.theme.CardTitle
import id.web.quakealert.ui.theme.ChipLabel
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.InfoPillFill
import id.web.quakealert.ui.theme.MmiRed
import id.web.quakealert.ui.theme.PillLabel
import id.web.quakealert.ui.theme.SectionHeaderPillFill
import id.web.quakealert.ui.theme.SegmentActiveFill
import id.web.quakealert.ui.theme.SegmentInactiveFill
import id.web.quakealert.ui.theme.TextPrimary
import id.web.quakealert.ui.theme.TextSecondary
import kotlin.math.roundToInt

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
 * All pills are sized to the widest option (via [IntrinsicSize.Max] + equal
 * [RowScope.weight]) so the "125 km / 250 km / 500 km" and "EN / ID" boxes are
 * uniform squares rather than hugging each label independently.
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
        modifier = modifier.width(IntrinsicSize.Max),
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
 * Read-only protection status, the card body that replaced the coverage-radius
 * slider.
 *
 * The slider was removed rather than merely disabled because the setting itself was
 * the mistake: it let someone trade away their own warning to get fewer
 * notifications, and the only person who would ever learn that was the wrong trade
 * is them, after an earthquake. Operational EEW systems make this call centrally for
 * exactly that reason.
 *
 * What is left is an explanation. Nothing here is tappable, and it does not pretend
 * to be — no switch shape, no chevron. It states the two rules in force
 * ([id.web.quakealert.domain.SafetyPolicy]) so a user who notices the slider is gone
 * can see what replaced it and that it is wider, not narrower, than what they could
 * have chosen.
 *
 * @param radiusLabel the fixed alert radius in the user's unit system.
 */
@Composable
fun ProtectionStatusCardBody(
    radiusLabel: String,
    modifier: Modifier = Modifier
) {
    Column(
        modifier = modifier.fillMaxWidth(),
        verticalArrangement = Arrangement.spacedBy(Dimens.SettingCardContentGap)
    ) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically
        ) {
            Text(text = "Automatic", style = CardTitle, color = TextPrimary)
            InfoPill(text = "Always on")
        }

        ProtectionRule(
            title = "Alerts within $radiusLabel",
            detail = "Any earthquake whose estimated centroid falls inside this " +
                "distance sounds the alarm. The radius is set by the system, the " +
                "same value the server uses to choose who to notify."
        )

        ProtectionRule(
            title = "Severe quakes ignore distance",
            detail = "MMI VII and above, or peak ground acceleration of " +
                "${SafetyPolicy.OVERRIDE_PGA_GAL.roundToInt()} gal or more, alarms " +
                "wherever you are. At that size there is no distance at which you " +
                "did not need to know."
        )
    }
}

/**
 * One rule inside [ProtectionStatusCardBody]: a bold claim and the reason for it.
 *
 * Split out so both rules are laid out identically — the point of the card is that
 * these are two facts of equal standing, not a headline and a footnote.
 */
@Composable
private fun ProtectionRule(title: String, detail: String) {
    Column(verticalArrangement = Arrangement.spacedBy(Dimens.SettingCardTitleGap)) {
        Text(text = title, style = ChipLabel, color = TextPrimary)
        Text(text = detail, style = CardSubtitle, color = TextSecondary)
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
