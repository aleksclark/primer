package com.aleksclark.primer.identity

sealed class SignInOutcome {
    data class SignedIn(val sessionId: String) : SignInOutcome()
    data class NeedsSecondFactor(
        val strategies: List<String>,
        val selectedStrategy: String,
        val message: String,
    ) : SignInOutcome()
    data class Incomplete(val message: String) : SignInOutcome()
    data class Failed(val message: String) : SignInOutcome()
}

sealed class SignOutOutcome {
    data object SignedOut : SignOutOutcome()
    data class Failed(val message: String) : SignOutOutcome()
}

object ClerkSignInPolicy {
    const val STRATEGY_TOTP = "totp"
    const val STRATEGY_BACKUP_CODE = "backup_code"
    const val STRATEGY_PHONE_CODE = "phone_code"
    const val STRATEGY_EMAIL_CODE = "email_code"

    private val supportedSecondFactors = listOf(
        STRATEGY_TOTP,
        STRATEGY_BACKUP_CODE,
        STRATEGY_PHONE_CODE,
        STRATEGY_EMAIL_CODE,
    )

    fun completedSessionId(statusName: String, createdSessionId: String?): String? {
        if (statusName != "COMPLETE") return null
        return createdSessionId?.takeIf { it.isNotBlank() }
    }

    fun incompleteMessage(statusName: String): String = when (statusName) {
        "NEEDS_SECOND_FACTOR" -> "This account requires a second factor that Control cannot continue."
        "NEEDS_NEW_PASSWORD" -> "This account must set a new password before Control can sign in."
        "NEEDS_FIRST_FACTOR", "NEEDS_IDENTIFIER" -> "Sign-in is incomplete. Check the email and password, or continue with Google."
        "NEEDS_CLIENT_TRUST" -> "Clerk needs additional client trust before this session can be used."
        "MISSING" -> "Google sign-in did not return a Clerk session."
        "SIGN_UP" -> "This Google account is not an existing Primer parent. Control does not create households."
        "UNKNOWN" -> "Clerk returned an unsupported sign-in state."
        else -> "Clerk sign-in did not create a new session."
    }

    fun supportedSecondStrategies(strategyNames: List<String>): List<String> =
        strategyNames.map { it.trim().lowercase() }
            .filter { it in supportedSecondFactors }
            .distinct()

    /**
     * Clerk Android 0.1.31 [prepareSecondFactor] prefers SMS whenever both SMS and
     * email codes are enrolled. Do not offer email in that case; the parent can
     * still use authenticator, backup, or SMS.
     */
    fun offeredSecondStrategies(strategyNames: List<String>): List<String> {
        val supported = supportedSecondStrategies(strategyNames).toSet()
        val offered = if (STRATEGY_PHONE_CODE in supported) {
            supported - STRATEGY_EMAIL_CODE
        } else {
            supported
        }
        return listOf(STRATEGY_TOTP, STRATEGY_PHONE_CODE, STRATEGY_EMAIL_CODE, STRATEGY_BACKUP_CODE)
            .filter { it in offered }
    }

    fun defaultSecondStrategy(strategyNames: List<String>): String? =
        offeredSecondStrategies(strategyNames).firstOrNull()

    fun needsPrepare(strategy: String): Boolean =
        strategy == STRATEGY_PHONE_CODE || strategy == STRATEGY_EMAIL_CODE

    fun secondFactorPrompt(strategy: String): String = when (strategy) {
        STRATEGY_TOTP -> "Enter the authenticator code for this account."
        STRATEGY_BACKUP_CODE -> "Enter a Clerk backup code for this account."
        STRATEGY_PHONE_CODE -> "Enter the verification code sent to the enrolled phone."
        STRATEGY_EMAIL_CODE -> "Enter the verification code sent to the enrolled email."
        else -> "This second factor is not supported in Control."
    }

    fun secondFactorLabel(strategy: String): String = when (strategy) {
        STRATEGY_TOTP -> "Authenticator"
        STRATEGY_BACKUP_CODE -> "Backup code"
        STRATEGY_PHONE_CODE -> "SMS code"
        STRATEGY_EMAIL_CODE -> "Email code"
        else -> strategy
    }

    fun activated(expectedSessionId: String, activeSessionId: String?): Boolean =
        expectedSessionId.isNotBlank() && activeSessionId == expectedSessionId
}

/** Declared Clerk 0.1.31 coordinates. Not a live runtime claim. */
object ClerkSdkCompatibility {
    const val ARTIFACT = "com.clerk:clerk-android-api:0.1.31"
    const val DECLARED_KOTLIN_STDLIB = "2.1.20"
    const val DECLARED_SERIALIZATION_JSON = "1.9.0"
    const val DECLARED_COROUTINES = "1.10.2"
    const val DECLARED_BROWSER = "1.9.0"
    const val TREE_KOTLIN = "2.2.0"
    const val TREE_AGP = "8.10.0"
    const val NOTE = "1.1.x ships Kotlin 2.4 metadata and cannot compile on this tree. 0.1.31 is the last API that compiled. Unit tests do not initialize a live Clerk backend."
}
