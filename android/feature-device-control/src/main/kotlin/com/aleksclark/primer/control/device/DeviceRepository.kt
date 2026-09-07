package com.aleksclark.primer.control.device

import com.aleksclark.primer.security.RecoveryHpke
import com.aleksclark.primertasks.client.CredentialProvider
import com.aleksclark.primertasks.client.DesiredState
import com.aleksclark.primertasks.client.IssueEnrollmentInput
import com.aleksclark.primertasks.client.Policy
import com.aleksclark.primertasks.client.PolicyRevision
import com.aleksclark.primertasks.client.RecoveryEnvelope
import com.aleksclark.primertasks.client.RecoveryHistoryPage
import com.aleksclark.primertasks.client.RecoveryIntentInput
import com.aleksclark.primertasks.client.Release
import com.aleksclark.primertasks.client.ReleaseTargetInput
import com.aleksclark.primertasks.client.StateChangeInput
import com.aleksclark.primertasks.client.TasksClient
import java.security.SecureRandom
import java.util.UUID

class DeviceRepository(
    apiBase: String,
    token: CredentialProvider,
    http: okhttp3.OkHttpClient = okhttp3.OkHttpClient(),
    private val client: TasksClient = TasksClient(baseUrl = apiBase, http = http, parentCredentials = token),
) {
    suspend fun list() = client.listManagedDevices()
    suspend fun get(id: String) = client.getManagedDevice(id)
    suspend fun issue() = client.issueManagedEnrollment(IssueEnrollmentInput())
    suspend fun abandon(id: String) = client.abandonManagedEnrollment(id)
    suspend fun desired(id: String): DesiredState = client.managedDeviceDesired(id)
    suspend fun quarantine(id: String, reason: String) = client.quarantineManagedDevice(id, StateChangeInput(reason = reason))
    suspend fun revoke(id: String, reason: String) = client.revokeManagedDevice(id, StateChangeInput(reason = reason))
    suspend fun recoveryHistory(id: String): RecoveryHistoryPage = client.listManagedRecoveryHistory(id)
    suspend fun releases() = client.listManagedReleases()

    suspend fun target(deviceId: String, release: Release, desired: DesiredState, requeue: Boolean = false) =
        client.setManagedDeviceRelease(
            deviceId,
            ReleaseTargetInput(
                baseTargetVersion = ReleaseCas.baseTargetVersion(ReleaseCas.matching(desired.releaseTargets, release)),
                releaseId = release.id,
                requeue = if (requeue) true else null,
            ),
        )

    suspend fun updatePolicy(deviceId: String, baseRevision: Long, policy: Policy): PolicyRevision =
        client.updateManagedDevicePolicy(deviceId, policyUpdate(baseRevision, policy))

    suspend fun addApprovedApp(deviceId: String, desired: DesiredState, draft: ApprovedAppDraft): PolicyRevision {
        val revision = desired.policyRevision ?: error("Device has no policy revision to edit.")
        val apps = revision.policy.approvedApps + draft.toApprovedApp()
        return updatePolicy(deviceId, revision.revision, revision.policy.withApprovedApps(apps))
    }

    suspend fun removeApprovedApp(deviceId: String, desired: DesiredState, packageName: String): PolicyRevision {
        val revision = desired.policyRevision ?: error("Device has no policy revision to edit.")
        val apps = revision.policy.approvedApps.filterNot { it.packageName == packageName }
        return updatePolicy(deviceId, revision.revision, revision.policy.withApprovedApps(apps))
    }

    suspend fun setParentUnlock(deviceId: String, desired: DesiredState, allow: Boolean): PolicyRevision {
        val revision = desired.policyRevision ?: error("Device has no policy revision to edit.")
        return updatePolicy(deviceId, revision.revision, revision.policy.withMaintenance(allow))
    }

    fun prepareRotation(deviceId: String, enrollmentPublicKey: String): RecoveryBinding {
        val publicJson = RecoveryHpke.decodePublicEnrollmentKey(enrollmentPublicKey)
        val random = SecureRandom()
        val codes = List(6) {
            ByteArray(16).also(random::nextBytes).joinToString("") { b -> "%02x".format(b) }
        }
        return RecoveryPrep.bind(
            deviceId = deviceId,
            enrollmentPublicKey = enrollmentPublicKey,
            requestId = UUID.randomUUID().toString(),
            codes = codes,
            publicJson = publicJson,
        )
    }

    suspend fun rotateRecovery(
        selectedDeviceId: String,
        enrollmentPublicKey: String?,
        prepared: RecoveryBinding?,
        acknowledged: Boolean,
    ) {
        val target = RecoveryPrep.submitTarget(prepared, selectedDeviceId, enrollmentPublicKey, acknowledged)
        val envelope = RecoveryHpke.encryptCodes(target.publicJson, target.deviceId, target.requestId, target.codes)
        client.createManagedRecovery(
            target.deviceId,
            RecoveryIntentInput(
                requestId = target.requestId,
                kind = "rotate_recovery_code",
                envelope = RecoveryEnvelope(alg = envelope.alg, ciphertext = envelope.ciphertext, keyId = envelope.keyId),
                parentAcknowledged = true,
            ),
        )
    }
}
