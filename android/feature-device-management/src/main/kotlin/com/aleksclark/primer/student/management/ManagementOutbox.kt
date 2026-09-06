package com.aleksclark.primer.student.management

data class OutboxEntry(
    val id: String,
    val kind: String,
    val body: String,
    val attempts: Int = 0,
)

interface ManagementOutbox {
    suspend fun put(entry: OutboxEntry)
    suspend fun get(id: String): OutboxEntry?
    suspend fun pending(): List<OutboxEntry>
    suspend fun markAttempt(id: String)
    suspend fun remove(id: String)
}

class InMemoryManagementOutbox : ManagementOutbox {
    private val items = linkedMapOf<String, OutboxEntry>()
    override suspend fun put(entry: OutboxEntry) {
        items[entry.id] = items[entry.id]?.copy(body = entry.body) ?: entry
    }
    override suspend fun get(id: String): OutboxEntry? = items[id]
    override suspend fun pending(): List<OutboxEntry> = items.values.toList()
    override suspend fun markAttempt(id: String) {
        items[id]?.let { items[id] = it.copy(attempts = it.attempts + 1) }
    }
    override suspend fun remove(id: String) { items.remove(id) }
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
