package com.aleksclark.primer.tv.app

import android.app.Application
import com.aleksclark.primer.tv.app.data.AppContainer
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob

/** Owns the process-wide dependency graph. */
class TvApplication : Application() {
    lateinit var container: AppContainer
        private set

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)

    override fun onCreate() {
        super.onCreate()
        container = AppContainer(this).also { it.start(scope) }
    }
}
