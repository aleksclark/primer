package com.aleksclark.primer.devicepolicy

import org.junit.Assert.*
import org.junit.Test

class RecoveryTest {
    private val now = RecoveryClock(1_000_000, 10_000, 7)
    private val code = "00112233-44556677-8899AABB-CCDDEEFF"
    private fun initial() = RecoveryState(verifiers = listOf(Recovery.verifier(code)))

    @Test fun `successful recovery consumes verifier and opens only bounded lease`() {
        val result = Recovery.attempt(initial(), code, now)
        assertTrue(result.accepted)
        assertTrue(result.state.verifiers.isEmpty())
        assertTrue(result.state.maintenanceActive(now))
        assertFalse(result.state.maintenanceActive(now.copy(elapsedMs = now.elapsedMs + Recovery.LEASE_MS)))
        assertFalse(Recovery.attempt(result.state, code, now).accepted)
    }
    @Test fun `reboot closes lease regardless of wall time`() {
        val state = Recovery.attempt(initial(), code, now).state
        assertFalse(state.maintenanceActive(now.copy(boot = 8, elapsedMs = 0)))
        assertTrue(state.maintenanceActive(now.copy(wallMs = 0)))
    }
    @Test fun `case and grouping are presentation only`() {
        assertTrue(Recovery.attempt(initial(), code.replace("-", "").lowercase(), now).accepted)
    }
    @Test fun `malformed code does not consume a verifier`() {
        val state = initial()
        val result = Recovery.attempt(state, "not a code", now)
        assertFalse(result.accepted)
        assertEquals(state.verifiers, result.state.verifiers)
        assertEquals(1, result.state.failures)
    }
    @Test fun `five failures block even a valid code until backoff elapses`() {
        var state = initial()
        repeat(5) { state = Recovery.attempt(state, "wrong", now).state }
        assertEquals(60_000L, state.waitMs(now))
        assertFalse(Recovery.attempt(state, code, now).accepted)
        val later = now.copy(wallMs = now.wallMs + 60_000, elapsedMs = now.elapsedMs + 60_000)
        assertTrue(Recovery.attempt(state, code, later).accepted)
    }
    @Test fun `rate-limit survives serialization-style reconstruction and reboot`() {
        var state = initial()
        repeat(5) { state = Recovery.attempt(state, "wrong", now).state }
        val reloaded = state.copy()
        assertFalse(Recovery.attempt(reloaded, code, now.copy(boot = 8, wallMs = Long.MAX_VALUE, elapsedMs = 0)).accepted)
        assertFalse(Recovery.attempt(reloaded, code, now.copy(boot = 9, wallMs = Long.MAX_VALUE, elapsedMs = 0)).accepted)
    }
    @Test fun `forward wall clock cannot bypass same-boot throttle`() {
        var state = initial()
        repeat(5) { state = Recovery.attempt(state, "wrong", now).state }
        assertFalse(Recovery.attempt(state, code, now.copy(wallMs = Long.MAX_VALUE)).accepted)
    }
    @Test fun `unknown boot identity fails closed`() {
        assertFalse(RecoveryState(leaseBoot = -1, leaseUntilElapsedMs = Long.MAX_VALUE).maintenanceActive(now.copy(boot = -1)))
        assertThrows(IllegalArgumentException::class.java) { Recovery.attempt(initial(), code, now.copy(boot = -1)) }
    }
    @Test fun `generated codes have 128-bit encoded length and unique salts`() {
        val codes = Recovery.generate(100)
        assertEquals(100, codes.toSet().size)
        assertTrue(codes.all { it.matches(Regex("[0-9A-F]{8}(-[0-9A-F]{8}){3}")) })
        assertNotEquals(Recovery.verifier(code), Recovery.verifier(code))
    }
    @Test fun `abandoning a prepared rotation preserves old recovery`() {
        val state = initial()
        val pending = Recovery.generate()
        assertFalse(Recovery.attempt(state, pending.first(), now).accepted)
        assertTrue(Recovery.attempt(state, code, now).accepted)
    }
    @Test fun `confirmed rotation invalidates old codes but preserves bounded lease`() {
        val state = RecoveryState(verifiers = listOf(Recovery.verifier(code)), leaseBoot = now.boot,
            leaseUntilElapsedMs = now.elapsedMs + 10_000)
        val codes = Recovery.generate()
        val rotated = Recovery.rotate(state, codes)
        assertFalse(Recovery.attempt(rotated, code, now).accepted)
        assertTrue(Recovery.attempt(rotated, codes.first(), now).accepted)
        assertEquals(state.leaseUntilElapsedMs, rotated.leaseUntilElapsedMs)
        assertTrue(rotated.maintenanceActive(now))
        assertFalse(rotated.maintenanceActive(now.copy(boot = 8)))
    }
    @Test fun `invalid or duplicate rotation batches cannot become recovery state`() {
        for (batch in listOf(emptyList(), List(6) { code }, List(6) { "bad$it" })) {
            assertThrows(IllegalArgumentException::class.java) { Recovery.rotate(initial(), batch) }
        }
    }
    @Test fun `only matching verifier is removed`() {
        val other = Recovery.generate().first()
        val state = RecoveryState(verifiers = listOf(Recovery.verifier(code), Recovery.verifier(other)))
        val result = Recovery.attempt(state, code, now)
        assertEquals(1, result.state.verifiers.size)
        assertTrue(Recovery.attempt(result.state, other, now).accepted)
    }
}
