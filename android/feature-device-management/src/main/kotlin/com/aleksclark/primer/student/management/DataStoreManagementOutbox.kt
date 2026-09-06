package com.aleksclark.primer.student.management

import android.content.Context
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import org.json.JSONArray
import org.json.JSONObject
import kotlinx.coroutines.flow.first

class DataStoreManagementOutbox(private val context: Context) : ManagementOutbox {
    private val key = stringPreferencesKey("outbox")

    override suspend fun put(entry: OutboxEntry) {
        mutate { items ->
            val existing = items[entry.id]
            items[entry.id] = existing?.copy(body = entry.body) ?: entry
        }
    }

    override suspend fun get(id: String): OutboxEntry? = snapshot()[id]

    override suspend fun pending(): List<OutboxEntry> = snapshot().values.toList()

    override suspend fun markAttempt(id: String) {
        mutate { items -> items[id]?.let { items[id] = it.copy(attempts = it.attempts + 1) } }
    }

    override suspend fun remove(id: String) {
        mutate { it.remove(id) }
    }

    private suspend fun snapshot(): LinkedHashMap<String, OutboxEntry> {
        val raw = context.managementDataStore.data.first()[key].orEmpty()
        val items = linkedMapOf<String, OutboxEntry>()
        if (raw.isBlank()) return items
        val array = JSONArray(raw)
        for (i in 0 until array.length()) {
            val obj = array.getJSONObject(i)
            val entry = OutboxEntry(
                id = obj.getString("id"),
                kind = obj.getString("kind"),
                body = obj.getString("body"),
                attempts = obj.optInt("attempts"),
            )
            items[entry.id] = entry
        }
        return items
    }

    private suspend fun mutate(block: (LinkedHashMap<String, OutboxEntry>) -> Unit) {
        val items = snapshot()
        block(items)
        val array = JSONArray(items.values.map {
            JSONObject()
                .put("id", it.id)
                .put("kind", it.kind)
                .put("body", it.body)
                .put("attempts", it.attempts)
        })
        context.managementDataStore.edit { it[key] = array.toString() }
    }
}
