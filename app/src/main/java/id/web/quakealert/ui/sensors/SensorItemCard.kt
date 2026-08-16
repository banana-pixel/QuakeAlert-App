package id.web.quakealert.ui.sensors

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
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.unit.sp
import id.web.quakealert.R
import id.web.quakealert.ui.theme.CardBorder
import id.web.quakealert.ui.theme.CardSurface
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.NunitoFontFamily
import id.web.quakealert.ui.theme.SensorChipBorder
import id.web.quakealert.ui.theme.SensorChipFill
import id.web.quakealert.ui.theme.SensorNodeIdText
import id.web.quakealert.ui.theme.StatusOfflineDot
import id.web.quakealert.ui.theme.StatusOfflineFill
import id.web.quakealert.ui.theme.StatusOnlineDot
import id.web.quakealert.ui.theme.StatusOnlineFill
import id.web.quakealert.ui.theme.TelemetryPillFill
import id.web.quakealert.ui.theme.TextPrimary


/**
 * A single sensor station entry (Figma node 1:1111). Layout, left → right:
 *  1. Leading chip column: a rounded cpu-chip badge above the "MPU 6050" label.
 *  2. Details column: station header ("Station" + accented NODE id), location,
 *     and two telemetry rows:
 *       Row 1: [• Online] [Last Ping : …]
 *       Row 2: [RSSI : …] [Latency : …]
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
        // "MPU 6050" — 8sp per Figma so the label never truncates in the column.
        Text(
            text = label,
            color = TextPrimary,
            fontFamily = NunitoFontFamily,
            fontWeight = FontWeight.Bold,
            fontSize = 8.sp,
            lineHeight = 10.sp,
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
        // Station header — "Station " (white) + accented cyan NODE id.
        val stationText = buildAnnotatedString {
            append("Station ")
            withStyle(SpanStyle(color = SensorNodeIdText)) {
                append(item.stationId)
            }
        }
        Text(
            text = stationText,
            color = TextPrimary,
            fontFamily = NunitoFontFamily,
            fontWeight = FontWeight.Bold,
            fontSize = 15.sp,
            lineHeight = 16.sp,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis
        )

        Text(
            text = item.location,
            color = TextPrimary,
            fontFamily = NunitoFontFamily,
            fontWeight = FontWeight.Light,
            fontSize = 11.sp,
            lineHeight = 12.sp,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis
        )

        // Row 1: status pill + Last Ping, horizontally aligned.
        Row(horizontalArrangement = Arrangement.spacedBy(Dimens.SensorChipRowGap)) {
            StatusPill(status = item.status)
            TelemetryPill(text = item.telemetry.lastPing)
        }

        // Row 2: RSSI + Latency, horizontally aligned.
        Row(horizontalArrangement = Arrangement.spacedBy(Dimens.SensorChipRowGap)) {
            TelemetryPill(text = item.telemetry.rssi)
            TelemetryPill(text = item.telemetry.latency)
        }
    }
}

/**
 * Green "Online" / red "Offline" status pill with a leading solid dot indicator
 * (Figma node 1:1123). Shares the capsule metrics of [TelemetryPill] so the two
 * sit flush on the same row.
 */
@Composable
private fun StatusPill(status: SensorStatus, modifier: Modifier = Modifier) {
    val shape = RoundedCornerShape(Dimens.RadiusSmall)
    val fill = if (status == SensorStatus.ONLINE) StatusOnlineFill else StatusOfflineFill
    val dot = if (status == SensorStatus.ONLINE) StatusOnlineDot else StatusOfflineDot
    val label = if (status == SensorStatus.ONLINE) "Online" else "Offline"
    Row(
        modifier = modifier
            .height(Dimens.SensorPillHeight)
            .clip(shape)
            .background(fill, shape)
            .border(Dimens.BorderThin, CardBorder, shape)
            .padding(horizontal = Dimens.SensorPillPaddingHorizontal),
        horizontalArrangement = Arrangement.spacedBy(Dimens.SensorStatusDotGap),
        verticalAlignment = Alignment.CenterVertically
    ) {
        Box(
            modifier = Modifier
                .size(Dimens.SensorStatusDotSize)
                .clip(CircleShape)
                .background(dot, CircleShape)
        )
        Text(
            text = label,
            color = TextPrimary,
            fontFamily = NunitoFontFamily,
            fontWeight = FontWeight.Medium,
            fontSize = 11.sp,
            lineHeight = 12.sp,
            maxLines = 1,
            softWrap = false
        )
    }
}

/**
 * A single telemetry read-out pill (Figma node 1:1126). Uses the same dimmed
 * capsule styling as the History card's "km Away" badge for visual consistency.
 */
@Composable
private fun TelemetryPill(text: String, modifier: Modifier = Modifier) {
    val shape = RoundedCornerShape(Dimens.RadiusSmall)
    Box(
        modifier = modifier
            .height(Dimens.SensorPillHeight)
            .clip(shape)
            .background(TelemetryPillFill, shape)
            .border(Dimens.BorderThin, CardBorder, shape)
            .padding(horizontal = Dimens.SensorPillPaddingHorizontal),
        contentAlignment = Alignment.Center
    ) {
        Text(
            text = text,
            color = TextPrimary,
            fontFamily = NunitoFontFamily,
            fontWeight = FontWeight.Medium,
            fontSize = 11.sp,
            lineHeight = 12.sp,
            maxLines = 1,
            softWrap = false
        )
    }
}
