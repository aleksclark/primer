package com.aleksclark.primer.identity

/**
 * Parent session adapter. Tokens stay in memory via the Clerk SDK.
 * Local household membership and server-side session revocation remain authority.
 */
interface ParentIdentity {
    val configured: Boolean
    suspend fun ready(): Boolean
    suspend fun isSignedIn(): Boolean
    suspend fun sessionToken(): String?
    suspend fun signIn()
    suspend fun signIn(email: String, password: String): Boolean = false
    suspend fun signOutProvider()
}
