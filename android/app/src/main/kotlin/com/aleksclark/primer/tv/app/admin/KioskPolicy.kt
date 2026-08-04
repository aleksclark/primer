package com.aleksclark.primer.tv.app.admin

import android.app.Activity
import android.app.ActivityManager
import android.app.UiModeManager
import android.app.admin.DevicePolicyManager
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.content.pm.PackageManager
import android.content.res.Configuration
import android.os.UserManager
import android.util.Log

internal object KioskPolicy {
    fun isTelevision(uiModeType: Int, buildCharacteristics: String): Boolean =
        uiModeType == Configuration.UI_MODE_TYPE_TELEVISION ||
            buildCharacteristics.split(',').any { it.trim() == "box" }

    fun isEligible(isTelevision: Boolean, isDeviceOwner: Boolean): Boolean =
        isTelevision && isDeviceOwner

    val userRestrictions: Set<String> = setOf(
        UserManager.DISALLOW_ADD_USER,
        UserManager.DISALLOW_REMOVE_USER,
        UserManager.DISALLOW_FACTORY_RESET,
        UserManager.DISALLOW_SAFE_BOOT,
        UserManager.DISALLOW_CREATE_WINDOWS,
        UserManager.DISALLOW_MOUNT_PHYSICAL_MEDIA,
    )
}

class KioskController(private val context: Context) {
    private val policyManager = context.getSystemService(DevicePolicyManager::class.java)
    private val uiModeManager = context.getSystemService(UiModeManager::class.java)
    private val admin = ComponentName(context, PrimerDeviceAdminReceiver::class.java)

    fun isTelevision(): Boolean = KioskPolicy.isTelevision(
        uiModeType = uiModeManager?.currentModeType ?: Configuration.UI_MODE_TYPE_UNDEFINED,
        buildCharacteristics = buildCharacteristics(),
    )

    fun isManagedTelevision(): Boolean = KioskPolicy.isEligible(
        isTelevision = isTelevision(),
        isDeviceOwner = policyManager?.isDeviceOwnerApp(context.packageName) == true,
    )

    private fun buildCharacteristics(): String = try {
        Class.forName("android.os.SystemProperties")
            .getMethod("get", String::class.java, String::class.java)
            .invoke(null, "ro.build.characteristics", "") as String
    } catch (_: ReflectiveOperationException) {
        ""
    }

    fun reconcile() {
        if (!isManagedTelevision()) return
        runPolicyCall("configure lock task") {
            policyManager.setLockTaskPackages(admin, arrayOf(context.packageName))
            policyManager.setLockTaskFeatures(admin, DevicePolicyManager.LOCK_TASK_FEATURE_NONE)
        }
        runPolicyCall("configure persistent home") {
            val home = ComponentName(context.packageName, "${context.packageName}.KioskAlias")
            context.packageManager.setComponentEnabledSetting(
                home,
                PackageManager.COMPONENT_ENABLED_STATE_ENABLED,
                PackageManager.DONT_KILL_APP,
            )
            val homeFilter = IntentFilter(Intent.ACTION_MAIN).apply {
                addCategory(Intent.CATEGORY_HOME)
                addCategory(Intent.CATEGORY_DEFAULT)
            }
            policyManager.addPersistentPreferredActivity(admin, homeFilter, home)
        }
        KioskPolicy.userRestrictions.forEach { restriction ->
            runPolicyCall("apply $restriction") {
                policyManager.addUserRestriction(admin, restriction)
            }
        }
    }

    fun enterLockTask(activity: Activity) {
        if (!isManagedTelevision()) return
        reconcile()
        val activityManager = context.getSystemService(ActivityManager::class.java)
        if (activityManager?.lockTaskModeState != ActivityManager.LOCK_TASK_MODE_NONE) return
        if (policyManager?.isLockTaskPermitted(context.packageName) != true) return
        runPolicyCall("start lock task") { activity.startLockTask() }
    }

    private inline fun runPolicyCall(operation: String, call: () -> Unit) {
        try {
            call()
        } catch (error: RuntimeException) {
            Log.e(TAG, "Unable to $operation", error)
        }
    }

    companion object {
        private const val TAG = "PrimerKiosk"

        fun adminComponent(context: Context): String =
            ComponentName(context, PrimerDeviceAdminReceiver::class.java).flattenToString()

        fun hasLeanback(context: Context): Boolean =
            context.packageManager.hasSystemFeature(PackageManager.FEATURE_LEANBACK)
    }
}
