package com.aleksclark.primer.student.management

import android.os.Build
import com.aleksclark.primer.devicepolicy.ApprovedApp
import com.aleksclark.primer.devicepolicy.ControlReadback
import com.aleksclark.primer.devicepolicy.InventoriedApp
import com.aleksclark.primer.devicepolicy.PolicyApplication
import com.aleksclark.primer.security.RecoveryCiphertext
import com.aleksclark.primer.security.RecoveryHpke
import com.aleksclark.primer.updates.ArchiveChecks
import com.aleksclark.primertasks.client.ControlResult
import com.aleksclark.primertasks.client.DesiredState
import com.aleksclark.primertasks.client.DeviceCapabilities
import com.aleksclark.primertasks.client.EnrollInput
import com.aleksclark.primertasks.client.InstalledApp
import com.aleksclark.primertasks.client.PolicyReportInput
import com.aleksclark.primertasks.client.RecoveryConfirmInput
import com.aleksclark.primertasks.client.RecoveryIntent
import com.aleksclark.primertasks.client.ReleaseReceiptInput
import com.aleksclark.primertasks.client.ReleaseTarget
import com.aleksclark.primertasks.client.TasksClient
import com.aleksclark.primertasks.client.TasksHttpException
import java.io.File
import java.time.Instant
import java.util.UUID
import kotlinx.coroutines.CancellationException
import kotlinx.serialization.json.Json

data class ManagementSyncResult(
    val message: String,
    val appliedRevision: Long = 0,
    val status: String = "requested",
    val retryable: Boolean = false,
)

