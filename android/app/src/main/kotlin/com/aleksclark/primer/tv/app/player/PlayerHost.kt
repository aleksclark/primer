package com.aleksclark.primer.tv.app.player

import android.content.Context
import android.database.ContentObserver
import android.media.AudioManager
import android.os.Handler
import android.os.Looper
import android.provider.Settings
import android.view.MotionEvent
import android.view.ViewGroup
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.viewinterop.AndroidView
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.repeatOnLifecycle
import androidx.media3.common.C
import androidx.media3.common.MediaItem
import androidx.media3.common.Player
import androidx.media3.common.TrackSelectionOverride
import androidx.media3.common.Tracks
import androidx.media3.datasource.okhttp.OkHttpDataSource
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.exoplayer.source.DefaultMediaSourceFactory
import androidx.media3.ui.PlayerView
import com.aleksclark.primer.tv.app.ui.player.PlaybackAccessibilityOverlay
import com.aleksclark.primer.tv.app.ui.player.PlaybackChromeOverlay
import com.aleksclark.primer.tv.core.domain.PlaybackControls
import com.aleksclark.primer.tv.core.playback.BroadcastSeek
import com.aleksclark.primer.tv.core.playback.PlaybackSessionController
import com.aleksclark.primer.tv.core.presentation.PlaybackOverlayModel
import com.aleksclark.primer.tv.core.presentation.PlaybackOverlayPolicy
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import okhttp3.OkHttpClient

private const val OVERLAY_TIMEOUT_MS = 4_000L

