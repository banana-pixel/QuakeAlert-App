package id.web.quakealert.ui.common

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
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
import androidx.compose.ui.semantics.selected
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import id.web.quakealert.R
import id.web.quakealert.data.UnitSystem
import id.web.quakealert.ui.theme.CardBorder
import id.web.quakealert.ui.theme.ChipLabel
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.FilterActiveFill
import id.web.quakealert.ui.theme.FilterInactiveFill
import id.web.quakealert.ui.theme.TextPrimary


/**
 * Shared row of filter controls beneath a screen header, used identically by
 * History (Figma node 1:711) and Sensors (Figma node 1:1105):
 *  - "All" pill
 *  - "Near - {radius}{unit}" pill (km or mi, driven by [UnitSystem])
 *  - a trailing calendar icon button
 *
 * The row is stateless and generic over the shared [QuakeFilter] enum so both
 * screens can reuse it without duplicating styling or token wiring. The current
 * [selectedFilter] and callbacks are hoisted to the caller.
 */
@Composable
fun QuakeFilterRow(
    selectedFilter: QuakeFilter,
    nearRadiusKm: Int,
    unitSystem: UnitSystem,
    onFilterSelected: (QuakeFilter) -> Unit,
    onCalendarClicked: () -> Unit,
    modifier: Modifier = Modifier
) {
    Row(
        modifier = modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.spacedBy(Dimens.FilterRowGap),
        verticalAlignment = Alignment.CenterVertically
    ) {
        FilterPill(
            label = "All",
            selected = selectedFilter == QuakeFilter.ALL,
            onClick = { onFilterSelected(QuakeFilter.ALL) }
        )
        FilterPill(
            label = "Near - ${unitSystem.convertFromKm(nearRadiusKm)}${unitSystem.distanceUnit}",
            selected = selectedFilter == QuakeFilter.NEAR,
            onClick = { onFilterSelected(QuakeFilter.NEAR) }
        )
        Spacer(modifier = Modifier.weight(1f))
        CalendarButton(onClick = onCalendarClicked)
    }
}

/**
 * Stadium/pill toggle used for the "All" and "Near" filters.
 *
 * Accessibility: the pill is 30dp tall per Figma, so [minimumInteractiveComponentSize]
 * pads its *touch* box out to the 48dp minimum. It is applied before the sizing and
 * background chrome, so the drawn capsule keeps its Figma height, radius, padding and
 * stroke exactly — only the invisible hit area (and therefore the row's own height)
 * grows. [Role.RadioButton] plus `selected` semantics let TalkBack announce the pair
 * as a single-choice group and say which pill is active.
 */
@Composable
private fun FilterPill(
    label: String,
    selected: Boolean,
    onClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    val shape = RoundedCornerShape(Dimens.RadiusSmall)
    val fill = if (selected) FilterActiveFill else FilterInactiveFill
    // Captured under a distinct name: inside the semantics lambda a bare
    // `selected` would resolve to SemanticsPropertyReceiver.selected (whose getter
    // throws) rather than to this parameter.
    val isSelected = selected
    Box(
        modifier = modifier
            .minimumInteractiveComponentSize()
            .height(Dimens.FilterPillHeight)
            .clip(shape)
            .background(fill, shape)
            .border(Dimens.BorderThin, CardBorder, shape)
            .clickable(role = Role.RadioButton, onClick = onClick)
            .semantics { this.selected = isSelected }
            .padding(
                horizontal = Dimens.FilterPillPaddingHorizontal,
                vertical = Dimens.FilterPillPaddingVertical
            ),
        contentAlignment = Alignment.Center
    ) {
        // Shared ChipLabel: centered metrics (includeFontPadding=false +
        // LineHeightStyle.Center) so the glyph sits optically centered in the
        // fixed-height pill instead of drifting toward the bottom edge.
        Text(text = label, style = ChipLabel)
    }
}


/**
 * Rounded-square (squircle) calendar icon button that opens a date-range picker.
 * Uses the same height, corner radius and border token as the filter pills so it
 * aligns flush with them (Figma node 1:701 / 1:1105).
 *
 * Accessibility: like the pills, the drawn square stays at its Figma 30dp while
 * [minimumInteractiveComponentSize] lifts the touch box to 48dp.
 */
@Composable
private fun CalendarButton(
    onClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    val shape = RoundedCornerShape(Dimens.RadiusSmall)
    Box(
        modifier = modifier
            .minimumInteractiveComponentSize()
            .size(Dimens.FilterPillHeight)
            .clip(shape)
            .background(FilterInactiveFill, shape)
            .border(Dimens.BorderThin, CardBorder, shape)
            .clickable(role = Role.Button, onClick = onClick)
            .padding(Dimens.CalendarButtonPadding),
        contentAlignment = Alignment.Center
    ) {
        Icon(
            painter = painterResource(id = R.drawable.ic_calendar),
            contentDescription = "Filter by date",
            tint = TextPrimary,
            modifier = Modifier.size(16.dp)
        )
    }
}
