package com.aleksclark.primer.tv.app.ui

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.aleksclark.primer.tv.app.player.PlayerHost
import com.aleksclark.primer.tv.core.domain.FormFactor
import com.aleksclark.primer.tv.core.playback.BroadcastSeek
import com.aleksclark.primer.tv.core.playback.PlaybackState
import okhttp3.OkHttpClient

/**
 * The single shell for both form factors.
 *
 * [formFactor] only selects layout metrics: routing, state, and playback rules
 * are identical, so the tablet and the TV box cannot drift apart.
 */
@Composable
fun TvShell(
    viewModel: TvViewModel,
    formFactor: FormFactor,
    httpClient: OkHttpClient,
    onExit: () -> Unit,
) {
    val metrics = when (formFactor) {
        FormFactor.TABLET -> ShellMetrics.Tablet
        FormFactor.TELEVISION -> ShellMetrics.Television
    }

    val settings by viewModel.settings.collectAsStateWithLifecycle()
    val destination by viewModel.destination.collectAsStateWithLifecycle()
    val pairing by viewModel.pairing.collectAsStateWithLifecycle()
    val catalog by viewModel.catalog.collectAsStateWithLifecycle()
    val selectedId by viewModel.selectedMediaItemId.collectAsStateWithLifecycle()
    val playback by viewModel.playback.collectAsStateWithLifecycle()
    val update by viewModel.update.collectAsStateWithLifecycle()
    val channel by viewModel.channel.collectAsStateWithLifecycle()
    val epg by viewModel.epg.collectAsStateWithLifecycle()
    val broadcastSeek by viewModel.broadcastSeek.collectAsStateWithLifecycle()

    LaunchedEffect(settings.isPaired) {
        viewModel.onSettingsLoaded(settings)
        if (settings.isPaired) viewModel.refreshCatalog()
    }

    BackHandler(enabled = true) {
        if (!viewModel.back()) onExit()
    }

    MaterialTheme(colorScheme = darkColorScheme()) {
        Box(
            modifier = Modifier
                .fillMaxSize()
                .background(Color(0xFF0F1115)),
        ) {
            when (destination) {
                Destination.PAIRING -> PairingScreen(
                    state = pairing,
                    metrics = metrics,
                    onBaseUrlChanged = viewModel::onBaseUrlChanged,
                    onCodeChanged = viewModel::onCodeChanged,
                    onSubmit = viewModel::submitPairing,
                )

                Destination.CATALOG -> CatalogScreen(
                    state = catalog,
                    baseUrl = settings.baseUrl.orEmpty(),
                    metrics = metrics,
                    onSelect = viewModel::openDetail,
                    onRefresh = viewModel::refreshCatalog,
                    onOpenSettings = viewModel::openSettings,
                    onOpenChannel = viewModel::openChannel,
                )

                Destination.CHANNEL -> ChannelScreen(
                    state = channel,
                    baseUrl = settings.baseUrl.orEmpty(),
                    metrics = metrics,
                    onTuneIn = viewModel::tuneIn,
                    onOpenEpg = viewModel::openEpg,
                    onRefresh = viewModel::refreshChannel,
                    onBack = { viewModel.back() },
                )

                Destination.EPG -> EpgScreen(
                    state = epg,
                    metrics = metrics,
                    onRefresh = viewModel::refreshEpg,
                    onBack = { viewModel.back() },
                )

                Destination.DETAIL -> DetailScreen(
                    card = selectedId?.let(catalog::card),
                    baseUrl = settings.baseUrl.orEmpty(),
                    metrics = metrics,
                    onPlay = { selectedId?.let(viewModel::play) },
                    onBack = { viewModel.back() },
                )

                Destination.SETTINGS -> SettingsScreen(
                    settings = settings,
                    metrics = metrics,
                    installedVersion = viewModel.installedVersionCode(),
                    update = update,
                    onCheckForUpdate = viewModel::checkForUpdate,
                    onInstallUpdate = viewModel::installUpdate,
                    onUnpair = viewModel::unpair,
                    onBack = { viewModel.back() },
                )

                Destination.PLAYER -> PlayerRoute(
                    viewModel = viewModel,
                    state = playback,
                    metrics = metrics,
                    httpClient = httpClient,
                    broadcastSeek = broadcastSeek,
                )
            }
        }
    }
}

@Composable
private fun PlayerRoute(
    viewModel: TvViewModel,
    state: PlaybackState,
    metrics: ShellMetrics,
    httpClient: OkHttpClient,
    broadcastSeek: BroadcastSeek?,
) {
    when (state) {
        is PlaybackState.Idle, is PlaybackState.RequestingGrant ->
            PlaybackMessage("Starting…", metrics) { viewModel.stopPlayback(completed = false) }

        is PlaybackState.Failed ->
            PlaybackMessage(state.error.message, metrics) { viewModel.stopPlayback(completed = false) }

        is PlaybackState.Finished ->
            PlaybackMessage(
                if (state.playConsumed) "That was this title's one viewing." else "Stopped.",
                metrics,
            ) { viewModel.stopPlayback(completed = false) }

        is PlaybackState.Playable -> PlayerHost(
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
        )
    }
}
