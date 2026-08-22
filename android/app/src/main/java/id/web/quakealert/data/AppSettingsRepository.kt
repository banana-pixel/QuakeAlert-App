package id.web.quakealert.data

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.booleanPreferencesKey
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.longPreferencesKey
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.SharedPreferencesMigration
import androidx.datastore.preferences.preferencesDataStore
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.launch

/**
 * The preferences DataStore file, declared as a top-level extension property so
 * all [AppSettingsRepository] instances share one store — the five ViewModels that
 * construct the repository directly would otherwise open five DataStores over the
 * same file, which throws at runtime.
 *
 * [SharedPreferencesMigration] carries over the two keys the SharedPreferences
 * implementation used (`onboarding_completed`, `unit_system`) under their original
 * names, so an upgrading install does not get shown onboarding again.
 */
private val Context.settingsDataStore: DataStore<Preferences> by preferencesDataStore(
    name = "quakealert_app_settings",
    produceMigrations = { context ->
        listOf(SharedPreferencesMigration(context, "quakealert_app_settings"))
    }
)

/**
 * App-level preferences: onboarding state, units, and the flags behind the
 * Settings screen.
 *
 * The alert radius is deliberately absent: it is a safety decision the system
 * makes, not a preference (see [id.web.quakealert.domain.SafetyPolicy]).
 *
 * Backed by Jetpack DataStore as .clinerules/20 rule 7 requires. The reactive
 * [Flow] API predates the migration and is unchanged, so call sites did not move;
 * the mutators stayed non-suspend for the same reason and dispatch their writes
 * onto [scope], a process-lifetime scope. A write that outlives its ViewModel is
 * the desired behaviour here — the value must land even if the screen goes away.
 *
 * Separate from [id.web.quakealert.data.local.SessionStore] on purpose: this is
 * disposable UI state, that one is identity.
 */
class AppSettingsRepository(context: Context) {

    private val dataStore = context.applicationContext.settingsDataStore

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)

    /**
     * Emits whether the user has finished onboarding. Defaults to `false` on a
     * fresh install so onboarding is the entry point.
     */
    val isOnboardingCompleted: Flow<Boolean> =
        read { it[KEY_ONBOARDING_COMPLETED] ?: false }

    /** Marks onboarding as finished so the app opens straight into MainScreen. */
    fun completeOnboarding() = write { it[KEY_ONBOARDING_COMPLETED] = true }

    /** Emits the selected distance unit system, [UnitSystem.METRIC] by default. */
    val unitSystem: Flow<UnitSystem> = read { prefs ->
        prefs[KEY_UNIT_SYSTEM]
            ?.let { stored -> runCatching { UnitSystem.valueOf(stored) }.getOrNull() }
            ?: UnitSystem.METRIC
    }

    /** Persists the distance unit system for the History / Sensors / Settings screens. */
    fun setUnitSystem(unit: UnitSystem) = write { it[KEY_UNIT_SYSTEM] = unit.name }

    /** Whether the app refreshes the stored position on start. On by default. */
    val autoSyncLocation: Flow<Boolean> = read { it[KEY_AUTO_SYNC_LOCATION] ?: true }

    fun setAutoSyncLocation(enabled: Boolean) = write { it[KEY_AUTO_SYNC_LOCATION] = enabled }

    /**
     * The user's own alert toggle, distinct from the `POST_NOTIFICATIONS` grant:
     * the OS permission says the app *may* post, this says the user *wants* it.
     */
    val notificationsEnabled: Flow<Boolean> = read { it[KEY_NOTIFICATIONS_ENABLED] ?: true }

    fun setNotificationsEnabled(enabled: Boolean) = write {
        it[KEY_NOTIFICATIONS_ENABLED] = enabled
    }

    /**
     * Epoch milliseconds of the last accepted `PUT /users/location`, or null when
     * the position has never been synced. Null is what the Settings row renders as
     * "Never", so it is deliberately not `0`.
     */
    val lastSyncAtMs: Flow<Long?> = read { it[KEY_LAST_SYNC_AT_MS] }

    fun setLastSyncAtMs(epochMs: Long) = write { it[KEY_LAST_SYNC_AT_MS] = epochMs }

    /**
     * The regional chat channel the server says applies to the last synced position,
     * or null when it has none.
     *
     * Stored here, and not derived on the Chat screen, because the fact is produced by
     * `PUT /users/location` and consumed somewhere else entirely: it is what tells Chat
     * that a room the user could not see a moment ago now exists. DataStore because it
     * has to survive process death — otherwise a cold start would look like a change and
     * reconnect the socket for nothing.
     */
    val regionCode: Flow<String?> = read { it[KEY_REGION_CODE] }

    /** Records the region the server derived; blank and null both mean "global only". */
    fun setRegionCode(code: String?) = write {
        val trimmed = code?.trim().orEmpty()
        if (trimmed.isEmpty()) it.remove(KEY_REGION_CODE) else it[KEY_REGION_CODE] = trimmed
    }

    /**
     * Whether the quiet ongoing status notification is shown.
     *
     * Default **false**: an ongoing notification the user did not ask for is clutter,
     * and the app is no less protective without it. See
     * [id.web.quakealert.service.StatusNotifier].
     */
    val statusNotification: Flow<Boolean> = read { it[KEY_STATUS_NOTIFICATION] ?: false }

    fun setStatusNotification(enabled: Boolean) = write {
        it[KEY_STATUS_NOTIFICATION] = enabled
    }

    /** UI language tag. Inert placeholder — only `en` ships today. */
    val language: Flow<String> = read { it[KEY_LANGUAGE] ?: DEFAULT_LANGUAGE }

    fun setLanguage(tag: String) = write { it[KEY_LANGUAGE] = tag }

    /** One-shot read of the auto-sync flag. */
    suspend fun readAutoSyncLocation(): Boolean = autoSyncLocation.first()

    /** One-shot read of the last position sync, null when never synced. */
    suspend fun readLastSyncAtMs(): Long? = lastSyncAtMs.first()

    private fun <T> read(transform: (Preferences) -> T): Flow<T> =
        dataStore.data.map(transform).distinctUntilChanged()

    private fun write(mutate: (androidx.datastore.preferences.core.MutablePreferences) -> Unit) {
        scope.launch { dataStore.edit(mutate) }
    }

    companion object {
        const val DEFAULT_LANGUAGE = "en"

        private val KEY_ONBOARDING_COMPLETED = booleanPreferencesKey("onboarding_completed")
        private val KEY_UNIT_SYSTEM = stringPreferencesKey("unit_system")
        private val KEY_AUTO_SYNC_LOCATION = booleanPreferencesKey("auto_sync_location")
        private val KEY_NOTIFICATIONS_ENABLED = booleanPreferencesKey("notifications_enabled")
        private val KEY_LAST_SYNC_AT_MS = longPreferencesKey("last_sync_at_ms")
        private val KEY_LANGUAGE = stringPreferencesKey("language")
        private val KEY_REGION_CODE = stringPreferencesKey("region_code")
        private val KEY_STATUS_NOTIFICATION = booleanPreferencesKey("status_notification")
    }
}
