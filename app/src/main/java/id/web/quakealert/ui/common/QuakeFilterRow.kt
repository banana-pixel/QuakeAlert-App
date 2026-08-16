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
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import id.web.quakealert.R
import id.web.quakealert.ui.theme.CardBorder
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.FilterActiveFill
import id.web.quakealert.ui.theme.FilterInactiveFill
import id.web.quakealert.ui.theme.NunitoFontFamily
import id.web.quakealert.ui.theme.TextPrimary

/**
 * Shared row of filter controls beneath a screen header, used identically by
 * History (Figma node 1:711) and Sensors (Figma node 1:1105):
 *  - "All" pill
 *  - "Near - {radius}km" pill
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
            label = "Near - ${nearRadiusKm}km",
            selected = selectedFilter == QuakeFilter.NEAR,
            onClick = { onFilterSelected(QuakeFilter.NEAR) }
        )
        Spacer(modifier = Modifier.weight(1f))
        CalendarButton(onClick = onCalendarClicked)
    }
}

/** Stadium/pill toggle used for the "All" and "Near" filters. */
@Composable
private fun FilterPill(
    label: String,
    selected: Boolean,
    onClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    val shape = RoundedCornerShape(Dimens.RadiusSmall)
    val fill = if (selected) FilterActiveFill else FilterInactiveFill
    Box(
        modifier = modifier
            .height(Dimens.FilterPillHeight)
            .clip(shape)
            .background(fill, shape)
            .border(Dimens.BorderThin, CardBorder, shape)
            .clickable(onClick = onClick)
            .padding(
                horizontal = Dimens.FilterPillPaddingHorizontal,
                vertical = Dimens.FilterPillPaddingVertical
            ),
        contentAlignment = Alignment.Center
    ) {
        Text(
            text = label,
            color = TextPrimary,
            fontFamily = NunitoFontFamily,
            fontWeight = FontWeight.Bold,
            fontSize = 13.sp
        )
    }
}

/**
 * Rounded-square (squircle) calendar icon button that opens a date-range picker.
 * Uses the same height, corner radius and border token as the filter pills so it
 * aligns flush with them (Figma node 1:701 / 1:1105).
 */
@Composable
private fun CalendarButton(
    onClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    val shape = RoundedCornerShape(Dimens.RadiusSmall)
    Box(
        modifier = modifier
            .size(Dimens.FilterPillHeight)
            .clip(shape)
            .background(FilterInactiveFill, shape)
            .border(Dimens.BorderThin, CardBorder, shape)
            .clickable(onClick = onClick)
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
