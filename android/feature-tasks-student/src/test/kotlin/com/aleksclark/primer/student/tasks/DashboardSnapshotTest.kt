package com.aleksclark.primer.student.tasks

import com.aleksclark.primertasks.client.OccurrenceResponse
import com.aleksclark.primertasks.client.StudentRequirement
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class DashboardSnapshotTest {
    @Test
    fun unpairedSnapshotAsksToPair() {
        val snapshot = studentDashboardSnapshot(TasksRestoreResult.Unpaired())
        assertFalse(snapshot.paired)
        assertEquals("Student", snapshot.greetingName)
        assertTrue(snapshot.pendingToday.isEmpty())
        assertEquals("Pair this device to see today's tasks.", snapshot.pendingSummary)
    }

    @Test
    fun pairedSnapshotKeepsOnlyUnfinishedTodayWork() {
        val snapshot = studentDashboardSnapshot(
            TasksRestoreResult.Paired(
                token = "token",
                metadata = StudentMetadata("student-1", "Maya", "https://tasks.example.test", "pair-1"),
                checklist = emptyList(),
                today = listOf(
                    occurrence(id = "a", title = "Fractions", status = "pending"),
                    occurrence(id = "b", title = "Essay", status = "in_progress"),
                    occurrence(id = "c", title = "Done", status = "completed"),
                    occurrence(id = "d", title = "Skipped", status = "excused"),
                    occurrence(id = "e", title = "Dropped", status = "canceled"),
                    occurrence(id = "f", title = "Waiting", status = "awaiting_verification"),
                ),
                upcoming = emptyList(),
            ),
        )
        assertTrue(snapshot.paired)
        assertEquals("Maya", snapshot.greetingName)
        assertEquals(listOf("a", "b", "f"), snapshot.pendingToday.map { it.id })
        assertEquals("3 pending today", snapshot.pendingSummary)
        assertEquals("Not started", snapshot.pendingToday[0].statusLabel)
        assertEquals("In progress", snapshot.pendingToday[1].statusLabel)
        assertEquals("Waiting for parent approval", snapshot.pendingToday[2].statusLabel)
    }

    @Test
    fun emptyTodayIsNoPendingWork() {
        val snapshot = studentDashboardSnapshot(
            TasksRestoreResult.Paired(
                token = "token",
                metadata = StudentMetadata("student-1", "Maya", "https://tasks.example.test", "pair-1"),
                checklist = emptyList(),
                today = emptyList(),
                upcoming = emptyList(),
            ),
        )
        assertEquals("No pending tasks today.", snapshot.pendingSummary)
    }

    @Test
    fun unavailableKeepsPairedNameWithoutInventingWork() {
        val snapshot = studentDashboardSnapshot(
            TasksRestoreResult.Unavailable(
                token = "token",
                metadata = StudentMetadata("student-1", "Maya", "https://tasks.example.test", "pair-1"),
                message = "Unable to load the checklist. Try again.",
            ),
        )
        assertTrue(snapshot.paired)
        assertEquals("Maya", snapshot.greetingName)
        assertTrue(snapshot.pendingToday.isEmpty())
        assertEquals("Unable to load the checklist. Try again.", snapshot.message)
    }

    private fun occurrence(
        id: String,
        title: String,
        status: String,
    ) = OccurrenceResponse(
        attemptNumber = 0,
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
        studentCapability = "parent_approval",
        studentId = "student-1",
        taskRevisionVersion = 1,
        timezone = "UTC",
        title = title,
    )
}
