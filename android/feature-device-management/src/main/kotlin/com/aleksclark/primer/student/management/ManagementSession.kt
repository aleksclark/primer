package com.aleksclark.primer.student.management

import android.os.Build
import com.aleksclark.primer.devicepolicy.ApprovedApp
import com.aleksclark.primer.devicepolicy.ControlReadback
import com.aleksclark.primer.devicepolicy.InventoriedApp
import com.aleksclark.primer.devicepolicy.PolicyApplication
import com.aleksclark.primer.security.RecoveryCiphertext
import com.aleksclark.primer.security.RecoveryHpke
import com.aleksclark.primertasks.client.ControlResult
import com.aleksclark.primertasks.client.DesiredState
import com.aleksclark.primertasks.client.DeviceCapabilities
import com.aleksclark.primertasks.client.EnrollInput
import com.aleksclark.primertasks.client.InstalledApp
import com.aleksclark.primertasks.client.PolicyReportInput
import com.aleksclark.primertasks.client.RecoveryConfirmInput
import com.aleksclark.primertasks.client.ReleaseReceiptInput
import com.aleksclark.primertasks.client.TasksClient
import com.aleksclark.primertasks.client.TasksHttpException
import java.time.Instant
import java.util.UUID
import kotlinx.coroutines.CancellationException

data class ManagementSyncResult(
    val message: String,
    val appliedRevision: Long = 0,
    val status: String = "requested",
)

