package id.web.quakealert.ui.updates

import id.web.quakealert.domain.OperatorUpdate
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Instant

/**
 * The same announcement can arrive twice — once from `GET /broadcasts` and once on
 * the socket — so the merge is what decides whether the list shows one notice or
 * two. Ordering is asserted alongside it because a live arrival has to land at the
 * top to be worth being live at all.
 */
class UpdatesUiStateTest {

    @Test
    fun `a live arrival goes to the top`() {
        val merged = listOf(update("b-1", "2026-08-23T08:00:00Z"))
            .mergedWith(update("b-2", "2026-08-23T09:00:00Z"))

        assertEquals(listOf("b-2", "b-1"), merged.map { it.id })
    }

    @Test
    fun `the same announcement twice is one row`() {
        val page = listOf(
            update("b-2", "2026-08-23T09:00:00Z"),
            update("b-1", "2026-08-23T08:00:00Z")
        )

        val merged = page.mergedWith(update("b-1", "2026-08-23T08:00:00Z"))

        assertEquals(listOf("b-2", "b-1"), merged.map { it.id })
    }

    @Test
    fun `a socket copy replaces the stored row rather than sitting beside it`() {
        val merged = listOf(update("b-1", "2026-08-23T08:00:00Z", title = "old"))
            .mergedWith(update("b-1", "2026-08-23T08:00:00Z", title = "corrected"))

        assertEquals(1, merged.size)
        assertEquals("corrected", merged[0].title)
    }

    @Test
    fun `sorts a page newest first whatever order the server sent`() {
        val sorted = listOf(
            update("b-1", "2026-08-21T08:00:00Z"),
            update("b-3", "2026-08-23T08:00:00Z"),
            update("b-2", "2026-08-22T08:00:00Z")
        ).newestFirst()

        assertEquals(listOf("b-3", "b-2", "b-1"), sorted.map { it.id })
    }

    @Test
    fun `empty means loaded with nothing, not loading and not failed`() {
        assertTrue(UpdatesUiState().isEmpty)
        assertFalse(UpdatesUiState(isLoading = true).isEmpty)
        assertFalse(
            UpdatesUiState(
                updates = listOf(
                    OperatorUpdateItem("b-1", "t", "b", "Nationwide", "just now")
                )
            ).isEmpty
        )
    }

    private fun update(id: String, at: String, title: String = id): OperatorUpdate =
        OperatorUpdate(
            id = id,
            title = title,
            body = "body",
            regionCode = null,
            publishedAt = Instant.parse(at)
        )
}
