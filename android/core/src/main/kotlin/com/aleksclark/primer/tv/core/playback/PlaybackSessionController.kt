package com.aleksclark.primer.tv.core.playback

import com.aleksclark.primer.tv.core.data.ApiError
import com.aleksclark.primer.tv.core.data.ApiResult
import com.aleksclark.primer.tv.core.data.GrantStore
import com.aleksclark.primer.tv.core.data.StoredGrant
import com.aleksclark.primer.tv.core.data.TvRepository
import com.aleksclark.primer.tv.core.domain.MediaClass
import com.aleksclark.primer.tv.core.domain.PlayGrant
import com.aleksclark.primer.tv.core.domain.PlaybackControls
import com.aleksclark.primer.tv.core.domain.PlaybackMode
import com.aleksclark.primer.tv.core.domain.PlaybackPolicy
import com.aleksclark.primer.tv.core.domain.SessionProgress
import com.aleksclark.primer.tv.core.domain.WatchOnce
import java.time.Instant
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch

/** How often progress is reported, per the plan's ~30s heartbeat cadence. */
const val HEARTBEAT_INTERVAL_MILLIS = 30_000L

/**
 * A position the player must jump to so it stays level with the broadcast.
 *
 * [token] increments on every correction so an identical position issued twice
 * still reaches the player: a state flow would otherwise swallow the repeat.
 */
data class BroadcastSeek(val positionSeconds: Int, val token: Long)

/** What re-syncing against the channel established. */
sealed interface BroadcastSync {
    /** The playhead is level enough with the broadcast to leave alone. */
    data object InSync : BroadcastSync

    /** The playhead lagged and has been pulled to [positionSeconds]. */
    data class Corrected(val positionSeconds: Int) : BroadcastSync

    /**
     * The channel has moved on to something else (or into a gap) while the app
     * was away. The caller must close this session and re-tune; the grant is
     * for a programme that is no longer airing.
     */
    data object ProgrammeChanged : BroadcastSync

    /** The channel could not be reached, so playback carries on unchanged. */
    data class Unavailable(val error: ApiError) : BroadcastSync

    /** The session is not a programmed one, so there is nothing to sync to. */
    data object NotProgrammed : BroadcastSync
}

/** Where a playback attempt currently stands. */
sealed interface PlaybackState {
    /** No grant requested yet. */
    data object Idle : PlaybackState

    /** Waiting on `POST /media/{id}/grant`. */
    data object RequestingGrant : PlaybackState

    /** Authorized; the player may load [grant]'s stream URL. */
    data class Playable(
        val grant: PlayGrant,
        val controls: PlaybackControls,
        val resumePositionSeconds: Int,
        val playConsumed: Boolean = false,
    ) : PlaybackState

    /** The session closed normally. [playConsumed] is the server's verdict. */
    data class Finished(val playConsumed: Boolean, val completed: Boolean) : PlaybackState

    /** Playback cannot proceed. [recoverable] false means a new grant will not help. */
    data class Failed(val error: ApiError, val recoverable: Boolean) : PlaybackState
}

/**
 * Owns one playback attempt end to end: acquiring the grant, keeping heartbeats
 * flowing, re-syncing programmed playback against the broadcast, and closing the
 * session out.
 *
 * The controller deliberately knows nothing about ExoPlayer. It pulls the
 * playhead through [PlayerProbe], which lets the whole grant/heartbeat lifecycle
 * be tested on the JVM with virtual time and no device.
 */
