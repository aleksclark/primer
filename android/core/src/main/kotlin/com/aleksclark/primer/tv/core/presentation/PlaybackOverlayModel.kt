package com.aleksclark.primer.tv.core.presentation

import com.aleksclark.primer.tv.core.domain.MediaClass
import com.aleksclark.primer.tv.core.domain.PlaybackControls
import com.aleksclark.primer.tv.core.domain.PlaybackMode
import com.aleksclark.primer.tv.core.domain.PlaybackPolicy
import com.aleksclark.primer.tv.core.playback.PlaybackState

/**
 * Which chrome the player surface draws on top of the video.
 *
 * Enforcement of pause/seek remains on [PlaybackControls] / PolicyPlayer —
 * this only decides what is visible.
 */
enum class PlaybackOverlayKind {
    /** Full transport: play/pause, seek bar, skip buttons. */
    FULL_TRANSPORT,

    /** Pause + non-interactive progress; no seek affordances. */
    PAUSE_PROGRESS,

    /** Minimal LIVE badge + title; back only. */
    LIVE_MINIMAL,
}

data class PlaybackOverlayModel(
    val kind: PlaybackOverlayKind,
    val title: String,
    val showLiveBadge: Boolean,
    val showTransportControls: Boolean,
    val seekInteractive: Boolean,
    val pauseAllowed: Boolean,
    val followsBroadcast: Boolean,
)

/**
 * Maps mode + class + controls into a single overlay description so UI and
 * tests share one source of truth.
 */
object PlaybackOverlayPolicy {

    fun forControls(
        controls: PlaybackControls,
        title: String = "",
        mode: PlaybackMode? = null,
    ): PlaybackOverlayModel {
        val kind = when {
            controls.followsBroadcast || mode == PlaybackMode.PROGRAMMED ->
                PlaybackOverlayKind.LIVE_MINIMAL
            controls.seekAllowed -> PlaybackOverlayKind.FULL_TRANSPORT
            controls.pauseAllowed -> PlaybackOverlayKind.PAUSE_PROGRESS
            else -> PlaybackOverlayKind.LIVE_MINIMAL
        }
        return PlaybackOverlayModel(
            kind = kind,
            title = title,
            showLiveBadge = kind == PlaybackOverlayKind.LIVE_MINIMAL,
            showTransportControls = controls.showTransportControls && kind != PlaybackOverlayKind.LIVE_MINIMAL,
            seekInteractive = controls.seekAllowed && kind == PlaybackOverlayKind.FULL_TRANSPORT,
            pauseAllowed = controls.pauseAllowed,
            followsBroadcast = controls.followsBroadcast,
        )
    }

    fun forPlayback(
        mode: PlaybackMode,
        mediaClass: MediaClass,
        title: String = "",
    ): PlaybackOverlayModel = forControls(
        controls = PlaybackPolicy.controlsFor(mode, mediaClass),
        title = title,
        mode = mode,
    )

    /**
     * User-facing copy for non-playable player states (loading, finished, failed).
     */
    fun messageFor(state: PlaybackState): PlaybackMessageModel? = when (state) {
        is PlaybackState.Idle, is PlaybackState.RequestingGrant -> PlaybackMessageModel(
            title = "Starting…",
            body = "Preparing playback.",
            primaryLabel = "Back",
            canRetry = false,
            isError = false,
        )
        is PlaybackState.Failed -> PlaybackMessageModel(
            title = if (state.recoverable) "Playback interrupted" else "Can't play this title",
            body = state.error.message,
            primaryLabel = if (state.recoverable) "Try again" else "Back",
            canRetry = state.recoverable,
            isError = true,
        )
        is PlaybackState.Finished -> PlaybackMessageModel(
            title = if (state.playConsumed) "One viewing used" else "Playback stopped",
            body = when {
                state.playConsumed -> "That was this title's one viewing."
                state.completed -> "Finished."
                else -> "Stopped."
            },
            primaryLabel = "Back",
            canRetry = false,
            isError = false,
        )
        is PlaybackState.Playable -> null
    }
}

/** Copy for full-screen player messages / error overlays. */
data class PlaybackMessageModel(
    val title: String,
    val body: String,
    val primaryLabel: String,
    val canRetry: Boolean,
    val isError: Boolean,
)
