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
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.selected
import androidx.compose.ui.semantics.semantics
import id.web.quakealert.R
import id.web.quakealert.data.UnitSystem
import id.web.quakealert.ui.theme.CardBorder
import id.web.quakealert.ui.theme.ChipLabel
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.FilterActiveFill
import id.web.quakealert.ui.theme.FilterInactiveFill
import id.web.quakealert.ui.theme.MmiOrange
import id.web.quakealert.ui.theme.TextPrimary


/**
 * Shared row of filter controls beneath a screen header, used identically by
 * History (Figma node 1:711) and Sensors (Figma node 1:1105):
 *  - "All" pill
 *  - "Near" pill
 *  - a trailing `filter-lines` button opening [QuakeFilterDialog], rendered only
 *    when [onFilterSheetClicked] is supplied
 *
 * The "Near" pill no longer prints its radius. The radius is now the user's to
 * choose in the sheet, so baking it into the pill label would either go stale or
 * make the pill grow and shrink as the sheet changed — the active radius is shown
 * in the sheet, and named again by the no-data card when a filter finds nothing.
 *
 * The row is stateless: the current [filter] and callbacks are hoisted to the
 * caller, which on both screens is the shared [QuakeFilterViewModel].
 *
 * @param unitSystem still required — the sheet's radius options are rendered in
 *   the user's unit, and this row hands it down.
 */
@Composable
fun QuakeFilterRow(
    filter: QuakeFilterState,
    unitSystem: UnitSystem,
    onModeSelected: (QuakeFilter) -> Unit,
    onFilterSheetClicked: (() -> Unit)? = null,
    modifier: Modifier = Modifier
) {
    Row(
        modifier = modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.spacedBy(Dimens.FilterRowGap),
        verticalAlignment = Alignment.CenterVertically
    ) {
        FilterPill(
            label = "All",
            selected = filter.mode == QuakeFilter.ALL,
            onClick = { onModeSelected(QuakeFilter.ALL) }
        )
        FilterPill(
            label = "Near",
            selected = filter.mode == QuakeFilter.NEAR,
            onClick = { onModeSelected(QuakeFilter.NEAR) }
        )
        // Omitted, not disabled, when the caller has no sheet to offer: a visible
        // control that does nothing when tapped reads as a broken app rather than a
        // missing feature.
        onFilterSheetClicked?.let { openSheet ->
            Spacer(modifier = Modifier.weight(1f))
            FilterSheetButton(
                activeCriteriaCount = filter.activeCriteriaCount,
                onClick = openSheet
            )
        }
    }
}

/**
 * Stadium/pill toggle used for the "All" and "Near" filters.
 *
 * The pill keeps its compact Figma 30dp height with no touch-target padding.
 * [Role.RadioButton] plus `selected` semantics let TalkBack announce the pair
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
 * Rounded-square (squircle) `filter-lines` button that opens [QuakeFilterDialog]
 * (Figma node 1:714, glyph component 149:1093). Uses the same height, corner
 * radius and border token as the filter pills so it aligns flush with them, kept
 * at its compact Figma 30dp with no touch-target padding.
 *
 * @param activeCriteriaCount how many sheet criteria are narrowing the query. A
 *   non-zero count paints a dot on the corner and is spoken in the content
 *   description, because a filter the user forgot about is indistinguishable from
 *   a network with nothing to report.
 */
@Composable
private fun FilterSheetButton(
    activeCriteriaCount: Int,
    onClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    val shape = RoundedCornerShape(Dimens.RadiusSmall)
    val description = if (activeCriteriaCount == 0) {
        "Filter"
    } else {
        "Filter, $activeCriteriaCount active"
    }
    Box(
        modifier = modifier
            .size(Dimens.FilterPillHeight)
            .clip(shape)
            .background(FilterInactiveFill, shape)
            .border(Dimens.BorderThin, CardBorder, shape)
            .clickable(role = Role.Button, onClick = onClick)
            .padding(Dimens.CalendarButtonPadding),
        contentAlignment = Alignment.Center
    ) {
        Icon(
            painter = painterResource(id = R.drawable.ic_filter_lines),
            contentDescription = description,
            tint = TextPrimary,
            modifier = Modifier.size(Dimens.FilterTriggerGlyphSize)
        )
        if (activeCriteriaCount > 0) {
            Box(
                modifier = Modifier
                    .align(Alignment.TopEnd)
                    // Nudged outward so the dot reads as a badge on the button's corner
                    // rather than as part of the glyph.
                    .offset(x = Dimens.FilterTriggerBadgeSize / 2, y = -Dimens.FilterTriggerBadgeSize / 2)
                    .size(Dimens.FilterTriggerBadgeSize)
                    .clip(RoundedCornerShape(Dimens.RadiusStadium))
                    .background(MmiOrange)
            )
        }
    }
}
