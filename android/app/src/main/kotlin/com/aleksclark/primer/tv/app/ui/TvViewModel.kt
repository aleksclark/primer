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
import com.aleksclark.primer.tv.core.domain.Catalog
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
import com.aleksclark.primer.tv.core.presentation.HeroAction
import com.aleksclark.primer.tv.core.presentation.HeroModel
import com.aleksclark.primer.tv.core.presentation.HomePresenter
import com.aleksclark.primer.tv.core.presentation.HomeUiState
import com.aleksclark.primer.tv.core.presentation.PairingPresenter
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.async
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.delay
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

    private val _home = MutableStateFlow(HomeUiState(loading = true, hero = HeroModel.Loading))
    val home: StateFlow<HomeUiState> = _home.asStateFlow()

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

    /** Last successful channel snapshot used by Home when the Channel screen is idle. */
    private var homeChannelNow = _channel.value.now

    private var controller: PlaybackSessionController? = null
    private var playbackMirror: Job? = null
    private var seekMirror: Job? = null

    private val _playback = MutableStateFlow<PlaybackState>(PlaybackState.Idle)
    val playback: StateFlow<PlaybackState> = _playback.asStateFlow()

    private val _update = MutableStateFlow<UpdateState>(UpdateState.UpToDate)
    val update: StateFlow<UpdateState> = _update.asStateFlow()

    /**
     * Short-lived non-blocking feedback (update checks, soft refresh notes).
     * Cleared automatically; never replaces a whole screen.
     */
    private val _statusMessage = MutableStateFlow<StatusMessage?>(null)
    val statusMessage: StateFlow<StatusMessage?> = _statusMessage.asStateFlow()
    private var statusJob: Job? = null

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
     * Last play request so a recoverable grant failure can retry without the
     * student re-navigating. Cleared when the session ends successfully or the
     * failure is permanent.
     */
    private var lastPlayRequest: PlayRequest? = null

    /**
     * Top-level screen that opened the current detail page so back restores
     * Channel/Guide/Settings origin instead of always dumping to Home.
     */
    private var detailOrigin: Destination = Destination.CATALOG

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
        _pairing.value = _pairing.value.copy(
            codeInput = PairingPresenter.normalizeCode(value),
            error = null,
        )
    }

    fun submitPairing() {
        val state = _pairing.value
        if (!state.canSubmit) return

        val baseUrl = normalizeBaseUrl(state.baseUrlInput)
        if (baseUrl == null) {
            _pairing.value = state.copy(error = "That server address is not a valid URL.")
            return
        }

        val code = PairingPresenter.normalizeCode(state.codeInput)
        _pairing.value = state.copy(submitting = true, error = null, codeInput = code)
        scope.launch {
            container.settingsStore.setBaseUrl(baseUrl)
            activeBaseUrl = baseUrl
            when (val result = container.repositoryFor(baseUrl).pair(code)) {
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
                    refreshHome()
                }
                is ApiResult.Err -> {
                    Log.w(TAG, "pair failed: ${result.error}")
                    val message = PairingPresenter.failureMessage(result.error.message).value
                    // Stay on the pairing card with the server address intact so
                    // the parent only retypes the code.
                    _pairing.value = _pairing.value.copy(
                        submitting = false,
                        error = message,
                        baseUrlInput = _pairing.value.baseUrlInput.ifBlank { baseUrl },
                    )
                    _destination.value = Destination.PAIRING
                }
            }
        }
    }

    fun unpair() {
        scope.launch {
            // Keep the server address on the form so re-pairing is one field.
            val retainedUrl = retainedServerUrl()
            container.settingsStore.clearPairing()
            container.noteToken(null)
            activeBaseUrl = null
            container.grantStore.clear()
            _catalog.value = CatalogUiState()
            _home.value = HomeUiState(loading = false, hero = HeroModel.Empty())
            homeChannelNow = null
            _consumedMediaItemIds.value = emptySet()
            _pairing.value = PairingUiState(baseUrlInput = retainedUrl)
            _destination.value = Destination.PAIRING
        }
    }

    // ---- home / catalog ------------------------------------------------

    /**
     * Reloads Home inputs (catalog + on-now). Preserves existing content while
     * refreshing so scroll/focus are not destroyed.
     */
    fun refreshHome() {
        val repository = repository()
        if (repository == null) {
            Log.w(TAG, "refreshHome skipped: no repository (baseUrl=${activeBaseUrl ?: settings.value.baseUrl})")
            return
        }
        _home.value = HomePresenter.beginRefresh(_home.value)
        val hadCatalog = _catalog.value.view != null
        _catalog.value = _catalog.value.copy(
            loading = !hadCatalog,
            error = if (hadCatalog) _catalog.value.error else null,
        )
        scope.launch {
            coroutineScope {
                val catalogDeferred = async { repository.catalog() }
                val nowDeferred = async { repository.now() }
                val catalogResult = catalogDeferred.await()
                val nowResult = nowDeferred.await()

                var catalogError: String? = null
                var nextCatalog: Catalog? = _catalog.value.catalog
                var nextView = _catalog.value.view

                when (catalogResult) {
                    is ApiResult.Ok -> {
                        Log.i(TAG, "catalog ok items=${catalogResult.value.entries.size}")
                        // Keep in-session Watched marks for titles still listed;
                        // only drop marks the server has already removed.
                        val retainedConsumed = retainConsumedStillListed(
                            consumed = _consumedMediaItemIds.value,
                            catalog = catalogResult.value,
                        )
                        _consumedMediaItemIds.value = retainedConsumed
                        nextCatalog = catalogResult.value
                        nextView = CatalogPresenter.present(catalogResult.value, retainedConsumed)
                        _catalog.value = CatalogUiState(
                            loading = false,
                            view = nextView,
                            catalog = nextCatalog,
                        )
                    }
                    is ApiResult.Err -> {
                        Log.w(TAG, "catalog failed: ${catalogResult.error}")
                        handleUnauthenticated(catalogResult.error)
                        catalogError = catalogResult.error.message
                        if (hadCatalog) {
                            _catalog.value = _catalog.value.copy(loading = false)
                        } else {
                            _catalog.value = CatalogUiState(loading = false, error = catalogError)
                        }
                    }
                }

                when (nowResult) {
                    is ApiResult.Ok -> {
                        homeChannelNow = nowResult.value
                        // Keep Channel screen in sync when it already has state or is visible.
                        if (_destination.value == Destination.CHANNEL || _channel.value.now != null) {
                            _channel.value = ChannelUiState(loading = false, now = nowResult.value)
                        } else {
                            _channel.value = _channel.value.copy(loading = false, now = nowResult.value)
                        }
                    }
                    is ApiResult.Err -> {
                        handleUnauthenticated(nowResult.error)
                        // Channel failure alone should not wipe Home content.
                        if (_channel.value.now == null) {
                            _channel.value = ChannelUiState(loading = false, error = nowResult.error.message)
                        } else {
                            _channel.value = _channel.value.copy(loading = false)
                        }
                        if (catalogError == null && nextView == null) {
                            catalogError = nowResult.error.message
                        }
                    }
                }

                recomputeHome(
                    catalog = nextCatalog,
                    channelNow = homeChannelNow,
                    loading = false,
                    refreshing = false,
                    error = catalogError,
                )
            }
        }
    }

    /** Catalog-only refresh used after on-demand playback finishes. */
    fun refreshCatalog() {
        val repository = repository()
        if (repository == null) {
            Log.w(TAG, "refreshCatalog skipped: no repository (baseUrl=${activeBaseUrl ?: settings.value.baseUrl})")
            return
        }
        val hadContent = _catalog.value.view != null
        _catalog.value = _catalog.value.copy(loading = !hadContent, error = if (hadContent) null else _catalog.value.error)
        if (hadContent) {
            _home.value = HomePresenter.beginRefresh(_home.value)
        }
        scope.launch {
            when (val result = repository.catalog()) {
                is ApiResult.Ok -> {
                    Log.i(TAG, "catalog ok items=${result.value.entries.size}")
                    val retainedConsumed = retainConsumedStillListed(
                        consumed = _consumedMediaItemIds.value,
                        catalog = result.value,
                    )
                    _consumedMediaItemIds.value = retainedConsumed
                    val view = CatalogPresenter.present(result.value, retainedConsumed)
                    _catalog.value = CatalogUiState(
                        loading = false,
                        view = view,
                        catalog = result.value,
                    )
                    recomputeHome(
                        catalog = result.value,
                        channelNow = homeChannelNow,
                        loading = false,
                        refreshing = false,
                        error = null,
                    )
                }
                is ApiResult.Err -> {
                    Log.w(TAG, "catalog failed: ${result.error}")
                    handleUnauthenticated(result.error)
                    if (hadContent) {
                        _catalog.value = _catalog.value.copy(loading = false)
                        _home.value = HomePresenter.applyRefreshFailure(_home.value, result.error.message)
                    } else {
                        _catalog.value = CatalogUiState(loading = false, error = result.error.message)
                        recomputeHome(
                            catalog = null,
                            channelNow = homeChannelNow,
                            loading = false,
                            refreshing = false,
                            error = result.error.message,
                        )
                    }
                }
            }
        }
    }

    private fun recomputeHome(
        catalog: Catalog? = _catalog.value.catalog,
        channelNow: com.aleksclark.primer.tv.core.domain.ChannelNow? = homeChannelNow,
        loading: Boolean = false,
        refreshing: Boolean = false,
        error: String? = null,
    ) {
        _home.value = HomePresenter.present(
            catalog = catalog,
            channelNow = channelNow,
            consumedMediaItemIds = _consumedMediaItemIds.value,
            loading = loading,
            refreshing = refreshing,
            error = error,
        )
    }

    // ---- channel -------------------------------------------------------

    /** Reloads what is on the channel now, from the server's clock. */
    fun refreshChannel() {
        val repository = repository() ?: return
        // Keep any prior on-now snapshot visible while reloading so a failed
        // refresh cannot flash the screen empty or drop Watch Live.
        _channel.value = _channel.value.copy(loading = true, error = null)
        scope.launch {
            when (val result = repository.now()) {
                is ApiResult.Ok -> {
                    homeChannelNow = result.value
                    _channel.value = ChannelUiState(loading = false, now = result.value)
                    // Keep Home hero aligned with live state when channel refreshes.
                    if (_catalog.value.catalog != null || _home.value.hasContent) {
                        recomputeHome(
                            catalog = _catalog.value.catalog,
                            channelNow = result.value,
                            loading = false,
                            refreshing = false,
                            error = _home.value.error?.value,
                        )
                    }
                }
                is ApiResult.Err -> {
                    handleUnauthenticated(result.error)
                    // Preserve prior on-now content; surface a non-blocking error.
                    _channel.value = _channel.value.copy(
                        loading = false,
                        error = result.error.message,
                    )
                }
            }
        }
    }

    /** Loads today's grid for the EPG screen. */
    fun refreshEpg() {
        val repository = repository() ?: return
        // Keep the last loaded day mounted across refresh failures.
        _epg.value = _epg.value.copy(loading = true, error = null)
        scope.launch {
            // No day is passed: the client does not know the channel's timezone,
            // so the server's own "today" is the only honest question to ask.
            when (val result = repository.schedule()) {
                is ApiResult.Ok -> _epg.value = EpgUiState(loading = false, day = result.value)
                is ApiResult.Err -> {
                    handleUnauthenticated(result.error)
                    _epg.value = _epg.value.copy(
                        loading = false,
                        error = result.error.message,
                    )
                }
            }
        }
    }

    /** Opens Home (catalog) as a top-level destination. */
    fun openHome() {
        _destination.value = Destination.CATALOG
        if (_catalog.value.view == null && !_catalog.value.loading) {
            refreshHome()
        }
    }

    /** Primary action on the Home hero. */
    fun playHero() {
        when (val hero = _home.value.hero) {
            is HeroModel.Live -> {
                // Prefer joining live immediately from Home when we already have
                // a tunable on-air snapshot; otherwise open the Channel screen.
                val programme = homeChannelNow?.onAir
                    ?.takeIf { it.scheduleEntryId == hero.scheduleEntryId && it.directPlayOk }
                if (programme != null) {
                    _channel.value = ChannelUiState(loading = false, now = homeChannelNow)
                    startPlayback(
                        mediaItemId = programme.mediaItemId,
                        mediaClass = programme.mediaClass,
                        runtimeSeconds = programme.runtimeSeconds,
                        mode = PlaybackMode.PROGRAMMED,
                        scheduleEntryId = programme.scheduleEntryId,
                        programme = programme,
                    )
                } else {
                    openChannel()
                }
            }
            is HeroModel.Featured -> when (hero.primaryAction) {
                HeroAction.PLAY -> {
                    if (hero.card.playable) {
                        openDetail(hero.mediaItemId)
                        play(hero.mediaItemId)
                    } else {
                        openDetail(hero.mediaItemId)
                    }
                }
                HeroAction.VIEW_DETAILS -> openDetail(hero.mediaItemId)
                else -> openDetail(hero.mediaItemId)
            }
            else -> Unit
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
        // Only join when the box can actually decode the airing title. The UI
        // disables Watch Live for non-direct-play, but guard here too so a stale
        // event cannot request a programmed grant that will fail at the player.
        val programme = _channel.value.onAir?.takeIf { it.directPlayOk } ?: return
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
     *
     * Credentials are cleared at most once per bounce so concurrent 401s from
     * catalog + channel do not thrash storage or wipe the inline message.
     */
    private suspend fun handleUnauthenticated(error: ApiError) {
        if (error !is ApiError.Unauthenticated) return
        if (_destination.value == Destination.PAIRING && settings.value.token.isNullOrBlank()) {
            // Already bounced; keep the retained URL and existing explanation.
            return
        }
        val retainedUrl = retainedServerUrl()
        container.settingsStore.clearPairing()
        container.noteToken(null)
        activeBaseUrl = null
        _pairing.value = PairingUiState(
            baseUrlInput = retainedUrl,
            error = error.message?.takeIf { it.isNotBlank() }
                ?: "This device must be paired again.",
        )
        _destination.value = Destination.PAIRING
    }

    /** Server address kept across unpair / token-revoke for the pairing form. */
    private fun retainedServerUrl(): String =
        activeBaseUrl?.takeIf { it.isNotBlank() }
            ?: settings.value.baseUrl?.takeIf { it.isNotBlank() }
            ?: _pairing.value.baseUrlInput

    fun openDetail(mediaItemId: String) {
        detailOrigin = when (val current = _destination.value) {
            Destination.CATALOG,
            Destination.CHANNEL,
            Destination.EPG,
            Destination.SETTINGS,
            -> current
            Destination.DETAIL, Destination.PLAYER, Destination.PAIRING -> detailOrigin
        }
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
        // Non-blocking: navigation stays free while the check runs.
        scope.launch {
            when (val result = repository.appRelease()) {
                is ApiResult.Ok -> {
                    val next = updater.stateFor(result.value)
                    _update.value = next
                    when (next) {
                        is UpdateState.Available -> showStatus(
                            "Version ${next.release.versionCode} is available.",
                        )
                        UpdateState.UpToDate -> showStatus("This device is up to date.")
                        else -> Unit
                    }
                }
                is ApiResult.Err -> {
                    // Unreachable / empty publish is not an error for the box.
                    _update.value = UpdateState.UpToDate
                    showStatus("This device is up to date.")
                }
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
            val next = updater.download(baseUrl, available.release, settings.value.token)
            _update.value = next
            if (next is UpdateState.Failed) {
                showStatus(next.message, isError = true)
            }
        }
    }

    /** Publishes a short-lived banner and clears it without blocking navigation. */
    fun showStatus(message: String, isError: Boolean = false, holdMs: Long = 3_500L) {
        statusJob?.cancel()
        _statusMessage.value = StatusMessage(text = message, isError = isError)
        statusJob = scope.launch {
            delay(holdMs)
            _statusMessage.value = null
        }
    }

    fun dismissStatus() {
        statusJob?.cancel()
        _statusMessage.value = null
    }

    fun openSettings() {
        _destination.value = Destination.SETTINGS
    }

    fun openPairing() {
        _destination.value = Destination.PAIRING
    }

    /**
     * Handles a back gesture. Returns false when the shell should exit.
     *
     * Top-level destinations (Home/Channel/Guide/Settings) treat non-Home as a
     * peer switch: back returns to Home rather than walking a nested stack.
     * Details restore [detailOrigin]; the player exits through [stopPlayback].
     */
    fun back(): Boolean = when (_destination.value) {
        Destination.CATALOG -> false
        Destination.PAIRING -> if (settings.value.isPaired) {
            _destination.value = Destination.CATALOG
            true
        } else {
            false
        }
        Destination.DETAIL -> {
            _destination.value = detailOrigin
            true
        }
        Destination.SETTINGS, Destination.CHANNEL, Destination.EPG -> {
            _destination.value = Destination.CATALOG
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
        // Consumed entertainment must not request another grant this session.
        // Guard both playable and alreadyWatched so a stale card cannot slip
        // through if presentation and domain drift.
        if (!card.playable || card.alreadyWatched) return
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

        lastPlayRequest = PlayRequest(
            mediaItemId = mediaItemId,
            mediaClass = mediaClass,
            runtimeSeconds = runtimeSeconds,
            mode = mode,
            scheduleEntryId = scheduleEntryId,
            programme = programme,
        )

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

    /**
     * Retries the last play attempt after a recoverable failure (network blip).
     * Permanent refusals and watched entertainment are not retried.
     */
    fun retryPlayback() {
        val request = lastPlayRequest ?: return
        if (request.mode == PlaybackMode.ON_DEMAND) {
            val card = _catalog.value.card(request.mediaItemId)
            if (card != null && (!card.playable || card.alreadyWatched)) return
        }
        // Drop the failed controller first so its collectors cannot overwrite
        // the retry's state, and so we do not leave a second heartbeat alive.
        val failed = controller
        controller = null
        playbackMirror?.cancel()
        playbackMirror = null
        seekMirror?.cancel()
        seekMirror = null
        if (failed != null) {
            scope.launch { runCatching { failed.finish(completed = false) } }
        }
        startPlayback(
            mediaItemId = request.mediaItemId,
            mediaClass = request.mediaClass,
            runtimeSeconds = request.runtimeSeconds,
            mode = request.mode,
            scheduleEntryId = request.scheduleEntryId,
            programme = request.programme,
        )
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
        lastPlayRequest = null
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

    /** Snapshot of one play attempt, used only for recoverable retries. */
    private data class PlayRequest(
        val mediaItemId: String,
        val mediaClass: MediaClass,
        val runtimeSeconds: Int,
        val mode: PlaybackMode,
        val scheduleEntryId: String?,
        val programme: Programme?,
    )

    private fun markConsumed(mediaItemId: String) {
        _consumedMediaItemIds.value = _consumedMediaItemIds.value + mediaItemId
        val catalog = _catalog.value.catalog
            ?: _catalog.value.view?.let { view ->
                Catalog(
                    entries = view.cards.map { it.entry },
                    serverTime = view.serverTime,
                )
            }
            ?: return
        val view = CatalogPresenter.present(catalog, _consumedMediaItemIds.value)
        _catalog.value = _catalog.value.copy(view = view, catalog = catalog)
        recomputeHome(
            catalog = catalog,
            channelNow = homeChannelNow,
            loading = false,
            refreshing = false,
            error = _home.value.error?.value,
        )
    }

    /**
     * Session Watched marks stay until the server stops listing the title.
     * Clearing them on every refresh would re-enable Play for entertainment
     * the student already finished during this app session.
     */
    private fun retainConsumedStillListed(
        consumed: Set<String>,
        catalog: Catalog,
    ): Set<String> {
        if (consumed.isEmpty()) return emptySet()
        val listed = catalog.entries.mapTo(mutableSetOf()) { it.mediaItemId }
        return consumed.intersect(listed)
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
