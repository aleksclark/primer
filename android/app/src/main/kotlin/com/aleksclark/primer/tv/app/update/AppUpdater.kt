package com.aleksclark.primer.tv.app.update

import android.app.PendingIntent
import android.app.admin.DevicePolicyManager
import android.content.Context
import android.content.Intent
import android.content.pm.PackageInfo
import android.content.pm.PackageInstaller
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import androidx.core.content.FileProvider
import com.aleksclark.primer.tv.core.domain.AppRelease
import com.aleksclark.primer.tv.core.net.API_PATH_PREFIX
import java.io.File
import java.security.MessageDigest
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.OkHttpClient
import okhttp3.Request

sealed interface UpdateState {
    data object UpToDate : UpdateState
    data class Available(val release: AppRelease) : UpdateState
    data object Downloading : UpdateState
    data object Installing : UpdateState
    data class Failed(val message: String) : UpdateState
}

internal object ApkChecks {
    private val sha256Pattern = Regex("^[0-9a-fA-F]{64}$")

    fun validExpectedDigest(expected: String): Boolean =
        expected.isBlank() || sha256Pattern.matches(expected)

    fun digestMatches(expected: String, actual: String): Boolean =
        expected.isBlank() || expected.equals(actual, ignoreCase = true)

    fun sizeMatches(expected: Long, actual: Long): Boolean =
        expected <= 0L || expected == actual
}

