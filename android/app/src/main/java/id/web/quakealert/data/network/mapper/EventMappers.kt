package id.web.quakealert.data.network.mapper

import id.web.quakealert.data.network.model.EventDto
import id.web.quakealert.domain.EarthquakeEvent
import id.web.quakealert.domain.EventStatus
import id.web.quakealert.domain.UserLocation
import id.web.quakealert.domain.distanceKmTo
import id.web.quakealert.ui.history.MmiSeverity
import id.web.quakealert.ui.history.QuakeHistoryItem
import java.time.Instant
import java.time.ZoneId
import kotlin.math.roundToInt

/**
 * Wire → domain. Unparseable timestamps and unknown status strings are coerced
 * rather than thrown on: one malformed row must not blank the whole history feed.
 */
fun EventDto.toDomain(): EarthquakeEvent = EarthquakeEvent(
    eventId = eventId,
    status = status.toEventStatus(),
    pgaGal = pga,
    mmi = mmi,
    intensityLabel = intensityLabel,
    latitude = latitude,
    longitude = longitude,
    // Always null by contract; passed through rather than defaulted to 0 so the
    // "unknown depth" fact survives into the domain layer.
    depthKm = depthKm,
    locationName = locationName,
    triggeredNodesCount = triggeredNodesCount,
    createdAt = createdAt.toInstantOrEpoch(),
    resolvedAt = resolvedAt?.toInstantOrNull()
)

fun List<EventDto>.toDomain(): List<EarthquakeEvent> = map { it.toDomain() }

/**
 * Domain → the History screen's UI model.
 *
 * Every field lands here pre-formatted because [QuakeHistoryItem] is a display DTO
 * shared by the list card, the detail overlay and the share sheet — formatting once
 * is what keeps the three from rendering the same quake three ways.
 *
 * Two fields have no server-side source and are worth knowing about before reading
 * a card:
 *  - `durationLabel` — the REST contract carries no shaking duration, so it renders
 *    as [QuakeFormat.UNAVAILABLE]. `resolved_at - created_at` is *not* a substitute:
 *    resolution is driven by the server's 90 s quiet-period cooldown, so deriving
 *    duration from it would show ~90 s for every quake.
 *  - `distanceKm` — needs the user's own position. When it is unknown (nothing has
 *    called `PUT /users/location` yet) this falls back to 0, which the card renders
 *    as "0 km Away". That is a display gap to close on the UI side, either by
 *    hiding the pill or showing a dash for an unknown position.
 *
 * @param userLocation last known device position, or null when it has never been set.
 * @param zone device time zone; UTC instants are converted only here.
 * @param now reference point for the relative-time label, injectable for tests.
 */
fun EarthquakeEvent.toHistoryItem(
    userLocation: UserLocation?,
    zone: ZoneId = ZoneId.systemDefault(),
    now: Instant = Instant.now()
): QuakeHistoryItem = QuakeHistoryItem(
    id = eventId,
    intensity = mmi,
    severity = severity(),
    location = locationName,
    date = QuakeFormat.date(createdAt, zone),
    time = QuakeFormat.time(createdAt, zone),
    distanceKm = userLocation.distanceKmTo(latitude, longitude)?.roundToInt() ?: 0,
    relativeTime = QuakeFormat.relativeTime(createdAt, now),
    pgaLabel = QuakeFormat.pga(pgaGal),
    durationLabel = QuakeFormat.UNAVAILABLE,
    coordinates = QuakeFormat.coordinates(latitude, longitude),
    latitude = latitude,
    longitude = longitude
)

fun List<EarthquakeEvent>.toHistoryItems(
    userLocation: UserLocation?,
    zone: ZoneId = ZoneId.systemDefault(),
    now: Instant = Instant.now()
): List<QuakeHistoryItem> = map { it.toHistoryItem(userLocation, zone, now) }

/**
 * Banner intensity line for a stored event, e.g. "Intensity : IV (moderate)".
 *
 * Shares [QuakeFormat.intensityBanner] with the realtime path so the Warning
 * banner reads identically whether it was seeded from REST or pushed over the
 * WebSocket.
 */
fun EarthquakeEvent.intensityBannerLabel(): String =
    QuakeFormat.intensityBanner(mmi = mmi, label = intensityLabel, fallbackWord = severity().name)

/**
 * Bare intensity read for the active alert card, e.g. "IV (moderate)" (Figma node
 * 1:1067) — the stored-event twin of
 * [id.web.quakealert.data.network.mapper.intensityValueLabel] on the realtime frame,
 * so a quake seeded from REST and the same quake pushed over the socket read alike.
 */
fun EarthquakeEvent.intensityValueLabel(): String =
    QuakeFormat.intensityValue(mmi = mmi, label = intensityLabel, fallbackWord = severity().name)


/**
 * Collapses the server's three-way severity into the two buckets the UI has a
 * treatment for.
 *
 * `intensity_label` is the primary signal ("strong" → red, "light"/"moderate" →
 * orange); [PGA_SEVERE_THRESHOLD_GAL] is the fallback when the label is missing,
 * using the same 137.2 gal boundary the server derives "strong" from
 * (server/internal/consensus/centroid.go), so the two paths cannot disagree.
 *
 * A "light" quake showing an orange "Moderate" badge is a known compression:
 * [MmiSeverity] has no third variant, and adding one is a UI change beyond this
 * layer. Erring toward the *higher* of the two available buckets is the right
 * direction for a life-safety readout.
 */
internal fun EarthquakeEvent.severity(): MmiSeverity = when {
    intensityLabel.isNotBlank() ->
        if (intensityLabel.equals(LABEL_STRONG, ignoreCase = true)) {
            MmiSeverity.SEVERE
        } else {
            MmiSeverity.MODERATE
        }
    pgaGal >= PGA_SEVERE_THRESHOLD_GAL -> MmiSeverity.SEVERE
    else -> MmiSeverity.MODERATE
}

/** Server's word for the top severity bucket. */
private const val LABEL_STRONG = "strong"

/** PGA (gal) at which the server starts calling an event "strong". */
private const val PGA_SEVERE_THRESHOLD_GAL = 137.2

private fun String.toEventStatus(): EventStatus =
    when {
        equals(EventStatus.RESOLVED.name, ignoreCase = true) -> EventStatus.RESOLVED
        // Anything unrecognised is treated as still happening: assuming an unknown
        // state is already over is the unsafe direction to guess in.
        else -> EventStatus.HAPPENING
    }

private fun String.toInstantOrNull(): Instant? =
    runCatching { Instant.parse(this) }.getOrNull()

/**
 * Falls back to the epoch for an unparseable `created_at`, which sorts the row to
 * the bottom of a DESC feed instead of dropping the event entirely.
 */
private fun String.toInstantOrEpoch(): Instant = toInstantOrNull() ?: Instant.EPOCH
