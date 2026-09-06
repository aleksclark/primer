package com.aleksclark.primer.control.device

import com.aleksclark.primertasks.client.ApprovedApp
import com.aleksclark.primertasks.client.LockTaskPolicy
import com.aleksclark.primertasks.client.MaintenancePolicy
import com.aleksclark.primertasks.client.Policy
import com.aleksclark.primertasks.client.PolicyReport
import com.aleksclark.primertasks.client.Release
import com.aleksclark.primertasks.client.ReleaseManifest
import com.aleksclark.primertasks.client.ReleaseTarget
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class DevicePolicyTest {
    @Test
    fun syncStatusCoversAppliedPendingPartialStale() {
        assertEquals(DeviceSyncStatus.NoPolicy, DeviceSync.status(0, 0, null))
        assertEquals(DeviceSyncStatus.Unknown, DeviceSync.status(1, 1, null))
        assertEquals(DeviceSyncStatus.Pending, DeviceSync.status(2, 1, null))
        assertEquals(
            DeviceSyncStatus.Partial,
            DeviceSync.status(2, 2, report("partial", stale = false, revision = 2)),
        )
        assertEquals(
            DeviceSyncStatus.Stale,
            DeviceSync.status(2, 1, report("ok", stale = true, revision = 1)),
        )
        assertEquals(
            DeviceSyncStatus.Failed,
            DeviceSync.status(1, 1, report("failed", stale = false, revision = 1)),
        )
        assertEquals(
            DeviceSyncStatus.Applied,
            DeviceSync.status(1, 1, report("applied", stale = false, revision = 1)),
        )
    }

    @Test
    fun approvedAppDraftRequiresSignerDigest() {
        val app = ApprovedAppDraft(
            packageName = "com.aleksclark.primer.student",
            signerSha256 = "A".repeat(64),
            label = "Student",
            required = true,
        ).toApprovedApp()
        assertEquals("com.aleksclark.primer.student", app.packageName)
        assertEquals("a".repeat(64), app.signerSha256)
        assertTrue(app.required == true)
    }

    @Test(expected = IllegalArgumentException::class)
    fun approvedAppDraftRejectsShortSigner() {
        ApprovedAppDraft(packageName = "com.example.app", signerSha256 = "deadbeef").toApprovedApp()
    }

    @Test
    fun policyUpdateKeepsBaseRevision() {
        val policy = Policy(
            approvedApps = listOf(ApprovedApp(packageName = "com.example.app", signerSha256 = "a".repeat(64))),
            lockTask = LockTaskPolicy(enabled = true),
            maintenance = MaintenancePolicy(allowParentUnlock = true),
        )
        val body = policyUpdate(3, policy.withMaintenance(false))
        assertEquals(3, body.baseRevision)
        assertFalse(body.policy.maintenance.allowParentUnlock)
    }

    @Test
    fun recoverySubmitRejectsDifferentDevice() {
        val prepared = RecoveryPrep.bind(
            deviceId = "device-a",
            enrollmentPublicKey = "key-a",
            requestId = "req-1",
            codes = listOf("c1"),
            publicJson = ByteArray(8) { 1 },
        )
        try {
            RecoveryPrep.submitTarget(prepared, "device-b", "key-a", acknowledged = true)
            throw AssertionError("expected device mismatch")
        } catch (error: IllegalStateException) {
            assertTrue(error.message!!.contains("different device"))
        }
    }

    @Test
    fun recoverySubmitRequiresAcknowledgement() {
        val prepared = RecoveryPrep.bind(
            deviceId = "device-a",
            enrollmentPublicKey = "key-a",
            requestId = "req-1",
            codes = listOf("c1"),
            publicJson = ByteArray(8) { 1 },
        )
        try {
            RecoveryPrep.submitTarget(prepared, "device-a", "key-a", acknowledged = false)
            throw AssertionError("expected acknowledgement")
        } catch (error: IllegalStateException) {
            assertTrue(error.message!!.contains("Store recovery codes"))
        }
    }

    @Test
    fun recoveryMatchesBoundPublicKey() {
        val prepared = RecoveryPrep.bind(
            deviceId = "device-a",
            enrollmentPublicKey = "key-a",
            requestId = "req-1",
            codes = listOf("c1"),
            publicJson = ByteArray(8) { 1 },
        )
        assertTrue(prepared.matches("device-a", "key-a"))
        assertFalse(prepared.matches("device-a", "key-b"))
    }

    @Test
    fun releaseCasUsesExistingTargetVersionNotZero() {
        val existing = ReleaseTarget(
            byteSize = 1,
            channel = "stable",
            id = "tgt-1",
            packageName = "com.aleksclark.primer.student",
            releaseId = "rel-old",
            sha256 = "ab",
            status = "active",
            targetVersion = 7,
            versionCode = 1,
            versionName = "1",
        )
        val release = Release(
            byteSize = 1,
            channel = "stable",
            id = "rel-new",
            manifest = ReleaseManifest(
                byteSize = 1,
                channel = "stable",
                minSdk = 26,
                packageName = "com.aleksclark.primer.student",
                sha256 = "cd",
                signerSha256 = "ef",
                supportedAbis = listOf("arm64-v8a"),
                versionCode = 2,
                versionName = "2",
            ),
            manifestPayloadBase64 = "e30=",
            manifestSignature = "sig",
            minSdk = 26,
            packageName = "com.aleksclark.primer.student",
            publishedAt = "2026-01-01T00:00:00Z",
            sha256 = "cd",
            signerSha256 = "ef",
            signingKeyId = "k1",
            status = "published",
            supportedAbis = listOf("arm64-v8a"),
            versionCode = 2,
            versionName = "2",
        )
        assertEquals(7, ReleaseCas.baseTargetVersion(ReleaseCas.matching(listOf(existing), release)))
        assertEquals(0, ReleaseCas.baseTargetVersion(null))
    }

    private fun report(status: String, stale: Boolean, revision: Long = 1) = PolicyReport(
        deviceId = "d1",
        id = "r1",
        policyRevision = revision,
        receivedAt = "2026-01-01T00:00:00Z",
        reportId = "rep-1",
        stale = stale,
        status = status,
    )
}
