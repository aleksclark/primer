package com.aleksclark.primer.student.tasks

import com.aleksclark.primertasks.client.OccurrenceResponse

/** Server-owned student work presentation. Never invents completion or extra actions. */
internal object StudentOccurrenceCopy {
    const val START = "Start task"
    const val SUBMIT = "Submit for parent approval"
    const val REFRESH = "Refresh from server"
    const val UNSUPPORTED =
        "This assigned work cannot be completed on this device. Ask a parent for a parent-approval task."
    const val WAITING_APPROVAL = "Waiting for a parent to approve, reject, or retry. Refresh to see the server state."
    const val REJECTED_RETRY = "A parent rejected this work. Start again if you should retry."
    const val APPROVED = "A parent approved this work."
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
        supported = supported,
        explanation = explanation,
    )
}

internal fun occurrenceStatusLabel(status: String, retried: Boolean): String = when (status) {
    "pending" -> if (retried) "Rejected — retry" else "Not started"
    "in_progress" -> "In progress"
    "awaiting_verification" -> "Waiting for parent approval"
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
