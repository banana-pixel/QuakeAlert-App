package id.web.quakealert.data

import android.content.Context
import android.content.SharedPreferences
import kotlinx.coroutines.channels.awaitClose
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.callbackFlow
import kotlinx.coroutines.flow.distinctUntilChanged

/**
 * Lightweight local preference store for app-level flags. Backed by
 * [SharedPreferences] so the onboarding-completed flag survives process death
 * and app restarts, without pulling in an extra dependency.
 *
 * The single source of truth is [isOnboardingCompleted], a cold [Flow] that
 * emits the current value immediately and then re-emits whenever the underlying
 * preference changes. This mirrors a DataStore-style reactive API so swapping in
 * Jetpack DataStore later is a drop-in change.
 */
class AppSettingsRepository(context: Context) {

    private val prefs: SharedPreferences =
        context.applicationContext.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)

    /**
     * Emits whether the user has finished the onboarding flow. Defaults to
     * `false` for a fresh install so the onboarding is shown as the entry point.
     */
    val isOnboardingCompleted: Flow<Boolean> = callbackFlow {
        // Emit the current value up front so collectors have state immediately.
        trySend(prefs.getBoolean(KEY_ONBOARDING_COMPLETED, false))

        val listener = SharedPreferences.OnSharedPreferenceChangeListener { p, key ->
            if (key == KEY_ONBOARDING_COMPLETED) {
                trySend(p.getBoolean(KEY_ONBOARDING_COMPLETED, false))
            }
        }
        prefs.registerOnSharedPreferenceChangeListener(listener)
        awaitClose { prefs.unregisterOnSharedPreferenceChangeListener(listener) }
    }.distinctUntilChanged()

    /** Marks onboarding as finished so the app opens straight into MainScreen. */
    fun completeOnboarding() {
        prefs.edit().putBoolean(KEY_ONBOARDING_COMPLETED, true).apply()
    }

    /**
     * Emits the selected distance unit system. Defaults to [UnitSystem.METRIC]
     * for a fresh install. Emits the current value up front and re-emits
     * whenever the preference changes, mirroring [isOnboardingCompleted].
     */
    val unitSystem: Flow<UnitSystem> = callbackFlow {
        trySend(readUnitSystem(prefs))

        val listener = SharedPreferences.OnSharedPreferenceChangeListener { p, key ->
            if (key == KEY_UNIT_SYSTEM) {
                trySend(readUnitSystem(p))
            }
        }
        prefs.registerOnSharedPreferenceChangeListener(listener)
        awaitClose { prefs.unregisterOnSharedPreferenceChangeListener(listener) }
    }.distinctUntilChanged()

    /** Persists the distance unit system for the History / Sensors / Settings screens. */
    fun setUnitSystem(unit: UnitSystem) {
        prefs.edit().putString(KEY_UNIT_SYSTEM, unit.name).apply()
    }

    private fun readUnitSystem(prefs: SharedPreferences): UnitSystem =
        prefs.getString(KEY_UNIT_SYSTEM, null)
            ?.let { stored -> runCatching { UnitSystem.valueOf(stored) }.getOrNull() }
            ?: UnitSystem.METRIC

    private companion object {
        const val PREFS_NAME = "quakealert_app_settings"
        const val KEY_ONBOARDING_COMPLETED = "onboarding_completed"
        const val KEY_UNIT_SYSTEM = "unit_system"
    }
}
