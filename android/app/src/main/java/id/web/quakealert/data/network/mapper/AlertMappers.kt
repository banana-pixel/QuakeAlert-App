package id.web.quakealert.data.network.mapper

import id.web.quakealert.BuildConfig
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
 *
 * Also returns null for a drill frame (`is_test`) unless [allowTestAlerts]. This is
 * the client half of the two fences that keep a drill away from the public: the
 * server publishes drills to the `test_alerts` FCM topic alone, which no release
 * build subscribes to (id.web.quakealert.data.push.PushRegistrar), and a drill that
 * reaches a release build anyway — replayed over the WebSocket, or delivered by a
 * mis-targeted push — dies here. Either fence alone would do it; the point of two is
 * that one configuration mistake cannot clear both. It sits in this function because
 * this is the one place both transports pass through: the FCM path builds a
 * [WsAlertMessageDto] and calls straight into it
 * (id.web.quakealert.data.network.mapper.toWsAlertMessageOrNull).
 *
 * @param allowTestAlerts whether a drill may become an alert on this build. A
 *   parameter rather than a bare `BuildConfig.DEBUG` read so both branches can be
 *   asserted in a unit test — the release branch is the one that matters and it is
 *   the one a debug-only test run could never otherwise exercise.
 */
fun WsAlertMessageDto.toDomainOrNull(
    allowTestAlerts: Boolean = BuildConfig.DEBUG
): WsAlertMessage? {
    val alertType = type.toAlertTypeOrNull() ?: return null
    if (isTest && !allowTestAlerts) return null
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
        nodeCount = nodeCount,
        isTest = isTest
    )
}

private fun String.toAlertTypeOrNull(): AlertType? =
    AlertType.entries.firstOrNull { it.name.equals(this, ignoreCase = true) }

/**
 * Domain → the shared event-detail UI model, so the Warning screen's "Recent
 * Earthquake" overlay renders a live alert through exactly the same component (and
 * share text) as a row from the History feed.
 *
 * `reportingNodesLabel` and an unknown `distanceKm` behave as described on
 * [id.web.quakealert.data.network.mapper.toHistoryItem]: the count comes from the
 * frame's own `node_count`, so a live alert and the stored event it becomes read the
 * same, and distance needs a known device position.
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
        reportingNodesLabel = QuakeFormat.reportingNodes(nodeCount),
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

