package id.web.quakealert.data.network.mapper

import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.util.Locale
import kotlin.math.abs

/**
 * Display formatting shared by the event and alert mappers, so a quake rendered
 * from the REST history and the same quake rendered from a live WebSocket frame
 * produce byte-identical strings.
 *
 * Locale is pinned to [Locale.US] on purpose: the UI copy is English ("20 Jun 2026",
 * "2 months ago") and a device set to another locale must not produce a card that
 * mixes languages mid-line. The *zone*, by contract, is the device's — timestamps
 * cross the wire as UTC and are converted only at the display boundary
 * (docs/CLIENT_SPEC.md §7).
 */
internal object QuakeFormat {

    /** e.g. "20 Jun 2026". */
    private val DATE = DateTimeFormatter.ofPattern("dd MMM yyyy", Locale.US)

    /** e.g. "07:19:18 WIB" — `zzz` renders the device zone's short name. */
    private val TIME = DateTimeFormatter.ofPattern("HH:mm:ss zzz", Locale.US)

    /** e.g. "09:41" — chat bubbles, where seconds and a zone name are noise. */
    private val CHAT_TIME = DateTimeFormatter.ofPattern("HH:mm", Locale.US)

    /**
     * Placeholder for a value the server contract does not carry.
     *
     * A hyphen rather than an em dash: this is printed inside composed rows
     * ("RSSI : - dBm"), and the app's user-visible copy holds no em dashes at all,
     * so a lone one here would be the only place the character appeared on screen.
     */
    const val UNAVAILABLE: String = "-"

    fun date(instant: Instant, zone: ZoneId): String = DATE.format(instant.atZone(zone))

    fun time(instant: Instant, zone: ZoneId): String = TIME.format(instant.atZone(zone))

    /**
     * Send time inside a chat bubble, e.g. "09:41".
     *
     * Minutes only, and no zone suffix: a chat bubble is read in the room it was
     * sent to, so the seconds and the zone name that a quake read-out needs would
     * only be noise here.
     */
    fun chatTime(instant: Instant, zone: ZoneId): String = CHAT_TIME.format(instant.atZone(zone))

    /** PGA in the canonical unit, e.g. "61.5 gal". Never converted to `g` here. */
    fun pga(pgaGal: Double): String = String.format(Locale.US, "%.1f gal", pgaGal)

    /**
     * How many stations reported the shaking, e.g. "3 stations".
     *
     * Replaces the shaking duration in the detail overlay's third metric cell. The
     * REST contract carries no duration: the firmware does send `dur_ms`, and the
     * server range-checks and HMAC-signs it, but `earthquake_events` has no column
     * for it, so it is discarded after verification. Printing a permanent "-" in a
     * cell taught the user nothing, while the node count is a fact the response
     * already carries and is the one that says how much to trust the reading.
     *
     * Zero is [UNAVAILABLE] rather than "0 stations": an event exists because
     * stations triggered, so a zero here is a missing field, not a real count.
     */
    fun reportingNodes(count: Int): String = when {
        count <= 0 -> UNAVAILABLE
        count == 1 -> "1 station"
        else -> "$count stations"
    }

    /**
     * Centroid as "-6.91750, 107.61910" — five decimals, matching the precision the
     * detail overlay was designed around (~1 m, well below the centroid's own error).
     */
    fun coordinates(latitude: Double, longitude: Double): String =
        String.format(Locale.US, "%.5f, %.5f", latitude, longitude)

    /**
     * Warning-banner intensity line, e.g. "Intensity : IV (moderate)" — the wording
     * the design uses on the banner (Figma 124:1297).
     *
     * Shared by the stored-event and realtime paths so the banner does not reword
     * itself when the WebSocket takes over from the REST seed. [fallbackWord] covers
     * a server that sent no `intensity_label`, so the line never trails an empty
     * bracket.
     */
    fun intensityBanner(mmi: String, label: String, fallbackWord: String): String =
        "Intensity : ${intensityValue(mmi, label, fallbackWord)}"

    /**
     * The bare intensity read, e.g. "IV (moderate)", for the active alert card
     * (Figma node 1:1067) — which renders "Estimated Intensity :" as its own label
     * line above the value and so must not carry the prefix.
     *
     * [intensityBanner] delegates here so the two screens can never drift into
     * spelling the same intensity differently.
     */
    fun intensityValue(mmi: String, label: String, fallbackWord: String): String {
        val word = label.takeIf { it.isNotBlank() } ?: fallbackWord
        val roman = mmi.takeIf { it.isNotBlank() } ?: UNAVAILABLE
        return "$roman (${word.lowercase(Locale.US)})"
    }

    /**
     * Coarse age of an event, e.g. "just now", "20 minutes ago", "2 months ago".
     *
     * Deliberately coarse — the exact timestamp is one line away on the same card,
     * and rounding "89 seconds" to "a minute ago" is what makes the list scannable.
     * Future timestamps (device clock behind the server) collapse to "just now"
     * rather than rendering a negative age.
     */
    fun relativeTime(instant: Instant, now: Instant): String {
        val seconds = now.epochSecond - instant.epochSecond
        if (seconds < MINUTE) return "just now"

        val (amount, unit) = when {
            seconds < HOUR -> seconds / MINUTE to "minute"
            seconds < DAY -> seconds / HOUR to "hour"
            seconds < MONTH -> seconds / DAY to "day"
            seconds < YEAR -> seconds / MONTH to "month"
            else -> seconds / YEAR to "year"
        }
        val plural = if (abs(amount) == 1L) "" else "s"
        return "$amount $unit$plural ago"
    }

    private const val MINUTE = 60L
    private const val HOUR = 60 * MINUTE
    private const val DAY = 24 * HOUR

    /** Calendar-average month/year lengths: this is a coarse label, not arithmetic. */
    private const val MONTH = 30 * DAY
    private const val YEAR = 365 * DAY
}