/**
 * Hosts ExoPlayer for one grant.
 *
 * Playback is deliberately direct-play only: no transcoding is requested of
 * Jellyfin, because the RK3318 box cannot keep up with a server-side transcode
 * and the admin UI already refuses to offer incompatible items.
 *
 * Transport chrome visibility follows [PlaybackControls] / [PlaybackOverlayModel].
 * Pause and seek are enforced by [PolicyPlayer] (command withdrawal + clamp),
 * not by merely hiding Compose buttons.
 *
 * Volume, captions, and audio-track controls ride the same timed overlay as
 * the transport chrome for on-demand titles, and a timed live overlay when
 * seek/pause are withdrawn.
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
    val audioManager = remember(context) {
        context.getSystemService(Context.AUDIO_SERVICE) as AudioManager
    }
    var mediaVolume by remember {
        mutableIntStateOf(audioManager.getStreamVolume(AudioManager.STREAM_MUSIC))
    }
    val maximumMediaVolume = remember(audioManager) {
        audioManager.getStreamMaxVolume(AudioManager.STREAM_MUSIC)
    }
    var subtitles by remember { mutableStateOf(emptyList<SubtitleOption>()) }
    var audioTracks by remember { mutableStateOf(emptyList<AudioOption>()) }
    val transportEnabled = overlay.showTransportControls && controls.showTransportControls
    val liveMediaOverlay = !transportEnabled
    var overlayVisible by remember(transportEnabled) { mutableStateOf(true) }
    var overlayEpoch by remember { mutableLongStateOf(0L) }
    var playerView by remember { mutableStateOf<PlayerView?>(null) }

    fun bumpOverlay() {
        overlayVisible = true
        overlayEpoch += 1
        if (transportEnabled) {
            playerView?.showController()
        }
    }

    val exoPlayer = remember(streamUrl) {
        ExoPlayer.Builder(context)
            .setMediaSourceFactory(
                DefaultMediaSourceFactory(OkHttpDataSource.Factory(httpClient)),
            )
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

    var furthestMs by remember(furthestPositionSeconds) {
        mutableLongStateOf(furthestPositionSeconds.coerceAtLeast(0) * 1000L)
    }
    LaunchedEffect(furthestPositionSeconds) {
        val fromState = furthestPositionSeconds.coerceAtLeast(0) * 1000L
        if (fromState > furthestMs) furthestMs = fromState
    }

    val player = remember(exoPlayer, controls) {
        PolicyPlayer(
            player = exoPlayer,
            controls = controls,
            furthestPositionMs = { furthestMs },
        )
    }

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

    val lifecycleOwner = LocalLifecycleOwner.current
    LaunchedEffect(lifecycleOwner, probe) {
        lifecycleOwner.repeatOnLifecycle(Lifecycle.State.RESUMED) {
            onProbeReady(probe)
            onResumed()
        }
    }

    LaunchedEffect(overlayVisible, overlayEpoch, transportEnabled, liveMediaOverlay) {
        if (!overlayVisible) return@LaunchedEffect
        if (!transportEnabled && !liveMediaOverlay) return@LaunchedEffect
        delay(OVERLAY_TIMEOUT_MS)
        overlayVisible = false
        if (transportEnabled) {
            playerView?.hideController()
        }
    }

    DisposableEffect(audioManager) {
        val observer = object : ContentObserver(Handler(Looper.getMainLooper())) {
            override fun onChange(selfChange: Boolean) {
                mediaVolume = audioManager.getStreamVolume(AudioManager.STREAM_MUSIC)
            }
        }
        context.contentResolver.registerContentObserver(Settings.System.CONTENT_URI, true, observer)
        onDispose { context.contentResolver.unregisterContentObserver(observer) }
    }

    fun updateTrackOptions(tracks: Tracks) {
        subtitles = subtitleOptions(
            tracks.groups.flatMapIndexed { groupIndex, group ->
                if (group.type != C.TRACK_TYPE_TEXT) return@flatMapIndexed emptyList()
                List(group.length) { trackIndex ->
                    val format = group.getTrackFormat(trackIndex)
                    SubtitleTrackDescriptor(
                        groupIndex = groupIndex,
                        trackIndex = trackIndex,
                        label = format.label,
                        language = format.language,
                        supported = group.isTrackSupported(trackIndex),
                        selected = group.isTrackSelected(trackIndex),
                    )
                }
            },
        )
        audioTracks = audioOptions(
            tracks.groups.flatMapIndexed { groupIndex, group ->
                if (group.type != C.TRACK_TYPE_AUDIO) return@flatMapIndexed emptyList()
                List(group.length) { trackIndex ->
                    val format = group.getTrackFormat(trackIndex)
                    AudioTrackDescriptor(
                        groupIndex = groupIndex,
                        trackIndex = trackIndex,
                        label = format.label,
                        language = format.language,
                        supported = group.isTrackSupported(trackIndex),
                        selected = group.isTrackSelected(trackIndex),
                    )
                }
            },
        )
    }

    DisposableEffect(exoPlayer) {
        val listener = object : Player.Listener {
            override fun onPlaybackStateChanged(state: Int) {
                if (state == Player.STATE_ENDED) onEnded()
            }

            override fun onTracksChanged(tracks: Tracks) {
                updateTrackOptions(tracks)
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
        updateTrackOptions(exoPlayer.currentTracks)
        onDispose {
            exoPlayer.removeListener(listener)
            exoPlayer.release()
        }
    }

    Box(
        modifier = modifier
            .fillMaxSize()
            .pointerInput(liveMediaOverlay) {
                if (!liveMediaOverlay) return@pointerInput
                detectTapGestures {
                    if (overlayVisible) {
                        overlayVisible = false
                    } else {
                        bumpOverlay()
                    }
                }
            },
    ) {
        AndroidView(
            modifier = Modifier.fillMaxSize(),
            factory = { ctx ->
                PlayerView(ctx).apply {
                    this.player = player
                    useController = transportEnabled
                    controllerShowTimeoutMs = if (transportEnabled) OVERLAY_TIMEOUT_MS.toInt() else 0
                    controllerHideOnTouch = transportEnabled
                    setShowNextButton(false)
                    setShowPreviousButton(false)
                    setShowFastForwardButton(controls.seekAllowed && overlay.seekInteractive)
                    setShowRewindButton(controls.seekAllowed && overlay.seekInteractive)
                    setShowSubtitleButton(false)
                    setShowVrButton(false)
                    setControllerVisibilityListener(
                        PlayerView.ControllerVisibilityListener { visibility ->
                            if (!transportEnabled) return@ControllerVisibilityListener
                            val visible = visibility == android.view.View.VISIBLE
                            overlayVisible = visible
                            if (visible) {
                                overlayEpoch += 1
                            }
                        },
                    )
                    if (liveMediaOverlay) {
                        setOnTouchListener { _, event ->
                            if (event.actionMasked == MotionEvent.ACTION_UP) {
                                if (overlayVisible) overlayVisible = false else bumpOverlay()
                                performClick()
                            }
                            true
                        }
                    }
                    layoutParams = ViewGroup.LayoutParams(
                        ViewGroup.LayoutParams.MATCH_PARENT,
                        ViewGroup.LayoutParams.MATCH_PARENT,
                    )
                    playerView = this
                    if (transportEnabled) {
                        showController()
                    }
                }
            },
            update = { view ->
                view.player = player
                view.useController = transportEnabled
                view.setShowFastForwardButton(controls.seekAllowed && overlay.seekInteractive)
                view.setShowRewindButton(controls.seekAllowed && overlay.seekInteractive)
                playerView = view
                if (!transportEnabled) {
                    view.hideController()
                }
            },
        )

        PlaybackChromeOverlay(overlay = overlay)
        if (mediaControlsVisible(overlayVisible)) {
            PlaybackAccessibilityOverlay(
                volumePercent = volumePercent(mediaVolume, maximumMediaVolume),
                subtitles = subtitles,
                audioTracks = audioTracks,
                onUserInteraction = { bumpOverlay() },
                onVolumeDown = {
                    audioManager.adjustStreamVolume(
                        AudioManager.STREAM_MUSIC,
                        AudioManager.ADJUST_LOWER,
                        0,
                    )
                    mediaVolume = audioManager.getStreamVolume(AudioManager.STREAM_MUSIC)
                },
                onVolumeUp = {
                    audioManager.adjustStreamVolume(
                        AudioManager.STREAM_MUSIC,
                        AudioManager.ADJUST_RAISE,
                        0,
                    )
                    mediaVolume = audioManager.getStreamVolume(AudioManager.STREAM_MUSIC)
                },
                onSubtitlesOff = {
                    exoPlayer.trackSelectionParameters = exoPlayer.trackSelectionParameters
                        .buildUpon()
                        .clearOverridesOfType(C.TRACK_TYPE_TEXT)
                        .setTrackTypeDisabled(C.TRACK_TYPE_TEXT, true)
                        .build()
                },
                onSubtitleSelected = { subtitle ->
                    val group = exoPlayer.currentTracks.groups.getOrNull(subtitle.groupIndex)
                        ?: return@PlaybackAccessibilityOverlay
                    exoPlayer.trackSelectionParameters = exoPlayer.trackSelectionParameters
                        .buildUpon()
                        .setTrackTypeDisabled(C.TRACK_TYPE_TEXT, false)
                        .clearOverridesOfType(C.TRACK_TYPE_TEXT)
                        .addOverride(
                            TrackSelectionOverride(group.mediaTrackGroup, listOf(subtitle.trackIndex)),
                        )
                        .build()
                },
                onAudioSelected = { audio ->
                    val group = exoPlayer.currentTracks.groups.getOrNull(audio.groupIndex)
                        ?: return@PlaybackAccessibilityOverlay
                    exoPlayer.trackSelectionParameters = exoPlayer.trackSelectionParameters
                        .buildUpon()
                        .setTrackTypeDisabled(C.TRACK_TYPE_AUDIO, false)
                        .clearOverridesOfType(C.TRACK_TYPE_AUDIO)
                        .addOverride(
                            TrackSelectionOverride(group.mediaTrackGroup, listOf(audio.trackIndex)),
                        )
                        .build()
                },
            )
        }
    }
}
