package com.aleksclark.primer.tv.app.player

data class SubtitleOption(
    val groupIndex: Int,
    val trackIndex: Int,
    val label: String,
    val selected: Boolean,
)

data class AudioOption(
    val groupIndex: Int,
    val trackIndex: Int,
    val label: String,
    val selected: Boolean,
)

data class SubtitleTrackDescriptor(
    val groupIndex: Int,
    val trackIndex: Int,
    val label: String?,
    val language: String?,
    val supported: Boolean,
    val selected: Boolean,
)

data class AudioTrackDescriptor(
    val groupIndex: Int,
    val trackIndex: Int,
    val label: String?,
    val language: String?,
    val supported: Boolean,
    val selected: Boolean,
)

fun subtitleOptions(tracks: List<SubtitleTrackDescriptor>): List<SubtitleOption> =
    labeledOptions(tracks.map {
        LabeledTrack(it.groupIndex, it.trackIndex, it.label, it.language, it.supported, it.selected)
    }, fallback = "Subtitles").map {
        SubtitleOption(it.groupIndex, it.trackIndex, it.label, it.selected)
    }

fun audioOptions(tracks: List<AudioTrackDescriptor>): List<AudioOption> =
    labeledOptions(tracks.map {
        LabeledTrack(it.groupIndex, it.trackIndex, it.label, it.language, it.supported, it.selected)
    }, fallback = "Audio").map {
        AudioOption(it.groupIndex, it.trackIndex, it.label, it.selected)
    }

fun volumePercent(current: Int, maximum: Int): Int = when {
    maximum <= 0 -> 0
    else -> ((current.coerceIn(0, maximum) * 100f) / maximum).toInt()
}

/** Media controls appear with the timed player overlay for on-demand and live. */
fun mediaControlsVisible(overlayVisible: Boolean): Boolean = overlayVisible

/**
 * Seconds until Home should re-fetch /now so the LIVE hero does not outlive the
 * airing. Targets one minute after the projected end to avoid end-boundary races.
 */
fun liveCardRefreshDelaySeconds(remainingSeconds: Int?, nextStartsInSeconds: Int?): Long? {
    val remaining = remainingSeconds?.takeIf { it > 0 }
    val nextStarts = nextStartsInSeconds?.takeIf { it > 0 }
    val base = when {
        remaining != null -> remaining
        nextStarts != null -> nextStarts
        else -> return null
    }
    return base.toLong() + 60L
}

private data class LabeledTrack(
    val groupIndex: Int,
    val trackIndex: Int,
    val label: String?,
    val language: String?,
    val supported: Boolean,
    val selected: Boolean,
)

private data class IndexedLabel(
    val groupIndex: Int,
    val trackIndex: Int,
    val label: String,
    val selected: Boolean,
)

private fun labeledOptions(
    tracks: List<LabeledTrack>,
    fallback: String,
): List<IndexedLabel> {
    val supported = tracks.filter { it.supported }
    val duplicateCounts = supported
        .groupingBy { it.label.clean() ?: it.language.clean() ?: fallback }
        .eachCount()
    val occurrences = mutableMapOf<String, Int>()
    return supported.map { track ->
        val base = track.label.clean() ?: track.language.clean() ?: fallback
        val occurrence = (occurrences[base] ?: 0) + 1
        occurrences[base] = occurrence
        IndexedLabel(
            groupIndex = track.groupIndex,
            trackIndex = track.trackIndex,
            label = if ((duplicateCounts[base] ?: 0) > 1) "$base $occurrence" else base,
            selected = track.selected,
        )
    }
}

private fun String?.clean(): String? = this?.trim()?.takeIf(String::isNotEmpty)
