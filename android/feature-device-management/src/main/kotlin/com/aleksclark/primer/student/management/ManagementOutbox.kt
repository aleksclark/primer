package com.aleksclark.primer.student.management

import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json

@Serializable
data class OutboxEntry(
    val id: String,
    val kind: String,
    val body: String,
    val origin: String,
    val deviceId: String,
    val attempts: Int = 0,
    val deadLetter: String? = null,
)

interface ManagementOutbox {
    suspend fun putIfAbsent(entry: OutboxEntry): OutboxEntry
    suspend fun get(id: String): OutboxEntry?
    suspend fun pending(origin: String, deviceId: String): List<OutboxEntry>
    suspend fun markAttempt(id: String, deadLetter: String? = null)
    suspend fun remove(id: String)
    suspend fun undelivered(origin: String, deviceId: String): Boolean
}

class InMemoryManagementOutbox : ManagementOutbox {
    private val items = linkedMapOf<String, OutboxEntry>()
    override suspend fun putIfAbsent(entry: OutboxEntry): OutboxEntry {
        val existing = items[entry.id]
        if (existing != null) return existing
        items[entry.id] = entry
        return entry
    }
    override suspend fun get(id: String): OutboxEntry? = items[id]
    override suspend fun pending(origin: String, deviceId: String): List<OutboxEntry> =
        items.values.filter { it.origin == origin && it.deviceId == deviceId && it.deadLetter == null }
    override suspend fun markAttempt(id: String, deadLetter: String?) {
        items[id]?.let { items[id] = it.copy(attempts = it.attempts + 1, deadLetter = deadLetter ?: it.deadLetter) }
    }
    override suspend fun remove(id: String) { items.remove(id) }
    override suspend fun undelivered(origin: String, deviceId: String): Boolean =
        items.values.any { it.origin == origin && it.deviceId == deviceId }
}

object ManagementAuth {
    fun clearsEnrollment(status: Int, code: String?): Boolean {
        if (status == 401) return true
        if (status != 403) return false
        return code in setOf("unauthorized", "revoked", "enrollment_revoked", "device_revoked")
    }
}

object RemotePolicyProjection {
    fun extraControls(
        lockTaskEnabled: Boolean,
        lockTaskPackages: List<String>?,
        allowKeyguard: Boolean?,
        allowOverview: Boolean?,
        allowStatusBar: Boolean?,
        requiredPackages: List<String>,
        userRestrictions: List<String>,
        allowParentUnlock: Boolean,
        studentPackage: String,
        localRestrictions: Set<String>,
    ): List<com.aleksclark.primer.devicepolicy.ControlReadback> {
        val controls = mutableListOf<com.aleksclark.primer.devicepolicy.ControlReadback>()
        fun unsupported(name: String, desired: String, reason: String) {
            controls += com.aleksclark.primer.devicepolicy.ControlReadback(
                name = name,
                desired = desired,
                actual = "unsupported",
                status = "unsupported",
                error = reason,
            )
        }
        if (!lockTaskEnabled) unsupported("lock-task", "enabled=false", "Student requires lock-task")
        lockTaskPackages?.filter { it != studentPackage }?.forEach {
            unsupported("lock-task-package:$it", it, "Lock-task packages besides Student are not applied from remote policy")
        }
        if (allowKeyguard == false) unsupported("lock-task-keyguard", "false", "Keyguard remains required")
        if (allowOverview == true) unsupported("lock-task-overview", "true", "Overview cannot be enabled remotely")
        if (allowStatusBar == true) unsupported("lock-task-status-bar", "true", "Status bar cannot be enabled remotely")
        requiredPackages.filter { it != studentPackage }.forEach {
            unsupported("required:$it", it, "Required packages besides Student are not auto-installed")
        }
        userRestrictions.filter { it !in localRestrictions }.forEach {
            unsupported("restriction:$it", it, "Restriction is not in Student's local enforcement set")
        }
        if (!allowParentUnlock) unsupported("maintenance", "allowParentUnlock=false", "Local recovery cannot be disabled remotely")
        return controls
    }
}

object OutboxCodec {
    private val json = Json { ignoreUnknownKeys = true; encodeDefaults = true }

    fun encode(items: Collection<OutboxEntry>): String =
        json.encodeToString(kotlinx.serialization.builtins.ListSerializer(OutboxEntry.serializer()), items.toList())

    fun decode(raw: String?): LinkedHashMap<String, OutboxEntry> {
        val items = linkedMapOf<String, OutboxEntry>()
        if (raw.isNullOrBlank()) return items
        json.decodeFromString(kotlinx.serialization.builtins.ListSerializer(OutboxEntry.serializer()), raw).forEach {
            items[it.id] = it
        }
        return items
    }
}
