package com.aleksclark.primer.student.tasks

import com.aleksclark.primertasks.client.OccurrenceResponse

/** Server-owned student work presentation. Never invents completion or extra actions. */
internal object StudentOccurrenceCopy {
    const val START = "Start task"
    const val RETRY = "Try this task again"
    const val SUBMIT = "Send to my parent"
    const val REFRESH = "Check for updates"
    const val UNSUPPORTED =
        "Ask your parent for help opening this task on a supported device."
    const val WAITING_APPROVAL = "Your work is saved. You can return to today’s tasks while your parent checks it."
    const val REJECTED_RETRY = "Try the task again with care. Your previous work is still saved."
    const val APPROVED = "Nice work. Your parent approved this task."
}

internal enum class OccurrenceStatusTone {
    Neutral,
    Accent,
    Attention,
    Filled,
}

internal data class OccurrencePresentation(
    val statusLabel: String,
    val tone: OccurrenceStatusTone,
    val canStart: Boolean,
    val canSubmit: Boolean,
    val studentActionRequired: Boolean,
    val supported: Boolean,
    val explanation: String?,
)

internal fun studentWorkSupported(capability: String?): Boolean = capability == "parent_approval"

internal fun presentOccurrence(occurrence: OccurrenceResponse): OccurrencePresentation {
    val supported = studentWorkSupported(occurrence.studentCapability)
    val retried = supported && occurrence.attemptNumber > 0
    val statusLabel = if (supported) occurrenceStatusLabel(occurrence.status, retried) else when (occurrence.status) {
        "pending" -> "Not started"
        "in_progress" -> "In progress"
        "awaiting_verification" -> "Awaiting verification"
        "completed" -> "Completed"
        "excused" -> "Excused"
        "canceled" -> "Canceled"
        else -> occurrence.status
    }
    val explanation = when {
        !supported -> StudentOccurrenceCopy.UNSUPPORTED
        occurrence.status == "awaiting_verification" -> StudentOccurrenceCopy.WAITING_APPROVAL
        occurrence.status == "pending" && retried -> StudentOccurrenceCopy.REJECTED_RETRY
        occurrence.status == "completed" -> StudentOccurrenceCopy.APPROVED
        else -> null
    }
    return OccurrencePresentation(
        statusLabel = statusLabel,
        tone = occurrenceStatusTone(occurrence.status, retried),
        canStart = supported && occurrence.status == "pending",
        canSubmit = supported && occurrence.status == "in_progress",
        studentActionRequired = supported && (occurrence.status == "pending" || occurrence.status == "in_progress"),
        supported = supported,
        explanation = explanation,
    )
}

internal fun occurrenceStatusLabel(status: String, retried: Boolean): String = when (status) {
    "pending" -> if (retried) "Needs another try" else "Ready to start"
    "in_progress" -> "In progress"
    "awaiting_verification" -> "Sent to your parent"
    "completed" -> "Approved"
    "excused" -> "Excused"
    "canceled" -> "Canceled"
    else -> status
}

internal fun occurrenceStatusTone(status: String, retried: Boolean): OccurrenceStatusTone = when (status) {
    "completed" -> OccurrenceStatusTone.Filled
    "awaiting_verification", "in_progress" -> OccurrenceStatusTone.Accent
    "pending" -> if (retried) OccurrenceStatusTone.Attention else OccurrenceStatusTone.Neutral
    else -> OccurrenceStatusTone.Attention
}

internal fun replaceOccurrence(
    items: List<OccurrenceResponse>,
    occurrence: OccurrenceResponse,
): List<OccurrenceResponse> = items.map { if (it.id == occurrence.id) occurrence else it }
