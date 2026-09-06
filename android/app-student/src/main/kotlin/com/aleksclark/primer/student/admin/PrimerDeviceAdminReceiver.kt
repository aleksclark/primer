package com.aleksclark.primer.student.admin

import android.app.admin.DeviceAdminReceiver
import android.content.Context
import android.content.Intent
import com.aleksclark.primer.student.StudentRuntime

/** Stable component identity: never rename after a device has been enrolled. */
class PrimerDeviceAdminReceiver : DeviceAdminReceiver() {
    override fun onEnabled(context: Context, intent: Intent) { StudentRuntime(context).reconcile() }
    override fun onProfileProvisioningComplete(context: Context, intent: Intent) {
        val runtime = StudentRuntime(context)
        runtime.reconcile()
        context.startActivity(Intent(context, com.aleksclark.primer.student.MainActivity::class.java)
            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
    }
}
