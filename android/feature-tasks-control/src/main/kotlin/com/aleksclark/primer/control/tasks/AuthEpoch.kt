package com.aleksclark.primer.control.tasks

import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicLong

/** Monotonic fence. Operations capture the value into [AuthContext]; there is no current-action global. */
class AuthEpoch {
    private val value = AtomicLong(0)
    fun current(): Long = value.get()
    fun bump(): Long = value.incrementAndGet()
    fun isCurrent(snapshot: Long): Boolean = snapshot == value.get()
}

/** Immutable credentials for one in-flight Control operation. */
data class AuthContext(
    val sessionId: String,
    val epoch: Long,
    val token: String,
)

class MutationGate {
    private val busy = AtomicBoolean(false)
    fun tryBegin(): Boolean = busy.compareAndSet(false, true)
    fun end() { busy.set(false) }
    fun isBusy(): Boolean = busy.get()
}

data class LogoutAttempt(
    val sessionId: String,
    val token: String,
    val serverRevoked: Boolean,
)

sealed class LogoutDecision {
    data object ClearSession : LogoutDecision()
    data class Incomplete(val message: String, val serverRevoked: Boolean) : LogoutDecision()
}

object FailClosedLogout {
    fun decide(serverRevoked: Boolean, providerSignedOut: Boolean, serverError: String?, providerError: String?): LogoutDecision {
        if (!serverRevoked) {
            return LogoutDecision.Incomplete(
                message = serverError ?: "Tasks could not revoke the parent session. Stay signed in and try again.",
                serverRevoked = false,
            )
        }
        if (!providerSignedOut) {
            return LogoutDecision.Incomplete(
                message = providerError ?: "Clerk could not sign out. The Tasks session was revoked; retry sign-out.",
                serverRevoked = true,
            )
        }
        return LogoutDecision.ClearSession
    }
}

object ScheduleIdentity {
    data class Revision(val templateId: String, val revisionId: String)

    fun fromPublishedTask(templateId: String?, revisionId: String?): Revision? {
        val template = templateId?.trim().orEmpty()
        val revision = revisionId?.trim().orEmpty()
        if (template.isEmpty() || revision.isEmpty()) return null
        return Revision(template, revision)
    }
}

enum class ConflictResource { None, Occurrence, Schedule, Device, Task, Student }

object ConflictRefresh {
    fun shouldRefreshOccurrence(resource: ConflictResource): Boolean = resource == ConflictResource.Occurrence
    fun shouldRefreshSchedule(resource: ConflictResource): Boolean = resource == ConflictResource.Schedule
    fun shouldRefreshDevice(resource: ConflictResource): Boolean = resource == ConflictResource.Device
}
