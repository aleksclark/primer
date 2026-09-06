package com.aleksclark.primer.devicepolicy

import java.security.MessageDigest
import java.security.SecureRandom

/** No Android dependencies: the persisted inputs and transitions are testable on the JVM. */
data class RecoveryClock(val wallMs: Long, val elapsedMs: Long, val boot: Int)
data class RecoveryState(
    val verifiers: List<String> = emptyList(),
    val failures: Int = 0,
    val retryWallMs: Long = 0,
    val retryElapsedMs: Long = 0,
    val retryBoot: Int = -1,
    val backoffMs: Long = 0,
    val leaseUntilElapsedMs: Long = 0,
    val leaseBoot: Int = -1,
) {
    fun maintenanceActive(now: RecoveryClock): Boolean =
        now.boot >= 0 && leaseBoot == now.boot && now.elapsedMs < leaseUntilElapsedMs

    fun remainingLeaseMs(now: RecoveryClock): Long =
        if (maintenanceActive(now)) leaseUntilElapsedMs - now.elapsedMs else 0

    fun waitMs(now: RecoveryClock): Long = when {
        backoffMs == 0L -> 0
        retryBoot == now.boot -> maxOf(retryWallMs - now.wallMs, retryElapsedMs - now.elapsedMs, 0)
        // Reboot/clock changes cannot shorten the penalty: wait at least one full backoff
        // after boot. Repeated boots keep restarting that wait rather than resetting failures.
        else -> maxOf(retryWallMs - now.wallMs, backoffMs - now.elapsedMs, 0)
    }
}

data class RecoveryAttempt(val state: RecoveryState, val accepted: Boolean, val event: String)

object Recovery {
    const val LEASE_MS = 5 * 60_000L
    private val random = SecureRandom()

    fun generate(count: Int = 6): List<String> = List(count) {
        ByteArray(16).also(random::nextBytes).hex().uppercase().chunked(8).joinToString("-")
    }

    /** Activate only after off-device custody is acknowledged. Generating a batch changes no state. */
    fun rotate(state: RecoveryState, codes: List<String>): RecoveryState {
        require(codes.size == 6 && codes.map(::normalize).toSet().size == 6 &&
            codes.all { normalize(it).matches(Regex("[0-9A-F]{32}")) }) { "Invalid recovery code batch" }
        return state.copy(verifiers = codes.map(::verifier), failures = 0, retryWallMs = 0,
            retryElapsedMs = 0, retryBoot = -1, backoffMs = 0)
    }

    fun verifier(code: String): String {
        val salt = ByteArray(16).also(random::nextBytes).hex()
        return "$salt:${digest(salt, normalize(code)).hex()}"
    }

    fun attempt(state: RecoveryState, code: String, now: RecoveryClock): RecoveryAttempt {
        require(now.boot >= 0) { "Boot identity unavailable; maintenance is disabled." }
        if (state.waitMs(now) > 0) return RecoveryAttempt(state, false, "recovery_rate_limited")
        val normalized = normalize(code)
        val found = state.verifiers.indexOfFirst { matches(it, normalized) }
        if (found < 0) {
            val failures = minOf(state.failures + 1, 30)
            val delay = if (failures < 5) 0L else minOf(60_000L * (1L shl minOf(failures - 5, 6)), 3_600_000L)
            return RecoveryAttempt(state.copy(
                failures = failures, backoffMs = delay, retryBoot = now.boot,
                retryWallMs = now.wallMs + delay, retryElapsedMs = now.elapsedMs + delay,
            ), false, "recovery_denied")
        }
        return RecoveryAttempt(openLease(state.copy(
            verifiers = state.verifiers.filterIndexed { index, _ -> index != found },
            failures = 0, backoffMs = 0, retryWallMs = 0, retryElapsedMs = 0,
        ), now, LEASE_MS), true, "maintenance_opened")
    }

    fun openLease(state: RecoveryState, now: RecoveryClock, durationMs: Long): RecoveryState {
        require(now.boot >= 0) { "Boot identity unavailable; maintenance is disabled." }
        require(durationMs > 0) { "Remote maintenance lease has expired" }
        val requested = now.elapsedMs + durationMs
        val until = if (state.maintenanceActive(now)) minOf(state.leaseUntilElapsedMs, requested) else requested
        return state.copy(leaseBoot = now.boot, leaseUntilElapsedMs = until)
    }

    private fun normalize(code: String) = code.replace("-", "").trim().uppercase()
    private fun matches(value: String, normalized: String): Boolean {
        if (!normalized.matches(Regex("[0-9A-F]{32}"))) return false
        val parts = value.split(':')
        if (parts.size != 2) return false
        val expected = parts[1].chunked(2).map { it.toIntOrNull(16)?.toByte() ?: return false }.toByteArray()
        return MessageDigest.isEqual(expected, digest(parts[0], normalized))
    }
    private fun digest(salt: String, code: String) =
        MessageDigest.getInstance("SHA-256").digest("$salt:$code".toByteArray(Charsets.UTF_8))
    private fun ByteArray.hex() = joinToString("") { "%02x".format(it) }
}
