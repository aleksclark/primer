package com.aleksclark.primer.updates

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class SelfUpdatePolicyTest {
    private val installed = ArchiveIdentity("com.aleksclark.primer.control", 1, setOf("key-a"), 26, emptySet())
    private val newer = installed.copy(version = 2)
    private val expected = SignedManifest(
        packageName = "com.aleksclark.primer.control",
        channel = "stable",
        versionCode = 2,
        versionName = "0.2.0",
        minSdk = 26,
        supportedAbis = emptyList(),
        signerSha256 = "key-a",
        sha256 = "b".repeat(64),
        byteSize = 12,
    )

    private fun decide(
        archive: ArchiveIdentity = newer,
        sdk: Int = 35,
        targetSdk: Int = 35,
        unattended: Boolean = true,
        expectedManifest: SignedManifest = expected,
    ) = SelfUpdatePolicy.decide(
        runningPackage = "com.aleksclark.primer.control",
        installed = installed,
        archive = archive,
        expected = expectedManifest,
        sdk = sdk,
        targetSdk = targetSdk,
        canUpdateWithoutUserAction = unattended,
        unknownSourcesAllowed = true,
    )

    @Test
    fun ordinaryAppMayBeUnattendedWhenSamePackageSignerSdkAndPermission() {
        val eligibility = decide()
        assertTrue(eligibility.unattendedEligible)
        assertFalse(eligibility.userActionRequired)
    }

    @Test
    fun olderSdkRequiresUserActionEvenWithPermissionFlag() {
        val eligibility = decide(sdk = 28, targetSdk = 28, unattended = true)
        assertFalse(eligibility.unattendedEligible)
        assertTrue(eligibility.userActionRequired)
    }

    @Test
    fun foreignPackageOrSignerCannotSelfUpdate() {
        org.junit.Assert.assertThrows(IllegalArgumentException::class.java) {
            decide(archive = newer.copy(packageName = "other"))
        }
        org.junit.Assert.assertThrows(IllegalArgumentException::class.java) {
            decide(archive = newer.copy(signers = setOf("key-b")))
        }
        org.junit.Assert.assertThrows(IllegalStateException::class.java) {
            decide(expectedManifest = expected.copy(packageName = "other"))
        }
    }
}
