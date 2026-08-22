package id.web.quakealert.ui.common

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.selected
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import id.web.quakealert.data.UnitSystem
import id.web.quakealert.data.network.QuakeApiClient
import id.web.quakealert.domain.SafetyPolicy
import id.web.quakealert.ui.theme.CardBorder
import id.web.quakealert.ui.theme.CardSubtitle
import id.web.quakealert.ui.theme.CardTitle
import id.web.quakealert.ui.theme.ChipLabel
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.EmergencyCtaBorder
import id.web.quakealert.ui.theme.EmergencyCtaFill
import id.web.quakealert.ui.theme.EventDetailModalGradient
import id.web.quakealert.ui.theme.FilterActiveFill
import id.web.quakealert.ui.theme.FilterInactiveFill
import id.web.quakealert.ui.theme.MmiOrange
import id.web.quakealert.ui.theme.QuakeAlertTheme
import id.web.quakealert.ui.theme.SectionTitle

/**
 * The filter sheet behind the [QuakeFilterRow] trigger (Figma node 1:709 covers
 * the row; the sheet itself has no design, so it borrows the overlay chrome of
 * [QuakeEventDetailModalDialog] rather than inventing a second modal language).
 *
 * Which criteria appear is the caller's choice, passed as [sections]:
 *  - **shaking intensity**, labelled in MMI and sent to `/events` as a PGA floor
 *    in gal (History);
 *  - **search radius**, a browse radius and nothing else (both tabs);
 *  - **time range**, sent as `since` (History);
 *  - **station status**, applied over the `/sensors` response (Sensors).
 *
 * Sensors is not offered intensity or time, and History is not offered station
 * status. A station has no shaking and no time of occurrence, so a sheet that
 * showed them there would be handing the user controls that cannot change the roll
 * in front of them.
 *
 * Choices are held locally and committed on "Apply". Editing the shared filter
 * live would re-query both tabs on every tap — several requests to reach one
 * answer, and a list flickering behind a sheet the user has not finished reading.
 *
 * @param filter the criteria currently in force, used to seed the draft.
 * @param sections which criteria this screen can act on; use
 *   [FilterSection.HISTORY] or [FilterSection.SENSORS].
 * @param unitSystem renders the radius options in the user's unit.
 * @param onApply commits the drafted state; wire to
 *   [QuakeFilterViewModel.onCriteriaApplied].
 * @param onReset clears every criterion and closes the sheet.
 */
@Composable
fun QuakeFilterDialog(
    filter: QuakeFilterState,
    sections: Set<FilterSection>,
    unitSystem: UnitSystem,
    onDismiss: () -> Unit,
    onApply: (QuakeFilterState) -> Unit,
    onReset: () -> Unit,
    modifier: Modifier = Modifier
) {
    Dialog(
        onDismissRequest = onDismiss,
        properties = DialogProperties(usePlatformDefaultWidth = false)
    ) {
        val shape = remember { RoundedCornerShape(Dimens.RadiusCard) }
        // One draft of the whole state rather than a variable per criterion: the
        // sections are conditional, and a per-criterion draft would need a branch
        // for every combination to assemble the result.
        var draft by remember(filter) { mutableStateOf(filter) }

        Column(
            modifier = modifier
                .padding(Dimens.ScreenHorizontalPadding)
                .fillMaxWidth()
                .clip(shape)
                .background(EventDetailModalGradient, shape)
                .border(Dimens.BorderThin, CardBorder, shape)
                .verticalScroll(rememberScrollState())
                .padding(Dimens.ModalPadding),
            verticalArrangement = Arrangement.spacedBy(Dimens.EventDetailSectionGap)
        ) {
            QuakeModalHeader(onDismiss = onDismiss, title = "Filter")

            if (FilterSection.INTENSITY in sections) FilterGroup(title = "Shaking Intensity") {
                QuakeIntensity.entries.forEach { option ->
                    FilterOptionRow(
                        label = option.label,
                        description = option.description,
                        selected = option == draft.intensity,
                        onClick = { draft = draft.copy(intensity = option) }
                    )
                }
            }

            if (FilterSection.DISTANCE in sections) FilterGroup(
                title = "Search Radius",
                // Said plainly and permanently, because the two radii are easy to
                // conflate and only one of them is a safety guarantee.
                note = "Applies to the \"Near\" pill only. Emergency alerts always use " +
                    "a fixed ${SafetyPolicy.ALERT_RADIUS_KM} km radius and cannot be changed."
            ) {
                OptionPillRow(
                    labels = QuakeSearchRadius.entries.map { it.label(unitSystem) },
                    selectedIndex = QuakeSearchRadius.entries.indexOf(draft.radius),
                    onSelect = { draft = draft.copy(radius = QuakeSearchRadius.entries[it]) }
                )
                if (draft.radius.km > QuakeApiClient.MAX_SENSOR_RANGE_KM) {
                    // Shown rather than applied silently: `/sensors` rejects anything
                    // above 500 km, so the Sensors tab would otherwise answer a
                    // different question than the one on screen.
                    Text(
                        text = "Sensors are listed within " +
                            "${unitSystem.formatDistance(QuakeApiClient.MAX_SENSOR_RANGE_KM)}. " +
                            "That is the furthest that tab can search.",
                        style = CardSubtitle.copy(color = MmiOrange)
                    )
                }
            }

            if (FilterSection.TIME in sections) FilterGroup(title = "Time Range") {
                OptionPillRow(
                    labels = QuakeTimeWindow.entries.map { it.label },
                    selectedIndex = QuakeTimeWindow.entries.indexOf(draft.timeWindow),
                    onSelect = { draft = draft.copy(timeWindow = QuakeTimeWindow.entries[it]) }
                )
            }

            if (FilterSection.STATION_STATUS in sections) FilterGroup(
                title = "Station Status",
                // An offline station is not noise to be hidden by default: it is a
                // gap in the very coverage this tab reports, and the roll says so.
                note = "Offline stations stay in the list unless you narrow it here."
            ) {
                OptionPillRow(
                    labels = QuakeStationStatus.entries.map { it.label },
                    selectedIndex = QuakeStationStatus.entries.indexOf(draft.stationStatus),
                    onSelect = {
                        draft = draft.copy(stationStatus = QuakeStationStatus.entries[it])
                    }
                )
            }

            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(Dimens.FilterRowGap)
            ) {
                FilterDialogAction(
                    label = "Reset",
                    onClick = onReset,
                    modifier = Modifier.weight(1f)
                )
                FilterDialogAction(
                    label = "Apply",
                    onClick = { onApply(draft) },
                    modifier = Modifier.weight(1f)
                )
            }
        }
    }
}

