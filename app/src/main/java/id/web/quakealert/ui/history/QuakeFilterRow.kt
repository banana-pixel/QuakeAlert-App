package id.web.quakealert.ui.history

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
 * Row of filter controls beneath the header (Figma node 1:711):
 *  - "All" pill
 *  - "Near - {radius}km" pill
 *  - a trailing calendar icon button
 *
 * The row is stateless: the currently [selectedFilter] and callbacks are
 * hoisted to the caller.
 */
@Composable
fun QuakeFilterRow(
    selectedFilter: HistoryFilter,
    nearRadiusKm: Int,
    onFilterSelected: (HistoryFilter) -> Unit,
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
            selected = selectedFilter == HistoryFilter.ALL,
            onClick = { onFilterSelected(HistoryFilter.ALL) }
        )
        FilterPill(
            label = "Near - ${nearRadiusKm}km",
            selected = selectedFilter == HistoryFilter.NEAR,
            onClick = { onFilterSelected(HistoryFilter.NEAR) }
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
    val fill = if (selected) FilterActiveFill else FilterInactiveFill
    Box(
        modifier = modifier
            .height(Dimens.FilterPillHeight)
            .clip(RoundedCornerShape(Dimens.RadiusSmall))
            .background(fill, RoundedCornerShape(Dimens.RadiusSmall))
            .border(Dimens.BorderThin, CardBorder, RoundedCornerShape(Dimens.RadiusSmall))
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
 * Rounded-square (squircle) calendar icon button that opens a date-range
 * picker. Uses the same height, corner radius and border token as the filter
 * pills so it aligns flush with them (Figma node 1:701).
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
