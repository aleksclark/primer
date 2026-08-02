package com.aleksclark.primer.tv.core.domain

import java.time.Duration
import java.time.Instant

/** One airing of the programmed channel. */
data class Programme(
    val scheduleEntryId: String,
    val mediaItemId: String,
    val title: String,
    val overview: String,
    val mediaClass: MediaClass,
    val subjectTags: List<String>,
    val runtimeSeconds: Int,
    val airsAt: Instant,
    val endsAt: Instant,
    val block: String,
    val joinInProgress: Boolean,
    val directPlayOk: Boolean,
    val imagePath: String,
) {
    /** How far into the programme the given instant falls, clamped to its span. */
    fun offsetSecondsAt(at: Instant): Int {
        if (at.isBefore(airsAt)) return 0
        val elapsed = Duration.between(airsAt, at).seconds
        return elapsed.coerceIn(0, runtimeSeconds.toLong()).toInt()
    }

    /** Whether the programme is on air at the given instant. The span is half-open. */
    fun airingAt(at: Instant): Boolean = !at.isBefore(airsAt) && at.isBefore(endsAt)
}

/**
 * The channel's state at one server-stamped instant.
 *
 * [serverTime] is the authority: box clocks drift, so every offset the player
 * uses is derived from what the server said, never from the local clock.
 */
data class ChannelNow(
    val onAir: Programme?,
    val offsetSeconds: Int,
    val startOffsetSeconds: Int,
    val next: Programme?,
    val nextStartsInSeconds: Int,
    val serverTime: Instant,
) {
    /** Whether the channel is between programmes. */
    val inGap: Boolean get() = onAir == null

    /** Seconds left in the current programme, or zero in a gap. */
    val remainingSeconds: Int
        get() = onAir?.let { (it.runtimeSeconds - offsetSeconds).coerceAtLeast(0) } ?: 0
}

/** One day of the programmed grid, as the EPG screen renders it. */
data class ChannelDay(
    val day: String,
    val timezone: String,
    val programmes: List<Programme>,
    val serverTime: Instant,
) {
    val isEmpty: Boolean get() = programmes.isEmpty()

    /** The programme on air at the server's own instant, if any. */
    fun onAir(): Programme? = programmes.firstOrNull { it.airingAt(serverTime) }
}
