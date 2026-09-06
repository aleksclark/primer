package com.aleksclark.primer.updates

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class SelfUpdatePolicyTest {
    private val installed = ArchiveIdentity("com.aleksclark.primer.control", 1, setOf("key-a"), 26, emptySet())
    private val newer = installed.copy(version = 2)

    @Test
    fun ordinaryAppMayBeUnattendedWhenSamePackageSignerAndSdk31() {
        val eligibility = SelfUpdatePolicy.decide(
            installed = installed,
            archive = newer,
            sdk = 35,
            canRequestUnattended = true,
            unknownSourcesAllowed = true,
            deviceOwner = false,
        )
        assertTrue(eligibility.canAttempt)
        assertTrue(eligibility.unattendedEligible)
        assertFalse(eligibility.userActionRequired)
    }

    @Test
    fun unattendedIsNeverAssumedFromDeviceOwnerForControl() {
        val eligibility = SelfUpdatePolicy.decide(
            installed = installed,
            archive = newer,
            sdk = 28,
            canRequestUnattended = false,
            unknownSourcesAllowed = true,
            deviceOwner = false,
        )
        assertTrue(eligibility.canAttempt)
        assertFalse(eligibility.unattendedEligible)
        assertTrue(eligibility.userActionRequired)
        assertTrue(eligibility.reason.contains("system install confirmation"))
    }

    @Test
    fun foreignPackageOrSignerCannotSelfUpdate() {
        assertFalse(
            SelfUpdatePolicy.decide(installed, newer.copy(packageName = "other"), 35, true, true, false).canAttempt,
        )
        assertFalse(
            SelfUpdatePolicy.decide(installed, newer.copy(signers = setOf("key-b")), 35, true, true, false).canAttempt,
        )
        assertEquals(
            "Self-update can only replace the running package",
            SelfUpdatePolicy.decide(installed, newer.copy(packageName = "other"), 35, true, true, false).reason,
        )
    }
}
