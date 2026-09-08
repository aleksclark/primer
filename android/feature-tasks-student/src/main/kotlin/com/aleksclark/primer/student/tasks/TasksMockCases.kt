package com.aleksclark.primer.student.tasks

import com.aleksclark.primertasks.client.ChecklistItem
import com.aleksclark.primertasks.client.OccurrenceResponse
import com.aleksclark.primertasks.client.StudentRequirement

internal object TasksMockCases {
    val checklistItems = listOf(
        ChecklistItem(description = "Read chapter 4 and mark two unfamiliar words.", id = "reading", status = "pending", title = "Read The Hobbit"),
    )

    val notStarted = occurrence(status = "pending")
    val inProgress = occurrence(id = "essay", title = "Revise history essay", status = "in_progress")
    val waitingForParent = occurrence(id = "reading", title = "Read The Hobbit", status = "awaiting_verification")
    val rejectedRetry = occurrence(id = "workbench", title = "Measure the workbench", status = "pending", attemptNumber = 1)
    val completed = occurrence(id = "science", title = "Record plant growth", status = "completed")
    val excused = occurrence(id = "map", title = "Label the river map", status = "excused")
    val canceled = occurrence(id = "vocabulary", title = "Review vocabulary", status = "canceled")
    val unsupported = occurrence(id = "cad", title = "Inspect the CAD model", status = "pending", capability = null)

    val today = listOf(notStarted, inProgress, waitingForParent, rejectedRetry, completed, excused, canceled, unsupported)
    val upcoming = listOf(occurrence(id = "geometry", title = "Sketch the garden plan", status = "pending"))

    private fun occurrence(
        id: String = "fractions",
        title: String = "Practice fractions",
        status: String,
        attemptNumber: Long = 0,
        capability: String? = "parent_approval",
    ) = OccurrenceResponse(
        attemptNumber = attemptNumber,
        dueAt = "2026-09-08T18:00:00Z",
        dueOffsetMinutes = 60,
        dueSemantics = "hard",
        id = id,
        instructions = "Complete problems 1 through 12 and show each reduction.",
        nominalAt = "2026-09-08T17:00:00Z",
        requirements = listOf(
            StudentRequirement(
                id = "parent-check",
                kind = "parent_approval",
                configVersion = 1,
                interaction = "parent_action",
                executor = "human",
            ),
        ),
        revisionId = "revision-1",
        scheduleId = "schedule-1",
        scheduleVersion = 1,
        status = status,
        studentCapability = capability,
        studentId = "ada",
        taskRevisionVersion = 1,
        timezone = "America/Chicago",
        title = title,
    )
}
