package com.aleksclark.primer.devicepolicy

import android.content.Context
import android.os.SystemClock
import android.provider.Settings
import org.json.JSONArray
import org.json.JSONObject

/** Credential-protected storage. Only salted, high-entropy verifiers, never plaintext codes. */
class RecoveryStore(context: Context) {
    private val context = context.applicationContext
    private val prefs = this.context.getSharedPreferences("parent-recovery", Context.MODE_PRIVATE)

    fun clock() = RecoveryClock(System.currentTimeMillis(), SystemClock.elapsedRealtime(),
        Settings.Global.getInt(context.contentResolver, Settings.Global.BOOT_COUNT, -1))

    fun read(): RecoveryState = synchronized(lock) { snapshot().state }

    fun snapshot(): RecoverySnapshot = synchronized(lock) {
        val json = JSONObject(prefs.getString("state", "{}")!!)
        RecoverySnapshot(
            state = RecoveryState(
                verifiers = json.optJSONArray("verifiers").strings(),
                failures = json.optInt("failures"),
                retryWallMs = json.optLong("retryWallMs"),
                retryElapsedMs = json.optLong("retryElapsedMs"),
                retryBoot = json.optInt("retryBoot", -1),
                backoffMs = json.optLong("backoffMs"),
                leaseUntilElapsedMs = json.optLong("leaseUntilElapsedMs"),
                leaseBoot = json.optInt("leaseBoot", -1),
            ),
            audit = JSONArray(prefs.getString("audit", "[]")!!).strings(),
            consumed = RecoveryJournal.decodeConsumed(prefs.getString("consumedRequests", "[]")),
        )
    }

    fun prepareCodes(): List<String> = Recovery.generate()

    fun activateCodes(codes: List<String>, bootstrap: Boolean) = synchronized(lock) {
        val current = snapshot()
        check(bootstrap || current.state.maintenanceActive(clock())) { "Parent maintenance expired before code activation" }
        commit(current.copy(state = Recovery.rotate(current.state, codes), audit = current.audit + "${clock().wallMs}:recovery_rotated"))
    }

    fun consumedRequestIds(): Set<String> = synchronized(lock) { snapshot().consumed.keys }

    fun ackReportId(requestId: String): String? = synchronized(lock) { snapshot().consumed[requestId]?.ackReportId }

    /** Atomic remote rotation. Replay of a consumed requestId cannot reset used codes. */
    fun activateRemoteCodes(requestId: String, codes: List<String>): RemoteIntentResult = synchronized(lock) {
        val current = snapshot()
        val now = clock()
        val existing = current.consumed[requestId]
        if (existing != null) {
            return RemoteIntentResult(current, firstApply = false, ackReportId = existing.ackReportId)
        }
        val result = RecoveryJournal.consume(
            snapshot = current,
            requestId = requestId,
            kind = "rotate_recovery_code",
            nextState = Recovery.rotate(current.state, codes),
            event = "recovery_remote_rotated:$requestId",
            nowWallMs = now.wallMs,
        )
        commit(result.snapshot)
        result
    }

    fun alreadyConsumed(requestId: String): Boolean = ackReportId(requestId) != null

    fun openRemoteLease(requestId: String, durationMs: Long): RemoteIntentResult = synchronized(lock) {
        val current = snapshot()
        val now = clock()
        val existing = current.consumed[requestId]
        if (existing != null) {
            return RemoteIntentResult(current, firstApply = false, ackReportId = existing.ackReportId)
        }
        val result = RecoveryJournal.consume(
            snapshot = current,
            requestId = requestId,
            kind = "maintenance_lease",
            nextState = Recovery.openLease(current.state, now, durationMs),
            event = "recovery_remote_lease:$requestId",
            nowWallMs = now.wallMs,
        )
        commit(result.snapshot)
        result
    }

    fun authorize(code: String): RecoveryAttempt = synchronized(lock) {
        val result = Recovery.attempt(snapshot().state, code, clock())
        val current = snapshot()
        commit(current.copy(state = result.state, audit = current.audit + "${clock().wallMs}:${result.event}"))
        result
    }

    fun close() = synchronized(lock) {
        val current = snapshot()
        if (current.state.leaseUntilElapsedMs != 0L) {
            commit(
                current.copy(
                    state = current.state.copy(leaseUntilElapsedMs = 0, leaseBoot = -1),
                    audit = current.audit + "${clock().wallMs}:maintenance_closed",
                ),
            )
        }
    }

    fun audit(): List<String> = synchronized(lock) { snapshot().audit }

    private fun commit(snapshot: RecoverySnapshot) {
        check(
            prefs.edit()
                .putString("state", RecoveryJournal.encodeState(snapshot.state))
                .putString("audit", RecoveryJournal.encodeAudit(snapshot.audit))
                .putString("consumedRequests", RecoveryJournal.encodeConsumed(snapshot.consumed))
                .commit(),
        ) { "Recovery state could not be persisted; no maintenance access granted." }
    }

    private fun JSONArray?.strings(): List<String> = if (this == null) emptyList() else
        (0 until length()).map { getString(it) }

    companion object { private val lock = Any() }
}
