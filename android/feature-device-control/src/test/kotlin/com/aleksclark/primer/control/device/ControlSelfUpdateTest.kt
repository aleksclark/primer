package com.aleksclark.primer.control.device

import com.aleksclark.primer.updates.SelfUpdateEligibility
import com.aleksclark.primer.updates.SignedManifest
import com.aleksclark.primertasks.client.Release
import com.aleksclark.primertasks.client.ReleaseManifest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class ControlSelfUpdateTest {
    @Test
    fun userActionRequiredMeansNotUnattended() {
        val plan = ControlSelfUpdate.present(
            SelfUpdateEligibility(
                unattendedEligible = false,
                userActionRequired = true,
                reason = "Android requires a system install confirmation",
                mustHandlePendingUserAction = true,
            ),
            unknownSourcesAllowed = true,
        )
        assertTrue(plan.confirmationRequired)
        assertTrue(plan.mustHandlePendingUserAction)
        assertFalse(plan.unattendedEligible)
        assertEquals(
            ControlSelfUpdatePhase.EligibleConfirm,
            ControlSelfUpdate.phase(plan, pendingConfirmation = false, sessionExists = false, failed = false),
        )
    }

    @Test
    fun unattendedStillRequiresPendingUserActionHandler() {
        val plan = ControlSelfUpdate.present(
            SelfUpdateEligibility(
                unattendedEligible = true,
                userActionRequired = false,
                reason = "Android may replace this package without a prompt",
                mustHandlePendingUserAction = true,
            ),
            unknownSourcesAllowed = true,
        )
        assertTrue(plan.unattendedEligible)
        assertTrue(plan.mustHandlePendingUserAction)
        assertTrue(plan.notificationFallback)
        assertEquals(
            ControlSelfUpdatePhase.EligibleUnattended,
            ControlSelfUpdate.phase(plan, pendingConfirmation = false, sessionExists = true, failed = false),
        )
    }

    @Test
    fun settingsFallbackWhenUnknownSourcesDisallowed() {
        val plan = ControlSelfUpdate.present(
            SelfUpdateEligibility(
                unattendedEligible = false,
                userActionRequired = true,
                reason = "Unknown-source installs are not permitted; system confirmation is required",
                mustHandlePendingUserAction = true,
            ),
            unknownSourcesAllowed = false,
        )
        assertTrue(plan.settingsRequired)
        assertEquals(
            ControlSelfUpdatePhase.NeedsSettings,
            ControlSelfUpdate.phase(plan, pendingConfirmation = false, sessionExists = false, failed = false),
        )
    }

    @Test
    fun pendingConfirmationWithoutSessionFailsClosed() {
        val plan = ControlSelfUpdate.present(
            SelfUpdateEligibility(false, true, "confirm", true),
            unknownSourcesAllowed = true,
        )
        assertEquals(
            ControlSelfUpdatePhase.Failed,
            ControlSelfUpdate.phase(plan, pendingConfirmation = true, sessionExists = false, failed = false),
        )
        assertEquals(
            ControlSelfUpdatePhase.WaitingConfirmation,
            ControlSelfUpdate.phase(plan, pendingConfirmation = true, sessionExists = true, failed = false),
        )
        assertEquals(
            ControlSelfUpdatePhase.Deferred,
            ControlSelfUpdate.phase(plan, pendingConfirmation = true, sessionExists = true, failed = false, deferred = true),
        )
    }

    @Test
    fun selectsNewerControlStableOnly() {
        val student = release(id = "student", packageName = "com.aleksclark.primer.student", versionCode = 99)
        val beta = release(id = "beta", channel = "beta", versionCode = 5)
        val older = release(id = "old", versionCode = 1)
        val current = release(id = "current", versionCode = 2)
        val newer = release(id = "new", versionCode = 4)
        val alsoNewer = release(id = "mid", versionCode = 3)
        val selected = ControlSelfUpdate.selectCandidate(
            listOf(student, beta, older, current, alsoNewer, newer),
            installedVersion = 2,
        )
        assertEquals("new", selected?.id)
        assertNull(ControlSelfUpdate.selectCandidate(listOf(student, beta, current), installedVersion = 2))
        assertFalse(ControlSelfUpdate.isNewer(2, 2))
    }

    @Test
    fun decodedManifestMustMatchOuterPublication() {
        val outer = release(id = "r1", versionCode = 3, sha256 = "b".repeat(64), byteSize = 12)
        val decoded = SignedManifest(
            packageName = ControlSelfUpdate.CONTROL_PACKAGE,
            channel = ControlSelfUpdate.CONTROL_CHANNEL,
            versionCode = 3,
            versionName = "0.3.0",
            minSdk = 28,
            supportedAbis = listOf("arm64-v8a"),
            signerSha256 = "a".repeat(64),
            sha256 = "b".repeat(64),
            byteSize = 12,
        )
        ControlSelfUpdate.requireMatchesOuter(decoded, outer)
        org.junit.Assert.assertThrows(IllegalStateException::class.java) {
            ControlSelfUpdate.requireMatchesOuter(decoded.copy(channel = "beta"), outer)
        }
        org.junit.Assert.assertThrows(IllegalStateException::class.java) {
            ControlSelfUpdate.requireMatchesOuter(decoded.copy(versionCode = 9), outer)
        }
        org.junit.Assert.assertThrows(IllegalStateException::class.java) {
            ControlSelfUpdate.requireMatchesOuter(decoded.copy(sha256 = "c".repeat(64)), outer)
        }
        org.junit.Assert.assertThrows(IllegalStateException::class.java) {
            ControlSelfUpdate.requireMatchesOuter(decoded.copy(packageName = "com.aleksclark.primer.student"), outer)
        }
    }

    private fun release(
        id: String,
        packageName: String = ControlSelfUpdate.CONTROL_PACKAGE,
        channel: String = ControlSelfUpdate.CONTROL_CHANNEL,
        versionCode: Long = 2,
        sha256: String = "b".repeat(64),
        byteSize: Long = 12,
    ) = Release(
        byteSize = byteSize,
        channel = channel,
        id = id,
        manifest = ReleaseManifest(
            byteSize = byteSize,
            channel = channel,
            minSdk = 28,
            packageName = packageName,
            sha256 = sha256,
            signerSha256 = "a".repeat(64),
            supportedAbis = listOf("arm64-v8a"),
            versionCode = versionCode,
            versionName = "0.$versionCode.0",
        ),
        manifestPayloadBase64 = "payload",
        manifestSignature = "sig",
        minSdk = 28,
        packageName = packageName,
        publishedAt = "2026-01-01T00:00:00Z",
        sha256 = sha256,
        signerSha256 = "a".repeat(64),
        signingKeyId = "ed25519-v1",
        status = "published",
        supportedAbis = listOf("arm64-v8a"),
        versionCode = versionCode,
        versionName = "0.$versionCode.0",
    )
}
