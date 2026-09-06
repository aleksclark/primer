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

    fun read(): RecoveryState = synchronized(lock) {
        val json = JSONObject(prefs.getString("state", "{}")!!)
        RecoveryState(
            verifiers = json.optJSONArray("verifiers").strings(),
            failures = json.optInt("failures"),
            retryWallMs = json.optLong("retryWallMs"),
            retryElapsedMs = json.optLong("retryElapsedMs"),
            retryBoot = json.optInt("retryBoot", -1),
            backoffMs = json.optLong("backoffMs"),
            leaseUntilElapsedMs = json.optLong("leaseUntilElapsedMs"),
            leaseBoot = json.optInt("leaseBoot", -1),
        )
    }

    // Pending plaintext stays only in the parent's current UI session. Nothing is
    // replaced in storage until explicit off-device-custody confirmation.
    fun prepareCodes(): List<String> = Recovery.generate()

    fun activateCodes(codes: List<String>, bootstrap: Boolean) = synchronized(lock) {
        val state = read()
        check(bootstrap || state.maintenanceActive(clock())) { "Parent maintenance expired before code activation" }
        save(Recovery.rotate(state, codes), "recovery_rotated")
    }

    fun authorize(code: String): RecoveryAttempt = synchronized(lock) {
        val result = Recovery.attempt(read(), code, clock())
        // Commit consumption, throttle and audit BEFORE callers can open maintenance.
        save(result.state, result.event)
        result
    }

    fun close() = synchronized(lock) {
        val old = read()
        if (old.leaseUntilElapsedMs != 0L) save(old.copy(leaseUntilElapsedMs = 0, leaseBoot = -1), "maintenance_closed")
    }

    fun audit(): List<String> = synchronized(lock) { JSONArray(prefs.getString("audit", "[]")!!).strings() }

    private fun save(state: RecoveryState, event: String) {
        val json = JSONObject().apply {
            put("verifiers", JSONArray(state.verifiers))
            put("failures", state.failures)
            put("retryWallMs", state.retryWallMs)
            put("retryElapsedMs", state.retryElapsedMs)
            put("retryBoot", state.retryBoot)
            put("backoffMs", state.backoffMs)
            put("leaseUntilElapsedMs", state.leaseUntilElapsedMs)
            put("leaseBoot", state.leaseBoot)
        }
        val events = (audit() + "${System.currentTimeMillis()}:$event").takeLast(200)
        check(prefs.edit().putString("state", json.toString()).putString("audit", JSONArray(events).toString()).commit()) {
            "Recovery state could not be persisted; no maintenance access granted."
        }
    }

    private fun JSONArray?.strings(): List<String> = if (this == null) emptyList() else
        (0 until length()).map { getString(it) }

    companion object { private val lock = Any() }
}
