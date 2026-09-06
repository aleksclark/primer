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
        assertTrue(ClerkSignInPolicy.incompleteMessage("UNKNOWN").contains("unsupported"))
    }

    @Test
    fun activationMustMatchCreatedIdNotAnyExistingSession() {
        assertTrue(ClerkSignInPolicy.activated("sess-new", "sess-new"))
        assertFalse(ClerkSignInPolicy.activated("sess-new", "sess-old"))
        assertFalse(ClerkSignInPolicy.activated("sess-new", null))
    }

    @Test
    fun compatibilityEvidenceIsPinnedToCompiledSdk() {
        assertEquals("com.clerk:clerk-android-api:0.1.31", ClerkSdkCompatibility.ARTIFACT)
        assertEquals("2.1.20", ClerkSdkCompatibility.DECLARED_KOTLIN_STDLIB)
        assertEquals("2.0.21", ClerkSdkCompatibility.TREE_KOTLIN)
        assertTrue(ClerkSdkCompatibility.NOTE.contains("live Clerk backend"))
    }
}
