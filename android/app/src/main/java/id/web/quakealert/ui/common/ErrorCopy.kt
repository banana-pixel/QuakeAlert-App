package id.web.quakealert.ui.common

import androidx.compose.runtime.Immutable
import id.web.quakealert.data.network.ApiException
import java.io.IOException
import java.net.SocketTimeoutException
import java.net.UnknownHostException
import javax.net.ssl.SSLException

/**
 * What a failure card offers the user to do about it.
 *
 * An explicit third case rather than a nullable retry: a request the server refused
 * as invalid will be refused again, and a "Retry" that cannot work is worse than no
 * button — it costs the user a tap to learn nothing, and teaches them the button
 * means nothing on the occasions when it does work.
 */
enum class ErrorAction { RETRY, RESET_FILTERS, NONE }

/**
 * User-facing copy for one failure: what happened, what still works, and the single
 * thing that could resolve it.
 *
 * @param title names what failed in the user's terms, never in the transport's.
 * @param message one or two sentences: what this means and what is still usable.
 * @param action the one offer, per [ErrorAction].
 */
@Immutable
data class ErrorCopy(
    val title: String,
    val message: String,
    val action: ErrorAction
)

/**
 * Maps a failure to the copy the user sees.
 *
 * Keyed on the *kind* of failure rather than on any text the failure carries, which
 * is the whole point of this mapper: the server speaks Indonesian to its own
 * operators ("range_km butuh latitude & longitude") and OkHttp speaks in host names
 * and socket states, and both used to reach the screen verbatim through
 * `throwable.message`. Neither is copy: one is in the wrong language, the other names
 * an implementation the user has no way to act on.
 *
 * Rules every branch below follows, and any new branch must:
 *  - name what failed in the user's terms, not the transport's;
 *  - say what still works, when anything does;
 *  - offer exactly one action, and only one that can actually help;
 *  - never print a status code, an exception name or a stack;
 *  - never blame the user for a failure the app or the network caused.
 *
 * The raw failure is not discarded, it is simply not shown: every call site logs it
 * at `Log.w` so a bug report still carries the real cause.
 *
 * @param isNarrowed whether a filter is currently narrowing the query. Only consulted
 *   for a rejected request, where clearing the filter is the one thing that might
 *   turn an unacceptable query into an acceptable one.
 */
fun errorCopy(throwable: Throwable, isNarrowed: Boolean = false): ErrorCopy = when {
    throwable is ApiException -> apiErrorCopy(throwable, isNarrowed)
    // Checked after ApiException, which is itself an IOException so that it travels
    // the same catch as a transport failure.
    throwable is UnknownHostException ||
        throwable is SocketTimeoutException ||
        throwable is SSLException ||
        throwable is IOException -> OFFLINE

    else -> UNKNOWN
}

/** The HTTP half of [errorCopy], split out to keep the branch list readable. */
private fun apiErrorCopy(failure: ApiException, isNarrowed: Boolean): ErrorCopy = when {
    // The client already re-bootstraps the anonymous identity once on a 401, so
    // reaching here means that retry failed too. Retry is still the offer: the
    // identity is anonymous and recoverable, so there is nothing for the user to
    // sign back in to.
    failure.isUnauthenticated -> ErrorCopy(
        title = "Sign-in expired",
        message = "QuakeAlert could not confirm this device with the alert network. " +
            "Your saved data is still shown.",
        action = ErrorAction.RETRY
    )

    // A ceiling we hit rather than a query we got wrong, and the one 4xx where
    // waiting and trying again genuinely works.
    failure.httpCode == 429 -> ErrorCopy(
        title = "Too many requests",
        message = "QuakeAlert asked the alert network too often. Wait a moment and try again.",
        action = ErrorAction.RETRY
    )

    failure.httpCode in 400..499 -> ErrorCopy(
        title = "That request was not accepted",
        message = if (isNarrowed) {
            "The alert network could not answer this combination of filters."
        } else {
            "The alert network could not answer this request. Your saved data is still shown."
        },
        action = if (isNarrowed) ErrorAction.RESET_FILTERS else ErrorAction.NONE
    )

    else -> UNKNOWN
}

/**
 * No usable link to the server. Deliberately does not distinguish a missing network
 * from a timeout from a TLS failure: all three mean the same thing to the user, and
 * the difference is only actionable to us, where the log already has it.
 */
private val OFFLINE = ErrorCopy(
    title = "You are offline",
    message = "QuakeAlert cannot reach the alert network. Your saved data is still shown.",
    action = ErrorAction.RETRY
)

/** A server fault or a failure we could not classify; both are ours, not the user's. */
private val UNKNOWN = ErrorCopy(
    title = "Something Went Wrong",
    message = "QuakeAlert could not finish that request. Try again in a moment.",
    action = ErrorAction.RETRY
)

/**
 * The copy shown when a load fails before it has been classified, and by previews.
 * Not a fallback the mapper can return: [errorCopy] classifies every [Throwable],
 * `else` branch included.
 */
val GenericErrorCopy: ErrorCopy = UNKNOWN
