package com.aleksclark.primer.updates

import android.app.PendingIntent
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.content.pm.PackageInfo
import android.content.pm.PackageInstaller
import android.content.pm.PackageManager
import android.os.Build
import android.os.Parcel
import android.util.Base64
import com.aleksclark.primer.updates.ArchiveChecks.hex
import java.io.File
import java.security.MessageDigest
import java.util.zip.ZipFile

/**
 * Unprivileged same-package PackageInstaller path for Control (and TV self-update).
 * Inspects the APK itself. Never assumes device-owner silent install.
 */
class SelfUpdateSession(
    private val context: Context,
    private val resultReceiver: ComponentName,
    private val storeName: String = "self-update",
) : SelfUpdateCommands {
    private val prefs = context.getSharedPreferences(storeName, Context.MODE_PRIVATE)
    private val installer = context.packageManager.packageInstaller
    @Volatile private var liveConfirmation: Intent? = null
    val status: String get() = snapshot().status
    val active: Boolean get() = snapshot().active
    val pendingConfirmation: Boolean get() = snapshot().pendingConfirmation
    val desiredVersion: Long get() = snapshot().desiredVersion
    val lastOutcome: InstallAttempt get() = snapshot().lastOutcome

    override fun snapshot(): SelfUpdateSessionState = synchronized(lock) {
        val sessionId = prefs.getInt("session", -1)
        val info = if (sessionId >= 0) installer.getSessionInfo(sessionId) else null
        SelfUpdateSessionState(
            status = prefs.getString("status", "No self-update attempted")!!,
            active = prefs.getBoolean("active", false),
            pendingConfirmation = prefs.getBoolean("pendingConfirmation", false),
            installerSessionLive = info != null,
            desiredVersion = prefs.getLong("desiredVersion", 0),
            lastOutcome = InstallAttempt(
                status = prefs.getString("outcomeStatus", "queued") ?: "queued",
                versionCode = prefs.getLong("outcomeVersion", -1).takeIf { it >= 0 },
                error = prefs.getString("outcomeError", null),
            ),
            hasConfirmationIntent = liveConfirmation != null || !prefs.getString("confirmation", null).isNullOrBlank(),
        )
    }

    override fun evaluate(apk: File, expected: SignedManifest): SelfUpdateEligibility = synchronized(lock) {
        val directory = File(context.cacheDir, "self-updates").apply { check(mkdirs() || isDirectory) }
        val snapshot = File.createTempFile("evaluate-", ".apk", directory)
        try {
            inspectVerified(apk, snapshot, expected).second
        } finally {
            snapshot.delete()
        }
    }

    override fun install(apk: File, expected: SignedManifest): InstallAttempt = synchronized(lock) {
        reconcile()
        check(!active) { "An installation is already in progress" }
        var sessionId: Int? = null
        val directory = File(context.cacheDir, "self-updates").apply { check(mkdirs() || isDirectory) }
        val snapshot = File.createTempFile("candidate-", ".apk", directory)
        return try {
            val eligibility = inspectVerified(apk, snapshot, expected).second
            val params = PackageInstaller.SessionParams(PackageInstaller.SessionParams.MODE_FULL_INSTALL).apply {
                setAppPackageName(context.packageName)
                setSize(snapshot.length())
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
                snapshot.inputStream().use { input ->
                    session.openWrite("base.apk", 0, snapshot.length()).use { output ->
                        input.copyTo(output)
                        session.fsync(output)
                    }
                }
                liveConfirmation = null
                persist(sessionId!!, expected.versionCode)
                val intent = Intent(ACTION_RESULT).setComponent(resultReceiver)
                val flags = PendingIntent.FLAG_UPDATE_CURRENT or
                    if (Build.VERSION.SDK_INT >= 31) PendingIntent.FLAG_MUTABLE else 0
                session.commit(PendingIntent.getBroadcast(context, sessionId!!, intent, flags).intentSender)
            }
            lastOutcome
        } catch (error: Exception) {
            sessionId?.let { runCatching { installer.abandonSession(it) } }
            fail("failed", sanitized(error))
            lastOutcome
        } finally {
            // This session owns the snapshot; callers retain ownership of their input.
            snapshot.delete()
        }
    }

    override fun handleResult(intent: Intent, onUserAction: ((Intent) -> Boolean)?): InstallAttempt = synchronized(lock) {
        if (intent.action != ACTION_RESULT || !active) return lastOutcome
        if (intent.getIntExtra(PackageInstaller.EXTRA_SESSION_ID, -1) != prefs.getInt("session", -2)) return lastOutcome
        when (val code = intent.getIntExtra(PackageInstaller.EXTRA_STATUS, PackageInstaller.STATUS_FAILURE)) {
            PackageInstaller.STATUS_SUCCESS -> {
                record("Installer accepted; checking running package", active = true, outcome = "installing", installed = null)
                reconcile()
            }
            PackageInstaller.STATUS_PENDING_USER_ACTION -> {
                @Suppress("DEPRECATION")
                val confirmation = intent.getParcelableExtra<Intent>(Intent.EXTRA_INTENT)
                liveConfirmation = confirmation
                if (confirmation != null) persistConfirmation(confirmation)
                val presented = try {
                    confirmation != null && onUserAction?.invoke(confirmation) == true
                } catch (_: RuntimeException) {
                    false
                }
                applyRecord(
                    SelfUpdateFlow.onPendingUserAction(
                        current = record().copy(hasConfirmation = confirmation != null),
                        hasConfirmation = confirmation != null,
                        presented = presented,
                    ),
                )
            }
            else -> fail("failed", "Android status $code")
        }
        lastOutcome
    }

    override fun resumeUserAction(onUserAction: (Intent) -> Boolean): InstallAttempt = synchronized(lock) {
        reconcile()
        val confirmation = liveConfirmation ?: restoreConfirmation()
        liveConfirmation = confirmation ?: liveConfirmation
        val live = snapshot().installerSessionLive
        val presented = try {
            confirmation != null && onUserAction(confirmation)
        } catch (_: RuntimeException) {
            false
        }
        applyRecord(
            SelfUpdateFlow.onResumeUserAction(
                current = record().copy(hasConfirmation = confirmation != null),
                sessionLive = live,
                presented = presented,
            ),
        )
        lastOutcome
    }

    override fun cancel(): InstallAttempt = synchronized(lock) {
        runCatching { installer.abandonSession(prefs.getInt("session", -1)) }
        liveConfirmation = null
        applyRecord(SelfUpdateFlow.onCancel(record()))
        lastOutcome
    }

    override fun clearFailed(): InstallAttempt = synchronized(lock) {
        reconcile()
        val current = record()
        if (current.active || current.pendingConfirmation) {
            error("Cancel the live install confirmation before retrying.")
        }
        val next = SelfUpdateFlow.onClearFailed(current)
        if (next === current) return lastOutcome
        runCatching { installer.abandonSession(prefs.getInt("session", -1)) }
        liveConfirmation = null
        applyRecord(next)
        lastOutcome
    }

    override fun reconcile(): InstallAttempt = synchronized(lock) {
        val installed = runCatching { installedInfo().longVersionCode }.getOrDefault(-1L)
        val desired = prefs.getLong("desiredVersion", 0)
        val sessionId = prefs.getInt("session", -1)
        val info = if (sessionId >= 0) installer.getSessionInfo(sessionId) else null
        when {
            desired > 0 && installed == desired -> {
                record("Confirmed installed version $installed", false, "confirmed", installed)
            }
            desired > 0 && installed > desired -> fail("failed", "Observed version $installed; previous update target superseded")
            info != null && !info.isSealed -> {
                runCatching { installer.abandonSession(info.sessionId) }
                fail("failed", "Installation interrupted before commit")
            }
            active && info == null -> applyRecord(SelfUpdateFlow.onMissingInstallerSession(record()))
        }
        lastOutcome
    }

    private fun record(): SelfUpdateRecord = SelfUpdateRecord(
        active = prefs.getBoolean("active", false),
        pendingConfirmation = prefs.getBoolean("pendingConfirmation", false),
        sessionId = prefs.getInt("session", -1),
        desiredVersion = prefs.getLong("desiredVersion", 0),
        status = prefs.getString("status", "No self-update attempted")!!,
        outcomeStatus = prefs.getString("outcomeStatus", "queued") ?: "queued",
        outcomeError = prefs.getString("outcomeError", null),
        hasConfirmation = liveConfirmation != null || !prefs.getString("confirmation", null).isNullOrBlank(),
    )

    private fun applyRecord(next: SelfUpdateRecord) {
        if (SelfUpdateFlow.shouldAbandon(next) && next.sessionId >= 0) {
            runCatching { installer.abandonSession(next.sessionId) }
        }
        if (!next.hasConfirmation) {
            liveConfirmation = null
        }
        val editor = prefs.edit()
            .putBoolean("active", next.active)
            .putBoolean("pendingConfirmation", next.pendingConfirmation)
            .putInt("session", next.sessionId)
            .putLong("desiredVersion", next.desiredVersion)
            .putString("status", next.status)
            .putString("outcomeStatus", next.outcomeStatus)
        if (next.outcomeError != null) editor.putString("outcomeError", next.outcomeError) else editor.remove("outcomeError")
        if (!next.hasConfirmation) editor.remove("confirmation")
        check(editor.commit()) { "Cannot persist self-update status" }
    }

    private fun persistConfirmation(confirmation: Intent) {
        val parcel = Parcel.obtain()
        try {
            confirmation.writeToParcel(parcel, 0)
            val encoded = Base64.encodeToString(parcel.marshall(), Base64.NO_WRAP)
            check(prefs.edit().putString("confirmation", encoded).commit()) { "Cannot persist install confirmation" }
        } finally {
            parcel.recycle()
        }
    }

    private fun restoreConfirmation(): Intent? {
        val encoded = prefs.getString("confirmation", null) ?: return null
        val bytes = runCatching { Base64.decode(encoded, Base64.NO_WRAP) }.getOrNull() ?: return null
        val parcel = Parcel.obtain()
        return try {
            parcel.unmarshall(bytes, 0, bytes.size)
            parcel.setDataPosition(0)
            Intent.CREATOR.createFromParcel(parcel)
        } catch (_: RuntimeException) {
            null
        } finally {
            parcel.recycle()
        }
    }

    private fun persist(sessionId: Int, desiredVersion: Long) {
        check(
            prefs.edit()
                .putBoolean("active", true)
                .putBoolean("pendingConfirmation", false)
                .putInt("session", sessionId)
                .putLong("desiredVersion", desiredVersion)
                .remove("outcomeVersion")
                .remove("confirmation")
                .putString("status", "Installing version $desiredVersion")
                .putString("outcomeStatus", "installing")
                .remove("outcomeError")
                .commit(),
        ) { "Could not persist self-update attempt" }
    }

    private fun record(status: String, active: Boolean, outcome: String, installed: Long?) {
        val editor = prefs.edit()
            .putString("status", status)
            .putBoolean("active", active)
            .putBoolean("pendingConfirmation", if (active) pendingConfirmation else false)
            .putString("outcomeStatus", outcome)
        if (installed != null) editor.putLong("outcomeVersion", installed) else editor.remove("outcomeVersion")
        if (!active) editor.remove("outcomeError")
        check(editor.commit()) { "Cannot persist self-update status" }
    }

    private fun fail(outcome: String, reason: String) {
        check(
            prefs.edit()
                .putBoolean("active", false)
                .putBoolean("pendingConfirmation", false)
                .putString("status", "Update failed: $reason")
                .putString("outcomeStatus", outcome)
                .remove("outcomeVersion")
                .putString("outcomeError", reason.take(200))
                .commit(),
        ) { "Cannot persist self-update status" }
    }

    private fun inspectVerified(apk: File, snapshot: File, expected: SignedManifest): Pair<ArchiveIdentity, SelfUpdateEligibility> {
        ArchiveChecks.validateExpected(expected.byteSize, expected.sha256)
        apk.inputStream().use { input ->
            snapshot.outputStream().use { output -> ArchiveChecks.copyVerified(input, output, expected.byteSize, expected.sha256) }
        }
        val installed = identity(installedInfo())
        val archive = inspect(snapshot)
        val eligibility = SelfUpdatePolicy.decide(
            runningPackage = context.packageName,
            installed = installed,
            archive = archive,
            expected = expected,
            sdk = Build.VERSION.SDK_INT,
            deviceAbis = Build.SUPPORTED_ABIS.toSet(),
            candidateTargetSdk = archiveTargetSdk(snapshot),
            canUpdateWithoutUserAction = context.packageManager.checkPermission(
                "android.permission.UPDATE_PACKAGES_WITHOUT_USER_ACTION",
                context.packageName,
            ) == PackageManager.PERMISSION_GRANTED,
            unknownSourcesAllowed = context.packageManager.canRequestPackageInstalls(),
        )
        return archive to eligibility
    }

    @Suppress("DEPRECATION")
    private fun installedInfo(): PackageInfo =
        context.packageManager.getPackageInfo(context.packageName, PackageManager.GET_SIGNING_CERTIFICATES)

    @Suppress("DEPRECATION")
    private fun archiveTargetSdk(apk: File): Int {
        val archive = context.packageManager.getPackageArchiveInfo(apk.path, 0) ?: error("Invalid Android APK archive")
        return archive.applicationInfo?.targetSdkVersion ?: 0
    }

    private fun inspect(apk: File): ArchiveIdentity {
        val archive = context.packageManager.getPackageArchiveInfo(apk.path, PackageManager.GET_SIGNING_CERTIFICATES)
            ?: error("Invalid Android APK archive")
        val nativeAbis = ZipFile(apk).use { zip ->
            zip.entries().asSequence().map { it.name }.filter { it.startsWith("lib/") && it.endsWith(".so") }
                .map { it.split('/')[1] }.toSet()
        }
        return identity(archive, nativeAbis)
    }

    private fun identity(info: PackageInfo, abis: Set<String> = emptySet()) = ArchiveIdentity(
        info.packageName,
        info.longVersionCode,
        info.signingInfo?.apkContentsSigners?.map { MessageDigest.getInstance("SHA-256").digest(it.toByteArray()).hex() }?.toSet() ?: emptySet(),
        info.applicationInfo?.minSdkVersion ?: Int.MAX_VALUE,
        abis,
    )

    private fun sanitized(error: Exception): String {
        val message = if (error is ArchiveRejected) error.message else error.javaClass.simpleName
        return message?.take(200)?.replace(Regex("(/|[A-Za-z]:\\\\)[^\\s]+"), "") ?: "Update failed"
    }

    companion object {
        const val ACTION_RESULT = "com.aleksclark.primer.update.SELF_UPDATE_RESULT"
        private val lock = Any()
    }
}
