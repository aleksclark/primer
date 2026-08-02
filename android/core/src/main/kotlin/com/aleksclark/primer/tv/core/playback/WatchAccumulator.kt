package com.aleksclark.primer.tv.core.playback

/**
 * Tracks how many seconds were actually watched, as distinct from the furthest
 * position reached.
 *
 * The two differ whenever the student seeks: jumping forward advances the
 * position without watching anything, and rewinding to re-watch a passage adds
 * watch time without advancing the position. Since educational sessions become
 * Primer's instructional-hours record (phase 5), the count has to reflect time
 * spent rather than the playhead.
 *
 * Time is fed in from a monotonic clock, so pausing the player or backgrounding
 * the app simply stops contributing.
 */
class WatchAccumulator {
    private var watchedMillis: Long = 0
    private var lastSampleMillis: Long? = null
    private var maxPositionMillis: Long = 0

    /** Seconds actually watched, rounded down. */
    val watchedSeconds: Int get() = (watchedMillis / 1000).toInt()

    /** Furthest playhead position reached, in whole seconds. */
    val maxPositionSeconds: Int get() = (maxPositionMillis / 1000).toInt()

    /**
     * Records a playback sample. [elapsedRealtimeMillis] must come from a
     * monotonic source; a sample taken while paused only updates the position.
     *
     * Gaps larger than [MAX_SAMPLE_GAP_MILLIS] are discarded rather than
     * credited: a process frozen in the background for an hour did not watch an
     * hour of video.
     */
    fun sample(positionMillis: Long, isPlaying: Boolean, elapsedRealtimeMillis: Long) {
        if (positionMillis > maxPositionMillis) {
            maxPositionMillis = positionMillis
        }
        val previous = lastSampleMillis
        lastSampleMillis = elapsedRealtimeMillis
        if (!isPlaying || previous == null) return

        val delta = elapsedRealtimeMillis - previous
        if (delta in 1..MAX_SAMPLE_GAP_MILLIS) {
            watchedMillis += delta
        }
    }

    /** Stops crediting time until the next playing sample re-establishes a baseline. */
    fun pause() {
        lastSampleMillis = null
    }

    /**
     * Seeds the counters from a session the server already knows about, so
     * resuming after the app was killed does not restart the tally at zero and
     * under-report the session.
     */
    fun restore(watchedSeconds: Int, maxPositionSeconds: Int) {
        watchedMillis = maxOf(watchedMillis, watchedSeconds.coerceAtLeast(0) * 1000L)
        maxPositionMillis = maxOf(maxPositionMillis, maxPositionSeconds.coerceAtLeast(0) * 1000L)
        lastSampleMillis = null
    }

    companion object {
        /**
         * Longest gap between samples still credited as watch time, generous
         * enough to cover a slow heartbeat tick but far short of a backgrounded
         * process.
         */
        const val MAX_SAMPLE_GAP_MILLIS = 60_000L
    }
}
