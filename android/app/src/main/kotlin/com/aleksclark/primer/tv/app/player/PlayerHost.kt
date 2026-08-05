package com.aleksclark.primer.tv.app.player

import android.view.ViewGroup
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.viewinterop.AndroidView
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.repeatOnLifecycle
import android.util.Log
import androidx.media3.common.MediaItem
import androidx.media3.common.PlaybackException
import androidx.media3.common.Player
import androidx.media3.datasource.okhttp.OkHttpDataSource
import androidx.media3.exoplayer.DefaultRenderersFactory
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.exoplayer.source.DefaultMediaSourceFactory
import androidx.media3.ui.PlayerView
import com.aleksclark.primer.tv.app.ui.player.PlaybackChromeOverlay
import com.aleksclark.primer.tv.core.domain.PlaybackControls
import com.aleksclark.primer.tv.core.playback.BroadcastSeek
import com.aleksclark.primer.tv.core.playback.PlaybackSessionController
import com.aleksclark.primer.tv.core.presentation.PlaybackOverlayModel
import com.aleksclark.primer.tv.core.presentation.PlaybackOverlayPolicy
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import okhttp3.OkHttpClient

/**
 * Hosts ExoPlayer for one grant.
 *
 * Playback is deliberately direct-play only: no transcoding is requested of
 * Jellyfin, because the RK3318 box cannot keep up with a server-side transcode
 * and the admin UI already refuses to offer incompatible items.
 *
 * Audio uses [DefaultRenderersFactory] with
 * [DefaultRenderersFactory.EXTENSION_RENDERER_MODE_ON] so platform MediaCodec
 * stays preferred (AAC/MP3) and the vendored Media3 FFmpeg extension covers
 * AC3/EAC3/DTS when the SoC has no hardware decoder.
 *
 * Transport chrome visibility follows [PlaybackControls] / [PlaybackOverlayModel].
 * Pause and seek are enforced by [PolicyPlayer] (command withdrawal + clamp),
 * not by merely hiding Compose buttons.
 *
 * [furthestPositionSeconds] is the on-demand seek ceiling. While seek is
 * allowed the host also samples the playhead so the ceiling rises as the
 * student watches, without waiting for the next heartbeat.
 */
