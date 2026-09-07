package com.aleksclark.primer.updates

/**
 * Shared self-update session transitions. Callers persist [SelfUpdateRecord];
 * this object does not read SharedPreferences keys.
 */
data class SelfUpdateRecord(
    val active: Boolean = false,
    val pendingConfirmation: Boolean = false,
    val sessionId: Int = -1,
    val desiredVersion: Long = 0,
    val status: String = "No self-update attempted",
    val outcomeStatus: String = "queued",
    val outcomeError: String? = null,
    val hasConfirmation: Boolean = false,
)

data class SelfUpdateSessionState(
    val status: String,
    val active: Boolean,
    val pendingConfirmation: Boolean,
    val installerSessionLive: Boolean,
    val desiredVersion: Long,
    val lastOutcome: InstallAttempt,
    val hasConfirmationIntent: Boolean,
)

object SelfUpdateFlow {
    fun onPendingUserAction(
        current: SelfUpdateRecord,
        hasConfirmation: Boolean,
        presented: Boolean,
    ): SelfUpdateRecord {
        if (!current.active) return current
        if (!hasConfirmation) {
            return fail(current, "blocked", "Android requires a system install confirmation")
        }
        val status = if (presented) {
            "Waiting for system install confirmation"
        } else {
            "Install confirmation is waiting. Return to Control or enable notifications."
        }
        return current.copy(
            pendingConfirmation = true,
            hasConfirmation = true,
            status = status,
            outcomeStatus = "blocked",
            outcomeError = if (presented) null else "Install confirmation was not shown",
        )
    }

    fun onResumeUserAction(
        current: SelfUpdateRecord,
        sessionLive: Boolean,
        presented: Boolean,
    ): SelfUpdateRecord {
        if (!current.pendingConfirmation) return current
        if (!sessionLive) {
            return fail(current, "failed", "Installation interrupted or rejected")
        }
        if (!current.hasConfirmation) {
            return fail(current, "blocked", "Android requires a system install confirmation")
        }
        return onPendingUserAction(current, hasConfirmation = true, presented = presented)
    }

    fun onMissingInstallerSession(current: SelfUpdateRecord): SelfUpdateRecord {
        if (!current.active) return current
        return fail(current, "failed", "Installation interrupted or rejected")
    }

    fun onCancel(current: SelfUpdateRecord): SelfUpdateRecord =
        fail(current, "failed", "Installation cancelled")

    fun onClearFailed(current: SelfUpdateRecord): SelfUpdateRecord {
        if (current.active || current.pendingConfirmation) return current
        if (current.outcomeStatus != "failed" && current.outcomeStatus != "blocked") return current
        return idle()
    }

    fun canCancel(state: SelfUpdateSessionState): Boolean =
        state.active || state.pendingConfirmation || state.installerSessionLive

    fun canRetry(state: SelfUpdateSessionState): Boolean {
        if (state.active || state.pendingConfirmation || state.installerSessionLive) return false
        val status = state.lastOutcome.status
        return status == "failed" || status == "blocked"
    }

    fun shouldAbandon(record: SelfUpdateRecord): Boolean = !record.active && record.sessionId >= 0

    fun idle(): SelfUpdateRecord = SelfUpdateRecord()

    private fun fail(current: SelfUpdateRecord, outcome: String, reason: String) = current.copy(
        active = false,
        pendingConfirmation = false,
        hasConfirmation = false,
        status = "Update failed: $reason",
        outcomeStatus = outcome,
        outcomeError = reason.take(200),
    )
}

interface SelfUpdateCommands {
    fun snapshot(): SelfUpdateSessionState
    fun reconcile(): InstallAttempt
    fun evaluate(apk: java.io.File, expected: SignedManifest): SelfUpdateEligibility
    fun install(apk: java.io.File, expected: SignedManifest): InstallAttempt
    fun handleResult(intent: android.content.Intent, onUserAction: ((android.content.Intent) -> Boolean)? = null): InstallAttempt
    fun resumeUserAction(onUserAction: (android.content.Intent) -> Boolean): InstallAttempt
    fun cancel(): InstallAttempt
    fun clearFailed(): InstallAttempt
}