/**
 * One titled group of options, with an optional explanatory [note] beneath the
 * heading.
 */
@Composable
private fun FilterGroup(
    title: String,
    modifier: Modifier = Modifier,
    note: String? = null,
    content: @Composable () -> Unit
) {
    Column(
        modifier = modifier.fillMaxWidth(),
        verticalArrangement = Arrangement.spacedBy(Dimens.StateTextGap)
    ) {
        Text(text = title, style = CardTitle)
        if (note != null) {
            Text(text = note, style = CardSubtitle)
        }
        content()
    }
}

/**
 * Full-width single-choice row used for the intensity buckets, where each option
 * carries a sentence of explanation that would not fit in a pill.
 */
@Composable
private fun FilterOptionRow(
    label: String,
    description: String,
    selected: Boolean,
    onClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    val shape = RoundedCornerShape(Dimens.RadiusSmall)
    val isSelected = selected
    Column(
        modifier = modifier
            .fillMaxWidth()
            .clip(shape)
            .background(if (selected) FilterActiveFill else FilterInactiveFill, shape)
            .border(Dimens.BorderThin, CardBorder, shape)
            .clickable(role = Role.RadioButton, onClick = onClick)
            .semantics { this.selected = isSelected }
            .padding(
                horizontal = Dimens.FilterPillPaddingHorizontal,
                vertical = Dimens.FilterPillPaddingVertical
            ),
        verticalArrangement = Arrangement.spacedBy(Dimens.StateTextGap / 2)
    ) {
        Text(text = label, style = ChipLabel)
        Text(text = description, style = CardSubtitle)
    }
}

/**
 * Wrapping row of single-choice pills, reusing the [QuakeFilterRow] pill chrome so
 * the sheet's controls are recognisably the same family as the row that opened it.
 */
@Composable
private fun OptionPillRow(
    labels: List<String>,
    selectedIndex: Int,
    onSelect: (Int) -> Unit,
    modifier: Modifier = Modifier
) {
    Column(
        modifier = modifier.fillMaxWidth(),
        verticalArrangement = Arrangement.spacedBy(Dimens.StateTextGap)
    ) {
        labels.chunked(2).forEachIndexed { rowIndex, rowLabels ->
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(Dimens.StateTextGap)
            ) {
                rowLabels.forEachIndexed { columnIndex, label ->
                    val index = rowIndex * 2 + columnIndex
                    val isSelected = index == selectedIndex
                    val shape = RoundedCornerShape(Dimens.RadiusSmall)
                    Box(
                        modifier = Modifier
                            .weight(1f)
                            .height(Dimens.FilterPillHeight)
                            .clip(shape)
                            .background(
                                if (isSelected) FilterActiveFill else FilterInactiveFill,
                                shape
                            )
                            .border(Dimens.BorderThin, CardBorder, shape)
                            .clickable(role = Role.RadioButton) { onSelect(index) }
                            .semantics { selected = isSelected },
                        contentAlignment = Alignment.Center
                    ) {
                        Text(text = label, style = ChipLabel)
                    }
                }
                // Keeps the last odd pill at half width instead of stretching it.
                if (rowLabels.size == 1) {
                    Box(modifier = Modifier.weight(1f))
                }
            }
        }
    }
}

/**
 * "Reset" / "Apply" capsule, on the shared overlay-action chrome
 * ([Dimens.ModalActionHeight] + 2dp stroke) the Details overlay uses.
 */
@Composable
private fun FilterDialogAction(
    label: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    val shape = RoundedCornerShape(Dimens.RadiusSmall)
    Box(
        modifier = modifier
            .height(Dimens.ModalActionHeight)
            .clip(shape)
            .background(EmergencyCtaFill, shape)
            .border(Dimens.BorderMedium, EmergencyCtaBorder, shape)
            .clickable(role = Role.Button, onClickLabel = label, onClick = onClick),
        contentAlignment = Alignment.Center
    ) {
        Text(text = label, style = ChipLabel)
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun QuakeFilterDialogPreview() {
    QuakeAlertTheme {
        // Previewed through its own sections rather than the Dialog window, which a
        // preview cannot host.
        Column(modifier = Modifier.padding(Dimens.ModalPadding)) {
            Text(text = "Filter", style = SectionTitle)
            FilterGroup(title = "Shaking Intensity") {
                QuakeIntensity.entries.forEach {
                    FilterOptionRow(
                        label = it.label,
                        description = it.description,
                        selected = it == QuakeIntensity.MODERATE,
                        onClick = {}
                    )
                }
            }
            FilterGroup(title = "Search Radius") {
                OptionPillRow(
                    labels = QuakeSearchRadius.entries.map { it.label(UnitSystem.METRIC) },
                    selectedIndex = 1,
                    onSelect = {}
                )
            }
        }
    }
}
