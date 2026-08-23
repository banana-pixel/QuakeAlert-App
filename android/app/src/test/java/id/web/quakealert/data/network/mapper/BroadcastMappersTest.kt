package id.web.quakealert.data.network.mapper

import id.web.quakealert.data.network.model.BroadcastDto
import id.web.quakealert.data.network.model.BroadcastsResponseDto
import id.web.quakealert.data.network.model.WsBroadcastMessageDto
import id.web.quakealert.domain.OperatorUpdate
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Instant

/**
 * An announcement reaches the device by three routes — REST, the socket and an FCM
 * data map — and all three must land on the same row under the same id, or a user
 * who was pushed a notice sees it twice in the list. The id is also the only field
 * a row cannot do without, so its absence is the one hard rejection here.
 */
class BroadcastMappersTest {

    private val now: Instant = Instant.parse("2026-08-23T10:00:00Z")

    @Test
    fun `maps a REST row and treats a blank region as national`() {
        val update = BroadcastDto(
            broadcastId = "b-1",
            title = "Maintenance tonight",
            body = "The sensors go offline at 23:00.",
            regionCode = "",
            createdAt = "2026-08-23T09:00:00Z"
        ).toDomainOrNull()

        requireNotNull(update)
        assertEquals("b-1", update.id)
        assertNull(update.regionCode)
        assertTrue(update.isNational)
        assertEquals(Instant.parse("2026-08-23T09:00:00Z"), update.publishedAt)
    }

    @Test
    fun `drops a row with no id but keeps the rest of the page`() {
        val page = BroadcastsResponseDto(
            broadcasts = listOf(
                BroadcastDto("", "no id", "body", null, "2026-08-23T09:00:00Z"),
                BroadcastDto("b-2", "kept", "body", "ID-jawa-barat", "2026-08-23T09:30:00Z")
            )
        ).toOperatorUpdates()

        assertEquals(listOf("b-2"), page.map { it.id })
    }

    @Test
    fun `leaves an unparseable timestamp at epoch rather than guessing`() {
        val update = BroadcastDto("b-3", "t", "b", null, "not-a-date").toDomainOrNull()

        assertEquals(Instant.EPOCH, requireNotNull(update).publishedAt)
        assertEquals("Date unknown", update.toUpdateItem(now).published)
    }

    @Test
    fun `maps a socket frame and refuses a frame of another type`() {
        val frame = WsBroadcastMessageDto(
            type = TYPE_ADMIN_BROADCAST,
            broadcastId = "b-4",
            title = "Drill",
            body = "A test alert will be sent.",
            regionCode = "ID-jawa-barat",
            timestamp = 1_756_000_000_000L
        )

        val update = requireNotNull(frame.toDomainOrNull())
        assertEquals("ID-jawa-barat", update.regionCode)
        assertEquals(Instant.ofEpochMilli(1_756_000_000_000L), update.publishedAt)

        // An alert envelope decoded as a broadcast would otherwise default every
        // field and appear in the list as an empty notice.
        assertNull(frame.copy(type = "EARTHQUAKE_ALERT").toDomainOrNull())
        assertNull(frame.copy(broadcastId = " ").toDomainOrNull())
    }

    @Test
    fun `maps an FCM data map and falls back to now for a missing timestamp`() {
        val update = mapOf(
            "type" to TYPE_ADMIN_BROADCAST,
            "broadcast_id" to " b-5 ",
            "title" to "Notice",
            "body" to "Body"
        ).toOperatorUpdateOrNull(nowMs = now.toEpochMilli())

        requireNotNull(update)
        assertEquals("b-5", update.id)
        assertTrue(update.isNational)
        assertEquals(now.toEpochMilli(), update.publishedAt.toEpochMilli())

        // An alert push must never be rendered as an announcement.
        assertNull(
            mapOf("type" to "EARTHQUAKE_ALERT", "broadcast_id" to "e-1")
                .toOperatorUpdateOrNull(nowMs = now.toEpochMilli())
        )
    }

    @Test
    fun `renders rows newest first with a readable scope`() {
        val items = listOf(
            update("old", at = "2026-08-21T10:00:00Z", region = null),
            update("new", at = "2026-08-23T09:00:00Z", region = "ID-jawa-barat")
        ).toUpdateItems(now)

        assertEquals(listOf("new", "old"), items.map { it.id })
        assertEquals("Jawa Barat", items[0].scope)
        assertEquals("1 hour ago", items[0].published)
        assertEquals("Nationwide", items[1].scope)
    }

    @Test
    fun `shows an unrecognised region key as it arrived and names an untitled notice`() {
        val item = OperatorUpdate(
            id = "b-6",
            title = "   ",
            body = "Body",
            regionCode = "GLOBAL",
            publishedAt = now
        ).toUpdateItem(now)

        assertEquals("GLOBAL", item.scope)
        assertEquals("QuakeAlert update", item.title)
    }

    private fun update(id: String, at: String, region: String?): OperatorUpdate =
        OperatorUpdate(
            id = id,
            title = id,
            body = "body",
            regionCode = region,
            publishedAt = Instant.parse(at)
        )
}
