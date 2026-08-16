package id.web.quakealert.ui.settings

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
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.style.TextAlign
import id.web.quakealert.R
import id.web.quakealert.ui.theme.CardBorder
import id.web.quakealert.ui.theme.CardSubtitle
import id.web.quakealert.ui.theme.CardSurface
import id.web.quakealert.ui.theme.CardTitle
import id.web.quakealert.ui.theme.ChipLabel
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.FilterActiveFill
import id.web.quakealert.ui.theme.InfoPillFill
import id.web.quakealert.ui.theme.PillLabel
import id.web.quakealert.ui.theme.SectionHeaderPillFill
import id.web.quakealert.ui.theme.SegmentInactiveFill
import id.web.quakealert.ui.theme.SettingCardBorder
import id.web.quakealert.ui.theme.SyncButtonFill
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
    val shape = RoundedCornerShape(Dimens.SectionHeaderPillRadius)
    Box(
        modifier = modifier
            .fillMaxWidth()
            .height(Dimens.SectionHeaderPillHeight)
            .clip(shape)
            .background(SectionHeaderPillFill, shape)
            .border(Dimens.BorderThin, CardBorder, shape)
            .padding(
                horizontal = Dimens.SectionHeaderPillPaddingHorizontal,
                vertical = Dimens.SectionHeaderPillPaddingVertical
            ),
        contentAlignment = Alignment.CenterStart
    ) {
        Text(text = title, style = CardTitle)
    }
}

/**
 * Generic setting row card (Figma node 1:857 / EL-002b7d17): a dark rounded card
 * with a leading text column (title + optional sub-line) and a trailing control
 * slot ([trailing]) laid out with [RowScope]. Reused for every toggle / segmented
 * / action row so the padding, border and radius stay identical.
 *
 * @param title primary card label (e.g. "Keep Alerting").
 * @param subtitle optional dimmed sub-line beneath the title; hidden when null.
 * @param onClick optional whole-card click (used by action rows); null = inert.
 * @param trailing trailing control content (switch, segmented pills, icon...).
 */
@Composable
fun SettingCard(
    title: String,
    modifier: Modifier = Modifier,
    subtitle: String? = null,
    onClick: (() -> Unit)? = null,
    trailing: @Composable RowScope.() -> Unit = {}
) {
    val shape = RoundedCornerShape(Dimens.SettingCardRadius)
    val base = modifier
        .fillMaxWidth()
        .clip(shape)
        .background(CardSurface, shape)
        .border(Dimens.BorderThin, SettingCardBorder, shape)
    val clickable = if (onClick != null) base.clickable(onClick = onClick) else base

    Row(
        modifier = clickable.padding(Dimens.SettingCardPadding),
        horizontalArrangement = Arrangement.spacedBy(Dimens.SettingCardContentGap),
        verticalAlignment = Alignment.CenterVertically
    ) {
        Column(
            modifier = Modifier.weight(1f),
            verticalArrangement = Arrangement.spacedBy(Dimens.SettingCardTitleGap)
        ) {
            Text(text = title, style = CardTitle)
            if (subtitle != null) {
                Text(text = subtitle, style = CardSubtitle)
            }
        }
        trailing()
    }
}

/**
 * Small info pill used on setting cards to surface metadata such as
 * "Last Sync : 2 min. ago" (Figma node 1:868). Flat translucent-black capsule
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
 * Circular "sync now" refresh icon button (Figma node 1:867) shown as the
 * trailing control on the location card.
 */
@Composable
fun SyncRefreshButton(
    onClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    Icon(
        painter = painterResource(id = R.drawable.ic_refresh),
        contentDescription = "Sync location now",
        tint = Color.Unspecified,
        modifier = modifier
            .size(Dimens.SyncRefreshIconSize)
            .clip(RoundedCornerShape(Dimens.SyncRefreshIconSize))
            .clickable(onClick = onClick)
    )
}

/**
 * Horizontal segmented toggle control (Figma node 1:872 Coverage / 1:912
 * Language): an inset dark container wrapping a row of equal-weight pills where
 * the selected [options] entry is highlighted with [FilterActiveFill]. Generic
 * over the option type so it can drive the [CoverageRange] and [AppLanguage]
 * controls (and any future segmented option) from one component.
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
    val containerShape = RoundedCornerShape(Dimens.SegmentContainerRadius)
    Row(
        modifier = modifier
            .clip(containerShape)
            .background(SegmentInactiveFill, containerShape)
            .border(Dimens.BorderThin, CardBorder, containerShape)
            .padding(Dimens.SegmentContainerPadding),
        horizontalArrangement = Arrangement.spacedBy(Dimens.SegmentRowGap),
        verticalAlignment = Alignment.CenterVertically
    ) {
        options.forEach { option ->
            SegmentPill(
                label = labelOf(option),
                selected = option == selected,
                onClick = { onSelect(option) }
            )
        }
    }
}

/** A single pill within [QuakeSegmentedControl]. */
@Composable
private fun SegmentPill(
    label: String,
    selected: Boolean,
    onClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    val shape = RoundedCornerShape(Dimens.SegmentPillRadius)
    val fill = if (selected) FilterActiveFill else Color.Transparent
    Box(
        modifier = modifier
            .clip(shape)
            .background(fill, shape)
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
            color = if (selected) TextPrimary else TextSecondary,
            textAlign = TextAlign.Center
        )
    }
}

