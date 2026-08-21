package id.web.quakealert.ui.common

import androidx.lifecycle.ViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update

/**
 * Owns the [QuakeFilterState] shared by the History and Sensors tabs, plus the
 * open/closed state of the filter sheet.
 *
 * **Why a ViewModel of its own.** Both tabs resolve their ViewModels with
 * `viewModel()` against the host Activity's store (see
 * [id.web.quakealert.ui.main.MainScreen]), so a `QuakeFilterViewModel` obtained
 * from `HistoryRoute` and from `SensorsRoute` is *the same instance*: setting the
 * filter on one tab is already set on the other, and the choice survives rotation
 * and tab switches. That is the cross-tab sync the filter needs without a
 * process-wide singleton — and without touching each tab's own state machine.
 *
 * **Why it is not persisted.** The instance dies with the process, which is the
 * intended lifetime. Someone who narrowed the list to severe quakes last week and
 * opens the app during an earthquake must see everything; a filter restored from
 * disk would quietly hide the event they opened the app for.
 *
 * The filter is *not* pushed into the tab ViewModels from here — the routes
 * observe it and call each tab's `applyFilter`, keeping the dependency one-way and
 * this class free of any tab's data loading.
 */
class QuakeFilterViewModel : ViewModel() {

    private val _filter = MutableStateFlow(QuakeFilterState())

    /** The criteria currently in force on both tabs. */
    val filter: StateFlow<QuakeFilterState> = _filter.asStateFlow()

    private val _isSheetOpen = MutableStateFlow(false)

    /** Whether the filter sheet is showing. Shared so it survives a rotation. */
    val isSheetOpen: StateFlow<Boolean> = _isSheetOpen.asStateFlow()

    /**
     * Switches the "All" / "Near" pill.
     *
     * Leaves the sheet criteria untouched: someone who filtered to MMI VI+ and then
     * taps "All" is widening the *area*, not asking to see light shaking again.
     */
    fun onModeSelected(mode: QuakeFilter) {
        _filter.update { if (it.mode == mode) it else it.copy(mode = mode) }
    }

    /**
     * Applies the sheet's drafted criteria wholesale when the user confirms it.
     *
     * The whole state rather than a criterion list: the sheet shows a different set
     * of criteria per tab, and the ones it did not show come back untouched from the
     * draft it was seeded with.
     */
    fun onCriteriaApplied(draft: QuakeFilterState) {
        _filter.update { current ->
            draft.copy(
                // Choosing a radius is only meaningful around a centre, so picking a
                // different one implies "Near"; leaving it alone keeps the pill as-is.
                mode = if (draft.radius != current.radius) QuakeFilter.NEAR else draft.mode
            )
        }
        _isSheetOpen.value = false
    }

    /** Back to the unfiltered feed, from the no-data card's "Reset Filters". */
    fun onFiltersReset() {
        _filter.value = QuakeFilterState()
    }

    /** One radius step wider, from the no-coverage card's "Widen Search Radius". */
    fun onRadiusWidened() {
        _filter.update { it.widened() }
    }

    fun onSheetOpened() {
        _isSheetOpen.value = true
    }

    fun onSheetDismissed() {
        _isSheetOpen.value = false
    }
}
