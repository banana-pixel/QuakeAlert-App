package id.web.quakealert.ui.common

/**
 * Whether a screen should reload itself now that connectivity has come back.
 *
 * One function for every tab that has a network-backed error state, so History and
 * Sensors cannot come to disagree about when an automatic reload is welcome.
 *
 * The answer is yes in exactly one case — the screen is showing a failure and has
 * nothing in flight:
 *
 *  - **Showing content:** left alone. A list that reloads under a reader scrolls the
 *    row they were looking at out from under them, and nothing on screen is wrong.
 *  - **Already loading, refreshing or paging:** left alone. The request in flight was
 *    started after the network came back, or is about to fail and set the error state
 *    that the *next* transition can act on. Firing a second load here would race it.
 *
 * The manual Retry button stays either way: this is the fast path, not the only one.
 *
 * @param isError the screen's failure flag.
 * @param isBusy true when any request is in flight (load, refresh or next page).
 */
internal fun shouldReloadOnReconnect(isError: Boolean, isBusy: Boolean): Boolean =
    isError && !isBusy
