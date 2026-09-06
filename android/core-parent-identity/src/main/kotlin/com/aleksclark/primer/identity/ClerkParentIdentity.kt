package com.aleksclark.primer.identity

import android.app.Application
import com.clerk.api.Clerk
import com.clerk.api.network.serialization.ClerkResult
import com.clerk.api.session.GetTokenOptions
import com.clerk.api.session.fetchToken
import com.clerk.api.signin.SignIn
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.withTimeout
import kotlinx.coroutines.flow.first
import java.util.concurrent.atomic.AtomicBoolean

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
    private val initialized = AtomicBoolean(false)

    fun initialize() {
        if (!configured || !initialized.compareAndSet(false, true)) return
        try {
            Clerk.initialize(application, publishableKey)
        } catch (error: CancellationException) {
            initialized.set(false)
            throw error
        } catch (error: Exception) {
            initialized.set(false)
            throw error
        }
    }

    override suspend fun ready(): Boolean {
        if (!configured) return true
        return try {
            if (!initialized.get()) initialize()
            withTimeout(READY_TIMEOUT_MS) { Clerk.isInitialized.first { it } }
        } catch (error: CancellationException) {
            throw error
        } catch (_: Exception) {
            false
        }
    }

    override suspend fun isSignedIn(): Boolean {
        if (!configured) return false
        if (!ready()) return false
        return Clerk.session != null
    }

    override suspend fun sessionId(): String? = Clerk.session?.id

    override suspend fun sessionToken(skipCache: Boolean): String? {
        val session = Clerk.session ?: return null
        val options = GetTokenOptions(skipCache = skipCache, expirationBuffer = TOKEN_BUFFER_MS)
        return try {
            when (val result = session.fetchToken(options)) {
                is ClerkResult.Success -> result.value.jwt
                is ClerkResult.Failure -> null
            }
        } catch (error: CancellationException) {
            throw error
        }
    }

    override suspend fun signIn(email: String, password: String): SignInOutcome {
        if (!configured) return SignInOutcome.Failed("Clerk publishable key is not set on this build.")
        if (!ready()) return SignInOutcome.Failed("Clerk did not become ready. Check the network and try again.")
        val signIn = when (
            val result = try {
                SignIn.create(SignIn.CreateParams.Strategy.Password(identifier = email, password = password))
            } catch (error: CancellationException) {
                throw error
            } catch (_: Exception) {
                return SignInOutcome.Failed("Unable to reach Clerk. Check your connection and try again.")
            }
        ) {
            is ClerkResult.Success -> result.value
            is ClerkResult.Failure -> return SignInOutcome.Failed(clerkFailure(result, "Clerk sign-in failed. Check the account and try again."))
        }
        val sessionId = ClerkSignInPolicy.completedSessionId(signIn.status.name, signIn.createdSessionId)
            ?: return SignInOutcome.Incomplete(ClerkSignInPolicy.incompleteMessage(signIn.status.name))
        val activated = when (val result = try {
            Clerk.setActive(sessionId)
        } catch (error: CancellationException) {
            throw error
        } catch (_: Exception) {
            return SignInOutcome.Failed("Unable to activate the Clerk session. Try again.")
        }) {
            is ClerkResult.Success -> result.value.id
            is ClerkResult.Failure -> return SignInOutcome.Failed(clerkFailure(result, "Clerk could not activate the new session."))
        }
        return if (ClerkSignInPolicy.activated(sessionId, activated ?: Clerk.session?.id)) {
            SignInOutcome.SignedIn(sessionId)
        } else {
            SignInOutcome.Failed("Clerk did not activate the newly created session.")
        }
    }

    override suspend fun signOutProvider(): SignOutOutcome {
        if (!configured) return SignOutOutcome.SignedOut
        return when (val result = try {
            Clerk.signOut()
        } catch (error: CancellationException) {
            throw error
        } catch (_: Exception) {
            return SignOutOutcome.Failed("Unable to sign out of Clerk. Try again.")
        }) {
            is ClerkResult.Success -> SignOutOutcome.SignedOut
            is ClerkResult.Failure -> SignOutOutcome.Failed(clerkFailure(result, "Clerk sign-out failed. Try again."))
        }
    }

    private fun clerkFailure(result: ClerkResult.Failure<*>, fallback: String): String {
        val error = result.error
        val fromSdk = (error as? com.clerk.api.network.model.error.ClerkErrorResponse)
            ?.errors
            ?.firstOrNull()
            ?.longMessage
            ?: (error as? com.clerk.api.network.model.error.ClerkErrorResponse)?.errors?.firstOrNull()?.message
        return fromSdk ?: result.throwable?.message ?: fallback
    }

    companion object {
        private const val READY_TIMEOUT_MS = 8_000L
        private const val TOKEN_BUFFER_MS = 60_000L
    }
}
