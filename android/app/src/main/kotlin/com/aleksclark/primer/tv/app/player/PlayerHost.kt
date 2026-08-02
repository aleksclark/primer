package com.aleksclark.primer.tv.app.player

import android.view.ViewGroup
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.viewinterop.AndroidView
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.repeatOnLifecycle
import androidx.media3.common.MediaItem
import androidx.media3.common.Player
import androidx.media3.datasource.okhttp.OkHttpDataSource
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.exoplayer.source.DefaultMediaSourceFactory
import androidx.media3.ui.PlayerView
import com.aleksclark.primer.tv.core.domain.PlaybackControls
import com.aleksclark.primer.tv.core.playback.BroadcastSeek
import com.aleksclark.primer.tv.core.playback.PlaybackSessionController
import okhttp3.OkHttpClient

/**
 * Hosts ExoPlayer for one grant.
 *
 * Playback is deliberately direct-play only: no transcoding is requested of
 * Jellyfin, because the RK3318 box cannot keep up with a server-side transcode
 * and the admin UI already refuses to offer incompatible items.
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
) {
    val context = LocalContext.current

    val exoPlayer = remember(streamUrl) {
        ExoPlayer.Builder(context)
            .setMediaSourceFactory(
                DefaultMediaSourceFactory(OkHttpDataSource.Factory(httpClient)),
            )
            .build()
            .apply {
                setMediaItem(MediaItem.fromUri(streamUrl))
                seekTo(startPositionSeconds * 1000L)
                playWhenReady = true
                prepare()
            }
    }

    // The policy wrapper is what actually locks the transport: the view hides
    // and disables its own controls from the player's advertised commands, so
    // nothing in the UI can bypass it.
    val player = remember(exoPlayer, controls) { PolicyPlayer(exoPlayer, controls) }

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
                    useController = controls.showTransportControls
                    setShowNextButton(false)
                    setShowPreviousButton(false)
                    layoutParams = ViewGroup.LayoutParams(
                        ViewGroup.LayoutParams.MATCH_PARENT,
                        ViewGroup.LayoutParams.MATCH_PARENT,
                    )
                }
            },
        )
    }
}
