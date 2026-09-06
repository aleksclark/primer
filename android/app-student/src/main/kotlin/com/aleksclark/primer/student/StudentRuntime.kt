package com.aleksclark.primer.student

import android.app.AlarmManager
import android.app.PendingIntent
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.os.Build
import com.aleksclark.primer.devicepolicy.DevicePolicyController
import com.aleksclark.primer.devicepolicy.RecoveryStore
import com.aleksclark.primer.student.admin.MaintenanceExpiryReceiver
import com.aleksclark.primer.student.admin.PrimerDeviceAdminReceiver
import com.aleksclark.primer.student.update.InstallResultReceiver
import com.aleksclark.primer.updates.ManagedUpdater

class StudentRuntime(private val context: Context) {
    val policy = DevicePolicyController(context, ComponentName(context, PrimerDeviceAdminReceiver::class.java),
        ComponentName(context.packageName, "${context.packageName}.StudentHome"))
    val recovery by lazy { RecoveryStore(context) }
    val updater by lazy { ManagedUpdater(context, ComponentName(context, InstallResultReceiver::class.java)) }
    private val alarms = context.getSystemService(AlarmManager::class.java)

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
        }
        return policy.reconcile()
    }

    fun applyRemoteRecovery(requestId: String, codes: List<String>): Boolean {
        check(policy.isOwner && policy.store.configured) { "Managed parent setup required" }
        return recovery.activateRemoteCodes(requestId, codes)
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
