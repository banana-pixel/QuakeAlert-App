package id.web.quakealert.ui.sensors

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.text.withStyle
import id.web.quakealert.R

import id.web.quakealert.ui.common.QuakePill
import id.web.quakealert.ui.theme.CardBorder
import id.web.quakealert.ui.theme.CardSubtitle
import id.web.quakealert.ui.theme.CardSurface
import id.web.quakealert.ui.theme.CardTitle
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.MicroCaption
import id.web.quakealert.ui.theme.SensorChipBorder
import id.web.quakealert.ui.theme.SensorChipFill
import id.web.quakealert.ui.theme.SensorNodeIdText
import id.web.quakealert.ui.theme.StatusOfflineDot
import id.web.quakealert.ui.theme.StatusOfflineFill
import id.web.quakealert.ui.theme.StatusOnlineDot
import id.web.quakealert.ui.theme.StatusOnlineFill
import id.web.quakealert.ui.theme.TextPrimary


/**
 * A single sensor station entry (Figma node 1:1111). Layout, left → right:
 *  1. Leading chip column: a rounded cpu-chip badge above the "MPU 6050" label.
 *  2. Details column: station header ("Station" + accented NODE id), location,
 *     and two telemetry rows:
 *       Row 1: [• Online] [Last Ping : …]
 *       Row 2: [RSSI : …] [Latency : …]
 *
 * The station header and location reuse the shared [CardTitle] / [CardSubtitle]
 * typography, and every status/telemetry capsule is a shared [QuakePill], so
 * this card stays byte-consistent with the History card.
 *
 * All state and events are hoisted; tapping the card invokes [onClick].
 */
@Composable
fun SensorItemCard(
    item: SensorStationItem,
    onClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    val cardShape = remember { RoundedCornerShape(Dimens.RadiusCard) }

    Row(
        modifier = modifier
            .fillMaxWidth()
            .clip(cardShape)
            .background(CardSurface, cardShape)
            .border(Dimens.BorderThin, CardBorder, cardShape)
            .clickable(onClick = onClick)
            .padding(
                start = Dimens.CardPaddingStart,
                top = Dimens.CardPaddingTop,
                end = Dimens.CardPaddingEnd,
                bottom = Dimens.CardPaddingBottom
            ),
        horizontalArrangement = Arrangement.spacedBy(Dimens.SensorCardContentGap),
        verticalAlignment = Alignment.CenterVertically
    ) {
        ChipColumn(label = item.chipLabel)
        DetailsColumn(item = item, modifier = Modifier.weight(1f))
    }
}

/** cpu-chip badge stacked above the sensor-module label (Figma node 1:1112). */
@Composable
private fun ChipColumn(label: String, modifier: Modifier = Modifier) {
    val chipShape = RoundedCornerShape(Dimens.SensorChipRadius)
    Column(
        modifier = modifier.width(Dimens.SensorChipColumnWidth),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(Dimens.SensorChipLabelGap)
    ) {
        Box(
            modifier = Modifier
                .size(Dimens.SensorChipBadgeSize)
                .clip(chipShape)
                .background(SensorChipFill, chipShape)
                .border(Dimens.BorderThin, SensorChipBorder, chipShape)
                .padding(Dimens.SensorChipIconPadding),
            contentAlignment = Alignment.Center
        ) {
            Icon(
                painter = painterResource(id = R.drawable.ic_cpu_chip),
                contentDescription = null,
                tint = TextPrimary,
                modifier = Modifier.size(Dimens.SensorChipIconSize)
            )
        }
        // "MPU 6050" — the card's smallest label. Figma ships 8sp; raised to the
        // app's 10sp legibility floor for micro-captions, which still fits the
        // SensorChipColumnWidth on one line.
        Text(
            text = label,
            style = MicroCaption,
            maxLines = 1,
            softWrap = false
        )
    }
}

/** Station header, location and telemetry rows (Figma node 1:1118). */
@Composable
private fun DetailsColumn(item: SensorStationItem, modifier: Modifier = Modifier) {
    Column(
        modifier = modifier,
        verticalArrangement = Arrangement.spacedBy(Dimens.SensorDetailsGap)
    ) {
        // Station header — "Station " (white) + accented cyan NODE id. Uses the
        // shared CardTitle so its base size/weight matches the History card's
        // location title exactly; only the NODE id span recolours.
        val stationText = buildAnnotatedString {
            append("Station ")
            withStyle(SpanStyle(color = SensorNodeIdText)) {
                append(item.stationId)
            }
        }
        Text(
            text = stationText,
            style = CardTitle,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis
        )

        // Location subtitle — shared dimmed CardSubtitle, matching the History
        // card's date/time metadata styling.
        Text(
            text = item.location,
            style = CardSubtitle,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis
        )

        // Row 1: status pill + Last Ping, horizontally aligned.
        Row(horizontalArrangement = Arrangement.spacedBy(Dimens.SensorChipRowGap)) {
            val online = item.status == SensorStatus.ONLINE
            QuakePill(
                text = if (online) "Online" else "Offline",
                fill = if (online) StatusOnlineFill else StatusOfflineFill,
                dotColor = if (online) StatusOnlineDot else StatusOfflineDot
            )
            QuakePill(text = item.telemetry.lastPing)
        }

        // Row 2: RSSI + Latency, horizontally aligned.
        Row(horizontalArrangement = Arrangement.spacedBy(Dimens.SensorChipRowGap)) {
            QuakePill(text = item.telemetry.rssi)
            QuakePill(text = item.telemetry.latency)
        }
    }
}
