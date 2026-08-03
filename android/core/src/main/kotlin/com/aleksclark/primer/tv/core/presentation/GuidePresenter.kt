package com.aleksclark.primer.tv.core.presentation

import com.aleksclark.primer.tv.core.domain.ChannelDay
import com.aleksclark.primer.tv.core.domain.Programme
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

/**
 * Pure reducer for the Guide destination. Labels past / current / future rows
 * from the server's stamped time and picks the row that should receive initial
 * scroll and focus.
 */
object GuidePresenter {

    const val ON_NOW = "On now"
    const val FINISHED = "Finished"
    const val EMPTY_MESSAGE = "Nothing scheduled today"

    private val clockFormatter: DateTimeFormatter = DateTimeFormatter.ofPattern("h:mm a")
    private val dayHeadingFormatter: DateTimeFormatter =
        DateTimeFormatter.ofLocalizedDate(FormatStyle.FULL)

    fun present(
        day: ChannelDay?,
        loading: Boolean = false,
        error: String? = null,
        zoneId: ZoneId = ZoneId.systemDefault(),
    ): GuideScreenModel {
        if (loading && day == null && error == null) {
            return GuideScreenModel(loading = true)
        }
        if (error != null && day == null) {
            return GuideScreenModel(error = UiText(error), loading = false)
        }
        if (day == null) {
            return GuideScreenModel(loading = loading)
        }
        // Prefer the household timezone stamped on the day so clock labels match
        // the family's wall clock rather than whatever zone the box happens to use.
        val resolvedZone = resolveZone(day.timezone, zoneId)
        return GuideScreenModel(
            guide = presentDay(day, resolvedZone),
            loading = false,
            refreshing = loading,
            error = error?.let(UiText::of),
        )
    }

    fun presentDay(day: ChannelDay, zoneId: ZoneId = ZoneId.systemDefault()): GuideUiModel {
        val rows = day.programmes.map { programme ->
            row(programme = programme, serverTime = day.serverTime, zoneId = zoneId)
        }
        val focus = focusIndex(day.programmes, day.serverTime)
        val focusId = rows.getOrNull(focus)?.scheduleEntryId
        return GuideUiModel(
            dayIso = day.day,
            dayHeading = dayHeading(day.day, zoneId),
            timezone = day.timezone,
            rows = rows,
            // Keep -1 for an empty day so callers do not pretend row 0 is focused.
            focusIndex = focus,
            focusScheduleEntryId = focusId,
        )
    }

    /** Resolves a server timezone id, falling back when missing or invalid. */
    fun resolveZone(timezone: String?, fallback: ZoneId = ZoneId.systemDefault()): ZoneId =
        timezone?.takeIf { it.isNotBlank() }
            ?.let { runCatching { ZoneId.of(it) }.getOrNull() }
            ?: fallback

    fun row(
        programme: Programme,
        serverTime: Instant,
        zoneId: ZoneId = ZoneId.systemDefault(),
    ): ProgrammeRowModel {
        val temporal = temporalState(programme, serverTime)
        val status = statusLabel(temporal)
        val classification = MediaLabels.classification(programme.mediaClass)
        val runtime = MediaLabels.runtime(programme.runtimeSeconds)
        val metadata = buildString {
            append(runtime)
            append(" · ")
            append(classification)
            if (status != null) {
                append(" · ")
                append(status)
            }
        }
        return ProgrammeRowModel(
            scheduleEntryId = programme.scheduleEntryId,
            mediaItemId = programme.mediaItemId,
            title = programme.title,
            timeLabel = clockTime(programme.airsAt, zoneId),
            runtimeSeconds = programme.runtimeSeconds,
            runtimeLabel = runtime,
            mediaClass = programme.mediaClass,
            classificationLabel = classification,
            temporal = temporal,
            statusLabel = status,
            metadataLabel = metadata,
            imagePath = programme.imagePath,
        )
    }

    /**
     * Half-open airing window matches [Programme.airingAt]: current if
     * airsAt ≤ t < endsAt; past if endsAt ≤ t; else future.
     */
    fun temporalState(programme: Programme, serverTime: Instant): ProgrammeTemporalState = when {
        programme.airingAt(serverTime) -> ProgrammeTemporalState.CURRENT
        !programme.endsAt.isAfter(serverTime) -> ProgrammeTemporalState.PAST
        else -> ProgrammeTemporalState.FUTURE
    }

    fun statusLabel(temporal: ProgrammeTemporalState): String? = when (temporal) {
        ProgrammeTemporalState.CURRENT -> ON_NOW
        ProgrammeTemporalState.PAST -> FINISHED
        ProgrammeTemporalState.FUTURE -> null
    }

    /**
     * Prefer the on-air row; if the channel is in a gap, the next future row;
     * otherwise the first row (or -1 when empty).
     */
    fun focusIndex(programmes: List<Programme>, serverTime: Instant): Int {
        if (programmes.isEmpty()) return -1
        val current = programmes.indexOfFirst { it.airingAt(serverTime) }
        if (current >= 0) return current
        val next = programmes.indexOfFirst { it.airsAt.isAfter(serverTime) || it.airsAt == serverTime }
        if (next >= 0) return next
        // Entire day is in the past — land on the last slot.
        return programmes.lastIndex
    }

    @Suppress("UNUSED_PARAMETER")
    fun dayHeading(dayIso: String, zoneId: ZoneId = ZoneId.systemDefault()): String {
        // zoneId is reserved for household-local day labels when the ISO day
        // boundary differs from the device zone.
        val parsed = runCatching { LocalDate.parse(dayIso) }.getOrNull()
            ?: return dayIso
        return parsed.format(dayHeadingFormatter)
    }

    fun clockTime(instant: Instant, zoneId: ZoneId = ZoneId.systemDefault()): String =
        clockFormatter.format(instant.atZone(zoneId))
}
