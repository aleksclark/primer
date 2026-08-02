package com.aleksclark.primer.tv.app.data

import android.content.Context
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.booleanPreferencesKey
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.intPreferencesKey
import androidx.datastore.preferences.core.longPreferencesKey
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import androidx.datastore.preferences.core.emptyPreferences
import androidx.datastore.core.DataStore
import com.aleksclark.primer.tv.core.data.DeviceSettings
import com.aleksclark.primer.tv.core.data.GrantStore
import com.aleksclark.primer.tv.core.data.SettingsStore
import com.aleksclark.primer.tv.core.data.StoredGrant
import java.io.IOException
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.catch
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map

private val Context.settingsDataStore: DataStore<Preferences> by preferencesDataStore(name = "device_settings")
private val Context.grantDataStore: DataStore<Preferences> by preferencesDataStore(name = "active_grant")

/** Maps [DeviceSettings] onto DataStore preferences. */
class DataStoreSettingsStore(private val store: DataStore<Preferences>) : SettingsStore {

    constructor(context: Context) : this(context.applicationContext.settingsDataStore)

    override val settings: Flow<DeviceSettings> = store.data
        // A corrupt or unreadable file must not wedge the app on a black
        // screen; an empty read simply presents the pairing flow again.
        .catch { cause -> if (cause is IOException) emit(emptyPreferences()) else throw cause }
        .map(::toSettings)

    override suspend fun current(): DeviceSettings = settings.first()

    override suspend fun setBaseUrl(baseUrl: String) {
        store.edit { it[KEY_BASE_URL] = baseUrl }
    }

    override suspend fun savePairing(token: String, deviceId: String, deviceName: String, deviceKind: String) {
        store.edit {
            it[KEY_TOKEN] = token
            it[KEY_DEVICE_ID] = deviceId
            it[KEY_DEVICE_NAME] = deviceName
            it[KEY_DEVICE_KIND] = deviceKind
        }
    }

    override suspend fun clearPairing() {
        store.edit {
            it.remove(KEY_TOKEN)
            it.remove(KEY_DEVICE_ID)
            it.remove(KEY_DEVICE_NAME)
            it.remove(KEY_DEVICE_KIND)
        }
    }

    companion object {
        private val KEY_BASE_URL = stringPreferencesKey("base_url")
        private val KEY_TOKEN = stringPreferencesKey("token")
        private val KEY_DEVICE_ID = stringPreferencesKey("device_id")
        private val KEY_DEVICE_NAME = stringPreferencesKey("device_name")
        private val KEY_DEVICE_KIND = stringPreferencesKey("device_kind")

        /** Exposed for tests, which drive a temp-directory DataStore. */
        fun toSettings(prefs: Preferences) = DeviceSettings(
            baseUrl = prefs[KEY_BASE_URL],
            token = prefs[KEY_TOKEN],
            deviceId = prefs[KEY_DEVICE_ID],
            deviceName = prefs[KEY_DEVICE_NAME],
            deviceKind = prefs[KEY_DEVICE_KIND],
        )
    }
}

/**
 * Persists the in-flight grant so a process killed mid-film rejoins its own
 * session instead of spending a second play.
 */
class DataStoreGrantStore(private val store: DataStore<Preferences>) : GrantStore {

    constructor(context: Context) : this(context.applicationContext.grantDataStore)

    override suspend fun load(): StoredGrant? {
        val prefs = store.data
            .catch { cause -> if (cause is IOException) emit(emptyPreferences()) else throw cause }
            .first()
        return toGrant(prefs)
    }

    override suspend fun save(grant: StoredGrant) {
        store.edit {
            it[KEY_GRANT_ID] = grant.grantId
            it[KEY_MEDIA_ITEM_ID] = grant.mediaItemId
            it[KEY_STREAM_URL] = grant.streamUrl
            it[KEY_START_OFFSET] = grant.startOffsetSeconds
            it[KEY_MODE] = grant.mode
            it[KEY_EXPIRES_AT] = grant.expiresAtEpochSeconds
            it[KEY_POSITION] = grant.positionSeconds
            it[KEY_WATCHED] = grant.watchedSeconds
            it[KEY_REDEEMED] = grant.redeemed
        }
    }

    override suspend fun clear() {
        store.edit { it.clear() }
    }

    companion object {
        private val KEY_GRANT_ID = stringPreferencesKey("grant_id")
        private val KEY_MEDIA_ITEM_ID = stringPreferencesKey("media_item_id")
        private val KEY_STREAM_URL = stringPreferencesKey("stream_url")
        private val KEY_START_OFFSET = intPreferencesKey("start_offset_seconds")
        private val KEY_MODE = stringPreferencesKey("mode")
        private val KEY_EXPIRES_AT = longPreferencesKey("expires_at_epoch_seconds")
        private val KEY_POSITION = intPreferencesKey("position_seconds")
        private val KEY_WATCHED = intPreferencesKey("watched_seconds")
        private val KEY_REDEEMED = booleanPreferencesKey("redeemed")

        /** Exposed for tests, which drive a temp-directory DataStore. */
        fun toGrant(prefs: Preferences): StoredGrant? {
            val grantId = prefs[KEY_GRANT_ID] ?: return null
            val mediaItemId = prefs[KEY_MEDIA_ITEM_ID] ?: return null
            val streamUrl = prefs[KEY_STREAM_URL] ?: return null
            return StoredGrant(
                grantId = grantId,
                mediaItemId = mediaItemId,
                streamUrl = streamUrl,
                startOffsetSeconds = prefs[KEY_START_OFFSET] ?: 0,
                mode = prefs[KEY_MODE] ?: "on_demand",
                expiresAtEpochSeconds = prefs[KEY_EXPIRES_AT] ?: 0L,
                positionSeconds = prefs[KEY_POSITION] ?: 0,
                watchedSeconds = prefs[KEY_WATCHED] ?: 0,
                redeemed = prefs[KEY_REDEEMED] ?: false,
            )
        }
    }
}
