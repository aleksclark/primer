package com.aleksclark.primer.student

import android.app.AlarmManager
import android.app.PendingIntent
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.os.Build
import android.os.SystemClock
import android.provider.Settings
import com.aleksclark.primer.devicepolicy.DevicePolicyController
import com.aleksclark.primer.devicepolicy.RecoveryStore
import com.aleksclark.primer.student.admin.MaintenanceExpiryReceiver
import com.aleksclark.primer.student.admin.PrimerDeviceAdminReceiver
import com.aleksclark.primer.student.management.DataStoreManagementOutbox
import com.aleksclark.primer.student.management.ManagementCredentialStore
import com.aleksclark.primer.student.management.ManagementSession
import com.aleksclark.primer.student.management.ManagementSyncResult
import com.aleksclark.primer.student.management.ManagementSyncWorker
import com.aleksclark.primer.student.management.InstallOutcome
import com.aleksclark.primer.student.management.RemoteReleaseSink
import com.aleksclark.primer.student.update.InstallResultReceiver
import com.aleksclark.primer.updates.ManagedUpdater
import com.aleksclark.primer.updates.SignedManifest
import com.aleksclark.primertasks.client.CredentialProvider
import com.aleksclark.primertasks.client.TasksClient
import java.io.File

class StudentRuntime(private val context: Context) {
    val policy = DevicePolicyController(context, ComponentName(context, PrimerDeviceAdminReceiver::class.java),
        ComponentName(context.packageName, "${context.packageName}.StudentHome"))
    val recovery by lazy { RecoveryStore(context) }
    val updater by lazy { ManagedUpdater(context, ComponentName(context, InstallResultReceiver::class.java)) }
    private val managementCredentials by lazy { ManagementCredentialStore(context) }
    private val managementOutbox by lazy { DataStoreManagementOutbox(context) }
    private val alarms = context.getSystemService(AlarmManager::class.java)

    private fun managementSession(): ManagementSession = ManagementSession(
        credentials = managementCredentials,
        clientFactory = { origin, token ->
            TasksClient(origin, managementCredentials = CredentialProvider { token() })
        },
        configuredHttpsOrigin = BuildConfig.CONFIGURED_API_ORIGIN,
        allowEmulatorOrigin = BuildConfig.DEBUG,
        deviceName = Build.MODEL,
        deviceModel = Build.MODEL,
        applyPolicy = { revision, apps, extras, origin, deviceId ->
            policy.rememberRemotePolicy(revision, apps, extras, origin, deviceId)
        },
        inventory = { policy.inventory() },
        applyRemoteRecovery = { requestId, codes -> applyRemoteRecovery(requestId, codes) },
        applyRemoteLease = { requestId, durationMs -> openRemoteLease(requestId, durationMs) },
        studentVersion = updater.version.toString(),
        outbox = managementOutbox,
        localRestrictions = DevicePolicyController.restrictions,
        releaseSink = object : RemoteReleaseSink {
            override val studentVersion: Long get() = updater.version
            override val installActive: Boolean get() = updater.active
            override val pendingTargetId: String get() = updater.pendingTargetId
            override val pendingTargetVersion: Long get() = updater.pendingTargetVersion
            override fun stagingDir(): File = File(context.cacheDir, "managed-updates")
            override fun installVerified(file: File, manifest: SignedManifest, authorized: () -> Boolean): InstallOutcome {
                val attempt = updater.installVerifiedFile(file, manifest) {
                    authorized() && policy.isOwner && policy.store.configured
                }
                return InstallOutcome(attempt.status, attempt.versionCode, attempt.error)
            }
        },
        trustRoot = BuildConfig.RELEASE_TRUST_ROOT,
        elapsedMs = { SystemClock.elapsedRealtime() },
        boot = { Settings.Global.getInt(context.contentResolver, Settings.Global.BOOT_COUNT, -1) },
    )

    fun canScheduleExpiry(): Boolean = Build.VERSION.SDK_INT < 31 || alarms.canScheduleExactAlarms()

