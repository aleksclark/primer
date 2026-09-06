package com.aleksclark.primer.student.admin

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.util.Log
import com.aleksclark.primer.student.StudentRuntime

class LifecycleReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        if (intent.action !in setOf(Intent.ACTION_LOCKED_BOOT_COMPLETED, Intent.ACTION_BOOT_COMPLETED, Intent.ACTION_MY_PACKAGE_REPLACED)) return
        val runtime = StudentRuntime(context)
        try {
            if (intent.action == Intent.ACTION_BOOT_COMPLETED && runtime.policy.isUnlocked) runtime.recovery.close()
            runtime.reconcile()
            runtime.startHome()
        } catch (e: RuntimeException) {
            runtime.policy.store.record("Lifecycle reconciliation failed: ${e.javaClass.simpleName}")
            Log.e("PrimerStudent", "Lifecycle policy reconciliation failed: ${e.javaClass.simpleName}")
        }
    }
}

class MaintenanceExpiryReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        if (intent.action != "com.aleksclark.primer.student.END_MAINTENANCE") return
        val runtime = StudentRuntime(context)
        try {
            runtime.closeMaintenance()
            runtime.startHome()
        } catch (e: RuntimeException) {
            runtime.policy.store.record("Maintenance expiry failed: ${e.javaClass.simpleName}")
            Log.e("PrimerStudent", "Maintenance expiry failed: ${e.javaClass.simpleName}")
        }
    }
}
