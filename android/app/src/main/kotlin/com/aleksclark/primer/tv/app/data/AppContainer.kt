package com.aleksclark.primer.tv.app.data

import android.content.Context
import com.aleksclark.primer.tv.app.update.AppUpdater
import com.aleksclark.primer.tv.core.data.GrantStore
import com.aleksclark.primer.tv.core.data.SettingsStore
import com.aleksclark.primer.tv.core.data.TvRepository
import com.aleksclark.primer.tv.core.net.buildOkHttpClient
import com.aleksclark.primer.tv.core.net.buildTvApi
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.launch
import okhttp3.OkHttpClient

/**
 * Hand-rolled dependency graph. The app has one screenful of dependencies, so a
 * DI framework would cost more than it saves.
 */
class AppContainer(
    val settingsStore: SettingsStore,
    val grantStore: GrantStore,
) {
    constructor(context: Context) : this(
        settingsStore = DataStoreSettingsStore(context),
        grantStore = DataStoreGrantStore(context),
    ) {
        updater = AppUpdater(context.applicationContext, httpClient)
    }

    /**
     * The device token, mirrored so the OkHttp interceptor can read it without
     * blocking. Reading DataStore inside an interceptor would suspend a network
     * thread on disk IO on every request, which on the RK3318 box is a real
     * stall; the mirror is updated by [start] instead.
     */
    @Volatile
    private var cachedToken: String? = null

    /**
     * Shared across API calls and ExoPlayer so playback carries the same device
     * token, and so both reuse one connection pool on a memory-tight box.
     */
    val httpClient: OkHttpClient = buildOkHttpClient(token = { cachedToken })

    /**
     * Self-update, when the platform can supply one. Null in a unit test, where
     * there is no Context and nothing to install onto.
     */
    var updater: AppUpdater? = null
        private set

    /** Begins mirroring the persisted token. Called once at application start. */
    fun start(scope: CoroutineScope) {
        scope.launch {
            settingsStore.settings.collect { cachedToken = it.token }
        }
    }

    /**
     * Repositories are cached per base URL: Retrofit binds a base URL at
     * construction, so changing the server address must rebuild the client.
     */
    private var cachedBaseUrl: String? = null
    private var cachedRepository: TvRepository? = null

    @Synchronized
    fun repositoryFor(baseUrl: String): TvRepository {
        val cached = cachedRepository
        if (cached != null && cachedBaseUrl == baseUrl) return cached
        val repository = TvRepository(buildTvApi(baseUrl, httpClient))
        cachedBaseUrl = baseUrl
        cachedRepository = repository
        return repository
    }
}
