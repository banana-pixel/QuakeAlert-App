package id.web.quakealert.data.local

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.doublePreferencesKey
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.longPreferencesKey
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import id.web.quakealert.domain.UserLocation
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map

/**
 * The app's single DataStore file. Declared as a top-level extension property so
 * every [SessionStore] instance shares one underlying store — constructing two
 * DataStores over the same file throws at runtime.
 */
private val Context.sessionDataStore: DataStore<Preferences> by preferencesDataStore(
    name = "quakealert_session"
)

/**
 * The anonymous identity issued by `POST /api/v1/auth/anonymous`, as persisted
 * locally.
 *
 * @param expiresAtMs epoch milliseconds of the JWT's `exp` claim, or null when the
 *   server sent no `expires_at` and the claim could not be decoded. Null means
 *   "unknown", not "expired" — the token is then used until the server rejects it.
 */
data class StoredSession(
    val token: String,
    val userId: String,
    val pseudonym: String,
    val expiresAtMs: Long?
)

/**
 * Persistent home of the anonymous session and the user's last known position.
 *
 * Uses Jetpack DataStore, as docs/CLIENT_SPEC.md §2 and .clinerules/20 rule 7
 * require for the token. [id.web.quakealert.data.AppSettingsRepository] still
 * holds UI preferences in SharedPreferences; the two stores are separate on
 * purpose — one is disposable UI state, this one is identity.
 *
 * The stored position is what makes distance read-outs work after a cold start
 * without waiting for a GPS fix, and it is the client-side half of the Haversine
 * gating in .clinerules/20 rule 2.
 */
class SessionStore(context: Context) {

    private val dataStore = context.applicationContext.sessionDataStore

    /** Emits the stored session, or null when there is none yet (fresh install). */
    val session: Flow<StoredSession?> = dataStore.data.map { prefs -> prefs.toSession() }

    /** Emits the last known user position, or null when it has never been set. */
    val userLocation: Flow<UserLocation?> = dataStore.data.map { prefs -> prefs.toUserLocation() }

    /** One-shot read of the stored session. */
    suspend fun readSession(): StoredSession? = dataStore.data.first().toSession()

    /** One-shot read of the last known user position. */
    suspend fun readUserLocation(): UserLocation? = dataStore.data.first().toUserLocation()

    /** Replaces the stored identity with a freshly issued one. */
    suspend fun saveSession(session: StoredSession) {
        dataStore.edit { prefs ->
            prefs[KEY_TOKEN] = session.token
            prefs[KEY_USER_ID] = session.userId
            prefs[KEY_PSEUDONYM] = session.pseudonym
            val expiry = session.expiresAtMs
            if (expiry != null) prefs[KEY_EXPIRES_AT] = expiry else prefs.remove(KEY_EXPIRES_AT)
        }
    }

    /**
     * Drops the identity after the server rejected it (401).
     *
     * Everything goes, not just the token: a new bootstrap mints a *new*
     * `user_id`, so keeping the old id or pseudonym around would leave the app
     * displaying an identity the server no longer knows.
     */
    suspend fun clearSession() {
        dataStore.edit { prefs ->
            prefs.remove(KEY_TOKEN)
            prefs.remove(KEY_USER_ID)
            prefs.remove(KEY_PSEUDONYM)
            prefs.remove(KEY_EXPIRES_AT)
        }
    }

    /**
     * Replaces the stored pseudonym after a successful reroll.
     *
     * Only the display name changes — the token and `user_id` are untouched, since
     * a reroll renames the identity rather than issuing a new one.
     */
    suspend fun savePseudonym(pseudonym: String) {
        dataStore.edit { prefs -> prefs[KEY_PSEUDONYM] = pseudonym }
    }

    /** Caches the position last accepted by `PUT /api/v1/users/location`. */
    suspend fun saveUserLocation(location: UserLocation) {
        dataStore.edit { prefs ->
            prefs[KEY_LATITUDE] = location.latitude
            prefs[KEY_LONGITUDE] = location.longitude
            val name = location.locationName
            if (name != null) prefs[KEY_LOCATION_NAME] = name else prefs.remove(KEY_LOCATION_NAME)
        }
    }

    private fun Preferences.toSession(): StoredSession? {
        val token = this[KEY_TOKEN]?.takeIf { it.isNotBlank() } ?: return null
        return StoredSession(
            token = token,
            userId = this[KEY_USER_ID].orEmpty(),
            pseudonym = this[KEY_PSEUDONYM].orEmpty(),
            expiresAtMs = this[KEY_EXPIRES_AT]
        )
    }

    private fun Preferences.toUserLocation(): UserLocation? {
        // Both coordinates or neither: a half-written position would silently
        // become a point on the equator / prime meridian.
        val lat = this[KEY_LATITUDE] ?: return null
        val lon = this[KEY_LONGITUDE] ?: return null
        return UserLocation(latitude = lat, longitude = lon, locationName = this[KEY_LOCATION_NAME])
    }

    private companion object {
        val KEY_TOKEN = stringPreferencesKey("auth_token")
        val KEY_USER_ID = stringPreferencesKey("auth_user_id")
        val KEY_PSEUDONYM = stringPreferencesKey("auth_pseudonym")
        val KEY_EXPIRES_AT = longPreferencesKey("auth_expires_at_ms")
        val KEY_LATITUDE = doublePreferencesKey("user_latitude")
        val KEY_LONGITUDE = doublePreferencesKey("user_longitude")
        val KEY_LOCATION_NAME = stringPreferencesKey("user_location_name")
    }
}
