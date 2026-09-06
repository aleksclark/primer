package com.aleksclark.primer.control

import android.app.Application
import com.aleksclark.primer.identity.ClerkParentIdentity

class ControlApp : Application() {
    lateinit var identity: ClerkParentIdentity
        private set

    override fun onCreate() {
        super.onCreate()
        identity = ClerkParentIdentity(this, BuildConfig.CLERK_PUBLISHABLE_KEY)
        identity.initialize()
    }
}