class ManagementSession(
    private val credentials: ManagementSecrets,
    private val clientFactory: (origin: String, token: () -> String?) -> TasksClient,
    private val configuredHttpsOrigin: String,
    private val allowEmulatorOrigin: Boolean,
    private val deviceName: String,
    private val deviceModel: String,
    private val applyPolicy: (
        revision: Long,
        apps: List<ApprovedApp>,
        extras: List<ControlReadback>,
        origin: String,
        deviceId: String,
    ) -> PolicyApplication,
    private val inventory: () -> List<InventoriedApp>,
    private val applyRemoteRecovery: (requestId: String, codes: List<String>) -> Boolean,
    private val applyRemoteLease: (requestId: String, durationMs: Long) -> Boolean,
    private val studentVersion: String,
    private val outbox: ManagementOutbox,
    private val localRestrictions: Set<String>,
    private val releaseSink: RemoteReleaseSink? = null,
    private val trustRoot: String = "",
    private val elapsedMs: () -> Long,
    private val boot: () -> Int,
    private val json: Json = Json { encodeDefaults = false; ignoreUnknownKeys = true },
) {
    suspend fun enroll(rawQr: String, replace: Boolean = false): ManagementSyncResult {
        val qr = ManagementEnrollmentQrParser.parse(rawQr, configuredHttpsOrigin, allowEmulatorOrigin)
            ?: return ManagementSyncResult("That QR is not a trusted Primer management enrollment code")
        val origin = qr.origin + qr.mount.ifBlank { "" }
        val existing = credentials.read()
        if (existing != null && existing.token.isNotBlank() && !replace) {
            return ManagementSyncResult("Management is already enrolled. Parent must explicitly replace this enrollment.")
        }
        val (publicKey, keyId) = credentials.publicEnrollment()
        val client = clientFactory(origin) { null }
        return try {
            val result = client.managementDeviceEnroll(
                EnrollInput(
                    code = qr.code,
                    deviceName = deviceName.take(80),
                    deviceModel = deviceModel.take(80),
                    stableDeviceKey = credentials.stableDeviceKey(),
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
            ManagementSyncResult("Unable to reach the management server.", retryable = true)
        }
    }

    suspend fun sync(): ManagementSyncResult {
        val binding = credentials.read() ?: return ManagementSyncResult("Management is not enrolled")
        if (binding.token.isBlank()) {
            return ManagementSyncResult("Management credential was revoked. Last-known policy remains; local recovery still works.")
        }
        val client = clientFactory(binding.origin) { binding.token }
        return try {
            flushOutbox(binding)
            applyDesired(client.managementDeviceDesired())
        } catch (cancelled: CancellationException) {
            throw cancelled
        } catch (error: TasksHttpException) {
            authFailure(binding, error)
        } catch (_: Exception) {
            ManagementSyncResult("Unable to reach the management server. Last-known policy remains.", retryable = true)
        }
    }

    private suspend fun applyDesired(desired: DesiredState): ManagementSyncResult {
        val binding = credentials.read() ?: return ManagementSyncResult("Management is not enrolled")
        if (desired.device.id != binding.deviceId) {
            return ManagementSyncResult("Desired state is for a different device. Local recovery still works.")
        }
        val serverNow = runCatching { Instant.parse(desired.serverTime).toEpochMilli() }.getOrNull()
            ?: return ManagementSyncResult("Management desired state is missing a valid serverTime.")
        desired.recovery.forEach { intent -> applyRecovery(intent, serverNow, binding) }
        val revision = desired.policyRevision
        val application = if (revision != null) {
            val extras = RemotePolicyProjection.extraControls(
                lockTaskEnabled = revision.policy.lockTask.enabled,
                lockTaskPackages = revision.policy.lockTask.packages,
                allowKeyguard = revision.policy.lockTask.allowKeyguard,
                allowOverview = revision.policy.lockTask.allowOverview,
                allowStatusBar = revision.policy.lockTask.allowStatusBar,
                requiredPackages = revision.policy.requiredPackages.orEmpty().map { it.packageName },
                userRestrictions = revision.policy.userRestrictions.orEmpty(),
                allowParentUnlock = revision.policy.maintenance.allowParentUnlock,
                studentPackage = "com.aleksclark.primer.student",
                localRestrictions = localRestrictions,
            )
            val apps = revision.policy.approvedApps.map { app ->
                ApprovedApp(app.packageName, app.label ?: app.packageName, setOf(app.signerSha256))
            }
            applyPolicy(revision.revision, apps, extras, binding.origin, binding.deviceId)
        } else {
            PolicyApplication(0, "requested", emptyList(), "No remote policy yet")
        }
        enqueuePolicyReport(binding, application)
        flushOutbox(binding)
        applyReleases(desired.releaseTargets, binding)
        flushOutbox(binding)
        return ManagementSyncResult(
            message = application.summary,
            appliedRevision = application.revision,
            status = application.status,
        )
    }

    private suspend fun applyRecovery(intent: RecoveryIntent, serverNowMs: Long, binding: ManagementBinding) {
        if (intent.deviceId != binding.deviceId) return
        val deliveryMs = runCatching { Instant.parse(intent.deliveryExpiresAt).toEpochMilli() }.getOrNull() ?: return
        val leaseMs = intent.leaseExpiresAt?.let { runCatching { Instant.parse(it).toEpochMilli() }.getOrNull() }
        val lease = RemoteLease(
            intentId = intent.id,
            kind = intent.kind,
            deliveryExpiresAtMs = deliveryMs,
            leaseExpiresAtMs = leaseMs,
            serverNowMs = serverNowMs,
            receivedElapsedMs = elapsedMs(),
            receivedBoot = boot(),
        )
        if (serverNowMs >= deliveryMs) return
        when (intent.kind) {
            "rotate_recovery_code" -> {
                val wire = intent.envelope ?: return
                if (!intent.parentAcknowledged) return
                val handle = credentials.privateHandle() ?: return
                val envelope = RecoveryCiphertext(wire.keyId, wire.alg, wire.ciphertext)
                if (envelope.keyId != binding.keyId) return
                val payload = RecoveryHpke.decryptCodes(handle, envelope, binding.deviceId, intent.id, binding.keyId)
                applyRemoteRecovery(intent.id, payload.codes)
                enqueueRecoveryAck(intent.id)
            }
            "maintenance_lease" -> {
                if (!RemoteLeasePolicy.canOpen(lease, serverNowMs, elapsedMs(), boot())) return
                val remaining = (lease.leaseExpiresAtMs ?: return) - serverNowMs
                applyRemoteLease(intent.id, remaining)
                enqueueRecoveryAck(intent.id)
            }
        }
    }

    private suspend fun enqueuePolicyReport(binding: ManagementBinding, application: PolicyApplication) {
        val id = "policy:${binding.deviceId}:${application.revision}:${application.status}"
        val existing = outbox.get(id)
        val reportId = existing?.let { json.decodeFromString(PolicyReportInput.serializer(), it.body).reportId }
            ?: UUID.randomUUID().toString()
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
        val body = PolicyReportInput(
            reportId = reportId,
            policyRevision = application.revision,
            status = application.status,
            installedStudentVersion = studentVersion,
            controls = application.controls.map { it.toWire() },
            installedApps = apps,
        )
        outbox.put(OutboxEntry(id, "policy-report", json.encodeToString(PolicyReportInput.serializer(), body)))
    }

    private suspend fun enqueueRecoveryAck(intentId: String) {
        val id = "recovery-ack:$intentId"
        val existing = outbox.get(id)
        val reportId = existing?.let { json.decodeFromString(RecoveryConfirmInput.serializer(), it.body).reportId }
            ?: UUID.randomUUID().toString()
        outbox.put(OutboxEntry(id, "recovery-ack", json.encodeToString(RecoveryConfirmInput.serializer(), RecoveryConfirmInput(reportId = reportId))))
    }

    private suspend fun enqueueReceipt(target: ReleaseTarget, status: String, versionCode: Long?, error: String?) {
        val id = "receipt:${target.id}:${target.targetVersion}:$status"
        val existing = outbox.get(id)
        val reportId = existing?.let { json.decodeFromString(ReleaseReceiptInput.serializer(), it.body).reportId }
            ?: UUID.randomUUID().toString()
        val body = ReleaseReceiptInput(
            reportId = reportId,
            status = status,
            targetId = target.id,
            targetVersion = target.targetVersion,
            installedVersionCode = versionCode,
            error = error,
        )
        outbox.put(OutboxEntry(id, "release-receipt", json.encodeToString(ReleaseReceiptInput.serializer(), body)))
    }

    private suspend fun flushOutbox(binding: ManagementBinding) {
        val client = clientFactory(binding.origin) { binding.token }
        for (entry in outbox.pending()) {
            if (entry.attempts >= 8) continue
            try {
                when (entry.kind) {
                    "policy-report" -> client.managementDeviceReport(json.decodeFromString(PolicyReportInput.serializer(), entry.body))
                    "recovery-ack" -> {
                        val intentId = entry.id.removePrefix("recovery-ack:")
                        client.managementDeviceConfirmRecovery(intentId, json.decodeFromString(RecoveryConfirmInput.serializer(), entry.body))
                    }
                    "release-receipt" -> client.managementDeviceReleaseReceipt(json.decodeFromString(ReleaseReceiptInput.serializer(), entry.body))
                }
                outbox.remove(entry.id)
            } catch (cancelled: CancellationException) {
                throw cancelled
            } catch (error: TasksHttpException) {
                if (ManagementAuth.clearsEnrollment(error.statusCode, error.code) && credentials.expectedToken() == binding.token) {
                    credentials.clearTokenOnly()
                    throw error
                }
                outbox.markAttempt(entry.id)
                if (error.statusCode in 400..499 && error.statusCode != 409 && error.statusCode != 429) continue
                throw error
            }
        }
    }

    private suspend fun applyReleases(targets: List<ReleaseTarget>, binding: ManagementBinding) {
        val sink = releaseSink ?: return
        for (target in targets) {
            if (target.packageName != ReleaseDelivery.STUDENT_PACKAGE) {
                enqueueReceipt(target, "blocked", null, "Package is not Student")
                continue
            }
            if (target.status in setOf("confirmed", "blocked", "failed")) continue
            if (sink.installActive) continue
            if (sink.studentVersion == target.versionCode) {
                enqueueReceipt(target, "confirmed", sink.studentVersion, null)
                continue
            }
            if (sink.studentVersion > target.versionCode) {
                enqueueReceipt(target, "failed", sink.studentVersion, "Installed version superseded this target")
                continue
            }
            val verifiedManifest = runCatching { ReleaseDelivery.verify(target, trustRoot) }
            val manifest = verifiedManifest.getOrNull()
            if (manifest == null) {
                enqueueReceipt(target, "failed", null, verifiedManifest.exceptionOrNull()?.message?.take(200) ?: "untrusted")
                continue
            }
            enqueueReceipt(target, "downloading", sink.studentVersion, null)
            val directory = sink.stagingDir().apply { check(mkdirs() || isDirectory) }
            val partial = File(directory, "${target.id}.partial")
            val verified = File(directory, "${target.id}.apk")
            try {
                val client = clientFactory(binding.origin) { binding.token }
                sink.remember(target.id, target.targetVersion)
                partial.outputStream().use { raw ->
                    ArchiveChecks.digestingSink(raw, manifest.byteSize, manifest.sha256).use { digesting ->
                        client.managementDeviceArtifact(target.releaseId, digesting)
                    }
                }
                enqueueReceipt(target, "verifying", sink.studentVersion, null)
                if (verified.exists()) check(verified.delete())
                check(partial.renameTo(verified)) { "Cannot stage verified APK" }
                enqueueReceipt(target, "installing", sink.studentVersion, null)
                sink.installVerified(verified, manifest.byteSize)
            } catch (cancelled: CancellationException) {
                throw cancelled
            } catch (error: TasksHttpException) {
                if (ManagementAuth.clearsEnrollment(error.statusCode, error.code) && credentials.expectedToken() == binding.token) {
                    credentials.clearTokenOnly()
                    throw error
                }
                enqueueReceipt(target, "failed", sink.studentVersion, "Unable to download release")
            } catch (error: Exception) {
                enqueueReceipt(target, "failed", sink.studentVersion, error.message?.take(200) ?: error.javaClass.simpleName)
            } finally {
                partial.delete()
            }
        }
    }

    private suspend fun authFailure(binding: ManagementBinding, error: TasksHttpException): ManagementSyncResult {
        if (ManagementAuth.clearsEnrollment(error.statusCode, error.code) && credentials.expectedToken() == binding.token) {
            credentials.clearTokenOnly()
            return ManagementSyncResult("Management credential was revoked. Last-known policy remains; local recovery still works.")
        }
        return ManagementSyncResult("Unable to refresh management policy.", retryable = error.statusCode >= 500)
    }
}

private fun ControlReadback.toWire() = ControlResult(
    name = name,
    desired = desired,
    actual = actual,
    status = status,
    error = error,
)
