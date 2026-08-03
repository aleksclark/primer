package com.aleksclark.primer.tv.app.ui

import android.util.Log
import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import com.aleksclark.primer.tv.app.data.AppContainer
import com.aleksclark.primer.tv.app.update.UpdateState
import com.aleksclark.primer.tv.core.data.ApiError
import com.aleksclark.primer.tv.core.data.ApiResult
import com.aleksclark.primer.tv.core.data.DeviceSettings
import com.aleksclark.primer.tv.core.data.TvRepository
import com.aleksclark.primer.tv.core.domain.CatalogPresenter
import com.aleksclark.primer.tv.core.domain.MediaClass
import com.aleksclark.primer.tv.core.domain.PlaybackControls
import com.aleksclark.primer.tv.core.domain.PlaybackMode
import com.aleksclark.primer.tv.core.domain.PlaybackPolicy
import com.aleksclark.primer.tv.core.domain.Programme
import com.aleksclark.primer.tv.core.net.normalizeBaseUrl
import com.aleksclark.primer.tv.core.playback.BroadcastSeek
import com.aleksclark.primer.tv.core.playback.BroadcastSync
import com.aleksclark.primer.tv.core.playback.PlaybackSessionController
import com.aleksclark.primer.tv.core.playback.PlaybackState
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch

/**
 * Drives every screen. One view model serves both shells so the tablet and TV
 * UIs are pure presentation over identical state.
 */
