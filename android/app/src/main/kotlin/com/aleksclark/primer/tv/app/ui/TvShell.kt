package com.aleksclark.primer.tv.app.ui

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.focusProperties
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.aleksclark.primer.tv.app.player.PlayerHost
import com.aleksclark.primer.tv.app.ui.channel.ChannelEvent
import com.aleksclark.primer.tv.app.ui.channel.ChannelScreen
import com.aleksclark.primer.tv.app.ui.components.StatusBanner
import com.aleksclark.primer.tv.app.ui.designsystem.PrimerTheme
import com.aleksclark.primer.tv.app.ui.designsystem.PrimerTvTheme
import com.aleksclark.primer.tv.app.ui.details.DetailScreen
import com.aleksclark.primer.tv.app.ui.guide.GuideEvent
import com.aleksclark.primer.tv.app.ui.guide.GuideScreen
import com.aleksclark.primer.tv.app.ui.home.HomeEvent
import com.aleksclark.primer.tv.app.ui.home.HomeScreen
import com.aleksclark.primer.tv.app.ui.navigation.TopLevelDestination
import com.aleksclark.primer.tv.app.ui.navigation.toTopLevelOrNull
import com.aleksclark.primer.tv.app.ui.pairing.PairingScreen
import com.aleksclark.primer.tv.app.ui.player.PlaybackErrorOverlay
import com.aleksclark.primer.tv.app.ui.player.PlaybackFinishedOverlay
import com.aleksclark.primer.tv.app.ui.scaffold.StreamingScaffold
import com.aleksclark.primer.tv.app.ui.settings.SettingsEvent
import com.aleksclark.primer.tv.app.ui.settings.SettingsScreen
import com.aleksclark.primer.tv.core.domain.FormFactor
import com.aleksclark.primer.tv.core.domain.PlaybackMode
import com.aleksclark.primer.tv.core.playback.BroadcastSeek
import com.aleksclark.primer.tv.core.playback.PlaybackState
import com.aleksclark.primer.tv.core.presentation.PlaybackOverlayPolicy
import okhttp3.OkHttpClient

/**
 * The single shell for both form factors.
 *
 * Top-level destinations sit in [StreamingScaffold]. Details is a child overlay
 * over the originating top-level surface so Home rail scroll/focus survive
 * open → back.
 */