@Composable
fun PlayerHost(
    streamUrl: String,
    startPositionSeconds: Int,
    controls: PlaybackControls,
    httpClient: OkHttpClient,
    onProbeReady: (PlaybackSessionController.PlayerProbe) -> Unit,
    onEnded: () -> Unit,
    onResumed: () -> Unit,
    modifier: Modifier = Modifier,
    broadcastSeek: BroadcastSeek? = null,
    overlay: PlaybackOverlayModel = PlaybackOverlayPolicy.forControls(controls),
    furthestPositionSeconds: Int = 0,
    onPositionSampled: (positionMillis: Long) -> Unit = {},
) {
    val context = LocalContext.current

    val exoPlayer = remember(streamUrl) {
        val renderersFactory = DefaultRenderersFactory(context)
            .setExtensionRendererMode(PlayerConfiguration.EXTENSION_RENDERER_MODE)
        ExoPlayer.Builder(context, renderersFactory)
            .setMediaSourceFactory(
                DefaultMediaSourceFactory(OkHttpDataSource.Factory(httpClient)),
            )
            // Default skip amounts for the transport rewind/forward buttons.
            .setSeekBackIncrementMs(10_000L)
            .setSeekForwardIncrementMs(10_000L)
            .build()
            .apply {
                setMediaItem(MediaItem.fromUri(streamUrl))
                seekTo(startPositionSeconds * 1000L)
                playWhenReady = true
                prepare()
            }
    }

    // Live ceiling so PolicyPlayer clamps against the latest watermark even as
    // the student watches further within this composition.
    var furthestMs by remember(furthestPositionSeconds) {
        mutableLongStateOf(furthestPositionSeconds.coerceAtLeast(0) * 1000L)
    }
    LaunchedEffect(furthestPositionSeconds) {
        val fromState = furthestPositionSeconds.coerceAtLeast(0) * 1000L
        if (fromState > furthestMs) furthestMs = fromState
    }

    // The policy wrapper is what actually locks the transport: the view hides
    // and disables its own controls from the player's advertised commands, so
    // nothing in the UI can bypass it. Seek requests are clamped to the window.
    val player = remember(exoPlayer, controls) {
        PolicyPlayer(
            player = exoPlayer,
            controls = controls,
            furthestPositionMs = { furthestMs },
        )
    }

    // Broadcast corrections go to the underlying player, not the policy
    // wrapper: the wrapper refuses seeks by design, and this is the one seek
    // the student neither asked for nor can influence — it exists to keep the
    // box level with what the server says is on air.
    LaunchedEffect(broadcastSeek?.token) {
        val seek = broadcastSeek ?: return@LaunchedEffect
        exoPlayer.seekTo(seek.positionSeconds * 1000L)
        exoPlayer.playWhenReady = true
    }

    val probe = remember(exoPlayer) {
        PlaybackSessionController.PlayerProbe {
            PlaybackSessionController.PlayerProbe.Sample(
                positionMillis = exoPlayer.currentPosition.coerceAtLeast(0),
                isPlaying = exoPlayer.isPlaying,
            )
        }
    }

    // Raise the local seek ceiling as the playhead advances so the student can
    // immediately scrub within newly watched material.
    LaunchedEffect(exoPlayer, controls.seekAllowed) {
        if (!controls.seekAllowed) return@LaunchedEffect
        while (isActive) {
            val pos = exoPlayer.currentPosition.coerceAtLeast(0L)
            if (pos > furthestMs) {
                furthestMs = pos
            }
            onPositionSampled(pos)
            delay(1_000L)
        }
    }

    // Backgrounding detaches the probe to stop the heartbeat loop, so coming
    // back has to hand it over again — otherwise progress would silently stop
    // being reported for the rest of the session. The probe is re-attached
    // before [onResumed] runs, so a broadcast re-sync can read a live playhead
    // rather than a stale zero.
    val lifecycleOwner = LocalLifecycleOwner.current
    LaunchedEffect(lifecycleOwner, probe) {
        lifecycleOwner.repeatOnLifecycle(Lifecycle.State.RESUMED) {
            onProbeReady(probe)
            onResumed()
        }
    }

    DisposableEffect(exoPlayer) {
        val listener = object : Player.Listener {
            override fun onPlaybackStateChanged(state: Int) {
                if (state == Player.STATE_ENDED) onEnded()
            }

            override fun onPlayerError(error: PlaybackException) {
                // Keep existing callbacks intact; log enough to diagnose codec
                // / renderer failures on the T9 box without crashing the UI.
                Log.e(
                    PLAYER_HOST_TAG,
                    PlayerConfiguration.formatPlayerError(
                        errorCode = error.errorCode,
                        errorCodeName = error.errorCodeName,
                        message = error.message,
                        cause = error.cause?.toString(),
                    ),
                    error,
                )
            }

            override fun onPositionDiscontinuity(
                oldPosition: Player.PositionInfo,
                newPosition: Player.PositionInfo,
                reason: Int,
            ) {
                val pos = newPosition.positionMs.coerceAtLeast(0L)
                if (pos > furthestMs) furthestMs = pos
                onPositionSampled(pos)
            }
        }
        exoPlayer.addListener(listener)
        onDispose {
            exoPlayer.removeListener(listener)
            exoPlayer.release()
        }
    }

    Box(modifier = modifier.fillMaxSize()) {
        AndroidView(
            modifier = Modifier.fillMaxSize(),
            factory = { ctx ->
                PlayerView(ctx).apply {
                    this.player = player
                    // The programmed channel offers no transport at all, so the
                    // controller is not merely emptied but never shown: an
                    // overlay with nothing in it still steals D-pad focus.
                    // Entertainment keeps the controller for pause + progress;
                    // Media3 disables seek when seek commands are withdrawn.
                    useController = overlay.showTransportControls && controls.showTransportControls
                    controllerShowTimeoutMs = if (controls.followsBroadcast) 0 else 4_000
                    controllerHideOnTouch = !controls.followsBroadcast
                    setShowNextButton(false)
                    setShowPreviousButton(false)
                    // Fast-forward / rewind buttons only when seek is allowed.
                    // Even if Media3 still draws a scrub bar, PolicyPlayer
                    // clamps/refuses seeks for entertainment / programmed.
                    setShowFastForwardButton(controls.seekAllowed && overlay.seekInteractive)
                    setShowRewindButton(controls.seekAllowed && overlay.seekInteractive)
                    setShowSubtitleButton(false)
                    setShowVrButton(false)
                    layoutParams = ViewGroup.LayoutParams(
                        ViewGroup.LayoutParams.MATCH_PARENT,
                        ViewGroup.LayoutParams.MATCH_PARENT,
                    )
                }
            },
            update = { view ->
                view.player = player
                view.useController = overlay.showTransportControls && controls.showTransportControls
                view.setShowFastForwardButton(controls.seekAllowed && overlay.seekInteractive)
                view.setShowRewindButton(controls.seekAllowed && overlay.seekInteractive)
            },
        )

        PlaybackChromeOverlay(overlay = overlay)
    }
}

private const val PLAYER_HOST_TAG = "PlayerHost"
