package com.aleksclark.primer.student.tasks

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class TasksSessionIsolationTest {
    @Test
    fun revokedPairingDoesNotInventOwnerOrRecoveryClears() {
        val unpaired = TasksRestoreResult.Unpaired("This device pairing is no longer active. Scan a new QR code.")
        assertNull(unpaired.retainedToken)
        assertNull(unpaired.retainedMetadata)
        assertEquals("This device pairing is no longer active. Scan a new QR code.", unpaired.message)
    }

    @Test
    fun transportFailureKeepsCachedIdentityWithoutClaimingSuccess() {
        val metadata = StudentMetadata("student-1", "Maya", "https://tasks.example.test", "pair-1")
        val unpaired = TasksRestoreResult.Unpaired("Unable to reach the Primer server. Try again.", "token", metadata)
        assertEquals("token", unpaired.retainedToken)
        assertEquals("Maya", unpaired.retainedMetadata?.displayName)
    }
}
