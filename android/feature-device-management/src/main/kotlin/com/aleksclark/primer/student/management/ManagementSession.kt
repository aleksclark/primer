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
    private val applyRemoteRecovery: (requestId: String, codes: List<String>) -> String,
    private val applyRemoteLease: (requestId: String, durationMs: Long) -> String,
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
            applyDesired(result.desired, elapsedMs(), boot())
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
        val requestElapsed = elapsedMs()
        val requestBoot = boot()
        val client = clientFactory(binding.origin) { binding.token }
        return try {
            val flush = flushOutbox(binding)
            val desired = applyDesired(client.managementDeviceDesired(), requestElapsed, requestBoot)
            val retryable = flush.retryable || desired.retryable || outbox.hasRetryable(binding.origin, binding.deviceId)
            if (retryable) desired.copy(retryable = true) else desired
        } catch (cancelled: CancellationException) {
            throw cancelled
        } catch (error: TasksHttpException) {
            authFailure(binding, error)
        } catch (_: Exception) {
            ManagementSyncResult("Unable to reach the management server. Last-known policy remains.", retryable = true)
        }
    }

    private suspend fun applyDesired(desired: DesiredState, requestElapsedMs: Long, requestBoot: Int): ManagementSyncResult {
        val binding = credentials.read() ?: return ManagementSyncResult("Management is not enrolled")
        if (desired.device.id != binding.deviceId) {
            return ManagementSyncResult("Desired state is for a different device. Local recovery still works.")
        }
        val serverNow = runCatching { Instant.parse(desired.serverTime).toEpochMilli() }.getOrNull()
            ?: return ManagementSyncResult("Management desired state is missing a valid serverTime.")
        val responseElapsed = elapsedMs()
        val responseBoot = boot()
        desired.recovery.forEach { intent -> applyRecovery(intent, serverNow, binding, requestElapsedMs, requestBoot, responseElapsed, responseBoot) }
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
        val reports = flushOutbox(binding)
        applyReleases(desired.releaseTargets, binding)
        val receipts = flushOutbox(binding)
        val retryable = reports.retryable || receipts.retryable || outbox.hasRetryable(binding.origin, binding.deviceId)
        val dead = outbox.hasDeadLetter(binding.origin, binding.deviceId)
        val suffix = when {
            retryable -> " Durable reports remain undelivered."
            dead -> " Some reports were rejected and will not retry."
            else -> ""
        }
        return ManagementSyncResult(
            message = application.summary + suffix,
            appliedRevision = application.revision,
            status = application.status,
            retryable = retryable,
        )
    }

    private suspend fun applyRecovery(
        intent: RecoveryIntent,
        serverNowMs: Long,
        binding: ManagementBinding,
        requestElapsedMs: Long,
        requestBoot: Int,
        responseElapsedMs: Long,
        responseBoot: Int,
    ) {
        if (intent.deviceId != binding.deviceId) return
        val deliveryMs = runCatching { Instant.parse(intent.deliveryExpiresAt).toEpochMilli() }.getOrNull() ?: return
        val leaseMs = intent.leaseExpiresAt?.let { runCatching { Instant.parse(it).toEpochMilli() }.getOrNull() }
        val lease = RemoteLease(
            intentId = intent.id,
            kind = intent.kind,
            deliveryExpiresAtMs = deliveryMs,
            leaseExpiresAtMs = leaseMs,
            serverNowMs = serverNowMs,
            receivedElapsedMs = requestElapsedMs,
            receivedBoot = requestBoot,
        )
        val conservative = RemoteLeasePolicy.conservative(lease, requestElapsedMs, responseElapsedMs, requestBoot, responseBoot)
        when (intent.kind) {
            "rotate_recovery_code" -> {
                if (conservative == null) return
                val wire = intent.envelope ?: return
                if (!intent.parentAcknowledged) return
                val handle = credentials.privateHandle() ?: return
                val envelope = RecoveryCiphertext(wire.keyId, wire.alg, wire.ciphertext)
                if (envelope.keyId != binding.keyId) return
                val payload = RecoveryHpke.decryptCodes(handle, envelope, binding.deviceId, intent.id, binding.keyId)
                val afterDecrypt = elapsedMs()
                val afterBoot = boot()
                if (RemoteLeasePolicy.conservative(lease, requestElapsedMs, afterDecrypt, requestBoot, afterBoot) == null) return
                val ackId = applyRemoteRecovery(intent.id, payload.codes)
                enqueueRecoveryAck(binding, intent.id, ackId)
            }
            "maintenance_lease" -> {
                if (conservative == null) return
                val ackId = applyRemoteLease(intent.id, conservative.remainingMs)
                enqueueRecoveryAck(binding, intent.id, ackId)
            }
        }
    }

    private suspend fun enqueuePolicyReport(binding: ManagementBinding, application: PolicyApplication) {
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
        val identity = json.encodeToString(
            PolicyReportInput.serializer(),
            PolicyReportInput(
                reportId = "00000000-0000-0000-0000-000000000000",
                policyRevision = application.revision,
                status = application.status,
                installedStudentVersion = studentVersion,
                controls = application.controls.map { it.toWire() },
                installedApps = apps,
            ),
        )
        val id = "policy:${binding.origin}:${binding.deviceId}:${identity.hashCode()}"
        if (outbox.get(id) != null) return
        val body = PolicyReportInput(
            reportId = UUID.randomUUID().toString(),
            policyRevision = application.revision,
            status = application.status,
            installedStudentVersion = studentVersion,
            controls = application.controls.map { it.toWire() },
            installedApps = apps,
        )
        outbox.putIfAbsent(
            OutboxEntry(
                id = id,
                kind = "policy-report",
                body = json.encodeToString(PolicyReportInput.serializer(), body),
                origin = binding.origin,
                deviceId = binding.deviceId,
            ),
        )
    }

    private suspend fun enqueueRecoveryAck(binding: ManagementBinding, intentId: String, reportId: String) {
        val id = "recovery-ack:${binding.origin}:${binding.deviceId}:$intentId"
        outbox.putIfAbsent(
            OutboxEntry(
                id = id,
                kind = "recovery-ack",
                body = json.encodeToString(RecoveryConfirmInput.serializer(), RecoveryConfirmInput(reportId = reportId)),
                origin = binding.origin,
                deviceId = binding.deviceId,
            ),
        )
    }

    private suspend fun enqueueReceipt(binding: ManagementBinding, target: ReleaseTarget, status: String, versionCode: Long?, error: String?) {
        val id = "receipt:${binding.origin}:${binding.deviceId}:${target.id}:${target.targetVersion}:$status"
        val body = ReleaseReceiptInput(
            reportId = UUID.randomUUID().toString(),
            status = status,
            targetId = target.id,
            targetVersion = target.targetVersion,
            installedVersionCode = versionCode,
            error = error,
        )
        outbox.putIfAbsent(
            OutboxEntry(
                id = id,
                kind = "release-receipt",
                body = json.encodeToString(ReleaseReceiptInput.serializer(), body),
                origin = binding.origin,
                deviceId = binding.deviceId,
            ),
        )
    }

    private suspend fun flushOutbox(binding: ManagementBinding): ManagementSyncResult {
        val client = clientFactory(binding.origin) { binding.token }
        var retryable = false
        for (entry in outbox.pending(binding.origin, binding.deviceId)) {
            try {
                when (entry.kind) {
                    "policy-report" -> client.managementDeviceReport(json.decodeFromString(PolicyReportInput.serializer(), entry.body))
                    "recovery-ack" -> {
                        val intentId = entry.id.substringAfterLast(':')
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
                val permanent = error.statusCode in 400..499 && error.statusCode != 409 && error.statusCode != 429
                val dead = if (permanent || entry.attempts + 1 >= 8) error.message?.take(200) ?: "undeliverable" else null
                outbox.markAttempt(entry.id, dead)
                if (!permanent) retryable = true
                if (!permanent && error.statusCode >= 500) {
                    return ManagementSyncResult("Unable to refresh management policy.", retryable = true)
                }
            }
        }
        return ManagementSyncResult("ok", retryable = retryable || outbox.hasRetryable(binding.origin, binding.deviceId))
    }

    private suspend fun applyReleases(targets: List<ReleaseTarget>, binding: ManagementBinding) {
        val sink = releaseSink ?: return
        for (target in targets) {
            val trusted = runCatching { ReleaseDelivery.verify(target, trustRoot) }
            val manifest = trusted.getOrNull()
            if (manifest == null) {
                enqueueReceipt(binding, target, "failed", null, trusted.exceptionOrNull()?.message?.take(200) ?: "untrusted")
                continue
            }
            if (target.packageName != ReleaseDelivery.STUDENT_PACKAGE) {
                enqueueReceipt(binding, target, "blocked", null, "Package is not Student")
                continue
            }
            if (target.status in setOf("confirmed", "blocked", "failed")) continue
            if (sink.installActive) continue
            if (sink.studentVersion == target.versionCode) {
                enqueueReceipt(binding, target, "confirmed", sink.studentVersion, null)
                continue
            }
            if (sink.studentVersion > target.versionCode) {
                enqueueReceipt(binding, target, "failed", sink.studentVersion, "Installed version superseded this target")
                continue
            }
            enqueueReceipt(binding, target, "downloading", sink.studentVersion, null)
            val directory = sink.stagingDir().apply { check(mkdirs() || isDirectory) }
            val partial = File(directory, "${target.id}.partial")
            val verified = File(directory, "${target.id}.apk")
            try {
                val client = clientFactory(binding.origin) { binding.token }
                val current = credentials.read()
                check(current?.token == binding.token && current.origin == binding.origin && current.deviceId == binding.deviceId) {
                    "Management enrollment changed before install"
                }
                partial.outputStream().use { raw ->
                    ArchiveChecks.digestingSink(raw, manifest.byteSize, manifest.sha256).use { digesting ->
                        client.managementDeviceArtifact(target.releaseId, digesting)
                    }
                }
                enqueueReceipt(binding, target, "verifying", sink.studentVersion, null)
                if (verified.exists()) check(verified.delete())
                check(partial.renameTo(verified)) { "Cannot stage verified APK" }
                enqueueReceipt(binding, target, "installing", sink.studentVersion, null)
                val live = credentials.read()
                val authorized = live?.token == binding.token && live.origin == binding.origin && live.deviceId == binding.deviceId
                val outcome = sink.installVerified(verified, manifest) { authorized }
                enqueueReceipt(binding, target, outcome.status, outcome.versionCode, outcome.error)
            } catch (cancelled: CancellationException) {
                throw cancelled
            } catch (error: TasksHttpException) {
                if (ManagementAuth.clearsEnrollment(error.statusCode, error.code) && credentials.expectedToken() == binding.token) {
                    credentials.clearTokenOnly()
                    throw error
                }
                enqueueReceipt(binding, target, "failed", sink.studentVersion, "Unable to download release")
            } catch (error: Exception) {
                enqueueReceipt(binding, target, "failed", sink.studentVersion, error.message?.take(200) ?: error.javaClass.simpleName)
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
