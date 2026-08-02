package com.aleksclark.primer.tv.core.domain

/**
 * How the item being played was authorized, which is the first thing that
 * decides what the player allows.
 */
enum class PlaybackMode(val wire: String) {
    /** Played from the rotation catalog, at the student's own pace. */
    ON_DEMAND("on_demand"),

    /** Joined from the programmed channel, at the server's broadcast position. */
    PROGRAMMED("programmed");

    companion object {
        /**
         * An unrecognised mode is read as programmed, the more restrictive of
         * the two: a client that cannot tell what it is playing must not hand
         * the student a scrub bar.
         */
        fun fromWire(value: String): PlaybackMode =
            entries.firstOrNull { it.wire == value } ?: PROGRAMMED
    }
}

/** What the player lets the student do. */
data class PlaybackControls(
    val pauseAllowed: Boolean,
    val seekAllowed: Boolean,
    /**
     * Whether the playhead is pinned to the server's broadcast position, so a
     * stall cannot leave the student watching behind the channel.
     */
    val followsBroadcast: Boolean = false,
) {
    /** Whether a scrubbable progress bar should be shown at all. */
    val showSeekBar: Boolean get() = seekAllowed

    /** Whether any transport control is worth drawing. */
    val showTransportControls: Boolean get() = pauseAllowed || seekAllowed
}

/**
 * Derives the control policy from how an item is being played.
 *
 * On demand, educational and mixed items are study material: the student is
 * expected to rewind and re-watch a passage, so seeking is allowed.
 * Entertainment is rationed by the watch-once ledger, and letting the student
 * scrub to the last minute would burn the play without watching it, so pausing
 * is allowed but seeking is not. An unrecognised class is treated as
 * entertainment.
 *
 * Programmed playback is a broadcast, so nothing is offered: no seek, no
 * fast-forward, no rewind, and no pause — a pause that resumed where it left
 * off would leave the student behind the channel, watching a scene the server
 * says has already aired. Back exits, and that is the whole of the transport.
 */
object PlaybackPolicy {
    /** The locked-down channel policy. */
    val PROGRAMMED_CONTROLS = PlaybackControls(
        pauseAllowed = false,
        seekAllowed = false,
        followsBroadcast = true,
    )

    fun onDemandControls(mediaClass: MediaClass): PlaybackControls = when (mediaClass) {
        MediaClass.EDUCATIONAL, MediaClass.MIXED -> PlaybackControls(pauseAllowed = true, seekAllowed = true)
        MediaClass.ENTERTAINMENT, MediaClass.UNKNOWN -> PlaybackControls(pauseAllowed = true, seekAllowed = false)
    }

    /** The policy for one playback attempt, given its mode and the item's class. */
    fun controlsFor(mode: PlaybackMode, mediaClass: MediaClass): PlaybackControls = when (mode) {
        PlaybackMode.PROGRAMMED -> PROGRAMMED_CONTROLS
        PlaybackMode.ON_DEMAND -> onDemandControls(mediaClass)
    }

    /**
     * How far the playhead may drift behind the broadcast position before it is
     * pulled back into step.
     *
     * Generous enough to absorb a rebuffer or a slow start on a LAN-attached
     * box; short enough that the student is never watching a different scene
     * from the one the channel is airing.
     */
    const val MAX_BROADCAST_DRIFT_SECONDS = 10

    /**
     * The position playback should be moved to, given the offset the server
     * last reported and where the local playhead has got to.
     *
     * Returns null when the playhead is close enough to leave alone. Only a
     * playhead *behind* the broadcast is corrected: one that has run ahead can
     * only be fixed by showing the student a scene twice, and the server's own
     * offset catches up on the next re-sync anyway.
     */
    fun broadcastCorrectionSeconds(
        serverOffsetSeconds: Int,
        playheadSeconds: Int,
        maxDriftSeconds: Int = MAX_BROADCAST_DRIFT_SECONDS,
    ): Int? {
        val drift = serverOffsetSeconds - playheadSeconds
        return if (drift > maxDriftSeconds) serverOffsetSeconds else null
    }
}

/**
 * Watch-once accounting mirrored from the server so the client can predict
 * whether finishing will burn the play.
 *
 * The server's threshold lives in `domain.CompletionThreshold`; it is duplicated
 * here only to warn the student *before* the play is charged. The server remains
 * the authority — [SessionProgress.playConsumed] is what actually counts.
 */
object WatchOnce {
    const val COMPLETION_THRESHOLD = 0.8

    /**
     * Whether a session has progressed far enough that the server will charge
     * the play. A runtime of zero means the duration is unknown, in which case
     * only an explicit completion counts.
     */
    fun completesPlay(completed: Boolean, maxPositionSeconds: Int, runtimeSeconds: Int): Boolean {
        if (completed) return true
        if (runtimeSeconds <= 0) return false
        return maxPositionSeconds >= COMPLETION_THRESHOLD * runtimeSeconds
    }

    /**
     * Whether to warn that watching on will use up the item's single play.
     *
     * Only entertainment is rationed, and only on demand: a programmed grant is
     * authorized by the grid and never touches an availability window, so the
     * channel cannot burn a play.
     */
    fun shouldWarnBeforePlay(mediaClass: MediaClass, mode: PlaybackMode = PlaybackMode.ON_DEMAND): Boolean =
        mode == PlaybackMode.ON_DEMAND && mediaClass.consumesPlay
}
