package id.web.quakealert.data

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.booleanPreferencesKey
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.intPreferencesKey
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
 * App-level preferences: onboarding state, units, coverage radius, and the flags
 * behind the Settings screen.
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

    /**
     * Emits the alert coverage radius in kilometres, clamped to [RADIUS_RANGE].
     *
     * The one value behind three things: the Haversine gate in
     * [id.web.quakealert.domain.AlertGate], and `range_km` on both `GET /events`
     * and `GET /sensors`. Clamped on read as well as write so a value written by
     * an older build can never widen the gate past what the server accepts.
     */
    val coverageRadiusKm: Flow<Int> = read { prefs ->
        (prefs[KEY_COVERAGE_RADIUS_KM] ?: DEFAULT_RADIUS_KM).coerceIn(RADIUS_RANGE)
    }

    /** Persists the coverage radius, clamped to [RADIUS_RANGE]. */
    fun setCoverageRadiusKm(km: Int) = write {
        it[KEY_COVERAGE_RADIUS_KM] = km.coerceIn(RADIUS_RANGE)
    }

    /**
     * The radius the server last confirmed, or null before any sync told it one.
     *
     * Kept apart from [coverageRadiusKm] because the two answer different
     * questions: that one is what the user chose, this one is what the backend
     * knows. The gap between them is what makes a radius change reach the server at
     * all — `PUT /users/location` is otherwise skipped whenever the device has not
     * moved a kilometre, so a slider moved at a desk would never be uploaded.
     */
    val syncedCoverageRadiusKm: Flow<Int?> = read { it[KEY_SYNCED_COVERAGE_RADIUS_KM] }

    /** Records the radius a `PUT /users/location` just had accepted. */
    fun setSyncedCoverageRadiusKm(km: Int) = write { it[KEY_SYNCED_COVERAGE_RADIUS_KM] = km }

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

    /** UI language tag. Inert placeholder — only `en` ships today. */
    val language: Flow<String> = read { it[KEY_LANGUAGE] ?: DEFAULT_LANGUAGE }

    fun setLanguage(tag: String) = write { it[KEY_LANGUAGE] = tag }

    /** One-shot read of the coverage radius, for callers not collecting a Flow. */
    suspend fun readCoverageRadiusKm(): Int = coverageRadiusKm.first()

    /** One-shot read of the auto-sync flag. */
    suspend fun readAutoSyncLocation(): Boolean = autoSyncLocation.first()

    /** One-shot read of the last position sync, null when never synced. */
    suspend fun readLastSyncAtMs(): Long? = lastSyncAtMs.first()

    /** One-shot read of the radius the server last confirmed, null when never. */
    suspend fun readSyncedCoverageRadiusKm(): Int? = syncedCoverageRadiusKm.first()

    private fun <T> read(transform: (Preferences) -> T): Flow<T> =
        dataStore.data.map(transform).distinctUntilChanged()

    private fun write(mutate: (androidx.datastore.preferences.core.MutablePreferences) -> Unit) {
        scope.launch { dataStore.edit(mutate) }
    }

    companion object {
        /** Bounds the Settings slider, and what the server will accept as `range_km`. */
        val RADIUS_RANGE = 50..300
        const val DEFAULT_RADIUS_KM = 150
        const val DEFAULT_LANGUAGE = "en"

        private val KEY_ONBOARDING_COMPLETED = booleanPreferencesKey("onboarding_completed")
        private val KEY_UNIT_SYSTEM = stringPreferencesKey("unit_system")
        private val KEY_COVERAGE_RADIUS_KM = intPreferencesKey("coverage_radius_km")
        private val KEY_SYNCED_COVERAGE_RADIUS_KM = intPreferencesKey("synced_coverage_radius_km")
        private val KEY_AUTO_SYNC_LOCATION = booleanPreferencesKey("auto_sync_location")
        private val KEY_NOTIFICATIONS_ENABLED = booleanPreferencesKey("notifications_enabled")
        private val KEY_LAST_SYNC_AT_MS = longPreferencesKey("last_sync_at_ms")
        private val KEY_LANGUAGE = stringPreferencesKey("language")
    }
}
