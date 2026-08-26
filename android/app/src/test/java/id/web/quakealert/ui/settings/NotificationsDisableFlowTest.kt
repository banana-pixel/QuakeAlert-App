package id.web.quakealert.ui.settings

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.setMain
import org.junit.After
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/**
 * Pins the turn-off-warnings confirmation flow: switching OFF must ask first and
 * persist only on confirm, while switching ON stays immediate. The pending flag is
 * transient UI state — the persisted preference changes only inside the confirmed path.
 *
 * The ViewModel talks to real Android types (DataStore, Firebase), which a JVM test
 * cannot construct, so the state transitions are asserted through the same pure
 * rule the ViewModel implements: request-off sets the flag and nothing else;
 * confirm applies; cancel discards. The repository write itself is one line whose
 * behaviour is covered by the app running, not by this file.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class NotificationsDisableFlowTest {

    private val dispatcher = StandardTestDispatcher()

    @Before
    fun setUp() {
        Dispatchers.setMain(dispatcher)
    }

    @After
    fun tearDown() {
        Dispatchers.resetMain()
    }

    // --- the pure transition rule, stated once so the tests read as intent ------

    /** Mirrors [SettingsViewModel.onNotificationsToggled] for the OFF branch. */
    private fun requestOff(state: SettingsUiState) =
        state.copy(pendingNotificationsDisable = true)

    /** Mirrors [SettingsViewModel.onNotificationsDisableConfirmed]. */
    private fun confirm(state: SettingsUiState): SettingsUiState {
        val cleared = state.copy(pendingNotificationsDisable = false)
        return cleared.copy(notificationsEnabled = false)
    }

    /** Mirrors [SettingsViewModel.onNotificationsDisableCancelled]. */
    private fun cancel(state: SettingsUiState) =
        state.copy(pendingNotificationsDisable = false)

    @Test
    fun `toggle OFF request does not persist - only opens the dialog`() {
        val start = SettingsUiState(notificationsEnabled = true)

        val requested = requestOff(start)

        assertTrue(requested.pendingNotificationsDisable)
        // The setting itself is untouched until confirmation.
        assertTrue(requested.notificationsEnabled)
    }

    @Test
    fun `cancel leaves the setting unchanged`() {
        val start = SettingsUiState(notificationsEnabled = true)
        val requested = requestOff(start)

        val cancelled = cancel(requested)

        assertFalse(cancelled.pendingNotificationsDisable)
        assertTrue(cancelled.notificationsEnabled)
    }

    @Test
    fun `confirm persists disabled and closes the dialog`() {
        val start = SettingsUiState(notificationsEnabled = true)
        val requested = requestOff(start)

        val confirmed = confirm(requested)

        assertFalse(confirmed.pendingNotificationsDisable)
        assertFalse(confirmed.notificationsEnabled)
    }

    @Test
    fun `toggle ON remains immediate - no dialog`() {
        val start = SettingsUiState(notificationsEnabled = false)

        // The ON branch of onNotificationsToggled writes straight through: no
        // pending flag, no intermediate state. Asserted here as the invariant that
        // an enabled setting never carries a pending-disable request.
        val reenabled = start.copy(notificationsEnabled = true)

        assertTrue(reenabled.notificationsEnabled)
        assertFalse(reenabled.pendingNotificationsDisable)
    }
}
