package com.aleksclark.primer.updates

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class SelfUpdatePolicyTest {
    private val installed = ArchiveIdentity("com.aleksclark.primer.control", 1, setOf("key-a"), 26, setOf("arm64-v8a"))
    private val newer = installed.copy(version = 2)
    private val expected = SignedManifest(
        packageName = "com.aleksclark.primer.control",
        channel = "stable",
        versionCode = 2,
        versionName = "0.2.0",
        minSdk = 26,
        supportedAbis = listOf("arm64-v8a"),
        signerSha256 = "key-a",
        sha256 = "b".repeat(64),
        byteSize = 12,
    )

    private fun decide(
        archive: ArchiveIdentity = newer,
        sdk: Int = 35,
        deviceAbis: Set<String> = setOf("arm64-v8a"),
        candidateTargetSdk: Int = 35,
        unattended: Boolean = true,
        unknownSources: Boolean = true,
        expectedManifest: SignedManifest = expected,
    ) = SelfUpdatePolicy.decide(
        runningPackage = "com.aleksclark.primer.control",
        installed = installed,
        archive = archive,
        expected = expectedManifest,
        sdk = sdk,
        deviceAbis = deviceAbis,
        candidateTargetSdk = candidateTargetSdk,
        canUpdateWithoutUserAction = unattended,
        unknownSourcesAllowed = unknownSources,
    )

    @Test
    fun ordinaryAppMayBeUnattendedWhenFloorAndPermissionMatch() {
        val eligibility = decide()
        assertTrue(eligibility.unattendedEligible)
        assertFalse(eligibility.userActionRequired)
        assertTrue(eligibility.mustHandlePendingUserAction)
    }

    @Test
    fun runningPlatformSelectsTheRequiredCandidateTargetSdk() {
        for ((os, floor) in mapOf(31 to 29, 32 to 29, 33 to 30, 34 to 31, 35 to 33, 36 to 34)) {
            val below = decide(sdk = os, candidateTargetSdk = floor - 1)
            assertFalse("Android $os must reject unattended targetSdk ${floor - 1}", below.unattendedEligible)
            assertTrue(below.userActionRequired)
            val eligible = decide(sdk = os, candidateTargetSdk = floor)
            assertTrue("Android $os must permit an unattended request for targetSdk $floor", eligible.unattendedEligible)
            assertFalse(eligible.userActionRequired)
            assertTrue(eligible.mustHandlePendingUserAction)
        }
        assertTrue(decide(sdk = 32, candidateTargetSdk = 35).unattendedEligible)
        assertFalse(decide(sdk = 36, candidateTargetSdk = 31).unattendedEligible)
        assertFalse(decide(sdk = 30, candidateTargetSdk = 35).unattendedEligible)
        assertFalse(decide(sdk = 37, candidateTargetSdk = 99).unattendedEligible)
    }

    @Test
    fun unknownSourcesBlockUnattended() {
        val eligibility = decide(unknownSources = false)
        assertFalse(eligibility.unattendedEligible)
        assertTrue(eligibility.userActionRequired)
    }

    @Test
    fun deviceAbiIncompatibilityIsRejected() {
        org.junit.Assert.assertThrows(IllegalArgumentException::class.java) {
            decide(archive = newer.copy(abis = setOf("x86_64")), deviceAbis = setOf("arm64-v8a"))
        }
    }

    @Test
    fun foreignPackageOrSignerCannotSelfUpdate() {
        org.junit.Assert.assertThrows(IllegalArgumentException::class.java) {
            decide(archive = newer.copy(packageName = "other"))
        }
        org.junit.Assert.assertThrows(IllegalArgumentException::class.java) {
            decide(archive = newer.copy(signers = setOf("key-b")))
        }
    }
}