class AppUpdater(
    private val context: Context,
    private val httpClient: OkHttpClient,
) {
    fun installedVersionCode(): Int = installedPackageInfo()?.longVersionCodeCompat()?.toInt() ?: 0

    fun stateFor(release: AppRelease): UpdateState =
        if (release.isNewerThan(installedVersionCode())) UpdateState.Available(release) else UpdateState.UpToDate

    suspend fun download(baseUrl: String, release: AppRelease, token: String?): UpdateState =
        withContext(Dispatchers.IO) {
            if (!ApkChecks.validExpectedDigest(release.sha256)) {
                return@withContext UpdateState.Failed("The server published an invalid update checksum.")
            }

            val directory = File(context.cacheDir, "updates").apply { mkdirs() }
            directory.listFiles()?.filter { it.name.endsWith(".partial") }?.forEach(File::delete)
            val target = directory.resolve("primer-tv-${release.versionCode}.apk")
            val partial = directory.resolve("${target.name}.partial")
            val url = baseUrl.trimEnd('/') + "/" + API_PATH_PREFIX +
                release.downloadPath.removePrefix("/api/v1/").trimStart('/')
            val request = Request.Builder().url(url).apply {
                if (!token.isNullOrBlank()) header("Authorization", "Bearer $token")
            }.build()

            try {
                httpClient.newCall(request).execute().use { response ->
                    if (!response.isSuccessful) {
                        return@withContext UpdateState.Failed("The server refused the download (HTTP ${response.code}).")
                    }
                    val body = response.body
                        ?: return@withContext UpdateState.Failed("The server sent an empty download.")
                    if (release.sizeBytes > 0L && body.contentLength() > 0L &&
                        body.contentLength() != release.sizeBytes
                    ) {
                        return@withContext UpdateState.Failed("The server reported an unexpected update size.")
                    }
                    partial.outputStream().buffered().use { output -> body.byteStream().copyTo(output) }
                }
            } catch (_: Exception) {
                partial.delete()
                return@withContext UpdateState.Failed("The update could not be downloaded.")
            }

            if (!ApkChecks.sizeMatches(release.sizeBytes, partial.length())) {
                partial.delete()
                return@withContext UpdateState.Failed("The update download was incomplete.")
            }
            if (!ApkChecks.digestMatches(release.sha256, sha256(partial))) {
                partial.delete()
                return@withContext UpdateState.Failed("The download was corrupted and has been discarded.")
            }
            target.delete()
            if (!partial.renameTo(target)) {
                partial.delete()
                return@withContext UpdateState.Failed("The verified update could not be prepared.")
            }

            validateArchive(target, release)?.let {
                target.delete()
                return@withContext UpdateState.Failed(it)
            }
            install(target)
        }

    private fun install(apk: File): UpdateState {
        val policyManager = context.getSystemService(DevicePolicyManager::class.java)
        return if (policyManager?.isDeviceOwnerApp(context.packageName) == true) {
            installSilently(apk)
        } else {
            InteractiveApkInstaller.launch(context, apk)
        }
    }

    private fun installSilently(apk: File): UpdateState = try {
        val installer = context.packageManager.packageInstaller
        val params = PackageInstaller.SessionParams(PackageInstaller.SessionParams.MODE_FULL_INSTALL).apply {
            setAppPackageName(context.packageName)
            setSize(apk.length())
        }
        val sessionId = installer.createSession(params)
        installer.openSession(sessionId).use { session ->
            apk.inputStream().use { input ->
                session.openWrite("primer-tv.apk", 0, apk.length()).use { output ->
                    input.copyTo(output)
                    session.fsync(output)
                }
            }
            val result = Intent(context, UpdateInstallReceiver::class.java).apply {
                action = UpdateInstallReceiver.ACTION_INSTALL_RESULT
                putExtra(UpdateInstallReceiver.EXTRA_APK_PATH, apk.absolutePath)
            }
            var flags = PendingIntent.FLAG_UPDATE_CURRENT
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) flags = flags or PendingIntent.FLAG_MUTABLE
            val callback = PendingIntent.getBroadcast(context, sessionId, result, flags)
            session.commit(callback.intentSender)
        }
        UpdateState.Installing
    } catch (_: Exception) {
        InteractiveApkInstaller.launch(context, apk)
    }

    private fun validateArchive(apk: File, release: AppRelease): String? {
        val archive = packageInfoForArchive(apk)
            ?: return "The downloaded file is not a valid Android package."
        if (archive.packageName != context.packageName) {
            return "The update belongs to a different application."
        }
        if (archive.longVersionCodeCompat() != release.versionCode.toLong() ||
            archive.longVersionCodeCompat() <= installedVersionCode().toLong()
        ) {
            return "The update version does not match the published release."
        }
        val installed = installedPackageInfo()
            ?: return "The installed application identity could not be verified."
        if (signers(installed).intersect(signers(archive)).isEmpty()) {
            return "The update is not signed by the installed application publisher."
        }
        return null
    }

    @Suppress("DEPRECATION")
    private fun installedPackageInfo(): PackageInfo? = try {
        context.packageManager.getPackageInfo(
            context.packageName,
            PackageManager.GET_SIGNING_CERTIFICATES,
        )
    } catch (_: PackageManager.NameNotFoundException) {
        null
    }

    @Suppress("DEPRECATION")
    private fun packageInfoForArchive(apk: File): PackageInfo? =
        context.packageManager.getPackageArchiveInfo(
            apk.absolutePath,
            PackageManager.GET_SIGNING_CERTIFICATES,
        )?.also { it.applicationInfo?.sourceDir = apk.absolutePath }

    @Suppress("DEPRECATION")
    private fun signers(info: PackageInfo): Set<String> {
        val signingInfo = info.signingInfo ?: return emptySet()
        val certificates = if (signingInfo.hasMultipleSigners()) {
            signingInfo.apkContentsSigners
        } else {
            signingInfo.signingCertificateHistory
        }
        return certificates.map { sha256(it.toByteArray()) }.toSet()
    }

    private fun PackageInfo.longVersionCodeCompat(): Long =
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) longVersionCode else {
            @Suppress("DEPRECATION")
            versionCode.toLong()
        }

    private fun sha256(file: File): String {
        val digest = MessageDigest.getInstance("SHA-256")
        file.inputStream().buffered().use { input ->
            val buffer = ByteArray(DEFAULT_BUFFER_SIZE)
            while (true) {
                val count = input.read(buffer)
                if (count < 0) break
                digest.update(buffer, 0, count)
            }
        }
        return digest.digest().toHex()
    }

    private fun sha256(bytes: ByteArray): String =
        MessageDigest.getInstance("SHA-256").digest(bytes).toHex()

    private fun ByteArray.toHex(): String = joinToString("") { "%02x".format(it) }
}

internal object InteractiveApkInstaller {
    fun launch(context: Context, apk: File): UpdateState = try {
        val uri: Uri = FileProvider.getUriForFile(context, "${context.packageName}.updates", apk)
        context.startActivity(Intent(Intent.ACTION_VIEW).apply {
            setDataAndType(uri, "application/vnd.android.package-archive")
            addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_ACTIVITY_NEW_TASK)
        })
        UpdateState.Installing
    } catch (_: Exception) {
        UpdateState.Failed("No package installer is available on this device.")
    }
}
