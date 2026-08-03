package com.aleksclark.primer.tv.core.presentation

import com.aleksclark.primer.tv.core.domain.MediaClass

/** Where a programme sits relative to the server's stamped instant. */
enum class ProgrammeTemporalState {
    PAST,
    CURRENT,
    FUTURE,
}

/** One guide row: time axis + title + classification + temporal emphasis. */
data class ProgrammeRowModel(
    val scheduleEntryId: String,
    val mediaItemId: String,
    val title: String,
    val timeLabel: String,
    val runtimeSeconds: Int,
    val runtimeLabel: String,
    val mediaClass: MediaClass,
    val classificationLabel: String,
    val temporal: ProgrammeTemporalState,
    /** "On now", "Finished", or null for upcoming slots. */
    val statusLabel: String?,
    val metadataLabel: String,
    val imagePath: String,
) {
    val isCurrent: Boolean get() = temporal == ProgrammeTemporalState.CURRENT
    val isPast: Boolean get() = temporal == ProgrammeTemporalState.PAST
    val isFuture: Boolean get() = temporal == ProgrammeTemporalState.FUTURE
}

/**
 * Day schedule ready for [ProgrammeGuide]. [focusIndex] is the row that should
 * receive initial scroll/focus: current programme, else the next future one,
 * else the last past row. Empty days use -1 (no focus target).
 */
data class GuideUiModel(
    val dayIso: String,
    val dayHeading: String,
    val timezone: String,
    val rows: List<ProgrammeRowModel>,
    val focusIndex: Int,
    val focusScheduleEntryId: String?,
) {
    val isEmpty: Boolean get() = rows.isEmpty()
}

/** Guide destination load envelope. */
data class GuideScreenModel(
    val guide: GuideUiModel? = null,
    val loading: Boolean = false,
    val refreshing: Boolean = false,
    val error: UiText? = null,
) {
    val showSkeletons: Boolean
        get() = loading && guide == null && error == null

    val isEmpty: Boolean
        get() = !loading && error == null && (guide == null || guide.isEmpty)

    val focusIndex: Int
        get() = guide?.focusIndex ?: 0
}
