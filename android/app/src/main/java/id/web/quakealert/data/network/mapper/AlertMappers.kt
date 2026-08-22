package id.web.quakealert.data.network.mapper

import id.web.quakealert.data.network.model.WsAlertMessageDto
import id.web.quakealert.domain.AlertType
import id.web.quakealert.domain.UserLocation
import id.web.quakealert.domain.WsAlertMessage
import id.web.quakealert.domain.distanceKmTo
import id.web.quakealert.ui.history.MmiSeverity
import id.web.quakealert.ui.history.QuakeHistoryItem
import java.time.Instant
import java.time.ZoneId
import kotlin.math.roundToInt

/**
 * Wire → domain for a realtime frame.
 *
 * Returns null for an unrecognised `type`. Dropping the frame is the correct
 * failure here: the three known types drive three different UI behaviours, and a
 * fourth one guessed into the wrong bucket would either raise a false alarm or
 * clear a real one.
 */
fun WsAlertMessageDto.toDomainOrNull(): WsAlertMessage? {
    val alertType = type.toAlertTypeOrNull() ?: return null
    return WsAlertMessage(
        type = alertType,
        eventId = eventId,
        mmi = mmi,
        intensityLabel = intensityLabel,
        pgaGal = pgaGal,
        centroidLat = centroidLat,
        centroidLon = centroidLon,
        locationName = locationName,
        timestampMs = timestamp,
        nodeCount = nodeCount
    )
}

private fun String.toAlertTypeOrNull(): AlertType? =
    AlertType.entries.firstOrNull { it.name.equals(this, ignoreCase = true) }

/**
 * Domain → the shared event-detail UI model, so the Warning screen's "Recent
 * Earthquake" overlay renders a live alert through exactly the same component (and
 * share text) as a row from the History feed.
 *
 * `durationLabel` and an unknown `distanceKm` behave as described on
 * [id.web.quakealert.data.network.mapper.toHistoryItem]: the alert payload carries
 * no shaking duration, and distance needs a known device position.
 */
fun WsAlertMessage.toHistoryItem(
    userLocation: UserLocation?,
    zone: ZoneId = ZoneId.systemDefault(),
    now: Instant = Instant.now()
): QuakeHistoryItem {
    val occurredAt = Instant.ofEpochMilli(timestampMs)
    return QuakeHistoryItem(
        // ADVISORY frames carry an empty event_id (they are never persisted), so
        // fall back to a stable synthetic key — a blank id would collide with any
        // other advisory in a keyed list.
        id = eventId.takeIf { it.isNotBlank() } ?: "advisory-$timestampMs",
        intensity = mmi,
        severity = severity(),
        location = locationName,
        date = QuakeFormat.date(occurredAt, zone),
        time = QuakeFormat.time(occurredAt, zone),
        distanceKm = userLocation.distanceKmTo(centroidLat, centroidLon)?.roundToInt(),
        relativeTime = QuakeFormat.relativeTime(occurredAt, now),
        pgaLabel = QuakeFormat.pga(pgaGal),
        durationLabel = QuakeFormat.UNAVAILABLE,
        coordinates = QuakeFormat.coordinates(centroidLat, centroidLon),
        latitude = centroidLat,
        longitude = centroidLon
    )
}

/**
 * Severity bucket for the alert banner, using the same rule as the REST feed so a
 * quake does not change colour when the screen switches from the live frame to the
 * stored event.
 */
fun WsAlertMessage.severity(): MmiSeverity =
    if (intensityLabel.equals("strong", ignoreCase = true) || pgaGal >= 137.2) {
        MmiSeverity.SEVERE
    } else {
        MmiSeverity.MODERATE
    }

/**
 * Banner intensity line, e.g. "Intensity : IV (moderate)".
 *
 * Delegates to [QuakeFormat.intensityBanner] so a live frame and a stored event
 * produce the same string; the local severity word is the fallback when the server
 * sent no label.
 */
fun WsAlertMessage.intensityBannerLabel(): String =
    QuakeFormat.intensityBanner(mmi = mmi, label = intensityLabel, fallbackWord = severity().name)

/**
 * Bare intensity read for the active alert card, e.g. "IV (moderate)" (Figma node
 * 1:1067). Same inputs as [intensityBannerLabel], without the "Intensity :" prefix
 * the card renders as its own label line.
 */
fun WsAlertMessage.intensityValueLabel(): String =
    QuakeFormat.intensityValue(mmi = mmi, label = intensityLabel, fallbackWord = severity().name)

