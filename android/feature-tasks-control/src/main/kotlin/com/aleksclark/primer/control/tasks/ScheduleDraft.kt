package com.aleksclark.primer.control.tasks

import com.aleksclark.primertasks.client.ScheduleInput
import java.time.DateTimeException
import java.time.LocalDate
import java.time.LocalTime
import java.time.ZoneId
import java.time.ZonedDateTime
import java.time.format.DateTimeFormatter

data class ScheduleDraft(
    val kind: String = "one_off",
    val date: String = LocalDate.now().toString(),
    val time: String = "09:00",
    val timezone: String = ZoneId.systemDefault().id,
    val rrule: String = "",
    val dueOffsetMinutes: Long = 0,
    val endAt: String = "",
)

fun scheduleDraftFrom(schedule: com.aleksclark.primertasks.client.Schedule): ScheduleDraft {
    val start = runCatching { java.time.Instant.parse(schedule.startAt).atZone(ZoneId.of(schedule.timezone)) }.getOrNull()
    val kind = when {
        schedule.kind == "one_off" -> "one_off"
        schedule.rrule?.contains("FREQ=DAILY") == true -> "daily"
        schedule.rrule?.contains("FREQ=WEEKLY") == true -> "weekly"
        else -> schedule.kind
    }
    return ScheduleDraft(
        kind = kind,
        date = start?.toLocalDate()?.toString() ?: LocalDate.now().toString(),
        time = start?.toLocalTime()?.format(DateTimeFormatter.ofPattern("HH:mm")) ?: "09:00",
        timezone = schedule.timezone,
        rrule = schedule.rrule.orEmpty(),
        dueOffsetMinutes = schedule.dueOffsetMinutes,
        endAt = schedule.endAt.orEmpty(),
    )
}

fun ScheduleDraft.toInput(studentId: String, templateId: String, revisionId: String): ScheduleInput {
    val start = zonedStart("$date $time", timezone)
    val rule = rrule.trim().ifEmpty { defaultRule(kind) }
    val recurring = kind != "one_off" || !rule.isNullOrBlank()
    return ScheduleInput(
        studentId = studentId,
        templateId = templateId,
        revisionId = revisionId,
        kind = if (recurring) "recurrence" else "one_off",
        timezone = timezone,
        startAt = start,
        rrule = if (recurring) rule else null,
        dueOffsetMinutes = dueOffsetMinutes,
        endAt = endAt.trim().ifEmpty { null },
    )
}

private fun defaultRule(kind: String) = when (kind) {
    "daily" -> "FREQ=DAILY;COUNT=7"
    "weekly" -> "FREQ=WEEKLY;COUNT=7"
    else -> null
}

private fun zonedStart(local: String, timezone: String): String {
    val parts = local.split(" ")
    require(parts.size == 2) { "Choose a start date and time." }
    val zone = try {
        ZoneId.of(timezone)
    } catch (_: DateTimeException) {
        throw IllegalArgumentException("Choose a valid time zone, such as America/Chicago.")
    }
    val date = LocalDate.parse(parts[0])
    val time = LocalTime.parse(parts[1], DateTimeFormatter.ofPattern("HH:mm"))
    return ZonedDateTime.of(date, time, zone).toInstant().toString()
}
