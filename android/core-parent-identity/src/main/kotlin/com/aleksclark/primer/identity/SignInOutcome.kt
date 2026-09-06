package com.aleksclark.primer.identity

sealed class SignInOutcome {
    data class SignedIn(val sessionId: String) : SignInOutcome()
    data class Incomplete(val message: String) : SignInOutcome()
    data class Failed(val message: String) : SignInOutcome()
}

sealed class SignOutOutcome {
    data object SignedOut : SignOutOutcome()
    data class Failed(val message: String) : SignOutOutcome()
}

object ClerkSignInPolicy {
    fun completedSessionId(statusName: String, createdSessionId: String?): String? {
        if (statusName != "COMPLETE") return null
        return createdSessionId?.takeIf { it.isNotBlank() }
    }

    fun incompleteMessage(statusName: String): String = when (statusName) {
        "NEEDS_SECOND_FACTOR" -> "This account requires a second factor. Control cannot complete MFA in this build."
        "NEEDS_NEW_PASSWORD" -> "This account must set a new password before Control can sign in."
        "NEEDS_FIRST_FACTOR", "NEEDS_IDENTIFIER" -> "Sign-in is incomplete. Check the email and password."
        "NEEDS_CLIENT_TRUST" -> "Clerk needs additional client trust before this session can be used."
        "UNKNOWN" -> "Clerk returned an unsupported sign-in state."
        else -> "Clerk sign-in did not create a new session."
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
    const val TREE_KOTLIN = "2.0.21"
    const val TREE_AGP = "8.7.3"
    const val NOTE = "1.1.x ships Kotlin 2.4 metadata and cannot compile on this tree. 0.1.31 is the last API that compiled. Unit tests do not initialize a live Clerk backend."
}
