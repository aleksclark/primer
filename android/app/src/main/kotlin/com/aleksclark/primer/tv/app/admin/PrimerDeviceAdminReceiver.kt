package com.aleksclark.primer.tv.app.admin

import android.app.admin.DeviceAdminReceiver
import android.content.Context
import android.content.Intent

class PrimerDeviceAdminReceiver : DeviceAdminReceiver() {
    override fun onEnabled(context: Context, intent: Intent) {
        super.onEnabled(context, intent)
        KioskController(context.applicationContext).reconcile()
    }

    override fun onProfileProvisioningComplete(context: Context, intent: Intent) {
        super.onProfileProvisioningComplete(context, intent)
        KioskController(context.applicationContext).reconcile()
    }
}
