package com.aleksclark.primer.control

import android.app.Application
import com.aleksclark.primer.identity.ClerkParentIdentity

class ControlApp : Application() {
    lateinit var identity: ClerkParentIdentity
        private set
    lateinit var updater: ControlSelfUpdateCoordinator
        private set

    override fun onCreate() {
        super.onCreate()
        identity = ClerkParentIdentity(this, BuildConfig.CLERK_PUBLISHABLE_KEY)
        identity.initialize()
        updater = ControlSelfUpdateCoordinator(this, BuildConfig.RELEASE_TRUST_ROOT)
    }
}
