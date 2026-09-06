package com.aleksclark.primer.devicepolicy

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class RecoveryJournalTest {
    private val now = RecoveryClock(1_000_000, 10_000, 7)
    private val initial = RecoverySnapshot(RecoveryState(verifiers = listOf(Recovery.verifier("00112233445566778899AABBCCDDEEFF"))))

    @Test
    fun replayKeepsSameAckAndDoesNotReapply() {
        val first = RecoveryJournal.consume(
            snapshot = initial,
            requestId = "r1",
            kind = "maintenance_lease",
            nextState = Recovery.openLease(initial.state, now, 1_000),
            event = "lease",
            nowWallMs = now.wallMs,
            newAckId = { "ack-1" },
        )
        assertTrue(first.firstApply)
        assertEquals("ack-1", first.ackReportId)
        val replay = RecoveryJournal.consume(
            snapshot = first.snapshot,
            requestId = "r1",
            kind = "maintenance_lease",
            nextState = Recovery.openLease(first.snapshot.state, now, 60_000),
            event = "lease-replay",
            nowWallMs = now.wallMs,
            newAckId = { "ack-2" },
        )
        assertFalse(replay.firstApply)
        assertEquals("ack-1", replay.ackReportId)
        assertEquals(first.snapshot.state.leaseUntilElapsedMs, replay.snapshot.state.leaseUntilElapsedMs)
        assertEquals(1, replay.snapshot.consumed.size)
    }

    @Test
    fun consumedIdsAreNotEvicted() {
        var snapshot = initial
        repeat(250) { index ->
            snapshot = RecoveryJournal.consume(
                snapshot = snapshot,
                requestId = "req-$index",
                kind = "rotate_recovery_code",
                nextState = snapshot.state,
                event = "rot-$index",
                nowWallMs = now.wallMs,
                newAckId = { "ack-$index" },
            ).snapshot
        }
        assertEquals(250, snapshot.consumed.size)
        assertEquals("ack-0", snapshot.consumed["req-0"]?.ackReportId)
        assertEquals("ack-249", snapshot.consumed["req-249"]?.ackReportId)
    }
}
