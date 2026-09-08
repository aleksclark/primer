package com.aleksclark.primer.student.tasks

import com.aleksclark.primertasks.client.OccurrenceResponse

data class PendingTodayTask(
    val id: String,
    val title: String,
    val statusLabel: String,
)

data class StudentDashboardSnapshot(
    val paired: Boolean,
    val studentName: String,
    val pendingToday: List<PendingTodayTask>,
    val message: String? = null,
) {
    val greetingName: String
        get() = when {
            studentName.isNotBlank() -> studentName
            else -> "Student"
        }

    val pendingSummary: String
        get() = when {
            !paired -> "Pair this device to see today's tasks."
            pendingToday.isEmpty() -> "No pending tasks today."
            pendingToday.size == 1 -> "1 pending today"
            else -> "${pendingToday.size} pending today"
        }
}

internal val terminalOccurrenceStatuses = setOf("completed", "excused", "canceled")

internal fun pendingTodayOccurrences(today: List<OccurrenceResponse>): List<OccurrenceResponse> =
    today.filter { it.status !in terminalOccurrenceStatuses }

internal fun studentDashboardSnapshot(result: TasksRestoreResult): StudentDashboardSnapshot =
    when (result) {
        TasksRestoreResult.Superseded -> StudentDashboardSnapshot(
            paired = false,
            studentName = "",
            pendingToday = emptyList(),
        )
        is TasksRestoreResult.Unpaired -> StudentDashboardSnapshot(
            paired = result.retainedMetadata != null,
            studentName = result.retainedMetadata?.displayName.orEmpty(),
            pendingToday = emptyList(),
            message = result.message,
        )
        is TasksRestoreResult.Unavailable -> StudentDashboardSnapshot(
            paired = true,
            studentName = result.metadata.displayName,
            pendingToday = emptyList(),
            message = result.message,
        )
        is TasksRestoreResult.Paired -> StudentDashboardSnapshot(
            paired = true,
            studentName = result.metadata.displayName,
            pendingToday = pendingTodayOccurrences(result.today).map { occurrence ->
                PendingTodayTask(
                    id = occurrence.id,
                    title = occurrence.title,
                    statusLabel = presentOccurrence(occurrence).statusLabel,
                )
            },
            message = result.message,
        )
    }
