package com.aleksclark.primer.identity

import android.app.Application
import com.clerk.api.Clerk
import com.clerk.api.network.model.error.ClerkErrorResponse
import com.clerk.api.network.serialization.ClerkResult
import com.clerk.api.session.GetTokenOptions
import com.clerk.api.session.fetchToken
import com.clerk.api.signin.SignIn
import com.clerk.api.signin.attemptSecondFactor
import com.clerk.api.signin.prepareSecondFactor
import com.clerk.api.sso.OAuthProvider
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withTimeout
import java.util.concurrent.atomic.AtomicBoolean

/**
 * Official Clerk Android SDK 0.1.31. Password or Google OAuth is the first
 * factor; TOTP, backup codes, SMS, and email codes continue through the same
 * SignIn object. Tokens stay in the SDK session. No WebView password capture,
 * JWT paste, cookie scraping, or /auth/callback shortcut.
 */
class ClerkParentIdentity(
    private val application: Application,
    private val publishableKey: String,
) : ParentIdentity {
    override val configured: Boolean = publishableKey.isNotBlank()
    private val initialized = AtomicBoolean(false)
    private val signInLock = Mutex()
    private var pendingSignIn: SignIn? = null

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
        signInLock.withLock { pendingSignIn = null }
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
        return finishSignIn(signIn, prepareIfNeeded = true)
    }

    override suspend fun signInWithGoogle(): SignInOutcome {
        if (!configured) return SignInOutcome.Failed("Clerk publishable key is not set on this build.")
        if (!ready()) return SignInOutcome.Failed("Clerk did not become ready. Check the network and try again.")
        signInLock.withLock { pendingSignIn = null }
        val oauth = when (
            val result = try {
                SignIn.authenticateWithRedirect(
                    SignIn.AuthenticateWithRedirectParams.OAuth(
                        provider = OAuthProvider.GOOGLE,
                        redirectUrl = ControlOriginPolicy.CLERK_NATIVE_OAUTH_REDIRECT,
                    ),
                )
            } catch (error: CancellationException) {
                throw error
            } catch (_: Exception) {
                return SignInOutcome.Failed("Unable to reach Google or Clerk. Check your connection and try again.")
            }
        ) {
            is ClerkResult.Success -> result.value
            is ClerkResult.Failure -> return SignInOutcome.Failed(
                clerkOauthFailure(result),
            )
        }
        oauth.signIn?.let { return finishSignIn(it, prepareIfNeeded = true) }
        if (oauth.signUp != null) {
            return SignInOutcome.Incomplete(ClerkSignInPolicy.incompleteMessage("SIGN_UP"))
        }
        return SignInOutcome.Incomplete(ClerkSignInPolicy.incompleteMessage("MISSING"))
    }

    override suspend fun prepareSecondFactor(strategy: String): SignInOutcome {
        if (!configured) return SignInOutcome.Failed("Clerk publishable key is not set on this build.")
        val current = signInLock.withLock { pendingSignIn }
            ?: return SignInOutcome.Failed("Sign-in expired. Enter email and password again.")
        val offered = ClerkSignInPolicy.offeredSecondStrategies(
            current.supportedSecondFactors.orEmpty().map { it.strategy },
        )
        if (strategy !in offered) {
            return SignInOutcome.Failed("That second factor is not available for this account.")
        }
        val prepared = if (ClerkSignInPolicy.needsPrepare(strategy)) {
            prepareSelected(current, strategy) ?: return keepPending(
                current,
                SignInOutcome.Failed("Clerk could not send the second-factor code. Try again."),
            )
        } else {
            current
        }
        signInLock.withLock { pendingSignIn = prepared }
        return SignInOutcome.NeedsSecondFactor(
            strategies = offered,
            selectedStrategy = strategy,
            message = ClerkSignInPolicy.secondFactorPrompt(strategy),
        )
    }

    override suspend fun continueSecondFactor(code: String, strategy: String): SignInOutcome {
        if (!configured) return SignInOutcome.Failed("Clerk publishable key is not set on this build.")
        val trimmed = code.trim()
        if (trimmed.isEmpty()) return SignInOutcome.Failed("Enter the second-factor code.")
        val current = signInLock.withLock { pendingSignIn }
            ?: return SignInOutcome.Failed("Sign-in expired. Enter email and password again.")
        val offered = ClerkSignInPolicy.offeredSecondStrategies(
            current.supportedSecondFactors.orEmpty().map { it.strategy },
        )
        if (strategy !in offered) {
            return SignInOutcome.Failed("That second factor is not available for this account.")
        }
        val attempted = when (
            val result = try {
                current.attemptSecondFactor(secondFactorParams(strategy, trimmed))
            } catch (error: CancellationException) {
                throw error
            } catch (_: Exception) {
                return keepPending(current, SignInOutcome.Failed("Unable to reach Clerk. Check your connection and try again."))
            }
        ) {
            is ClerkResult.Success -> result.value
            is ClerkResult.Failure -> return keepPending(
                current,
                SignInOutcome.Failed(clerkFailure(result, "That second-factor code was not accepted. Try again.")),
            )
        }
        return finishSignIn(attempted, prepareIfNeeded = false)
    }

    override suspend fun cancelIncompleteSignIn() {
        signInLock.withLock { pendingSignIn = null }
    }

    override suspend fun signOutProvider(): SignOutOutcome {
        if (!configured) return SignOutOutcome.SignedOut
        signInLock.withLock { pendingSignIn = null }
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

    private suspend fun finishSignIn(signIn: SignIn, prepareIfNeeded: Boolean): SignInOutcome {
        ClerkSignInPolicy.completedSessionId(signIn.status.name, signIn.createdSessionId)?.let { sessionId ->
            signInLock.withLock { pendingSignIn = null }
            return activate(sessionId)
        }
        if (signIn.status.name != "NEEDS_SECOND_FACTOR") {
            signInLock.withLock { pendingSignIn = null }
            return SignInOutcome.Incomplete(ClerkSignInPolicy.incompleteMessage(signIn.status.name))
        }
        val offered = ClerkSignInPolicy.offeredSecondStrategies(
            signIn.supportedSecondFactors.orEmpty().map { it.strategy },
        )
        val selected = ClerkSignInPolicy.defaultSecondStrategy(offered)
        if (selected == null || offered.isEmpty()) {
            signInLock.withLock { pendingSignIn = null }
            return SignInOutcome.Incomplete(ClerkSignInPolicy.incompleteMessage("NEEDS_SECOND_FACTOR"))
        }
        val prepared = if (prepareIfNeeded && ClerkSignInPolicy.needsPrepare(selected)) {
            prepareSelected(signIn, selected) ?: return SignInOutcome.Failed(
                "Clerk could not send the second-factor code. Try again.",
            )
        } else {
            signIn
        }
        signInLock.withLock { pendingSignIn = prepared }
        return SignInOutcome.NeedsSecondFactor(
            strategies = offered,
            selectedStrategy = selected,
            message = ClerkSignInPolicy.secondFactorPrompt(selected),
        )
    }

    private suspend fun prepareSelected(signIn: SignIn, strategy: String): SignIn? {
        val result = try {
            when (strategy) {
                ClerkSignInPolicy.STRATEGY_PHONE_CODE -> signIn.prepareSecondFactor(phoneNumberId = null)
                ClerkSignInPolicy.STRATEGY_EMAIL_CODE -> signIn.prepareSecondFactor(emailAddressId = null)
                else -> return signIn
            }
        } catch (error: CancellationException) {
            throw error
        } catch (_: Exception) {
            return null
        }
        return when (result) {
            is ClerkResult.Success -> result.value
            is ClerkResult.Failure -> null
        }
    }

    private suspend fun activate(sessionId: String): SignInOutcome {
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

    private suspend fun keepPending(signIn: SignIn, outcome: SignInOutcome): SignInOutcome {
        signInLock.withLock { pendingSignIn = signIn }
        return outcome
    }

    private fun secondFactorParams(strategy: String, code: String): SignIn.AttemptSecondFactorParams =
        when (strategy) {
            ClerkSignInPolicy.STRATEGY_TOTP -> SignIn.AttemptSecondFactorParams.TOTP(code = code)
            ClerkSignInPolicy.STRATEGY_BACKUP_CODE -> SignIn.AttemptSecondFactorParams.BackupCode(code = code)
            ClerkSignInPolicy.STRATEGY_PHONE_CODE -> SignIn.AttemptSecondFactorParams.PhoneCode(code = code)
            ClerkSignInPolicy.STRATEGY_EMAIL_CODE -> SignIn.AttemptSecondFactorParams.EmailCode(code = code)
            else -> error("unsupported second factor")
        }

    private fun clerkOauthFailure(result: ClerkResult.Failure<*>): String {
        val message = clerkFailure(result, "Google sign-in was cancelled or not accepted. Try again.")
        val redirect = ControlOriginPolicy.CLERK_NATIVE_OAUTH_REDIRECT
        return if (message.contains("redirect", ignoreCase = true) || message.contains("authorized", ignoreCase = true)) {
            "Clerk rejected the native Google return URL. In Clerk Dashboard → Paths / Redirect URLs, allow $redirect. MFA and authorized parties stay unchanged."
        } else {
            message
        }
    }

    private fun clerkFailure(result: ClerkResult.Failure<*>, fallback: String): String {
        val error = result.error
        val fromSdk = (error as? ClerkErrorResponse)
            ?.errors
            ?.firstOrNull()
            ?.longMessage
            ?: (error as? ClerkErrorResponse)?.errors?.firstOrNull()?.message
        return fromSdk ?: result.throwable?.message ?: fallback
    }

    companion object {
        private const val READY_TIMEOUT_MS = 8_000L
        private const val TOKEN_BUFFER_MS = 60_000L
    }
}
