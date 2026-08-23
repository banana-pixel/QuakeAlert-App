package id.web.quakealert.data.network.mapper

import id.web.quakealert.data.network.model.BroadcastDto
import id.web.quakealert.data.network.model.BroadcastsResponseDto
import id.web.quakealert.data.network.model.WsBroadcastMessageDto
import id.web.quakealert.domain.OperatorUpdate
import id.web.quakealert.ui.updates.OperatorUpdateItem
import java.time.Instant
import java.util.Locale

/** The `type` discriminator of an operator announcement (server `dispatch.TypeBroadcast`). */
const val TYPE_ADMIN_BROADCAST: String = "ADMIN_BROADCAST"

/**
 * REST → domain.
 *
 * Returns null when `broadcast_id` is blank: that field is the dedup key the
 * notification and the list are both keyed on, and a row without one cannot be told
 * apart from the next.
 */
fun BroadcastDto.toDomainOrNull(): OperatorUpdate? {
    if (broadcastId.isBlank()) return null
    return OperatorUpdate(
        id = broadcastId,
        title = title,
        body = body,
        regionCode = regionCode?.takeIf { it.isNotBlank() },
        publishedAt = createdAt.toInstantOrNull() ?: Instant.EPOCH
    )
}

/** Drops unusable rows rather than failing the page: one bad row is not an outage. */
fun List<BroadcastDto>.toOperatorUpdates(): List<OperatorUpdate> = mapNotNull { it.toDomainOrNull() }

fun BroadcastsResponseDto.toOperatorUpdates(): List<OperatorUpdate> = broadcasts.toOperatorUpdates()

/**
 * Socket frame → domain, so an announcement that arrives live and the same one read
 * back from `GET /broadcasts` are indistinguishable downstream.
 *
 * The `type` is re-checked here even though the socket already sorted on it: this
 * function is also the one the tests exercise, and a frame decoded as the wrong
 * envelope would otherwise succeed with every field defaulted.
 */
fun WsBroadcastMessageDto.toDomainOrNull(): OperatorUpdate? {
    if (!type.equals(TYPE_ADMIN_BROADCAST, ignoreCase = true)) return null
    if (broadcastId.isBlank()) return null
    return OperatorUpdate(
        id = broadcastId,
        title = title,
        body = body,
        regionCode = regionCode.takeIf { it.isNotBlank() },
        publishedAt = Instant.ofEpochMilli(timestamp)
    )
}

/**
 * FCM `data` map → domain. Every value in an FCM payload is a string (a platform
 * constraint), so the timestamp is parsed here rather than by the serializer.
 *
 * @param nowMs used when `timestamp` is missing or unparseable. Unlike an alert this
 *   is only cosmetic — nothing about an announcement is gated on its age — but a
 *   notice dated 1970 would sort to the bottom of the list forever.
 */
fun Map<String, String>.toOperatorUpdateOrNull(
    nowMs: Long = System.currentTimeMillis()
): OperatorUpdate? = WsBroadcastMessageDto(
    type = this["type"]?.trim().orEmpty(),
    broadcastId = this["broadcast_id"]?.trim().orEmpty(),
    title = this["title"]?.trim().orEmpty(),
    body = this["body"]?.trim().orEmpty(),
    regionCode = this["region_code"]?.trim().orEmpty(),
    timestamp = this["timestamp"]?.trim()?.toLongOrNull() ?: nowMs
).toDomainOrNull()

/** An unparseable RFC3339 timestamp yields null for the caller to decide about. */
private fun String.toInstantOrNull(): Instant? = runCatching { Instant.parse(this) }.getOrNull()

/**
 * Domain → the rows the Updates overlay renders, newest first.
 *
 * Pure, and kept out of the composable for the same reason the chat date separators
 * are: the wording is the one piece of judgement in this path, and it depends on a
 * clock a test has to control.
 *
 * @param now injected so the relative ages are reproducible in a test.
 */
fun List<OperatorUpdate>.toUpdateItems(now: Instant = Instant.now()): List<OperatorUpdateItem> =
    sortedByDescending { it.publishedAt }.map { it.toUpdateItem(now) }

fun OperatorUpdate.toUpdateItem(now: Instant = Instant.now()): OperatorUpdateItem =
    OperatorUpdateItem(
        id = id,
        title = title.ifBlank { "QuakeAlert update" },
        body = body,
        scope = if (isNational) SCOPE_NATIONAL else regionScopeLabel(regionCode),
        // Epoch is what the mapper leaves behind when the server sent an unparseable
        // timestamp. "2 months ago" would be a fabrication; saying so is not.
        published = if (publishedAt == Instant.EPOCH) {
            "Date unknown"
        } else {
            QuakeFormat.relativeTime(publishedAt, now)
        }
    )

/** Every device was told, and the list says so rather than leaving the scope blank. */
private const val SCOPE_NATIONAL = "Nationwide"

/**
 * `ID-jawa-barat` → "Jawa Barat".
 *
 * Presentational only. The key itself is never rebuilt from this string — it is the
 * server's normalisation and the client passes it back verbatim, exactly as with a
 * chat channel id (docs/CHAT_DESIGN.md §3). A key this does not recognise is shown
 * as it arrived rather than dropped: a scope the user cannot read still beats a
 * notice that looks like it was aimed at nobody.
 */
private fun regionScopeLabel(regionCode: String?): String {
    val raw = regionCode?.trim().orEmpty()
    if (raw.isEmpty()) return SCOPE_NATIONAL
    val area = raw.substringAfter('-', missingDelimiterValue = "").trim()
    if (area.isEmpty()) return raw
    return area.split('-', '_', ' ')
        .filter { it.isNotBlank() }
        .joinToString(" ") { word ->
            word.lowercase(Locale.US).replaceFirstChar { it.uppercase(Locale.US) }
        }
}