@Composable
fun TvShell(
    viewModel: TvViewModel,
    formFactor: FormFactor,
    httpClient: OkHttpClient,
    onExit: () -> Unit,
) {
    val settings by viewModel.settings.collectAsStateWithLifecycle()
    val destination by viewModel.destination.collectAsStateWithLifecycle()
    val pairing by viewModel.pairing.collectAsStateWithLifecycle()
    val catalog by viewModel.catalog.collectAsStateWithLifecycle()
    val home by viewModel.home.collectAsStateWithLifecycle()
    val selectedId by viewModel.selectedMediaItemId.collectAsStateWithLifecycle()
    val playback by viewModel.playback.collectAsStateWithLifecycle()
    val update by viewModel.update.collectAsStateWithLifecycle()
    val statusMessage by viewModel.statusMessage.collectAsStateWithLifecycle()
    val channel by viewModel.channel.collectAsStateWithLifecycle()
    val epg by viewModel.epg.collectAsStateWithLifecycle()
    val broadcastSeek by viewModel.broadcastSeek.collectAsStateWithLifecycle()

    var lastTopLevel by rememberSaveable { mutableStateOf(TopLevelDestination.HOME.name) }
    val selectedTopLevel = destination.toTopLevelOrNull()
        ?: runCatching { TopLevelDestination.valueOf(lastTopLevel) }.getOrDefault(TopLevelDestination.HOME)

    LaunchedEffect(selectedTopLevel) {
        lastTopLevel = selectedTopLevel.name
    }

    LaunchedEffect(settings.isPaired) {
        viewModel.onSettingsLoaded(settings)
        if (settings.isPaired) viewModel.refreshHome()
    }

    BackHandler(enabled = true) {
        if (!viewModel.back()) onExit()
    }

    PrimerTvTheme(formFactor = formFactor) {
        Box(
            modifier = Modifier
                .fillMaxSize()
                .background(PrimerTheme.colors.background),
        ) {
            when (destination) {
                Destination.PAIRING -> PairingScreen(
                    state = pairing,
                    onBaseUrlChanged = viewModel::onBaseUrlChanged,
                    onCodeChanged = viewModel::onCodeChanged,
                    onSubmit = viewModel::submitPairing,
                )

                Destination.PLAYER -> PlayerRoute(
                    viewModel = viewModel,
                    state = playback,
                    httpClient = httpClient,
                    broadcastSeek = broadcastSeek,
                )

                // Details is a child route over the originating top-level screen.
                // Keep Home (and peers) composed underneath so rail scroll/focus
                // survive open → back without a full recomposition reset.
                Destination.CATALOG,
                Destination.CHANNEL,
                Destination.EPG,
                Destination.SETTINGS,
                Destination.DETAIL,
                -> StreamingScaffold(
                    formFactor = formFactor,
                    selected = selectedTopLevel,
                    onSelect = { topLevel ->
                        when (topLevel) {
                            TopLevelDestination.HOME -> viewModel.openHome()
                            TopLevelDestination.CHANNEL -> viewModel.openChannel()
                            TopLevelDestination.GUIDE -> viewModel.openEpg()
                            TopLevelDestination.SETTINGS -> viewModel.openSettings()
                        }
                    },
                    // Settings is a bottom-nav item on phone during Phase A so
                    // the destination remains reachable without a top app bar.
                    showSettingsInBottomBar = true,
                ) { contentPadding ->
                    val detailsOpen = destination == Destination.DETAIL
                    Box(
                        modifier = Modifier
                            .fillMaxSize()
                            .padding(contentPadding),
                    ) {
                        // Origin top-level content stays mounted while Details is open.
                        val underlying = if (detailsOpen) {
                            selectedTopLevel
                        } else {
                            destination.toTopLevelOrNull() ?: selectedTopLevel
                        }
                        Box(
                            modifier = Modifier
                                .fillMaxSize()
                                // Block D-pad into the retained origin while Details covers it.
                                .focusProperties { canFocus = !detailsOpen },
                        ) {
                            when (underlying) {
                                TopLevelDestination.HOME -> HomeScreen(
                                    state = home,
                                    baseUrl = settings.baseUrl.orEmpty(),
                                    selectedMediaId = selectedId,
                                    detailsOpen = detailsOpen,
                                    onEvent = { event ->
                                        when (event) {
                                            HomeEvent.Refresh -> viewModel.refreshHome()
                                            is HomeEvent.SelectMedia -> viewModel.openDetail(event.id)
                                            HomeEvent.OpenChannel -> viewModel.openChannel()
                                            HomeEvent.PlayHero -> viewModel.playHero()
                                        }
                                    },
                                )

                                TopLevelDestination.CHANNEL -> ChannelScreen(
                                    state = channel,
                                    baseUrl = settings.baseUrl.orEmpty(),
                                    onEvent = { event ->
                                        when (event) {
                                            ChannelEvent.WatchLive -> viewModel.tuneIn()
                                            ChannelEvent.Refresh -> viewModel.refreshChannel()
                                        }
                                    },
                                )

                                TopLevelDestination.GUIDE -> GuideScreen(
                                    state = epg,
                                    onEvent = { event ->
                                        when (event) {
                                            GuideEvent.Refresh -> viewModel.refreshEpg()
                                            // Browse-first: rows are focusable for D-pad, but
                                            // programmed titles are not catalog detail targets.
                                            is GuideEvent.SelectProgramme -> Unit
                                        }
                                    },
                                )

                                TopLevelDestination.SETTINGS -> SettingsScreen(
                                    settings = settings,
                                    installedVersion = viewModel.installedVersionCode(),
                                    update = update,
                                    onEvent = { event ->
                                        when (event) {
                                            SettingsEvent.CheckForUpdate -> viewModel.checkForUpdate()
                                            SettingsEvent.InstallUpdate -> viewModel.installUpdate()
                                            SettingsEvent.Unpair -> viewModel.unpair()
                                        }
                                    },
                                )
                            }
                        }

                        if (detailsOpen) {
                            // Opaque overlay so the retained origin screen cannot
                            // receive D-pad focus or clicks while Details is open.
                            Box(
                                modifier = Modifier
                                    .fillMaxSize()
                                    .background(PrimerTheme.colors.background),
                            ) {
                                DetailScreen(
                                    card = selectedId?.let(catalog::card),
                                    baseUrl = settings.baseUrl.orEmpty(),
                                    onPlay = { selectedId?.let(viewModel::play) },
                                    onBack = { viewModel.back() },
                                    // Phone uses system back; TV keeps a subtle Back action.
                                    showBackButton = formFactor == FormFactor.TELEVISION,
                                )
                            }
                        }

                        // Transient refresh/update feedback — never a full-page flash.
                        StatusBanner(
                            message = statusMessage?.text,
                            isError = statusMessage?.isError == true,
                            modifier = Modifier.align(Alignment.TopCenter),
                        )
                    }
                }
            }
        }
    }
}

@Composable
private fun PlayerRoute(
    viewModel: TvViewModel,
    state: PlaybackState,
    httpClient: OkHttpClient,
    broadcastSeek: BroadcastSeek?,
) {
    when (state) {
        is PlaybackState.Idle, is PlaybackState.RequestingGrant -> {
            val message = PlaybackOverlayPolicy.messageFor(state)!!
            PlaybackErrorOverlay(
                message = message,
                onPrimary = { viewModel.stopPlayback(completed = false) },
            )
        }

        is PlaybackState.Failed -> {
            val message = PlaybackOverlayPolicy.messageFor(state)!!
            PlaybackErrorOverlay(
                message = message,
                onPrimary = {
                    if (message.canRetry) {
                        viewModel.retryPlayback()
                    } else {
                        viewModel.stopPlayback(completed = false)
                    }
                },
                onSecondary = if (message.canRetry) {
                    { viewModel.stopPlayback(completed = false) }
                } else {
                    null
                },
            )
        }

        is PlaybackState.Finished -> {
            val message = PlaybackOverlayPolicy.messageFor(state)!!
            PlaybackFinishedOverlay(
                message = message,
                onBack = { viewModel.stopPlayback(completed = false) },
            )
        }

        is PlaybackState.Playable -> {
            // Map from grant-backed controls so chrome cannot drift from the
            // player-layer policy (seek/pause still enforced by PolicyPlayer).
            val overlay = PlaybackOverlayPolicy.forControls(
                controls = state.controls,
                title = viewModel.playingTitle(),
                mode = PlaybackMode.fromWire(state.grant.mode),
            )
            PlayerHost(
                streamUrl = state.grant.streamUrl,
                startPositionSeconds = state.resumePositionSeconds,
                controls = state.controls,
                httpClient = httpClient,
                onProbeReady = viewModel::attachPlayer,
                onEnded = { viewModel.stopPlayback(completed = true) },
                // Coming back to the foreground re-asks the server where the
                // broadcast is rather than trusting the box's own clock. On demand
                // this is a no-op.
                onResumed = viewModel::resyncBroadcast,
                broadcastSeek = broadcastSeek,
                overlay = overlay,
            )
        }
    }
}
