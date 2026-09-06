package com.aleksclark.primer.identity

import android.app.Application
import com.clerk.api.Clerk
import com.clerk.api.network.serialization.ClerkResult
import com.clerk.api.session.fetchToken
import com.clerk.api.signin.SignIn
import kotlinx.coroutines.flow.first

/**
 * Official Clerk Android SDK 0.1.31, pinned because 1.1.x ships Kotlin 2.4 metadata
 * while this Gradle tree is Kotlin 2.0.21. Sign-in uses Clerk's password strategy
 * (same as the official 0.1.31 quickstart). Tokens stay in the SDK session.
 * No WebView password capture, JWT paste, cookie scraping, or /auth/callback shortcut.
 */
class ClerkParentIdentity(
    private val application: Application,
    private val publishableKey: String,
) : ParentIdentity {
    override val configured: Boolean = publishableKey.isNotBlank()
    @Volatile private var initialized = false

    fun initialize() {
        if (!configured || initialized) return
        Clerk.initialize(application, publishableKey)
        initialized = true
    }

    override suspend fun ready(): Boolean {
        if (!configured) return true
        initialize()
        return Clerk.isInitialized.first { it }
    }

    override suspend fun isSignedIn(): Boolean {
        if (!configured) return false
        ready()
        return Clerk.session != null
    }

    override suspend fun sessionToken(): String? {
        val session = Clerk.session ?: return null
        return when (val result = session.fetchToken()) {
            is ClerkResult.Success -> result.value.jwt
            is ClerkResult.Failure -> null
        }
    }

    override suspend fun signIn() {
        if (!configured) return
        ready()
        // Hosted Account Portal is not in 0.1.31. Control uses Clerk's official
        // password SignIn.create from that SDK version. Live Clerk acceptance is
        // still a parent/physical gate.
    }

    override suspend fun signIn(email: String, password: String): Boolean {
        if (!configured) return false
        ready()
        return when (
            val result = SignIn.create(SignIn.CreateParams.Strategy.Password(identifier = email, password = password))
        ) {
            is ClerkResult.Success -> true
            is ClerkResult.Failure -> false
        }
    }

    override suspend fun signOutProvider() {
        if (!configured) return
        Clerk.signOut()
    }
}
