package com.aleksclark.primer.devicepolicy

import android.app.Activity
import android.app.ActivityManager
import android.app.ActivityOptions
import android.app.KeyguardManager
import android.app.admin.DevicePolicyManager
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.content.pm.PackageManager
import android.os.Build
import android.os.UserManager
import android.provider.Settings
import android.Manifest

class DevicePolicyController(
    private val context: Context,
    private val admin: ComponentName,
    private val home: ComponentName,
) {
    private val dpm = context.getSystemService(DevicePolicyManager::class.java)
    val store = PolicyStore(context)
    val isOwner: Boolean get() = dpm.isDeviceOwnerApp(context.packageName)
    val isUnlocked: Boolean get() = context.getSystemService(UserManager::class.java).isUserUnlocked
    val inMaintenance: Boolean get() = isUnlocked && RecoveryStore(context).let { it.read().maintenanceActive(it.clock()) }
    val lockTaskState: Int get() = context.getSystemService(ActivityManager::class.java).lockTaskModeState

    fun availableApps(): List<ApprovedApp> {
        val intent = Intent(Intent.ACTION_MAIN).addCategory(Intent.CATEGORY_LAUNCHER)
        return context.packageManager.queryIntentActivities(intent, 0)
            .map { it.activityInfo.packageName }.distinct().filter { it != context.packageName }
            .mapNotNull { pkg ->
                try {
                    val info = context.packageManager.getApplicationInfo(pkg, 0)
                    ApprovedApp(pkg, context.packageManager.getApplicationLabel(info).toString(),
                        PackageIdentity.signers(context.packageManager, pkg)).takeIf { it.signers.isNotEmpty() }
                } catch (_: PackageManager.NameNotFoundException) { null }
            }.sortedBy { it.label.lowercase() }
    }

    fun reconcile(): String {
        if (!isOwner) return "Unmanaged: Student is not device owner".also(store::record)
        if (!store.configured) return "Device owner; parent setup required (kiosk not enabled)".also(store::record)
        val errors = mutableListOf<String>()
        fun apply(operation: String, block: () -> Unit) {
            try { block() } catch (e: RuntimeException) { errors += "$operation: ${e.javaClass.simpleName}" }
        }
        val trusted = store.apps().filter { app ->
            val actual = try { PackageIdentity.signers(context.packageManager, app.packageName) }
                catch (_: PackageManager.NameNotFoundException) { emptySet() }
            (actual.isNotEmpty() && actual == app.signers).also {
                if (!it) errors += "Unavailable or changed signer: ${app.packageName}"
            }
        }
        val packages = (listOf(context.packageName) + trusted.map { it.packageName } +
            if (inMaintenance) maintenancePackages() else emptyList()).distinct().toTypedArray()
        apply("lock-task allowlist") {
            dpm.setLockTaskPackages(admin, packages)
            check(dpm.getLockTaskPackages(admin).toSet() == packages.toSet())
        }
        var features = DevicePolicyManager.LOCK_TASK_FEATURE_HOME or
            DevicePolicyManager.LOCK_TASK_FEATURE_KEYGUARD or DevicePolicyManager.LOCK_TASK_FEATURE_GLOBAL_ACTIONS
        if (Build.VERSION.SDK_INT >= 30) features = features or DevicePolicyManager.LOCK_TASK_FEATURE_BLOCK_ACTIVITY_START_IN_TASK
        apply("lock-task features") {
            dpm.setLockTaskFeatures(admin, features)
            check(dpm.getLockTaskFeatures(admin) == features)
        }
        apply("persistent home") {
            context.packageManager.setComponentEnabledSetting(home, PackageManager.COMPONENT_ENABLED_STATE_ENABLED,
                PackageManager.DONT_KILL_APP)
            dpm.addPersistentPreferredActivity(admin, IntentFilter(Intent.ACTION_MAIN).apply {
                addCategory(Intent.CATEGORY_HOME); addCategory(Intent.CATEGORY_DEFAULT)
            }, home)
            val resolved = context.packageManager.resolveActivity(Intent(Intent.ACTION_MAIN).addCategory(Intent.CATEGORY_HOME),
                PackageManager.MATCH_DEFAULT_ONLY)?.activityInfo
            check(resolved?.packageName == context.packageName) { "Home readback differs" }
        }
        for (restriction in restrictions) apply(restriction) {
            dpm.addUserRestriction(admin, restriction)
            check(dpm.getUserRestrictions(admin).getBoolean(restriction))
        }
        apply("prevent Student uninstall") {
            dpm.setUninstallBlocked(admin, context.packageName, true)
            check(dpm.isUninstallBlocked(admin, context.packageName))
        }
        val report = if (errors.isEmpty()) {
            if (inMaintenance) "Applied: parent maintenance (bounded)" else "Applied: approved-app policy"
        } else "Partial/failed: ${errors.joinToString("; ")}"
        store.record(report)
        return report
    }

    fun applyLastKnownRemote(): String {
        if (!isOwner || !store.configured) return reconcile()
        val remote = store.remoteApps()
        if (remote.isEmpty()) return reconcile()
        val studentSigners = PackageIdentity.signers(context.packageManager, context.packageName)
        val (apps, _) = PolicyGuard.sanitizeRemoteApps(
            requested = remote,
            studentPackage = context.packageName,
            studentSigners = studentSigners,
            installedSigners = { pkg ->
                runCatching { PackageIdentity.signers(context.packageManager, pkg) }.getOrDefault(emptySet())
            },
        )
        if (apps.isNotEmpty()) store.configure(apps)
        return reconcile()
    }

    fun rememberRemotePolicy(
        revision: Long,
        apps: List<ApprovedApp>,
        extraControls: List<ControlReadback> = emptyList(),
        origin: String = store.lastRemoteOrigin(),
        deviceId: String = store.lastRemoteDeviceId(),
    ): PolicyApplication {
        val studentSigners = PackageIdentity.signers(context.packageManager, context.packageName)
        val (sanitized, appControls) = PolicyGuard.sanitizeRemoteApps(
            requested = apps,
            studentPackage = context.packageName,
            studentSigners = studentSigners,
            installedSigners = { pkg ->
                runCatching { PackageIdentity.signers(context.packageManager, pkg) }.getOrDefault(emptySet())
            },
        )
        val controls = extraControls + appControls
        val status = PolicyGuard.overallStatus(controls)
        if (sanitized.isNotEmpty() && status != "failed") {
            store.rememberRemote(revision, sanitized, origin, deviceId)
            store.configure(sanitized)
        }
        val summary = reconcile()
        return PolicyApplication(revision, status, controls, summary)
    }

    fun inventory(): List<InventoriedApp> {
        val student = runCatching {
            val info = context.packageManager.getPackageInfo(context.packageName, 0)
            InventoriedApp(
                packageName = context.packageName,
                label = "Primer Student",
                versionName = info.versionName.orEmpty(),
                versionCode = info.longVersionCode,
                signerSha256 = PackageIdentity.signers(context.packageManager, context.packageName).firstOrNull().orEmpty(),
            )
        }.getOrNull()
        val launchable = availableApps().map { app ->
            val info = runCatching { context.packageManager.getPackageInfo(app.packageName, 0) }.getOrNull()
            InventoriedApp(
                packageName = app.packageName,
                label = app.label,
                versionName = info?.versionName.orEmpty(),
                versionCode = info?.longVersionCode ?: 0,
                signerSha256 = app.signers.firstOrNull().orEmpty(),
            )
        }
        return listOfNotNull(student) + launchable
    }

    fun enterLockTask(activity: Activity): Boolean {
        if (!isOwner || !store.configured || context.getSystemService(KeyguardManager::class.java).isKeyguardLocked) return false
        reconcile()
        if (lockTaskState == ActivityManager.LOCK_TASK_MODE_LOCKED) return true
        // Never fall back to user-removable screen pinning.
        check(dpm.isLockTaskPermitted(context.packageName)) { "Student is not lock-task allowlisted" }
        activity.startLockTask()
        // Android can complete this transition asynchronously, especially during boot.
        // Caller must observe actual mode; returning here means requested, not confirmed.
        return true
    }

    fun launchApproved(app: ApprovedApp) {
        check(isOwner && store.configured)
        check(store.apps().any { it == app }) { "App is not approved" }
        check(PackageIdentity.signers(context.packageManager, app.packageName) == app.signers) { "App signer changed" }
        val intent = context.packageManager.getLaunchIntentForPackage(app.packageName)
            ?: error("Approved app has no launchable activity")
        context.startActivity(intent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK),
            ActivityOptions.makeBasic().apply { setLockTaskEnabled(true) }.toBundle())
    }

    fun openSettings() {
        check(isOwner && inMaintenance) { "Parent maintenance required" }
        reconcile()
        context.startActivity(Intent(Settings.ACTION_SETTINGS).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK),
            ActivityOptions.makeBasic().apply { setLockTaskEnabled(true) }.toBundle())
    }

    /** Explicit, parent-authorized qualification teardown; no wipe and no ownership transfer. */
    @Suppress("DEPRECATION")
    fun removeOwnership(activity: Activity) {
        check(isOwner && inMaintenance) { "Parent maintenance required" }
        if (lockTaskState != ActivityManager.LOCK_TASK_MODE_NONE) activity.stopLockTask()
        restrictions.forEach { dpm.clearUserRestriction(admin, it) }
        dpm.setUninstallBlocked(admin, context.packageName, false)
        dpm.clearPackagePersistentPreferredActivities(admin, context.packageName)
        dpm.setLockTaskPackages(admin, emptyArray())
        dpm.clearDeviceOwnerApp(context.packageName)
        check(!isOwner) { "Android did not remove device ownership" }
        context.packageManager.setComponentEnabledSetting(home, PackageManager.COMPONENT_ENABLED_STATE_DISABLED,
            PackageManager.DONT_KILL_APP)
        store.reset()
        RecoveryStore(context).close()
    }

    fun pairingCapability(): PairingCapability = PairingCapabilityPolicy.evaluate(
        cameraGranted = context.checkSelfPermission(Manifest.permission.CAMERA) == PackageManager.PERMISSION_GRANTED,
        owner = isOwner,
        inMaintenance = inMaintenance,
        permissionControllerPackage = permissionControllerPackage(),
        photoPickerPackage = photoPickerPackage(),
    )

    fun grantCameraForPairing(): String {
        check(isOwner && inMaintenance) { "Parent maintenance required" }
        dpm.setPermissionGrantState(
            admin,
            context.packageName,
            Manifest.permission.CAMERA,
            DevicePolicyManager.PERMISSION_GRANT_STATE_GRANTED,
        )
        val granted = context.checkSelfPermission(Manifest.permission.CAMERA) == PackageManager.PERMISSION_GRANTED ||
            dpm.getPermissionGrantState(admin, context.packageName, Manifest.permission.CAMERA) ==
            DevicePolicyManager.PERMISSION_GRANT_STATE_GRANTED
        check(granted) { "Android did not grant camera for pairing" }
        reconcile()
        return "Camera granted for pairing during this maintenance window"
    }

    private fun permissionControllerPackage(): String? {
        val intents = listOf(
            Intent("android.intent.action.MANAGE_PERMISSIONS"),
            Intent("android.intent.action.REVIEW_PERMISSIONS"),
            Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS).setData(android.net.Uri.fromParts("package", context.packageName, null)),
        )
        return intents.firstNotNullOfOrNull {
            context.packageManager.resolveActivity(it, PackageManager.MATCH_DEFAULT_ONLY)?.activityInfo?.packageName
        }?.takeIf { it != context.packageName }
    }

    private fun photoPickerPackage(): String? {
        val intents = listOf(
            Intent("android.provider.action.PICK_IMAGES"),
            Intent(Intent.ACTION_OPEN_DOCUMENT).addCategory(Intent.CATEGORY_OPENABLE).setType("image/*"),
        )
        return intents.firstNotNullOfOrNull {
            context.packageManager.resolveActivity(it, PackageManager.MATCH_DEFAULT_ONLY)?.activityInfo?.packageName
        }
    }

    private fun maintenancePackages(): List<String> {
        val pm = context.packageManager
        val repair = listOf(
            Intent(Settings.ACTION_SETTINGS),
            Intent(Intent.ACTION_OPEN_DOCUMENT).addCategory(Intent.CATEGORY_OPENABLE).setType("*/*"),
        ).mapNotNull { pm.resolveActivity(it, PackageManager.MATCH_DEFAULT_ONLY)?.activityInfo?.packageName }
        // Android may need to explain an APK-verification decision to the parent.
        // Only platform-authorized verifier packages, only during authenticated maintenance.
        // This does NOT disable scanning or grant any exception to the APK being verified.
        val verifiers = pm.queryBroadcastReceivers(
            Intent(Intent.ACTION_PACKAGE_NEEDS_VERIFICATION).setType("application/vnd.android.package-archive"), 0,
        ).map { it.activityInfo.packageName }.filter {
            pm.checkPermission("android.permission.PACKAGE_VERIFICATION_AGENT", it) == PackageManager.PERMISSION_GRANTED
        }
        val delegates = PairingCapabilityPolicy.maintenanceDelegates(
            permissionControllerPackage = permissionControllerPackage(),
            photoPickerPackage = photoPickerPackage(),
            documentsUiPackage = repair.firstOrNull(),
        )
        return (repair + verifiers + delegates).distinct()
    }

    companion object {
        // Deliberately leave debugging enabled during supervised hardware qualification.
        // This is NOT the full hardened daily-use policy; no hidden release/debug difference.
        val restrictions: Set<String> = setOf(
            UserManager.DISALLOW_ADD_USER, UserManager.DISALLOW_REMOVE_USER,
            UserManager.DISALLOW_FACTORY_RESET, UserManager.DISALLOW_SAFE_BOOT,
            UserManager.DISALLOW_CREATE_WINDOWS, UserManager.DISALLOW_MOUNT_PHYSICAL_MEDIA,
            UserManager.DISALLOW_INSTALL_UNKNOWN_SOURCES,
        )
    }
}