class TvViewModel(
    private val container: AppContainer,
    /** Overridden in tests so the whole view model runs on virtual time. */
    injectedScope: CoroutineScope? = null,
) : ViewModel() {

    private val scope: CoroutineScope = injectedScope ?: viewModelScope

    val settings: StateFlow<DeviceSettings> = container.settingsStore.settings
        .stateIn(scope, SharingStarted.Eagerly, DeviceSettings())

    private val _pairing = MutableStateFlow(PairingUiState())
    val pairing: StateFlow<PairingUiState> = _pairing.asStateFlow()

    private val _catalog = MutableStateFlow(CatalogUiState())
    val catalog: StateFlow<CatalogUiState> = _catalog.asStateFlow()

    private val _channel = MutableStateFlow(ChannelUiState())
    val channel: StateFlow<ChannelUiState> = _channel.asStateFlow()

    private val _epg = MutableStateFlow(EpgUiState())
    val epg: StateFlow<EpgUiState> = _epg.asStateFlow()

    private val _destination = MutableStateFlow(Destination.CATALOG)
    val destination: StateFlow<Destination> = _destination.asStateFlow()

    private val _selectedMediaItemId = MutableStateFlow<String?>(null)
    val selectedMediaItemId: StateFlow<String?> = _selectedMediaItemId.asStateFlow()

    /**
     * Items whose play the server charged during this app session. They stay
     * visible but greyed until the next catalog refresh drops them, so the row
     * the student just watched does not vanish under the cursor.
     */
    private val _consumedMediaItemIds = MutableStateFlow<Set<String>>(emptySet())

    private var controller: PlaybackSessionController? = null
    private var playbackMirror: Job? = null
    private var seekMirror: Job? = null

    private val _playback = MutableStateFlow<PlaybackState>(PlaybackState.Idle)
    val playback: StateFlow<PlaybackState> = _playback.asStateFlow()

    private val _update = MutableStateFlow<UpdateState>(UpdateState.UpToDate)
    val update: StateFlow<UpdateState> = _update.asStateFlow()

    /**
     * Corrections the programmed player must apply to stay level with the
     * broadcast. Mirrored off the controller so the player composable does not
     * have to reach into it.
     */
    private val _broadcastSeek = MutableStateFlow<BroadcastSeek?>(null)
    val broadcastSeek: StateFlow<BroadcastSeek?> = _broadcastSeek.asStateFlow()

    /** The mode the current playback attempt was authorized in. */
    private var playbackMode: PlaybackMode = PlaybackMode.ON_DEMAND

    /** The programme being watched, when tuned to the channel. */
    private var tunedProgramme: Programme? = null

    /**
     * The repository for the currently configured server, or null when unpaired.
     *
     * Prefer the URL just written during pairing over [settings]: that StateFlow
     * is fed by DataStore and can lag a write by a tick, which would make the
     * first post-pair catalog call look unauthenticated and bounce the UI.
     */
    private var activeBaseUrl: String? = null

    private fun repository(): TvRepository? {
        val baseUrl = activeBaseUrl
            ?: settings.value.baseUrl?.takeIf { it.isNotBlank() }
            ?: return null
        return container.repositoryFor(baseUrl)
    }

    // ---- pairing -------------------------------------------------------

    fun onBaseUrlChanged(value: String) {
        _pairing.value = _pairing.value.copy(baseUrlInput = value, error = null)
    }

    fun onCodeChanged(value: String) {
        // Codes are drawn from an uppercase alphabet and compared case-
        // sensitively on the server, so force uppercase as the parent types.
        _pairing.value = _pairing.value.copy(codeInput = value.uppercase(), error = null)
    }

    fun submitPairing() {
        val state = _pairing.value
        if (!state.canSubmit) return

        val baseUrl = normalizeBaseUrl(state.baseUrlInput)
        if (baseUrl == null) {
            _pairing.value = state.copy(error = "That server address is not a valid URL.")
            return
        }

        _pairing.value = state.copy(submitting = true, error = null)
        scope.launch {
            container.settingsStore.setBaseUrl(baseUrl)
            activeBaseUrl = baseUrl
            when (val result = container.repositoryFor(baseUrl).pair(state.codeInput.uppercase())) {
                is ApiResult.Ok -> {
                    val pairing = result.value
                    Log.i(TAG, "pair ok device=${pairing.device.id} tokenLen=${pairing.token.length}")
                    container.settingsStore.savePairing(
                        token = pairing.token,
                        deviceId = pairing.device.id,
                        deviceName = pairing.device.name,
                        deviceKind = pairing.device.kind,
                    )
                    // Interceptor reads a cache, not DataStore. Publish the
                    // token before any authenticated call or the first catalog
                    // request races the Flow and comes back 401.
                    container.noteToken(pairing.token)
                    _pairing.value = PairingUiState(baseUrlInput = baseUrl)
                    _destination.value = Destination.CATALOG
                    refreshCatalog()
                }
                is ApiResult.Err -> {
                    Log.w(TAG, "pair failed: ${result.error}")
                    _pairing.value = _pairing.value.copy(
                        submitting = false,
                        error = result.error.message,
                    )
                }
            }
        }
    }

    fun unpair() {
        scope.launch {
            container.settingsStore.clearPairing()
            container.noteToken(null)
            activeBaseUrl = null
            container.grantStore.clear()
            _catalog.value = CatalogUiState()
            _consumedMediaItemIds.value = emptySet()
            _destination.value = Destination.PAIRING
        }
    }

    // ---- catalog -------------------------------------------------------

    fun refreshCatalog() {
        val repository = repository()
        if (repository == null) {
            Log.w(TAG, "refreshCatalog skipped: no repository (baseUrl=${activeBaseUrl ?: settings.value.baseUrl})")
            return
        }
        _catalog.value = _catalog.value.copy(loading = true, error = null)
        scope.launch {
            when (val result = repository.catalog()) {
                is ApiResult.Ok -> {
                    Log.i(TAG, "catalog ok items=${result.value.entries.size}")
                    _catalog.value = CatalogUiState(
                        loading = false,
                        view = CatalogPresenter.present(result.value, _consumedMediaItemIds.value),
                    )
                }
                is ApiResult.Err -> {
                    Log.w(TAG, "catalog failed: ${result.error}")
                    handleUnauthenticated(result.error)
                    _catalog.value = CatalogUiState(loading = false, error = result.error.message)
                }
            }
        }
    }

    // ---- channel -------------------------------------------------------

    /** Reloads what is on the channel now, from the server's clock. */
    fun refreshChannel() {
        val repository = repository() ?: return
        _channel.value = _channel.value.copy(loading = true, error = null)
        scope.launch {
            when (val result = repository.now()) {
                is ApiResult.Ok -> _channel.value = ChannelUiState(loading = false, now = result.value)
                is ApiResult.Err -> {
                    handleUnauthenticated(result.error)
                    _channel.value = ChannelUiState(loading = false, error = result.error.message)
                }
            }
        }
    }

    /** Loads today's grid for the EPG screen. */
    fun refreshEpg() {
        val repository = repository() ?: return
        _epg.value = _epg.value.copy(loading = true, error = null)
        scope.launch {
            // No day is passed: the client does not know the channel's timezone,
            // so the server's own "today" is the only honest question to ask.
            when (val result = repository.schedule()) {
                is ApiResult.Ok -> _epg.value = EpgUiState(loading = false, day = result.value)
                is ApiResult.Err -> {
                    handleUnauthenticated(result.error)
                    _epg.value = EpgUiState(loading = false, error = result.error.message)
                }
            }
        }
    }

    fun openChannel() {
        _destination.value = Destination.CHANNEL
        refreshChannel()
    }

    fun openEpg() {
        _destination.value = Destination.EPG
        refreshEpg()
    }

    /**
     * Tunes to the channel: requests a programmed grant for whatever is airing
     * and joins it at the server's offset.
     *
     * The programme is re-read from the channel state rather than passed in, so
     * a stale EPG cell cannot ask to play something that has since gone off air.
     */
    fun tuneIn() {
        val programme = _channel.value.onAir ?: return
        startPlayback(
            mediaItemId = programme.mediaItemId,
            mediaClass = programme.mediaClass,
            runtimeSeconds = programme.runtimeSeconds,
            mode = PlaybackMode.PROGRAMMED,
            scheduleEntryId = programme.scheduleEntryId,
            programme = programme,
        )
    }

    /**
     * Re-syncs programmed playback against the broadcast, for the app coming
     * back to the foreground.
     *
     * The local clock is never used to work out how much was missed: the server
     * is asked. If the channel has moved on, the session is closed and the
     * student is returned to the channel screen rather than left watching a
     * programme that is no longer airing.
     */
    fun resyncBroadcast() {
        val session = controller ?: return
        if (playbackMode != PlaybackMode.PROGRAMMED) return
        scope.launch {
            if (session.resyncBroadcast() == BroadcastSync.ProgrammeChanged) {
                stopPlayback(completed = false)
                refreshChannel()
            }
        }
    }

    /**
     * Returns to pairing when the device's token is no longer accepted. The
     * token was revoked or the server was rebuilt; there is nothing to show and
     * re-pairing is the only way out.
     */
    private suspend fun handleUnauthenticated(error: ApiError) {
        if (error !is ApiError.Unauthenticated) return
        container.settingsStore.clearPairing()
        container.noteToken(null)
        activeBaseUrl = null
        _destination.value = Destination.PAIRING
    }

    fun openDetail(mediaItemId: String) {
        _selectedMediaItemId.value = mediaItemId
        _destination.value = Destination.DETAIL
    }

    /** The versionCode currently running, shown on the settings screen. */
    fun installedVersionCode(): Int = container.updater?.installedVersionCode() ?: 0

    /**
     * Asks the server what it publishes. A server with no build uploaded, or
     * one that cannot be reached, simply leaves the device up to date: the box
     * should keep running what it has rather than nag about a failed check.
     */
    fun checkForUpdate() {
        val updater = container.updater ?: return
        val repository = repository() ?: return
        scope.launch {
            when (val result = repository.appRelease()) {
                is ApiResult.Ok -> _update.value = updater.stateFor(result.value)
                is ApiResult.Err -> _update.value = UpdateState.UpToDate
            }
        }
    }

    /** Downloads and installs the available update. */
    fun installUpdate() {
        val updater = container.updater ?: return
        val available = _update.value as? UpdateState.Available ?: return
        val baseUrl = settings.value.baseUrl ?: return
        _update.value = UpdateState.Downloading
        scope.launch {
            _update.value = updater.download(baseUrl, available.release, settings.value.token)
        }
    }

    fun openSettings() {
        _destination.value = Destination.SETTINGS
    }

    fun openPairing() {
        _destination.value = Destination.PAIRING
    }

    /** Handles a back gesture. Returns false when the shell should exit. */
    fun back(): Boolean = when (_destination.value) {
        Destination.CATALOG -> false
        Destination.PAIRING -> if (settings.value.isPaired) {
            _destination.value = Destination.CATALOG
            true
        } else {
            false
        }
        Destination.DETAIL, Destination.SETTINGS, Destination.CHANNEL -> {
            _destination.value = Destination.CATALOG
            true
        }
        Destination.EPG -> {
            _destination.value = Destination.CHANNEL
            true
        }
        // Back is the whole of the programmed player's transport: it exits.
        Destination.PLAYER -> {
            stopPlayback(completed = false)
            true
        }
    }

    // ---- playback ------------------------------------------------------

    /** Plays an on-demand catalog item. */
    fun play(mediaItemId: String) {
        val card = _catalog.value.card(mediaItemId) ?: return
        startPlayback(
            mediaItemId = mediaItemId,
            mediaClass = card.entry.mediaClass,
            runtimeSeconds = card.entry.runtimeSeconds,
            mode = PlaybackMode.ON_DEMAND,
        )
    }

    /**
     * Requests a grant and moves to the player. The controller is rebuilt per
     * attempt so a previous session's heartbeat loop cannot outlive it.
     */
    private fun startPlayback(
        mediaItemId: String,
        mediaClass: MediaClass,
        runtimeSeconds: Int,
        mode: PlaybackMode,
        scheduleEntryId: String? = null,
        programme: Programme? = null,
    ) {
        val repository = repository() ?: return

        val session = PlaybackSessionController(
            repository = repository,
            grantStore = container.grantStore,
            scope = scope,
        )
        controller = session
        playbackMode = mode
        tunedProgramme = programme
        _broadcastSeek.value = null
        _destination.value = Destination.PLAYER

        // Each attempt gets one mirror of the controller's state. The previous
        // one is cancelled first, otherwise every play would leave a collector
        // running for the life of the view model and a stale session could
        // overwrite the live one's state.
        playbackMirror?.cancel()
        playbackMirror = scope.launch {
            session.state.collect { state ->
                _playback.value = state
                if (state is PlaybackState.Playable && state.playConsumed) {
                    markConsumed(mediaItemId)
                }
            }
        }
        seekMirror?.cancel()
        seekMirror = scope.launch {
            session.broadcastSeek.collect { _broadcastSeek.value = it }
        }
        scope.launch {
            session.start(
                mediaItemId = mediaItemId,
                mediaClass = mediaClass,
                runtimeSeconds = runtimeSeconds,
                mode = mode,
                scheduleEntryId = scheduleEntryId,
            )
        }
    }

    /** Starts heartbeating against the live player. */
    fun attachPlayer(probe: PlaybackSessionController.PlayerProbe) {
        controller?.attachPlayer(probe)
    }

    /**
     * Stops heartbeating without closing the session, for the app going to the
     * background. The grant stays persisted so playback can resume.
     */
    fun detachPlayer() {
        val session = controller ?: return
        scope.launch { session.detachPlayer() }
    }

    /**
     * Closes the session and returns to the catalog.
     *
     * Guarded against a double call: the player reports both "ended" and the
     * back gesture, and completing a session twice would report a second
     * playback session to the server.
     */
    fun stopPlayback(completed: Boolean) {
        val session = controller
        val programmed = playbackMode == PlaybackMode.PROGRAMMED
        val mediaItemId = _selectedMediaItemId.value
        controller = null
        playbackMirror?.cancel()
        playbackMirror = null
        seekMirror?.cancel()
        seekMirror = null
        playbackMode = PlaybackMode.ON_DEMAND
        tunedProgramme = null
        _broadcastSeek.value = null
        _destination.value = when {
            // Leaving the channel returns to the channel, not to a catalog
            // detail page for a programme the student never chose.
            programmed -> Destination.CHANNEL
            mediaItemId != null -> Destination.DETAIL
            else -> Destination.CATALOG
        }

        if (session == null) {
            _playback.value = PlaybackState.Idle
            return
        }
        scope.launch {
            session.finish(completed)
            val finished = session.state.value
            if (!programmed && finished is PlaybackState.Finished && finished.playConsumed && mediaItemId != null) {
                markConsumed(mediaItemId)
            }
            _playback.value = PlaybackState.Idle
            if (programmed) refreshChannel() else refreshCatalog()
        }
    }

    private fun markConsumed(mediaItemId: String) {
        _consumedMediaItemIds.value = _consumedMediaItemIds.value + mediaItemId
        val view = _catalog.value.view ?: return
        _catalog.value = _catalog.value.copy(
            view = CatalogPresenter.present(
                com.aleksclark.primer.tv.core.domain.Catalog(
                    entries = view.cards.map { it.entry },
                    serverTime = view.serverTime,
                ),
                _consumedMediaItemIds.value,
            ),
        )
    }

    /** What the player being shown is allowed to do. */
    fun playbackControls(): PlaybackControls = PlaybackPolicy.controlsFor(playbackMode, playingMediaClass())

    /** The title the player chrome should show. */
    fun playingTitle(): String =
        tunedProgramme?.title
            ?: _selectedMediaItemId.value?.let { _catalog.value.card(it)?.entry?.title }
            ?: ""

    private fun playingMediaClass(): MediaClass =
        tunedProgramme?.mediaClass
            ?: _selectedMediaItemId.value?.let { _catalog.value.card(it)?.entry?.mediaClass }
            ?: MediaClass.UNKNOWN

    /** Chooses the first destination once persisted settings are known. */
    fun onSettingsLoaded(settings: DeviceSettings) {
        if (settings.baseUrl?.isNotBlank() == true) {
            activeBaseUrl = settings.baseUrl
        }
        if (!settings.isPaired && _destination.value != Destination.PAIRING) {
            _destination.value = Destination.PAIRING
            _pairing.value = _pairing.value.copy(baseUrlInput = settings.baseUrl.orEmpty())
        }
    }

    class Factory(private val container: AppContainer) : ViewModelProvider.Factory {
        @Suppress("UNCHECKED_CAST")
        override fun <T : ViewModel> create(modelClass: Class<T>): T = TvViewModel(container) as T
    }

    private companion object {
        const val TAG = "PrimerTV"
    }
}