    fun openMaintenance(code: String): String {
        check(policy.isOwner && policy.store.configured) { "Managed parent setup required" }
        check(canScheduleExpiry()) { "Exact maintenance-expiry alarms are unavailable; repair setup before opening maintenance" }
        val result = recovery.authorize(code)
        if (!result.accepted) return if (result.state.waitMs(recovery.clock()) > 0) "Recovery rate limited; wait before retrying" else "Invalid or already used recovery code"
        try { scheduleExpiry() } catch (e: RuntimeException) {
            recovery.close()
            policy.reconcile()
            throw IllegalStateException("Cannot schedule maintenance expiry; access remains locked", e)
        }
        policy.reconcile()
        return "Parent maintenance opened for five minutes"
    }

    fun activateRecoveryCodes(codes: List<String>) {
        check(policy.isOwner) { "Student device ownership required" }
        recovery.activateCodes(codes, bootstrap = !policy.store.configured)
    }

    fun closeMaintenance() {
        recovery.close()
        alarms.cancel(expiryIntent())
        policy.reconcile()
    }

    fun reconcile(): String {
        if (policy.isUnlocked) {
            if (policy.inMaintenance) {
                try { scheduleExpiry() } catch (_: RuntimeException) { recovery.close() }
            } else recovery.close()
            updater.reconcile()
            policy.applyLastKnownRemote()
            val scheduled = ManagementSyncWorker.schedule(context)
            if (!scheduled) policy.store.record("WorkManager catch-up could not be scheduled")
        }
        return policy.reconcile()
    }

    suspend fun enrollManagement(rawQr: String, replace: Boolean = false): ManagementSyncResult {
        check(policy.isOwner && (policy.inMaintenance || !policy.store.configured)) { "Parent setup or maintenance required" }
        val result = managementSession().enroll(rawQr, replace)
        val scheduled = ManagementSyncWorker.schedule(context)
        if (!scheduled) policy.store.record("WorkManager catch-up could not be scheduled")
        policy.applyLastKnownRemote()
        return result
    }

    suspend fun syncManagement(): ManagementSyncResult {
        if (!policy.isOwner || !policy.store.configured) {
            return ManagementSyncResult("Managed parent setup required")
        }
        return managementSession().sync()
    }

    fun applyRemoteRecovery(requestId: String, codes: List<String>): String {
        check(policy.isOwner && policy.store.configured) { "Managed parent setup required" }
        return recovery.activateRemoteCodes(requestId, codes).ackReportId
    }

    fun openRemoteLease(requestId: String, durationMs: Long): String {
        check(policy.isOwner && policy.store.configured) { "Managed parent setup required" }
        val existing = recovery.ackReportId(requestId)
        if (existing != null) return existing
        check(canScheduleExpiry()) { "Exact maintenance-expiry alarms are unavailable" }
        val result = recovery.openRemoteLease(requestId, durationMs)
        if (result.firstApply) {
            try { scheduleExpiry() } catch (e: RuntimeException) {
                recovery.close()
                policy.reconcile()
                throw IllegalStateException("Cannot schedule maintenance expiry; access remains locked", e)
            }
            policy.reconcile()
        }
        return result.ackReportId
    }

    fun startHome() {
        if (policy.isOwner && policy.store.configured && policy.isUnlocked) {
            context.startActivity(Intent(context, MainActivity::class.java)
                .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP))
        }
    }

    private fun scheduleExpiry() {
        check(canScheduleExpiry())
        val state = recovery.read()
        check(state.maintenanceActive(recovery.clock()))
        alarms.setExactAndAllowWhileIdle(AlarmManager.ELAPSED_REALTIME_WAKEUP,
            state.leaseUntilElapsedMs, expiryIntent())
    }
    private fun expiryIntent() = PendingIntent.getBroadcast(context, 1,
        Intent(context, MaintenanceExpiryReceiver::class.java).setAction("com.aleksclark.primer.student.END_MAINTENANCE"),
        PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE)
}
