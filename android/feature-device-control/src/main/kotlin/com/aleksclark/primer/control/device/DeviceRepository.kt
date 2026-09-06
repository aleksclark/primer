package com.aleksclark.primer.control.device

import com.aleksclark.primer.security.RecoveryHpke
import com.aleksclark.primertasks.client.CredentialProvider
import com.aleksclark.primertasks.client.IssueEnrollmentInput
import com.aleksclark.primertasks.client.RecoveryEnvelope
import com.aleksclark.primertasks.client.RecoveryIntentInput
import com.aleksclark.primertasks.client.ReleaseTargetInput
import com.aleksclark.primertasks.client.StateChangeInput
import com.aleksclark.primertasks.client.TasksClient
import java.security.SecureRandom
import java.util.UUID

class DeviceRepository(
    apiBase: String,
    token: CredentialProvider,
    private val client: TasksClient = TasksClient(baseUrl = apiBase, parentCredentials = token),
) {
    suspend fun list() = client.listManagedDevices()
    suspend fun get(id: String) = client.getManagedDevice(id)
    suspend fun issue() = client.issueManagedEnrollment(IssueEnrollmentInput())
    suspend fun abandon(id: String) = client.abandonManagedEnrollment(id)
    suspend fun desired(id: String) = client.managedDeviceDesired(id)
    suspend fun quarantine(id: String, reason: String) = client.quarantineManagedDevice(id, StateChangeInput(reason = reason))
    suspend fun revoke(id: String, reason: String) = client.revokeManagedDevice(id, StateChangeInput(reason = reason))
    suspend fun releases() = client.listManagedReleases()
    suspend fun target(deviceId: String, releaseId: String, baseTargetVersion: Long = 0) =
        client.setManagedDeviceRelease(deviceId, ReleaseTargetInput(baseTargetVersion = baseTargetVersion, releaseId = releaseId))

    data class PreparedRotation(
        val requestId: String,
        val codes: List<String>,
        val publicJson: ByteArray,
    )

    fun prepareRotation(enrollmentPublicKey: String): PreparedRotation {
        val publicJson = RecoveryHpke.decodePublicEnrollmentKey(enrollmentPublicKey)
        val random = SecureRandom()
        val codes = List(6) {
            ByteArray(16).also(random::nextBytes).joinToString("") { b -> "%02x".format(b) }
        }
        return PreparedRotation(requestId = UUID.randomUUID().toString(), codes = codes, publicJson = publicJson)
    }

    suspend fun rotateRecovery(deviceId: String, prepared: PreparedRotation, acknowledged: Boolean) {
        check(acknowledged) { "parent must acknowledge one-time custody before rotation" }
        val envelope = RecoveryHpke.encryptCodes(prepared.publicJson, deviceId, prepared.requestId, prepared.codes)
        client.createManagedRecovery(
            deviceId,
            RecoveryIntentInput(
                requestId = prepared.requestId,
                kind = "rotate_recovery_code",
                envelope = RecoveryEnvelope(alg = envelope.alg, ciphertext = envelope.ciphertext, keyId = envelope.keyId),
                parentAcknowledged = true,
            ),
        )
    }
}
