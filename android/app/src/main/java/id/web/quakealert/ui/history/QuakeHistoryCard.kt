package id.web.quakealert.ui.history

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight

import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment

import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow

import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import id.web.quakealert.R
import id.web.quakealert.ui.common.QuakePill
import id.web.quakealert.ui.theme.CardBorder
import id.web.quakealert.ui.theme.CardSubtitle
import id.web.quakealert.ui.theme.CardSurface
import id.web.quakealert.ui.theme.CardTitle
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.MapPlaceholder
import id.web.quakealert.ui.theme.MmiOrange
import id.web.quakealert.ui.theme.MmiOrangeContainer
import id.web.quakealert.ui.theme.MmiRed
import id.web.quakealert.ui.theme.MmiRedContainer
import id.web.quakealert.ui.theme.NunitoFontFamily
import id.web.quakealert.ui.theme.ShareButtonFill
import id.web.quakealert.ui.theme.TextPrimary


/**
 * A single earthquake history entry (Figma node 1:715). Layout, left → right:
 *  1. Leading column: circular MMI intensity badge above a map thumbnail.
 *  2. Details column: location title, date/time metadata, distance badge + share.
 *  3. Trailing vertical accent bar acting as a "see more" affordance.
 */
@Composable
fun QuakeHistoryCard(
    item: QuakeHistoryItem,
    onShareClicked: () -> Unit,
    onSeeMoreClicked: () -> Unit,
    modifier: Modifier = Modifier
) {
    val accent = if (item.severity == MmiSeverity.SEVERE) MmiRed else MmiOrange
    val badgeContainer =
        if (item.severity == MmiSeverity.SEVERE) MmiRedContainer else MmiOrangeContainer
    // Hoist the repeated card shape so clip/background/border share one instance
    // instead of allocating three identical shapes per (re)composition.
    val cardShape = remember { RoundedCornerShape(Dimens.RadiusCard) }

    Row(
        modifier = modifier
            .fillMaxWidth()
            .height(Dimens.CardHeight)
            .clip(cardShape)
            .background(CardSurface, cardShape)
            .border(Dimens.BorderThin, CardBorder, cardShape)
            .clickable(onClick = onSeeMoreClicked)

            .padding(
                start = Dimens.CardPaddingStart,
                top = Dimens.CardPaddingTop,
                end = Dimens.CardPaddingEnd,
                bottom = Dimens.CardPaddingBottom
            ),
        horizontalArrangement = Arrangement.spacedBy(Dimens.CardContentGap),
        verticalAlignment = Alignment.CenterVertically
    ) {
        LeadingColumn(intensity = item.intensity, accent = accent, container = badgeContainer)

        DetailsColumn(
            item = item,
            onShareClicked = onShareClicked,
            modifier = Modifier.weight(1f)
        )

        SeeMoreBar(accent = accent)
    }
}

/** Circular MMI badge stacked above a map thumbnail (Figma node 1:716). */
@Composable
private fun LeadingColumn(
    intensity: String,
    accent: Color,
    container: Color,
    modifier: Modifier = Modifier
) {
    Column(
        modifier = modifier.width(Dimens.CardLeadingColumnWidth),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(Dimens.CardLeadingColumnGap)
    ) {
        Box(
            modifier = Modifier
                .size(Dimens.MmiBadgeSize)
                .clip(CircleShape)
                .background(container, CircleShape)
                .border(Dimens.MmiBadgeBorder, accent, CircleShape),
            contentAlignment = Alignment.Center
        ) {
            Text(
                text = intensity,
                color = accent,
                fontFamily = NunitoFontFamily,
                fontWeight = FontWeight.ExtraBold,
                fontSize = 15.sp
            )
        }

        Box(
            modifier = Modifier
                .fillMaxWidth()
                .height(Dimens.MapThumbHeight)
                .clip(RoundedCornerShape(Dimens.RadiusSmall))
                .background(MapPlaceholder, RoundedCornerShape(Dimens.RadiusSmall)),
            contentAlignment = Alignment.Center
        ) {
            Icon(
                painter = painterResource(id = R.drawable.ic_map),
                contentDescription = null,
                tint = Color.Black,
                modifier = Modifier.size(20.dp)
            )
        }
    }
}

/** Location + timestamp metadata and the distance/share footer row. */
@Composable
private fun DetailsColumn(
    item: QuakeHistoryItem,
    onShareClicked: () -> Unit,
    modifier: Modifier = Modifier
) {
    Column(
        modifier = modifier.fillMaxHeight(),
        verticalArrangement = Arrangement.SpaceBetween
    ) {
        Column(verticalArrangement = Arrangement.spacedBy(Dimens.DetailTitleDistanceGap)) {
            // Shared CardTitle — identical base size/weight to the Sensor card's
            // "Station NODE-xxxx" header (single source of truth).
            Text(
                text = item.location,
                style = CardTitle,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis
            )
            // Shared CardSubtitle (dimmed) — matches the Sensor card's location line.
            Text(text = item.date, style = CardSubtitle)
            Text(text = item.time, style = CardSubtitle)
        }

        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.spacedBy(Dimens.DetailFooterGap),
            verticalAlignment = Alignment.CenterVertically
        ) {
            // Shared QuakePill capsule — same fill/stroke/shape as the Sensor
            // telemetry pills.
            QuakePill(
                text = item.distanceLabel,
                modifier = Modifier.weight(1f)
            )
            ShareButton(onClick = onShareClicked)
        }

    }
}


/** Small share icon button (Figma node 1:728). */

@Composable
private fun ShareButton(onClick: () -> Unit, modifier: Modifier = Modifier) {
    Box(
        modifier = modifier
            .size(width = Dimens.ShareButtonWidth, height = Dimens.ShareButtonHeight)
            .clip(RoundedCornerShape(Dimens.RadiusSmall))
            .background(ShareButtonFill, RoundedCornerShape(Dimens.RadiusSmall))
            .clickable(onClick = onClick),
        contentAlignment = Alignment.Center
    ) {
        Icon(
            painter = painterResource(id = R.drawable.ic_share),
            contentDescription = "Share",
            tint = TextPrimary,
            modifier = Modifier.size(13.dp)
        )
    }
}

/** Right-hand vertical accent bar hinting at a "see more" tap target. */
@Composable
private fun SeeMoreBar(accent: Color, modifier: Modifier = Modifier) {
    Box(
        modifier = modifier
            .fillMaxHeight()
            .width(Dimens.SeeMoreBarWidth)
            .clip(RoundedCornerShape(Dimens.RadiusSmall))
            .background(accent, RoundedCornerShape(Dimens.RadiusSmall)),
        contentAlignment = Alignment.Center
    ) {
        Icon(
            painter = painterResource(id = R.drawable.ic_arrow_right),
            contentDescription = "See more",
            tint = Color.White,
            modifier = Modifier.size(13.dp)
        )
    }
}
