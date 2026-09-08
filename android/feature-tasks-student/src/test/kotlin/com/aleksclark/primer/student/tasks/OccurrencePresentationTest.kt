package com.aleksclark.primer.student.tasks

import com.aleksclark.primertasks.client.OccurrenceResponse
import com.aleksclark.primertasks.client.StudentRequirement
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class OccurrencePresentationTest {
    @Test
    fun parentApprovalPendingCanStartButNotSubmit() {
        val presented = presentOccurrence(occurrence(status = "pending", capability = "parent_approval"))
        assertTrue(presented.supported)
        assertTrue(presented.canStart)
        assertFalse(presented.canSubmit)
        assertTrue(presented.studentActionRequired)
        assertEquals("Ready to start", presented.statusLabel)
        assertNull(presented.explanation)
    }

    @Test
    fun parentApprovalInProgressCanSubmit() {
        val presented = presentOccurrence(occurrence(status = "in_progress", capability = "parent_approval"))
        assertTrue(presented.canSubmit)
        assertFalse(presented.canStart)
        assertTrue(presented.studentActionRequired)
        assertEquals("In progress", presented.statusLabel)
    }

    @Test
    fun waitingShowsServerOwnedApprovalState() {
        val presented = presentOccurrence(
            occurrence(status = "awaiting_verification", capability = "parent_approval", attemptNumber = 1),
        )
        assertFalse(presented.canStart)
        assertFalse(presented.canSubmit)
        assertFalse(presented.studentActionRequired)
        assertEquals("Sent to your parent", presented.statusLabel)
        assertEquals(StudentOccurrenceCopy.WAITING_APPROVAL, presented.explanation)
    }

    @Test
    fun rejectedPendingShowsRetryWithoutInventingCompletion() {
        val presented = presentOccurrence(
            occurrence(status = "pending", capability = "parent_approval", attemptNumber = 1),
        )
        assertTrue(presented.canStart)
        assertEquals("Needs another try", presented.statusLabel)
        assertEquals(StudentOccurrenceCopy.REJECTED_RETRY, presented.explanation)
        assertEquals(OccurrenceStatusTone.Attention, presented.tone)
    }

    @Test
    fun approvedHidesMutations() {
        val presented = presentOccurrence(
            occurrence(status = "completed", capability = "parent_approval", attemptNumber = 1),
        )
        assertFalse(presented.canStart)
        assertFalse(presented.canSubmit)
        assertEquals("Approved", presented.statusLabel)
        assertEquals(StudentOccurrenceCopy.APPROVED, presented.explanation)
        assertEquals(OccurrenceStatusTone.Filled, presented.tone)
    }

    @Test
    fun unsupportedHidesStartAndSubmit() {
        val presented = presentOccurrence(occurrence(status = "pending", capability = "unsupported"))
        assertFalse(presented.supported)
        assertFalse(presented.canStart)
        assertFalse(presented.canSubmit)
        assertEquals(StudentOccurrenceCopy.UNSUPPORTED, presented.explanation)
    }

    @Test
    fun missingCapabilityIsUnsupported() {
        val presented = presentOccurrence(occurrence(status = "pending", capability = null))
        assertFalse(presented.supported)
        assertFalse(presented.canStart)
        assertEquals(StudentOccurrenceCopy.UNSUPPORTED, presented.explanation)
    }

    @Test
    fun unsupportedStatesDoNotInventParentApprovalOrRejection() {
        for (capability in listOf(null, "unsupported")) {
            for ((status, label) in mapOf("pending" to "Not started", "awaiting_verification" to "Awaiting verification", "completed" to "Completed")) {
                val presented = presentOccurrence(occurrence(status = status, capability = capability, attemptNumber = 2))
                assertEquals(label, presented.statusLabel)
                assertEquals(StudentOccurrenceCopy.UNSUPPORTED, presented.explanation)
                assertFalse(presented.canStart)
                assertFalse(presented.canSubmit)
            }
        }
    }

    @Test
    fun studentOrderingFinishesStartedWorkBeforeStartingAnotherTask() {
        val ready = occurrence(id = "ready", status = "pending")
        val sent = occurrence(id = "sent", status = "awaiting_verification")
        val retry = occurrence(id = "retry", status = "pending", attemptNumber = 1)
        val active = occurrence(id = "active", status = "in_progress")
        val completed = occurrence(id = "completed", status = "completed")

        assertEquals(
            listOf("active", "retry", "ready", "sent", "completed"),
            sortOccurrencesForStudent(listOf(ready, sent, completed, retry, active)).map { it.id },
        )
        assertTrue(terminalOccurrenceStatus("completed"))
        assertTrue(terminalOccurrenceStatus("excused"))
        assertTrue(terminalOccurrenceStatus("canceled"))
        assertFalse(terminalOccurrenceStatus("awaiting_verification"))
    }

    @Test
    fun replaceOccurrenceUpdatesMatchingRowOnly() {
        val first = occurrence(id = "occ-1", status = "pending")
        val second = occurrence(id = "occ-2", status = "pending")
        val updated = occurrence(id = "occ-1", status = "completed", capability = "parent_approval", attemptNumber = 1)
        val replaced = replaceOccurrence(listOf(first, second), updated)
        assertEquals("completed", replaced[0].status)
        assertEquals("pending", replaced[1].status)
    }

    private fun occurrence(
        id: String = "occ-1",
        status: String,
        capability: String? = "parent_approval",
        attemptNumber: Long = 0,
    ) = OccurrenceResponse(
        attemptNumber = attemptNumber,
        dueAt = "2026-01-01T00:00:00Z",
        dueOffsetMinutes = 0,
        dueSemantics = "hard",
        id = id,
        instructions = "Explain",
        nominalAt = "2026-01-01T00:00:00Z",
        requirements = listOf(
            StudentRequirement(
                id = "req-1",
                kind = "parent_approval",
                configVersion = 1,
                interaction = "parent_action",
                executor = "human",
            ),
        ),
        revisionId = "r",
        scheduleId = "s",
        scheduleVersion = 1,
        status = status,
        studentCapability = capability,
        studentId = "student-1",
        taskRevisionVersion = 1,
        timezone = "UTC",
        title = "Fractions",
    )
}
