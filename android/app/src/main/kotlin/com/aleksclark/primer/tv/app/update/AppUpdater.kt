package com.aleksclark.primer.tv.app.update

import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import androidx.core.content.FileProvider
import com.aleksclark.primer.tv.core.domain.AppRelease
import com.aleksclark.primer.tv.core.net.API_PATH_PREFIX
import java.io.File
import java.security.MessageDigest
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.OkHttpClient
import okhttp3.Request

/** Where an update stands. */
sealed interface UpdateState {
    /** Nothing newer than what is installed. */
    data object UpToDate : UpdateState

    /** [release] is newer and can be fetched. */
    data class Available(val release: AppRelease) : UpdateState

    /** The APK is downloading. */
    data object Downloading : UpdateState

    /** Downloaded and verified; the installer has been handed the file. */
    data object Installing : UpdateState

    /** The update could not be applied. */
    data class Failed(val message: String) : UpdateState
}

/**
 * Fetches and installs the APK the server publishes.
 *
 * The box has no app store, so this is the only way it stays current. The
 * download is verified against the server's digest before the installer is
 * invoked: a truncated download would otherwise surface as an opaque "app not
 * installed" on a device with no one sitting at it.
 */
class AppUpdater(
    private val context: Context,
    private val httpClient: OkHttpClient,
) {
    /** The versionCode currently running. */
    fun installedVersionCode(): Int = try {
        val info = context.packageManager.getPackageInfo(context.packageName, 0)
        @Suppress("DEPRECATION")
        info.versionCode
    } catch (_: PackageManager.NameNotFoundException) {
        0
    }

    /** Whether [release] is worth offering to the student's device. */
    fun stateFor(release: AppRelease): UpdateState =
        if (release.isNewerThan(installedVersionCode())) UpdateState.Available(release) else UpdateState.UpToDate

    /**
     * Downloads, verifies, and hands the APK to the package installer.
     *
     * Installation itself needs a user confirmation on Android 9, so this
     * returns once the installer is showing rather than once the update has
     * been applied.
     */
    suspend fun download(baseUrl: String, release: AppRelease, token: String?): UpdateState =
        withContext(Dispatchers.IO) {
            val target = File(context.cacheDir, "updates").apply { mkdirs() }
                .resolve("primer-tv-${release.versionCode}.apk")

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
                    target.outputStream().use { out -> body.byteStream().copyTo(out) }
                }
            } catch (e: Exception) {
                target.delete()
                return@withContext UpdateState.Failed(e.message ?: "The download failed.")
            }

            if (release.sha256.isNotBlank() && sha256(target) != release.sha256) {
                target.delete()
                return@withContext UpdateState.Failed("The download was corrupted and has been discarded.")
            }

            install(target)
        }

    /** Hands the verified APK to the system installer. */
    private fun install(apk: File): UpdateState = try {
        val uri = FileProvider.getUriForFile(context, "${context.packageName}.updates", apk)
        val intent = Intent(Intent.ACTION_VIEW).apply {
            setDataAndType(uri, "application/vnd.android.package-archive")
            addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
            addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        }
        context.startActivity(intent)
        UpdateState.Installing
    } catch (e: Exception) {
        UpdateState.Failed(e.message ?: "No installer is available on this device.")
    }

    private fun sha256(file: File): String {
        val digest = MessageDigest.getInstance("SHA-256")
        file.inputStream().use { input ->
            val buffer = ByteArray(64 * 1024)
            while (true) {
                val read = input.read(buffer)
                if (read <= 0) break
                digest.update(buffer, 0, read)
            }
        }
        return digest.digest().joinToString("") { "%02x".format(it) }
    }
}