class ManagementSession(
    private val credentials: ManagementSecrets,
    private val clientFactory: (origin: String, token: () -> String?) -> TasksClient,
    private val configuredHttpsOrigin: String,
    private val allowEmulatorOrigin: Boolean,
    private val deviceName: String,
    private val deviceModel: String,
    private val stableDeviceKey: String,
    private val applyPolicy: (revision: Long, apps: List<ApprovedApp>) -> PolicyApplication,
    private val inventory: () -> List<InventoriedApp>,
    private val applyRemoteRecovery: (requestId: String, codes: List<String>) -> Boolean,
    private val studentVersion: String,
    private val clockMs: () -> Long = { System.currentTimeMillis() },
    private val elapsedMs: () -> Long,
    private val boot: () -> Int,
) {
    suspend fun enroll(rawQr: String): ManagementSyncResult {
        val qr = ManagementEnrollmentQrParser.parse(rawQr, configuredHttpsOrigin, allowEmulatorOrigin)
            ?: return ManagementSyncResult("That QR is not a trusted Primer management enrollment code")
        val origin = qr.origin + qr.mount.ifBlank { "" }
        val (publicKey, keyId) = credentials.publicEnrollment()
        val client = clientFactory(origin) { null }
        return try {
            val result = client.managementDeviceEnroll(
                EnrollInput(
                    code = qr.code,
                    deviceName = deviceName.take(80),
                    deviceModel = deviceModel.take(80),
                    stableDeviceKey = stableDeviceKey,
                    enrollmentKeyId = keyId,
                    enrollmentPublicKey = publicKey,
                    capabilities = DeviceCapabilities(
                        androidApi = Build.VERSION.SDK_INT.toLong(),
                        supportedAbis = runCatching { Build.SUPPORTED_ABIS.toList() }.getOrDefault(emptyList()),
                        deviceOwner = true,
                        lockTaskSupported = true,
                    ),
                ),
            )
            credentials.save(
                ManagementBinding(
                    token = result.token,
                    origin = origin,
                    deviceId = result.device.id,
                    keyId = keyId,
                ),
            )
            applyDesired(result.desired)
        } catch (cancelled: CancellationException) {
            throw cancelled
        } catch (error: TasksHttpException) {
            ManagementSyncResult(
                when (error.statusCode) {
                    410 -> "That enrollment QR is expired or already used. Request a new parent enrollment."
                    else -> "Management enrollment failed."
                },
            )
        } catch (_: Exception) {
            ManagementSyncResult("Unable to reach the management server.")
        }
    }

    suspend fun sync(): ManagementSyncResult {
        val binding = credentials.read() ?: return ManagementSyncResult("Management is not enrolled")
        val client = clientFactory(binding.origin) { binding.token }
        return try {
            applyDesired(client.managementDeviceDesired())
        } catch (cancelled: CancellationException) {
            throw cancelled
        } catch (error: TasksHttpException) {
            if (error.statusCode == 401 || error.statusCode == 403) {
                credentials.clearTokenOnly()
                ManagementSyncResult("Management credential was revoked. Last-known policy remains; local recovery still works.")
            } else {
                ManagementSyncResult("Unable to refresh management policy.")
            }
        } catch (_: Exception) {
            ManagementSyncResult("Unable to reach the management server. Last-known policy remains.")
        }
    }

    private suspend fun applyDesired(desired: DesiredState): ManagementSyncResult {
        val binding = credentials.read() ?: return ManagementSyncResult("Management is not enrolled")
        val serverNow = runCatching { Instant.parse(desired.serverTime).toEpochMilli() }.getOrDefault(clockMs())
        desired.recovery.forEach { intent ->
            applyRecovery(intent.id, intent.kind, intent.envelope?.let {
                RecoveryCiphertext(it.keyId, it.alg, it.ciphertext)
            }, intent.deliveryExpiresAt, intent.leaseExpiresAt, serverNow, binding)
        }
        val revision = desired.policyRevision
        val application = if (revision != null) {
            val apps = revision.policy.approvedApps.map { app ->
                ApprovedApp(app.packageName, app.label ?: app.packageName, setOf(app.signerSha256))
            }
            applyPolicy(revision.revision, apps)
        } else {
            PolicyApplication(0, "requested", emptyList(), "No remote policy yet")
        }
        report(binding, application)
        return ManagementSyncResult(
            message = application.summary,
            appliedRevision = application.revision,
            status = application.status,
        )
    }

    private suspend fun applyRecovery(
        intentId: String,
        kind: String,
        envelope: RecoveryCiphertext?,
        deliveryExpiresAt: String,
        leaseExpiresAt: String?,
        serverNowMs: Long,
        binding: ManagementBinding,
    ) {
        val deliveryMs = runCatching { Instant.parse(deliveryExpiresAt).toEpochMilli() }.getOrNull() ?: return
        val leaseMs = leaseExpiresAt?.let { runCatching { Instant.parse(it).toEpochMilli() }.getOrNull() }
        val lease = RemoteLease(
            intentId = intentId,
            kind = kind,
            deliveryExpiresAtMs = deliveryMs,
            leaseExpiresAtMs = leaseMs,
            serverNowMs = serverNowMs,
            receivedElapsedMs = elapsedMs(),
            receivedBoot = boot(),
        )
        if (kind == "rotate_recovery_code" && envelope != null) {
            val handle = credentials.privateHandle() ?: return
            val payload = RecoveryHpke.decryptCodes(handle, envelope, binding.deviceId, intentId, binding.keyId)
            if (applyRemoteRecovery(intentId, payload.codes)) {
                confirm(binding, intentId)
            }
            return
        }
        if (kind == "maintenance_lease") {
            if (!RemoteLeasePolicy.canOpen(lease, clockMs(), elapsedMs(), boot())) return
        }
    }

    private suspend fun report(binding: ManagementBinding, application: PolicyApplication) {
        val client = clientFactory(binding.origin) { binding.token }
        val apps = inventory().take(64).map { app ->
            InstalledApp(
                packageName = app.packageName,
                label = app.label,
                signerSha256 = app.signerSha256,
                versionCode = app.versionCode,
                versionName = app.versionName,
                self = app.packageName == "com.aleksclark.primer.student",
            )
        }
        val controls = application.controls.map { it.toWire() }
        runCatching {
            client.managementDeviceReport(
                PolicyReportInput(
                    reportId = UUID.randomUUID().toString(),
                    policyRevision = application.revision,
                    status = application.status,
                    installedStudentVersion = studentVersion,
                    controls = controls,
                    installedApps = apps,
                ),
            )
        }
    }

    private suspend fun confirm(binding: ManagementBinding, intentId: String) {
        val client = clientFactory(binding.origin) { binding.token }
        runCatching {
            client.managementDeviceConfirmRecovery(
                intentId,
                RecoveryConfirmInput(reportId = UUID.randomUUID().toString()),
            )
        }
    }

    suspend fun reportRelease(targetId: String, targetVersion: Long, status: String, versionCode: Long?, error: String?) {
        val binding = credentials.read() ?: return
        val client = clientFactory(binding.origin) { binding.token }
        runCatching {
            client.managementDeviceReleaseReceipt(
                ReleaseReceiptInput(
                    reportId = UUID.randomUUID().toString(),
                    status = status,
                    targetId = targetId,
                    targetVersion = targetVersion,
                    installedVersionCode = versionCode,
                    error = error,
                ),
            )
        }
    }
}

private fun ControlReadback.toWire() = ControlResult(
    name = name,
    desired = desired,
    actual = actual,
    status = status,
    error = error,
)
