package id.web.quakealert.ui.settings

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
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
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.style.TextAlign
import id.web.quakealert.R
import id.web.quakealert.ui.theme.AboutButtonFill
import id.web.quakealert.ui.theme.AboutCardBorder
import id.web.quakealert.ui.theme.AboutCardGradient
import id.web.quakealert.ui.theme.BorderLight
import id.web.quakealert.ui.theme.CardBorder
import id.web.quakealert.ui.theme.CardSubtitle
import id.web.quakealert.ui.theme.CardSurface

import id.web.quakealert.ui.theme.CardTitle
import id.web.quakealert.ui.theme.ChipLabel
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.InfoPillFill
import id.web.quakealert.ui.theme.PillLabel
import id.web.quakealert.ui.theme.SectionHeaderPillFill
import id.web.quakealert.ui.theme.SegmentActiveFill
import id.web.quakealert.ui.theme.SegmentInactiveFill
import id.web.quakealert.ui.theme.SettingCardBorder
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
 * Generic setting row card (Figma node 1:868 / EL-002b7d17): a dark rounded card
 * with a leading text column (title + optional [detail] slot beneath it) and a
 * trailing control slot ([trailing]) laid out with [RowScope]. Reused for every
 * toggle / segmented / action row so the padding, border and radius stay
 * identical.
 *
 * The Figma cards do not carry a dimmed sub-line under the title; instead some
 * cards (e.g. "Sync Location Now") host an inline detail element such as the
 * "Last Sync : ..." info pill, exposed here via [detail].
 *
 * @param title primary card label (e.g. "Keep Alerting").
 * @param onClick optional whole-card click (used by action rows); null = inert.
 * @param detail optional content laid out beneath the title in the same column
 *   (e.g. an [InfoPill]); empty by default.
 * @param trailing trailing control content (switch, segmented pills, icon...).
 */
@Composable
fun SettingCard(
    title: String,
    modifier: Modifier = Modifier,
    onClick: (() -> Unit)? = null,
    detail: @Composable ColumnScope.() -> Unit = {},
    trailing: @Composable RowScope.() -> Unit = {}
) {
    val shape = RoundedCornerShape(Dimens.SettingCardRadius)
    val base = modifier
        .fillMaxWidth()
        .clip(shape)
        .background(CardSurface, shape)
        .border(Dimens.BorderMedium, SettingCardBorder, shape)
    val clickable = if (onClick != null) base.clickable(onClick = onClick) else base

    Row(
        modifier = clickable.padding(
            horizontal = Dimens.SettingCardPaddingHorizontal,
            vertical = Dimens.SettingCardPaddingVertical
        ),
        horizontalArrangement = Arrangement.spacedBy(Dimens.SettingCardContentGap),

        verticalAlignment = Alignment.CenterVertically
    ) {
        Column(
            modifier = Modifier.weight(1f),
            verticalArrangement = Arrangement.spacedBy(Dimens.SettingCardTitleGap)
        ) {
            Text(text = title, style = CardTitle)
            detail()
        }
        trailing()
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
 */
@Composable
fun SyncRefreshButton(
    onClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    Icon(
        painter = painterResource(id = R.drawable.ic_refresh_cw),
        contentDescription = "Sync location now",
        tint = TextPrimary,
        modifier = modifier
            .size(Dimens.SyncRefreshIconSize)
            .clip(RoundedCornerShape(Dimens.SyncRefreshIconSize))
            .clickable(onClick = onClick)
    )
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
