package id.web.quakealert.ui.common

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import id.web.quakealert.ui.theme.CardBorder
import id.web.quakealert.ui.theme.CardSurface
import id.web.quakealert.ui.theme.CardTitle
import id.web.quakealert.ui.theme.Dimens

/**
 * Shared flat, dark rounded row card (Figma setting-row node 1:868) reused
 * across features (Settings setting rows, Onboarding permission / test-alert
 * cards). A [CardSurface] fill with a 1dp [CardBorder] stroke, a leading text
 * column (title + optional [detail] slot beneath it) and a trailing control
 * slot ([trailing]) laid out with [RowScope]. Centralizing it here keeps the
 * padding, border, radius and minimum height byte-identical everywhere instead
 * of each screen re-declaring its own card chrome.
 *
 * @param title primary card label (e.g. "Keep Alerting").
 * @param onClick optional whole-card click (used by action / permission rows);
 *   null = inert.
 * @param detail optional content laid out beneath the title in the same column
 *   (e.g. an info pill or a status badge); empty by default.
 * @param trailing trailing control content (switch, segmented pills, icon...).
 */
@Composable
fun QuakeCard(
    title: String,
    modifier: Modifier = Modifier,
    onClick: (() -> Unit)? = null,
    detail: @Composable ColumnScope.() -> Unit = {},
    trailing: @Composable RowScope.() -> Unit = {}
) {
    val shape = RoundedCornerShape(Dimens.SettingCardRadius)
    val base = modifier
        .fillMaxWidth()
        .heightIn(min = Dimens.SettingCardHeight)
        .clip(shape)
        .background(CardSurface, shape)
        .border(Dimens.BorderThin, CardBorder, shape)
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
