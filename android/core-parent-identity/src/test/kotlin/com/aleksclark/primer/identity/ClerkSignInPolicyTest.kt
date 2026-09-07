package com.aleksclark.primer.identity

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class ClerkSignInPolicyTest {
    @Test
    fun completeRequiresNewSessionId() {
        assertEquals("sess-new", ClerkSignInPolicy.completedSessionId("COMPLETE", "sess-new"))
        assertNull(ClerkSignInPolicy.completedSessionId("COMPLETE", null))
        assertNull(ClerkSignInPolicy.completedSessionId("COMPLETE", " "))
        assertNull(ClerkSignInPolicy.completedSessionId("NEEDS_SECOND_FACTOR", "sess-old"))
    }

    @Test
    fun incompleteStatesAreHonest() {
        assertTrue(ClerkSignInPolicy.incompleteMessage("NEEDS_SECOND_FACTOR").contains("second factor"))
        assertTrue(ClerkSignInPolicy.incompleteMessage("NEEDS_NEW_PASSWORD").contains("new password"))
        assertTrue(ClerkSignInPolicy.incompleteMessage("SIGN_UP").contains("does not create households"))
        assertTrue(ClerkSignInPolicy.incompleteMessage("UNKNOWN").contains("unsupported"))
    }

    @Test
    fun activationMustMatchCreatedIdNotAnyExistingSession() {
        assertTrue(ClerkSignInPolicy.activated("sess-new", "sess-new"))
        assertFalse(ClerkSignInPolicy.activated("sess-new", "sess-old"))
        assertFalse(ClerkSignInPolicy.activated("sess-new", null))
    }

    @Test
    fun offeredSecondFactorsPreferTotpAndHideEmailWhenSmsIsPresent() {
        val offered = ClerkSignInPolicy.offeredSecondStrategies(
            listOf("email_code", "phone_code", "totp", "backup_code", "web3_wallet"),
        )
        assertEquals(
            listOf("totp", "phone_code", "backup_code"),
            offered,
        )
        assertEquals("totp", ClerkSignInPolicy.defaultSecondStrategy(offered))
        assertFalse(ClerkSignInPolicy.needsPrepare("totp"))
        assertTrue(ClerkSignInPolicy.needsPrepare("phone_code"))
        assertTrue(ClerkSignInPolicy.secondFactorPrompt("totp").contains("authenticator"))
    }

    @Test
    fun emailCodeIsOfferedWhenSmsIsNotEnrolled() {
        val offered = ClerkSignInPolicy.offeredSecondStrategies(listOf("email_code"))
        assertEquals(listOf("email_code"), offered)
        assertEquals("email_code", ClerkSignInPolicy.defaultSecondStrategy(offered))
        assertTrue(ClerkSignInPolicy.needsPrepare("email_code"))
    }

    @Test
    fun unsupportedOnlySecondFactorStaysIncomplete() {
        assertTrue(ClerkSignInPolicy.offeredSecondStrategies(listOf("web3_wallet", "passkey")).isEmpty())
        assertNull(ClerkSignInPolicy.defaultSecondStrategy(listOf("web3_wallet")))
    }

    @Test
    fun compatibilityEvidenceIsPinnedToCompiledSdk() {
        assertEquals("com.clerk:clerk-android-api:0.1.31", ClerkSdkCompatibility.ARTIFACT)
        assertEquals("2.1.20", ClerkSdkCompatibility.DECLARED_KOTLIN_STDLIB)
        assertEquals("2.2.0", ClerkSdkCompatibility.TREE_KOTLIN)
        assertEquals("8.10.0", ClerkSdkCompatibility.TREE_AGP)
        assertTrue(ClerkSdkCompatibility.NOTE.contains("live Clerk backend"))
    }
}
