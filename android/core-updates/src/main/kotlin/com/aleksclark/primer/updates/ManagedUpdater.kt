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

data class InstallAttempt(
    val status: String,
    val versionCode: Long? = null,
    val error: String? = null,
)

class ManagedUpdater(
    private val context: Context,
    private val resultReceiver: ComponentName,
) {
    private val prefs = context.getSharedPreferences("managed-update", Context.MODE_PRIVATE)
    private val installer = context.packageManager.packageInstaller
    val status: String get() = prefs.getString("status", "No update attempted")!!
    val version: Long get() = installed().longVersionCode
    val active: Boolean get() = prefs.getBoolean("active", false)
    val pendingTargetId: String get() = prefs.getString("releaseTargetId", "").orEmpty()
    val pendingTargetVersion: Long get() = prefs.getLong("releaseTargetVersion", 0)
    val lastOutcome: InstallAttempt get() = InstallAttempt(
        status = prefs.getString("outcomeStatus", "queued") ?: "queued",
        versionCode = prefs.getLong("outcomeVersion", -1).takeIf { it >= 0 },
        error = prefs.getString("outcomeError", null),
    )

    @Suppress("DEPRECATION")
    private fun installed(): PackageInfo = installedFor(context.packageName)

    @Suppress("DEPRECATION")
    private fun installedFor(packageName: String): PackageInfo =
        context.packageManager.getPackageInfo(packageName, PackageManager.GET_SIGNING_CERTIFICATES)

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
            resetAttempt("Verifying local APK")
            context.contentResolver.openInputStream(uri).use { input ->
                requireNotNull(input) { "Selected APK could not be opened" }
                partial.outputStream().use { ArchiveChecks.copyVerified(input, it, size, checksum) }
            }
            if (verified.exists()) check(verified.delete()) { "Cannot clear previous staging file" }
            check(partial.renameTo(verified)) { "Cannot prepare verified APK" }
            sessionId = commitVerified(verified, expected = null, authorized = authorized)
            status
        } catch (e: Exception) {
            sessionId?.let { id -> runCatching { installer.abandonSession(id) } }
            val reason = if (e is ArchiveRejected) e.message else e.javaClass.simpleName
            fail("failed", "Update failed: $reason")
            status
        } finally { partial.delete(); verified.delete() }
    }

    fun installVerifiedFile(
        verified: File,
        expected: SignedManifest,
        authorized: () -> Boolean,
        approvedSigners: Set<String>? = null,
        allowFirstInstall: Boolean = false,
        targetId: String? = null,
        targetVersion: Long? = null,
    ): InstallAttempt = synchronized(lock) {
        check(authorized()) { "Authorization required" }
        check(context.getSystemService(DevicePolicyManager::class.java).isDeviceOwnerApp(context.packageName)) {
            "Silent install requires Student device ownership"
        }
        reconcile()
        check(!active) { "An installation is already in progress" }
        var sessionId: Int? = null
        return try {
            sessionId = commitVerified(verified, expected, authorized, approvedSigners, allowFirstInstall, targetId, targetVersion)
            lastOutcome
        } catch (e: Exception) {
            sessionId?.let { id -> runCatching { installer.abandonSession(id) } }
            val reason = if (e is ArchiveRejected) e.message else e.javaClass.simpleName
            fail("failed", reason ?: "Update failed")
            lastOutcome
        } finally {
            verified.delete()
        }
    }

    private fun commitVerified(
        verified: File,
        expected: SignedManifest?,
        authorized: () -> Boolean,
        approvedSigners: Set<String>? = null,
        allowFirstInstall: Boolean = false,
        targetId: String? = null,
        targetVersion: Long? = null,
    ): Int {
        @Suppress("DEPRECATION")
        val archive = context.packageManager.getPackageArchiveInfo(verified.path, PackageManager.GET_SIGNING_CERTIFICATES)
            ?: error("Invalid Android APK archive")
        val nativeAbis = ZipFile(verified).use { zip ->
            zip.entries().asSequence().map { it.name }.filter { it.startsWith("lib/") && it.endsWith(".so") }
                .map { it.split('/')[1] }.toSet()
        }
        val candidate = identity(archive, nativeAbis)
        val installedIdentity = runCatching { identity(installedFor(expected?.packageName ?: context.packageName), emptySet()) }.getOrNull()
        ArchiveChecks.validateArchive(
            installed = installedIdentity,
            archive = candidate,
            sdk = Build.VERSION.SDK_INT,
            abis = Build.SUPPORTED_ABIS.toSet(),
            expectedPackage = expected?.packageName ?: context.packageName,
            expectedSigners = approvedSigners ?: expected?.let { setOf(it.signerSha256) } ?: installedIdentity?.signers,
            allowFirstInstall = allowFirstInstall,
        )
        if (expected != null) {
            check(candidate.packageName == expected.packageName) { "APK belongs to another application" }
            check(candidate.version == expected.versionCode) { "APK version differs from target" }
            check(candidate.signers == setOf(expected.signerSha256)) { "APK signing identity differs" }
            check(candidate.minSdk == expected.minSdk) { "APK minSdk differs from target" }
            ArchiveChecks.validateExpected(expected.byteSize, expected.sha256)
            check(verified.length() == expected.byteSize) { "APK is incomplete" }
        }
        check(authorized()) { "Authorization expired before installation" }
        val params = PackageInstaller.SessionParams(PackageInstaller.SessionParams.MODE_FULL_INSTALL).apply {
            setAppPackageName(expected?.packageName ?: context.packageName)
            setSize(verified.length())
            setInstallReason(PackageManager.INSTALL_REASON_POLICY)
            if (Build.VERSION.SDK_INT >= 33) setPackageSource(PackageInstaller.PACKAGE_SOURCE_LOCAL_FILE)
            if (Build.VERSION.SDK_INT >= 31) setRequireUserAction(PackageInstaller.SessionParams.USER_ACTION_NOT_REQUIRED)
        }
        val sessionId = installer.createSession(params)
        try {
            installer.openSession(sessionId).use { session ->
                verified.inputStream().use { input ->
                    session.openWrite("base.apk", 0, verified.length()).use { output -> input.copyTo(output); session.fsync(output) }
                }
                check(authorized()) { "Authorization expired before commit" }
                persistAttempt(sessionId, candidate.version, expected, targetId, targetVersion)
                val intent = Intent(ACTION_RESULT).setComponent(resultReceiver)
                val flags = PendingIntent.FLAG_UPDATE_CURRENT or
                    if (Build.VERSION.SDK_INT >= 31) PendingIntent.FLAG_MUTABLE else 0
                val callback = PendingIntent.getBroadcast(context, sessionId, intent, flags)
                session.commit(callback.intentSender)
            }
            return sessionId
        } catch (error: Exception) {
            runCatching { installer.abandonSession(sessionId) }
            throw error
        }
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
                runCatching { installer.abandonSession(prefs.getInt("session", -1)) }
                fail("blocked", "Android requires parent/system installation approval")
            }
            else -> {
                val verificationFailure = intent.getStringExtra(PackageInstaller.EXTRA_STATUS_MESSAGE)
                    ?.startsWith("INSTALL_FAILED_VERIFICATION_FAILURE") == true
                val reason = if (verificationFailure) "Android package verification rejected the APK; parent review is required"
                    else "Android status $code"
                fail("failed", reason)
            }
        }
    }

    fun reconcile(): String = synchronized(lock) {
        val target = prefs.getLong("target", 0)
        if (target > 0 && version == target) {
            record("Confirmed installed version $target", active = false, outcome = "confirmed", versionCode = version)
        } else if (target > 0 && version > target) {
            fail("failed", "Observed version $version; previous update target $target superseded")
        } else if (active) {
            val info = installer.getSessionInfo(prefs.getInt("session", -1))
            if (info == null) fail("failed", "Installation interrupted or rejected; installed version is $version")
            else if (!info.isSealed) {
                try {
                    installer.abandonSession(info.sessionId)
                    fail("failed", "Installation interrupted before commit; retry from maintenance")
                } catch (error: RuntimeException) {
                    fail("failed", "Interrupted installation cleanup failed: ${error.javaClass.simpleName}")
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

    private fun resetAttempt(status: String) {
        check(
            prefs.edit()
                .remove("target")
                .remove("session")
                .remove("releaseTargetId")
                .remove("releaseTargetVersion")
                .putBoolean("active", false)
                .putString("status", status)
                .putString("outcomeStatus", "queued")
                .remove("outcomeError")
                .commit(),
        ) { "Cannot persist new update attempt" }
    }

    private fun persistAttempt(
        sessionId: Int,
        versionCode: Long,
        expected: SignedManifest?,
        targetId: String? = null,
        targetVersion: Long? = null,
    ) {
        val editor = prefs.edit()
            .putBoolean("active", true)
            .putInt("session", sessionId)
            .putLong("target", versionCode)
            .putString("status", "Installing version $versionCode")
            .putString("outcomeStatus", "installing")
            .remove("outcomeVersion")
        if (expected != null) {
            editor.putString("expectedSha256", expected.sha256)
                .putLong("expectedSize", expected.byteSize)
                .putString("expectedSigner", expected.signerSha256)
        }
        if (!targetId.isNullOrBlank()) editor.putString("releaseTargetId", targetId) else editor.remove("releaseTargetId")
        if (targetVersion != null && targetVersion > 0) editor.putLong("releaseTargetVersion", targetVersion) else editor.remove("releaseTargetVersion")
        check(editor.commit()) { "Could not persist install attempt" }
    }

    private fun record(status: String, active: Boolean, outcome: String? = null, versionCode: Long? = null) {
        val editor = prefs.edit().putString("status", status).putBoolean("active", active)
        if (outcome != null) editor.putString("outcomeStatus", outcome)
        if (versionCode != null) editor.putLong("outcomeVersion", versionCode)
        check(editor.commit()) { "Cannot persist installation status" }
    }

    private fun fail(outcome: String, reason: String) {
        check(
            prefs.edit()
                .putString("status", "Update failed: $reason")
                .putBoolean("active", false)
                .putString("outcomeStatus", outcome)
                .putString("outcomeError", reason.take(200))
                .commit(),
        ) { "Cannot persist installation status" }
    }

    companion object {
        const val ACTION_RESULT = "com.aleksclark.primer.student.INSTALL_RESULT"
        private val lock = Any()
    }
}
