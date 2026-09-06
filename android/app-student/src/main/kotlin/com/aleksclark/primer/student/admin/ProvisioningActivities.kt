package com.aleksclark.primer.student.admin

import android.app.Activity
import android.app.admin.DevicePolicyManager
import android.content.Intent
import android.os.Build
import android.os.Bundle
import com.aleksclark.primer.student.MainActivity
import com.aleksclark.primer.student.StudentRuntime

class ProvisioningModeActivity : Activity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        if (Build.VERSION.SDK_INT >= 31) {
            val allowed = intent.getIntegerArrayListExtra(DevicePolicyManager.EXTRA_PROVISIONING_ALLOWED_PROVISIONING_MODES)
            if (allowed?.contains(DevicePolicyManager.PROVISIONING_MODE_FULLY_MANAGED_DEVICE) == true) {
                setResult(RESULT_OK, Intent().putExtra(DevicePolicyManager.EXTRA_PROVISIONING_MODE,
                    DevicePolicyManager.PROVISIONING_MODE_FULLY_MANAGED_DEVICE))
            } else setResult(RESULT_CANCELED)
        } else setResult(RESULT_CANCELED)
        finish()
    }
}

class PolicyComplianceActivity : MainActivity() {
    override val enforceKiosk = false
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        if (!runtime.policy.isOwner) {
            setResult(RESULT_CANCELED)
            finish()
        } else if (runtime.policy.store.configured) onParentSetupComplete()
    }
    override fun onParentSetupComplete() {
        // Return control to ManagedProvisioning/Setup Wizard; don't trap the setup task.
        runtime.reconcile()
        setResult(RESULT_OK)
        finish()
    }
}
