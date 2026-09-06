package com.aleksclark.primer.control.tasks

import com.aleksclark.primertasks.client.Schedule
import com.aleksclark.primertasks.client.ScheduleInput
import java.time.DateTimeException
import java.time.Instant
import java.time.LocalDate
import java.time.LocalDateTime
import java.time.LocalTime
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.DateTimeParseException

data class ScheduleDraft(
    val kind: String = "one_off",
    val date: String = LocalDate.now().toString(),
    val time: String = "09:00",
    val timezone: String = ZoneId.systemDefault().id,
    val rrule: String = "",
    val dueOffsetText: String = "0",
    val endAt: String = "",
    val originalStartAt: String? = null,
)

fun scheduleDraftFrom(schedule: Schedule): ScheduleDraft {
    val zone = runCatching { ZoneId.of(schedule.timezone) }.getOrNull()
    val instant = runCatching { Instant.parse(schedule.startAt) }.getOrNull()
    val zoned = if (zone != null && instant != null) instant.atZone(zone) else null
    val kind = when {
        schedule.kind == "one_off" -> "one_off"
        schedule.rrule?.contains("FREQ=DAILY") == true -> "daily"
        schedule.rrule?.contains("FREQ=WEEKLY") == true -> "weekly"
        else -> schedule.kind
    }
    return ScheduleDraft(
        kind = kind,
        date = zoned?.toLocalDate()?.toString() ?: "",
        time = zoned?.toLocalTime()?.let(::formatLocalTime) ?: "",
        timezone = schedule.timezone,
        rrule = if (kind == "one_off") "" else schedule.rrule.orEmpty(),
        dueOffsetText = schedule.dueOffsetMinutes.toString(),
        endAt = schedule.endAt.orEmpty(),
        originalStartAt = schedule.startAt,
    )
}

fun ScheduleDraft.toInput(studentId: String, templateId: String, revisionId: String): ScheduleInput {
    val due = dueOffsetText.trim().toLongOrNull()
        ?: throw IllegalArgumentException("Due offset must be a whole number of minutes.")
    require(due >= 0) { "Due offset cannot be negative." }
    val start = resolveStart()
    val oneOff = kind == "one_off"
    val rule = if (oneOff) null else rrule.trim().ifEmpty { defaultRule(kind) }
    if (!oneOff && rule.isNullOrBlank()) {
        throw IllegalArgumentException("Choose a recurrence rule, or set kind to one_off.")
    }
    val end = endAt.trim().ifEmpty { null }
    if (end != null) {
        runCatching { Instant.parse(end) }.getOrElse {
            throw IllegalArgumentException("End at must be an RFC3339 timestamp.")
        }
    }
    return ScheduleInput(
        studentId = studentId,
        templateId = templateId,
        revisionId = revisionId,
        kind = if (oneOff) "one_off" else "recurrence",
        timezone = timezone,
        startAt = start,
        rrule = rule,
        dueOffsetMinutes = due,
        endAt = end,
    )
}

private fun ScheduleDraft.resolveStart(): String {
    val zone = try {
        ZoneId.of(timezone)
    } catch (_: DateTimeException) {
        throw IllegalArgumentException("Choose a valid time zone, such as America/Chicago.")
    }
    val original = originalStartAt?.let { runCatching { Instant.parse(it) }.getOrNull() }
    if (original != null && matchesOriginal(original, zone)) {
        return original.toString()
    }
    val localDate = try {
        LocalDate.parse(date)
    } catch (_: DateTimeParseException) {
        throw IllegalArgumentException("Start date must be YYYY-MM-DD.")
    }
    val localTime = parseLocalTime(time)
    val local = LocalDateTime.of(localDate, localTime)
    val offsets = zone.rules.getValidOffsets(local)
    when {
        offsets.isEmpty() -> throw IllegalArgumentException("That local time does not exist in $timezone (DST gap).")
        offsets.size > 1 -> throw IllegalArgumentException("That local time is ambiguous in $timezone (DST overlap). Choose another minute or keep the original start.")
        else -> return local.atZone(zone).toInstant().toString()
    }
}

private fun ScheduleDraft.matchesOriginal(original: Instant, zone: ZoneId): Boolean {
    val zoned = original.atZone(zone)
    return date == zoned.toLocalDate().toString() &&
        time == formatLocalTime(zoned.toLocalTime()) &&
        timezone == zone.id
}

private fun formatLocalTime(time: LocalTime): String = when {
    time.nano != 0 -> time.format(DateTimeFormatter.ofPattern("HH:mm:ss.SSS"))
    time.second != 0 -> time.format(DateTimeFormatter.ofPattern("HH:mm:ss"))
    else -> time.format(DateTimeFormatter.ofPattern("HH:mm"))
}

private fun parseLocalTime(raw: String): LocalTime {
    val value = raw.trim()
    val formats = listOf("HH:mm:ss.SSS", "HH:mm:ss", "HH:mm")
    formats.forEach { pattern ->
        runCatching { return LocalTime.parse(value, DateTimeFormatter.ofPattern(pattern)) }
    }
    throw IllegalArgumentException("Time must be HH:MM, HH:MM:SS, or HH:MM:SS.mmm.")
}

private fun defaultRule(kind: String) = when (kind) {
    "daily" -> "FREQ=DAILY;COUNT=7"
    "weekly" -> "FREQ=WEEKLY;COUNT=7"
    else -> null
}
