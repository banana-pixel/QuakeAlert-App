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
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.unit.sp
import id.web.quakealert.R
import id.web.quakealert.ui.theme.CardBorder
import id.web.quakealert.ui.theme.CardSurface
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.NunitoFontFamily
import id.web.quakealert.ui.theme.SensorChipBorder
import id.web.quakealert.ui.theme.SensorChipFill
import id.web.quakealert.ui.theme.SensorNodeIdText
import id.web.quakealert.ui.theme.StatusOfflineFill
import id.web.quakealert.ui.theme.StatusOnlineFill
import id.web.quakealert.ui.theme.TelemetryPillFill
import id.web.quakealert.ui.theme.TextPrimary
import id.web.quakealert.ui.theme.TextSecondary

/**
 * A single sensor station entry (Figma node 1:1111). Layout, left → right:
 *  1. Leading chip column: a rounded cpu-chip badge above the module label.
 *  2. Details column: station id (with highlighted NODE suffix), location, a
 *     status chip (Online/Offline), and a row of telemetry pills.
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
        verticalArrangement = Arrangement.spacedBy(Dimens.SensorChipIconPadding)
    ) {
        Box(
            modifier = Modifier
                .size(Dimens.SensorChipColumnWidth)
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
        Text(
            text = label,
            color = TextPrimary,
            fontFamily = NunitoFontFamily,
            fontWeight = FontWeight.Bold,
            fontSize = 11.sp,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis
        )
    }
}

/** Station id, location, status chip and telemetry pills (Figma node 1:1120). */
@Composable
private fun DetailsColumn(item: SensorStationItem, modifier: Modifier = Modifier) {
    Column(
        modifier = modifier,
        verticalArrangement = Arrangement.spacedBy(Dimens.SensorDetailsGap)
    ) {
        // Station id — the "NODE-XXXX" suffix is accented per Figma.
        val stationText = buildAnnotatedString {
            append("ID : ")
            withStyle(SpanStyle(color = SensorNodeIdText, fontWeight = FontWeight.ExtraBold)) {
                append(item.stationId)
            }
        }
        Text(
            text = stationText,
            color = TextPrimary,
            fontFamily = NunitoFontFamily,
            fontWeight = FontWeight.ExtraBold,
            fontSize = 16.sp,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis
        )

        Text(
            text = item.location,
            color = TextSecondary,
            fontFamily = NunitoFontFamily,
            fontWeight = FontWeight.SemiBold,
            fontSize = 13.sp,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis
        )

        StatusChip(status = item.status)

        // Telemetry pills wrap into two rows to fit the fixed details width.
        Row(horizontalArrangement = Arrangement.spacedBy(Dimens.SensorChipRowGap)) {
            TelemetryPill(text = item.telemetry.lastPing)
            TelemetryPill(text = item.telemetry.latency)
        }
        TelemetryPill(text = item.telemetry.rssi)
    }
}

/** Green "Online" / red "Offline" status chip (Figma node 1:1123). */
@Composable
private fun StatusChip(status: SensorStatus, modifier: Modifier = Modifier) {
    val shape = RoundedCornerShape(Dimens.RadiusSmall)
    val fill = if (status == SensorStatus.ONLINE) StatusOnlineFill else StatusOfflineFill
    val label = if (status == SensorStatus.ONLINE) "Online" else "Offline"
    Box(
        modifier = modifier
            .clip(shape)
            .background(fill, shape)
            .border(Dimens.BorderThin, CardBorder, shape)
            .padding(
                horizontal = Dimens.SensorStatusChipPaddingHorizontal,
                vertical = Dimens.SensorStatusChipPaddingVertical
            ),
        contentAlignment = Alignment.Center
    ) {
        Text(
            text = label,
            color = TextPrimary,
            fontFamily = NunitoFontFamily,
            fontWeight = FontWeight.Bold,
            fontSize = 12.sp
        )
    }
}

/** A single telemetry read-out pill (Figma node 1:1126). */
@Composable
private fun TelemetryPill(text: String, modifier: Modifier = Modifier) {
    val shape = RoundedCornerShape(Dimens.RadiusSmall)
    Box(
        modifier = modifier
            .clip(shape)
            .background(TelemetryPillFill, shape)
            .padding(
                horizontal = Dimens.SensorTelemetryPillPaddingHorizontal,
                vertical = Dimens.SensorTelemetryPillPaddingVertical
            ),
        contentAlignment = Alignment.Center
    ) {
        Text(
            text = text,
            color = TextPrimary,
            fontFamily = NunitoFontFamily,
            fontWeight = FontWeight.Bold,
            fontSize = 11.sp,
            maxLines = 1
        )
    }
}
