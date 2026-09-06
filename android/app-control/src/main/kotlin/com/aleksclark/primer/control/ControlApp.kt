package com.aleksclark.primer.control

import android.app.Application
import com.aleksclark.primer.identity.ClerkParentIdentity

class ControlApp : Application() {
    lateinit var identity: ClerkParentIdentity
        private set
    lateinit var updater: ControlSelfUpdateCoordinator
        private set
    @Volatile var resumedActivity: android.app.Activity? = null

    override fun onCreate() {
        super.onCreate()
        identity = ClerkParentIdentity(this, BuildConfig.CLERK_PUBLISHABLE_KEY)
        identity.initialize()
        updater = ControlSelfUpdateCoordinator(
            context = this,
            trustRoot = BuildConfig.RELEASE_TRUST_ROOT,
            presenter = AndroidUserActionPresenter(this) { resumedActivity },
        )
    }
}
