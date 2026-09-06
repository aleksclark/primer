package com.aleksclark.primer.updates

import android.app.PendingIntent
import android.app.admin.DevicePolicyManager
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.content.pm.PackageInfo
import android.content.pm.PackageInstaller
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import com.aleksclark.primer.updates.ArchiveChecks.hex
import java.io.File
import java.security.MessageDigest
import java.util.zip.ZipFile

/** Local parent-selected APK qualification path. Remote release transport comes later. */
class ManagedUpdater(
    private val context: Context,
    private val resultReceiver: ComponentName,
) {
    private val prefs = context.getSharedPreferences("managed-update", Context.MODE_PRIVATE)
    private val installer = context.packageManager.packageInstaller
    val status: String get() = prefs.getString("status", "No update attempted")!!
    val version: Long get() = installed().longVersionCode
    private val active: Boolean get() = prefs.getBoolean("active", false)

    @Suppress("DEPRECATION")
    private fun installed(): PackageInfo = context.packageManager.getPackageInfo(context.packageName, PackageManager.GET_SIGNING_CERTIFICATES)

    /** Runs on a worker thread. The capability must be checked again immediately before commit. */
    fun install(uri: Uri, size: Long, checksum: String, authorized: () -> Boolean): String = synchronized(lock) {
        check(authorized()) { "Parent maintenance required" }
        check(context.getSystemService(DevicePolicyManager::class.java).isDeviceOwnerApp(context.packageName)) {
            "Silent install requires Student device ownership"
        }
        reconcile()
        check(!active) { "An installation is already in progress" }
        ArchiveChecks.validateExpected(size, checksum)
        val directory = File(context.cacheDir, "managed-updates").apply { check(mkdirs() || isDirectory) }
        val partial = File(directory, "candidate.partial")
        val verified = File(directory, "candidate.apk")
        var sessionId: Int? = null
        try {
            check(prefs.edit().remove("target").remove("session").putBoolean("active", false)
                .putString("status", "Verifying local APK").commit()) { "Cannot persist new update attempt" }
            context.contentResolver.openInputStream(uri).use { input ->
                requireNotNull(input) { "Selected APK could not be opened" }
                partial.outputStream().use { ArchiveChecks.copyVerified(input, it, size, checksum) }
            }
            if (verified.exists()) check(verified.delete()) { "Cannot clear previous staging file" }
            check(partial.renameTo(verified)) { "Cannot prepare verified APK" }
            @Suppress("DEPRECATION")
            val archive = context.packageManager.getPackageArchiveInfo(verified.path, PackageManager.GET_SIGNING_CERTIFICATES)
                ?: error("Invalid Android APK archive")
            val nativeAbis = ZipFile(verified).use { zip ->
                zip.entries().asSequence().map { it.name }.filter { it.startsWith("lib/") && it.endsWith(".so") }
                    .map { it.split('/')[1] }.toSet()
            }
            ArchiveChecks.validateArchive(identity(installed(), emptySet()), identity(archive, nativeAbis),
                Build.VERSION.SDK_INT, Build.SUPPORTED_ABIS.toSet())
            check(authorized()) { "Parent maintenance expired before installation" }
            val params = PackageInstaller.SessionParams(PackageInstaller.SessionParams.MODE_FULL_INSTALL).apply {
                setAppPackageName(context.packageName)
                setSize(size)
                setInstallReason(PackageManager.INSTALL_REASON_POLICY)
                if (Build.VERSION.SDK_INT >= 33) setPackageSource(PackageInstaller.PACKAGE_SOURCE_LOCAL_FILE)
                if (Build.VERSION.SDK_INT >= 31) setRequireUserAction(PackageInstaller.SessionParams.USER_ACTION_NOT_REQUIRED)
            }
            sessionId = installer.createSession(params)
            installer.openSession(sessionId).use { session ->
                verified.inputStream().use { input ->
                    session.openWrite("base.apk", 0, size).use { output -> input.copyTo(output); session.fsync(output) }
                }
                check(authorized()) { "Parent maintenance expired before commit" }
                check(prefs.edit().putBoolean("active", true).putInt("session", sessionId)
                    .putLong("target", archive.longVersionCode).putString("status", "Installing version ${archive.longVersionCode}")
                    .commit()) { "Could not persist install attempt" }
                val intent = Intent(ACTION_RESULT).setComponent(resultReceiver)
                val flags = PendingIntent.FLAG_UPDATE_CURRENT or
                    if (Build.VERSION.SDK_INT >= 31) PendingIntent.FLAG_MUTABLE else 0
                val callback = PendingIntent.getBroadcast(context, sessionId, intent, flags)
                session.commit(callback.intentSender)
            }
            status
        } catch (e: Exception) {
            sessionId?.let { id -> runCatching { installer.abandonSession(id) } }
            // Only application-defined validation messages are safe to display; no URI/path from providers.
            val reason = if (e is ArchiveRejected) e.message else e.javaClass.simpleName
            record("Update failed: $reason", active = false)
            status
        } finally { partial.delete(); verified.delete() }
    }

    fun handleResult(intent: Intent) = synchronized(lock) {
        if (intent.action != ACTION_RESULT || !active) return@synchronized
        if (intent.getIntExtra(PackageInstaller.EXTRA_SESSION_ID, -1) != prefs.getInt("session", -2)) return@synchronized
        when (val code = intent.getIntExtra(PackageInstaller.EXTRA_STATUS, PackageInstaller.STATUS_FAILURE)) {
            PackageInstaller.STATUS_SUCCESS -> {
                record("Installer accepted; checking running package", active = true)
                reconcile()
            }
            PackageInstaller.STATUS_PENDING_USER_ACTION -> {
                // Never expose an unrestricted package installer to a managed student.
                runCatching { installer.abandonSession(prefs.getInt("session", -1)) }
                record("Blocked: Android requires parent/system installation approval", active = false)
            }
            else -> {
                val verificationFailure = intent.getStringExtra(PackageInstaller.EXTRA_STATUS_MESSAGE)
                    ?.startsWith("INSTALL_FAILED_VERIFICATION_FAILURE") == true
                // Do not expose the platform's raw message (it can contain paths/URIs).
                val reason = if (verificationFailure) "Android package verification rejected the APK; parent review is required"
                    else "Android status $code"
                record("Installation failed: $reason", active = false)
            }
        }
    }

    fun reconcile(): String = synchronized(lock) {
        val target = prefs.getLong("target", 0)
        if (target > 0 && version == target) {
            record("Confirmed installed version $target", active = false)
        } else if (target > 0 && version > target) {
            // An operator may repair with a higher build. Do not attribute that external
            // replacement to this attempt or leave a superseded target active forever.
            record("Observed version $version; previous update target $target superseded", active = false)
        } else if (active) {
            val info = installer.getSessionInfo(prefs.getInt("session", -1))
            if (info == null) record("Installation interrupted or rejected; installed version is $version", active = false)
            else if (!info.isSealed) {
                // Process death between persisting the session ID and committing must
                // not leave the updater wedged on an uncommitted session forever.
                try {
                    installer.abandonSession(info.sessionId)
                    record("Installation interrupted before commit; retry from maintenance", active = false)
                } catch (error: RuntimeException) {
                    record("Interrupted installation cleanup failed: ${error.javaClass.simpleName}", active = false)
                }
            }
        }
        status
    }

    private fun identity(info: PackageInfo, abis: Set<String>) = ArchiveIdentity(
        info.packageName, info.longVersionCode,
        info.signingInfo?.apkContentsSigners?.map { MessageDigest.getInstance("SHA-256").digest(it.toByteArray()).hex() }?.toSet() ?: emptySet(),
        info.applicationInfo?.minSdkVersion ?: Int.MAX_VALUE, abis,
    )
    private fun record(status: String, active: Boolean) {
        if (status == this.status && active == this.active) return
        check(prefs.edit().putString("status", status).putBoolean("active", active).commit()) { "Cannot persist installation status" }
    }
    companion object {
        const val ACTION_RESULT = "com.aleksclark.primer.student.INSTALL_RESULT"
        private val lock = Any()
    }
}
