package id.web.quakealert.ui.updates

import androidx.compose.runtime.Immutable
import id.web.quakealert.domain.OperatorUpdate
import id.web.quakealert.ui.common.ErrorCopy
import java.time.Instant

/**
 * One announcement as the list renders it.
 *
 * The formatting is resolved in the ViewModel rather than in the composable so the
 * copy is testable without a Compose harness, which is the same split every other
 * screen in the app uses.
 *
 * @param scope why the user was told: "Nationwide" or the province key the notice was
 *   aimed at. Shown because it is the difference between a notice that concerns
 *   everyone and one that concerns where the user is, and a list that hid it would
 *   leave the user guessing why a message about another province arrived at all.
 * @param published a relative age ("2 hours ago"), matching the earthquake cards.
 */
@Immutable
data class OperatorUpdateItem(
    val id: String,
    val title: String,
    val body: String,
    val scope: String,
    val published: String
)

/**
 * State of the Updates overlay.
 *
 * Three outcomes and no fourth: in flight, a list (possibly empty), or a classified
 * failure. An empty list is a real answer — operators publishing nothing is the
 * normal state of this screen — so it renders as an empty state, never as an error.
 *
 * @param error non-null only for a failure; [ErrorCopy] so a dropped connection reads
 *   the same sentence here as on every other tab.
 */
@Immutable
data class UpdatesUiState(
    val isLoading: Boolean = false,
    val updates: List<OperatorUpdateItem> = emptyList(),
    val error: ErrorCopy? = null
) {
    val isEmpty: Boolean get() = !isLoading && error == null && updates.isEmpty()
}

/**
 * Newest first, and stable: the server already sorts, but a socket frame is appended
 * to a page that was loaded earlier, so the order is re-established here rather than
 * trusted.
 */
internal fun List<OperatorUpdate>.newestFirst(): List<OperatorUpdate> =
    sortedByDescending { it.publishedAt }

/** Drops a repeat of an announcement already held, keyed on the server's id. */
internal fun List<OperatorUpdate>.mergedWith(incoming: OperatorUpdate): List<OperatorUpdate> =
    (listOf(incoming) + filterNot { it.id == incoming.id }).newestFirst()

/** Epoch means the server sent an unparseable timestamp; see the mapper. */
internal fun Instant.isUnknownTime(): Boolean = this == Instant.EPOCH
