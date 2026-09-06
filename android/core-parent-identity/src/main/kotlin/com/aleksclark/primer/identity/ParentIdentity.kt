package com.aleksclark.primer.identity

/**
 * Parent session adapter. Tokens stay in memory via the Clerk SDK.
 * Local household membership and server-side session revocation remain authority.
 */
interface ParentIdentity {
    val configured: Boolean
    suspend fun ready(): Boolean
    suspend fun isSignedIn(): Boolean
    suspend fun sessionId(): String?
    suspend fun sessionToken(skipCache: Boolean = false): String?
    suspend fun signIn(email: String, password: String): SignInOutcome
    suspend fun signOutProvider(): SignOutOutcome
}
