package com.aleksclark.primer.updates

import android.app.PendingIntent
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.content.pm.PackageInstaller
import android.content.pm.PackageManager
import android.os.Build
import java.io.File

/**
 * Unprivileged same-package PackageInstaller path for Control (and TV self-update).
 * Never assumes device-owner silent install. Student must keep [ManagedUpdater].
 */
class SelfUpdateSession(
    private val context: Context,
    private val resultReceiver: ComponentName,
    private val storeName: String = "self-update",
) {
    private val prefs = context.getSharedPreferences(storeName, Context.MODE_PRIVATE)
    private val installer = context.packageManager.packageInstaller
    val status: String get() = prefs.getString("status", "No self-update attempted")!!
    val active: Boolean get() = prefs.getBoolean("active", false)
    val lastOutcome: InstallAttempt get() = InstallAttempt(
        status = prefs.getString("outcomeStatus", "queued") ?: "queued",
        versionCode = prefs.getLong("outcomeVersion", -1).takeIf { it >= 0 },
        error = prefs.getString("outcomeError", null),
    )
    val pendingConfirmation: Boolean get() = prefs.getBoolean("pendingConfirmation", false)

    fun install(verified: File, expected: SignedManifest, eligibility: SelfUpdateEligibility): InstallAttempt = synchronized(lock) {
        check(eligibility.canAttempt) { eligibility.reason }
        check(expected.packageName == context.packageName) { "Self-update can only replace the running package" }
        check(!active) { "An installation is already in progress" }
        var sessionId: Int? = null
        return try {
            val params = PackageInstaller.SessionParams(PackageInstaller.SessionParams.MODE_FULL_INSTALL).apply {
                setAppPackageName(context.packageName)
                setSize(verified.length())
                if (Build.VERSION.SDK_INT >= 33) setPackageSource(PackageInstaller.PACKAGE_SOURCE_LOCAL_FILE)
                if (Build.VERSION.SDK_INT >= 31) {
                    setRequireUserAction(
                        if (eligibility.unattendedEligible) PackageInstaller.SessionParams.USER_ACTION_NOT_REQUIRED
                        else PackageInstaller.SessionParams.USER_ACTION_REQUIRED,
                    )
                }
            }
            sessionId = installer.createSession(params)
            installer.openSession(sessionId).use { session ->
                verified.inputStream().use { input ->
                    session.openWrite("base.apk", 0, verified.length()).use { output ->
                        input.copyTo(output)
                        session.fsync(output)
                    }
                }
                persist(sessionId!!, expected.versionCode, "installing")
                val intent = Intent(ACTION_RESULT).setComponent(resultReceiver)
                val flags = PendingIntent.FLAG_UPDATE_CURRENT or
                    if (Build.VERSION.SDK_INT >= 31) PendingIntent.FLAG_MUTABLE else 0
                session.commit(PendingIntent.getBroadcast(context, sessionId!!, intent, flags).intentSender)
            }
            lastOutcome
        } catch (error: Exception) {
            sessionId?.let { runCatching { installer.abandonSession(it) } }
            fail("failed", error.message?.take(200) ?: error.javaClass.simpleName)
            lastOutcome
        } finally {
            verified.delete()
        }
    }

    fun handleResult(intent: Intent, onUserAction: ((Intent) -> Boolean)? = null): InstallAttempt = synchronized(lock) {
        if (intent.action != ACTION_RESULT || !active) return lastOutcome
        if (intent.getIntExtra(PackageInstaller.EXTRA_SESSION_ID, -1) != prefs.getInt("session", -2)) return lastOutcome
        when (val code = intent.getIntExtra(PackageInstaller.EXTRA_STATUS, PackageInstaller.STATUS_FAILURE)) {
            PackageInstaller.STATUS_SUCCESS -> {
                record("Installer accepted; checking running package", active = true, outcome = "installing")
                reconcile(context.packageManager.getPackageInfo(context.packageName, 0).longVersionCode)
            }
            PackageInstaller.STATUS_PENDING_USER_ACTION -> {
                @Suppress("DEPRECATION")
                val confirmation = intent.getParcelableExtra<Intent>(Intent.EXTRA_INTENT)
                val presented = confirmation != null && onUserAction?.invoke(confirmation) == true
                if (presented) {
                    prefs.edit().putBoolean("pendingConfirmation", true)
                        .putString("status", "Waiting for system install confirmation")
                        .putString("outcomeStatus", "blocked")
                        .commit()
                } else {
                    runCatching { installer.abandonSession(prefs.getInt("session", -1)) }
                    fail("blocked", "Android requires a system install confirmation")
                }
            }
            else -> fail("failed", "Android status $code")
        }
        lastOutcome
    }

    fun reconcile(installedVersion: Long): InstallAttempt = synchronized(lock) {
        val target = prefs.getLong("target", 0)
        when {
            target > 0 && installedVersion == target -> record("Confirmed installed version $target", false, "confirmed", installedVersion)
            target > 0 && installedVersion > target -> fail("failed", "Observed version $installedVersion; previous update target $target superseded")
            active && installer.getSessionInfo(prefs.getInt("session", -1)) == null && !pendingConfirmation ->
                fail("failed", "Installation interrupted or rejected; installed version is $installedVersion")
        }
        lastOutcome
    }

    private fun persist(sessionId: Int, versionCode: Long, outcome: String) {
        check(
            prefs.edit()
                .putBoolean("active", true)
                .putBoolean("pendingConfirmation", false)
                .putInt("session", sessionId)
                .putLong("target", versionCode)
                .putLong("outcomeVersion", versionCode)
                .putString("status", "Installing version $versionCode")
                .putString("outcomeStatus", outcome)
                .commit(),
        ) { "Could not persist self-update attempt" }
    }

    private fun record(status: String, active: Boolean, outcome: String, versionCode: Long? = null) {
        val editor = prefs.edit().putString("status", status).putBoolean("active", active).putString("outcomeStatus", outcome)
        if (versionCode != null) editor.putLong("outcomeVersion", versionCode)
        check(editor.commit()) { "Cannot persist self-update status" }
    }

    private fun fail(outcome: String, reason: String) {
        check(
            prefs.edit()
                .putBoolean("active", false)
                .putBoolean("pendingConfirmation", false)
                .putString("status", "Update failed: $reason")
                .putString("outcomeStatus", outcome)
                .putString("outcomeError", reason.take(200))
                .commit(),
        ) { "Cannot persist self-update status" }
    }

    companion object {
        const val ACTION_RESULT = "com.aleksclark.primer.update.SELF_UPDATE_RESULT"
        private val lock = Any()
    }
}