class PlaybackSessionController(
    private val repository: TvRepository,
    private val grantStore: GrantStore,
    private val scope: CoroutineScope,
    private val clock: () -> Instant = Instant::now,
    private val elapsedRealtime: () -> Long = { System.nanoTime() / 1_000_000 },
    private val heartbeatIntervalMillis: Long = HEARTBEAT_INTERVAL_MILLIS,
) {
    /** How the controller reads the live playhead. */
    fun interface PlayerProbe {
        fun sample(): Sample

        data class Sample(val positionMillis: Long, val isPlaying: Boolean)
    }

    private val _state = MutableStateFlow<PlaybackState>(PlaybackState.Idle)
    val state: StateFlow<PlaybackState> = _state.asStateFlow()

    private val _lastProgress = MutableStateFlow<SessionProgress?>(null)
    val lastProgress: StateFlow<SessionProgress?> = _lastProgress.asStateFlow()

    private val accumulator = WatchAccumulator()
    private var heartbeatJob: Job? = null
    private var probe: PlayerProbe? = null
    private var activeGrant: PlayGrant? = null
    private var redeemed = false
    private var mediaClass: MediaClass = MediaClass.UNKNOWN
    private var runtimeSeconds: Int = 0
    private var mode: PlaybackMode = PlaybackMode.ON_DEMAND
    private var scheduleEntryId: String? = null
    private var seekToken: Long = 0

    private val _broadcastSeek = MutableStateFlow<BroadcastSeek?>(null)

    /**
     * Corrections the programmed player must apply to stay level with the
     * broadcast. Null until the first one is needed.
     */
    val broadcastSeek: StateFlow<BroadcastSeek?> = _broadcastSeek.asStateFlow()

    /**
     * Starts or resumes playback of an item.
     *
     * On demand, a persisted grant for the same item is reused when it is still
     * resumable, which is what keeps an app killed mid-film from spending a
     * second play. A stale or foreign stored grant is discarded first so it
     * cannot leak into an unrelated session.
     *
     * Programmed playback never resumes a stored grant: the channel has moved on
     * while the app was gone, so a stored offset would drop the student into a
     * scene that is no longer airing. A fresh grant is always requested, and the
     * server refuses it outright if the programme has ended.
     *
     * @param scheduleEntryId the airing being joined, for programmed playback.
     *   Re-syncing uses it to notice the channel moving on.
     */
    suspend fun start(
        mediaItemId: String,
        mediaClass: MediaClass,
        runtimeSeconds: Int,
        mode: PlaybackMode = PlaybackMode.ON_DEMAND,
        scheduleEntryId: String? = null,
    ) {
        this.mediaClass = mediaClass
        this.runtimeSeconds = runtimeSeconds
        this.mode = mode
        this.scheduleEntryId = scheduleEntryId
        val controls = PlaybackPolicy.controlsFor(mode, mediaClass)

        val stored = grantStore.load()
        val resumable = stored != null &&
            mode == PlaybackMode.ON_DEMAND &&
            stored.mode == PlaybackMode.ON_DEMAND.wire &&
            stored.mediaItemId == mediaItemId &&
            stored.resumableAt(clock())
        if (resumable) {
            adopt(stored!!, controls)
            return
        }
        if (stored != null) {
            grantStore.clear()
        }

        _state.value = PlaybackState.RequestingGrant
        when (val result = repository.grant(mediaItemId, mode)) {
            is ApiResult.Ok -> {
                val grant = result.value
                activeGrant = grant
                redeemed = false
                accumulator.restore(watchedSeconds = 0, maxPositionSeconds = grant.startOffsetSeconds)
                persist()
                _state.value = PlaybackState.Playable(
                    grant = grant,
                    controls = controls,
                    resumePositionSeconds = grant.startOffsetSeconds,
                )
            }
            is ApiResult.Err -> _state.value = PlaybackState.Failed(
                error = result.error,
                // A refusal or a missing token will not be fixed by asking again;
                // a network blip, 5xx, or temporary media-source outage will.
                recoverable = result.error is ApiError.Network ||
                    result.error is ApiError.Unexpected ||
                    result.error is ApiError.Unavailable,
            )
        }
    }

    /** Resumes a stored grant without spending a new one. */
    private fun adopt(stored: StoredGrant, controls: PlaybackControls) {
        val grant = stored.toGrant(serverTime = clock())
        activeGrant = grant
        redeemed = stored.redeemed
        accumulator.restore(
            watchedSeconds = stored.watchedSeconds,
            maxPositionSeconds = stored.positionSeconds,
        )
        _state.value = PlaybackState.Playable(
            grant = grant,
            controls = controls,
            resumePositionSeconds = maxOf(stored.positionSeconds, grant.startOffsetSeconds),
        )
    }

    /**
     * Begins heartbeating against [probe]. Safe to call again after a lifecycle
     * bounce: the previous loop is cancelled so only one runs at a time.
     */
    fun attachPlayer(probe: PlayerProbe) {
        this.probe = probe
        heartbeatJob?.cancel()
        heartbeatJob = scope.launch {
            while (isActive) {
                delay(heartbeatIntervalMillis)
                sampleAndBeat()
            }
        }
    }

    /**
     * Stops the heartbeat loop but keeps the session open, for the app going to
     * the background. The grant stays persisted so playback can be picked back
     * up; [finish] is what actually closes the session.
     */
    suspend fun detachPlayer() {
        heartbeatJob?.cancel()
        heartbeatJob = null
        sampleAndBeat()
        accumulator.pause()
        probe = null
        persist()
    }

    /**
     * Re-checks the broadcast position against the channel and pulls the
     * playhead back into step if it has fallen behind.
     *
     * This is what the programmed player calls on resume. The local clock is
     * never consulted: a box whose RTC is minutes out would otherwise compute a
     * confident and wrong offset. If the channel has moved on to another
     * programme the caller is told, because the grant it is holding no longer
     * authorises what is airing.
     */
    suspend fun resyncBroadcast(): BroadcastSync {
        if (mode != PlaybackMode.PROGRAMMED) return BroadcastSync.NotProgrammed

        val now = when (val result = repository.now()) {
            is ApiResult.Ok -> result.value
            // A channel that cannot be reached is not a reason to stop playing:
            // the stream is still running and the next resume tries again.
            is ApiResult.Err -> return BroadcastSync.Unavailable(result.error)
        }

        val onAir = now.onAir
        val expected = scheduleEntryId
        if (onAir == null || (expected != null && onAir.scheduleEntryId != expected)) {
            return BroadcastSync.ProgrammeChanged
        }

        val playheadSeconds = ((probe?.sample()?.positionMillis ?: 0L) / 1000).toInt()
        val correction = PlaybackPolicy.broadcastCorrectionSeconds(
            serverOffsetSeconds = now.offsetSeconds,
            playheadSeconds = playheadSeconds,
        ) ?: return BroadcastSync.InSync

        seekToken++
        _broadcastSeek.value = BroadcastSeek(positionSeconds = correction, token = seekToken)
        return BroadcastSync.Corrected(correction)
    }

    /** Takes one sample and reports it. Exposed so the UI can flush on pause. */
    suspend fun sampleAndBeat() {
        val grant = activeGrant ?: return
        val sample = probe?.sample() ?: return
        accumulator.sample(
            positionMillis = sample.positionMillis,
            isPlaying = sample.isPlaying,
            elapsedRealtimeMillis = elapsedRealtime(),
        )
        beat(grant)
    }

    /**
     * Sends one heartbeat.
     *
     * A network failure is swallowed on purpose: the film should keep playing
     * through a Wi-Fi hiccup, and the next tick re-sends cumulative counters, so
     * a dropped beat costs nothing. A refusal or a dead token does end playback,
     * because there is no session left to report against.
     */
    private suspend fun beat(grant: PlayGrant) {
        when (
            val result = repository.heartbeat(
                grantId = grant.grantId,
                positionSeconds = accumulator.maxPositionSeconds,
                watchedSeconds = accumulator.watchedSeconds,
            )
        ) {
            is ApiResult.Ok -> {
                redeemed = true
                _lastProgress.value = result.value
                persist()
                if (result.value.playConsumed) {
                    val current = _state.value
                    if (current is PlaybackState.Playable) {
                        _state.value = current.copy(playConsumed = true)
                    }
                }
            }
            is ApiResult.Err -> when (result.error) {
                is ApiError.Network, is ApiError.Unexpected -> Unit
                else -> {
                    heartbeatJob?.cancel()
                    heartbeatJob = null
                    grantStore.clear()
                    _state.value = PlaybackState.Failed(result.error, recoverable = false)
                }
            }
        }
    }

    /**
     * Closes the session out on finish or exit.
     *
     * [completed] should be true only when playback actually reached the end;
     * the server uses it together with the 80% threshold to decide whether an
     * entertainment play is charged.
     */
    suspend fun finish(completed: Boolean) {
        heartbeatJob?.cancel()
        heartbeatJob = null

        val grant = activeGrant
        if (grant == null) {
            _state.value = PlaybackState.Finished(playConsumed = false, completed = completed)
            return
        }

        probe?.sample()?.let { sample ->
            accumulator.sample(
                positionMillis = sample.positionMillis,
                isPlaying = sample.isPlaying,
                elapsedRealtimeMillis = elapsedRealtime(),
            )
        }

        val result = repository.complete(
            grantId = grant.grantId,
            positionSeconds = accumulator.maxPositionSeconds,
            watchedSeconds = accumulator.watchedSeconds,
            completed = completed,
        )

        probe = null
        activeGrant = null
        _broadcastSeek.value = null

        when (result) {
            is ApiResult.Ok -> {
                grantStore.clear()
                _lastProgress.value = result.value
                _state.value = PlaybackState.Finished(
                    playConsumed = result.value.playConsumed,
                    completed = result.value.completed,
                )
            }
            is ApiResult.Err -> {
                // The session could not be closed. A redeemed grant is kept so a
                // later launch can still report the tail of the session; an
                // unredeemed one is worthless and dropped.
                if (!redeemed) grantStore.clear() else persist()
                _state.value = PlaybackState.Finished(
                    playConsumed = WatchOnceEstimate.consumed(mode, mediaClass, completed, accumulator.maxPositionSeconds, runtimeSeconds),
                    completed = completed,
                )
            }
        }
    }

    /** Whether the server has charged this item's play during this session. */
    val playConsumed: Boolean
        get() = _lastProgress.value?.playConsumed == true

    private suspend fun persist() {
        val grant = activeGrant ?: return
        grantStore.save(
            StoredGrant.of(
                grant = grant,
                positionSeconds = accumulator.maxPositionSeconds,
                watchedSeconds = accumulator.watchedSeconds,
                redeemed = redeemed,
            ),
        )
    }
}

/**
 * Local prediction of the server's watch-once verdict, used only when the
 * completion call itself failed and there is no authoritative answer to show.
 */
internal object WatchOnceEstimate {
    fun consumed(
        mode: PlaybackMode,
        mediaClass: MediaClass,
        completed: Boolean,
        maxPositionSeconds: Int,
        runtimeSeconds: Int,
    ): Boolean {
        // A programmed grant is authorized by the grid, never by an
        // availability window, so watching the channel cannot burn a play.
        if (mode == PlaybackMode.PROGRAMMED) return false
        if (!mediaClass.consumesPlay) return false
        return WatchOnce.completesPlay(completed, maxPositionSeconds, runtimeSeconds)
    }
}
