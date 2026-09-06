package com.aleksclark.primer.student.management

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import kotlinx.coroutines.flow.first

class PreferencesManagementOutbox(
    private val store: DataStore<Preferences>,
) : ManagementOutbox {
    private val key = stringPreferencesKey("outbox")

    override suspend fun putIfAbsent(entry: OutboxEntry): OutboxEntry {
        var stored = entry
        store.edit { prefs ->
            val items = OutboxCodec.decode(prefs[key])
            stored = items[entry.id] ?: entry.also { items[entry.id] = it }
            prefs[key] = OutboxCodec.encode(items.values)
        }
        return stored
    }

    override suspend fun get(id: String): OutboxEntry? =
        OutboxCodec.decode(store.data.first()[key])[id]

    override suspend fun pending(origin: String, deviceId: String): List<OutboxEntry> =
        OutboxCodec.decode(store.data.first()[key]).values.filter {
            it.origin == origin && it.deviceId == deviceId && it.deadLetter == null
        }

    override suspend fun markAttempt(id: String, deadLetter: String?) {
        store.edit { prefs ->
            val items = OutboxCodec.decode(prefs[key])
            items[id]?.let { items[id] = it.copy(attempts = it.attempts + 1, deadLetter = deadLetter ?: it.deadLetter) }
            prefs[key] = OutboxCodec.encode(items.values)
        }
    }

    override suspend fun remove(id: String) {
        store.edit { prefs ->
            val items = OutboxCodec.decode(prefs[key])
            items.remove(id)
            prefs[key] = OutboxCodec.encode(items.values)
        }
    }

    override suspend fun undelivered(origin: String, deviceId: String): Boolean =
        hasRetryable(origin, deviceId)

    override suspend fun hasRetryable(origin: String, deviceId: String): Boolean =
        OutboxCodec.decode(store.data.first()[key]).values.any {
            it.origin == origin && it.deviceId == deviceId && it.deadLetter == null
        }

    override suspend fun hasDeadLetter(origin: String, deviceId: String): Boolean =
        OutboxCodec.decode(store.data.first()[key]).values.any {
            it.origin == origin && it.deviceId == deviceId && it.deadLetter != null
        }
}

class DataStoreManagementOutbox(context: Context) : ManagementOutbox by PreferencesManagementOutbox(context.managementDataStore)
