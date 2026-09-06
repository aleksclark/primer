package com.aleksclark.primer.tv.app.update

import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.content.pm.PackageInfo
import android.content.pm.PackageManager
import android.os.Build
import com.aleksclark.primer.tv.core.domain.AppRelease
import com.aleksclark.primer.tv.core.net.API_PATH_PREFIX
import com.aleksclark.primer.updates.ArchiveChecks
import com.aleksclark.primer.updates.ArchiveRejected
import com.aleksclark.primer.updates.SelfUpdateSession
import com.aleksclark.primer.updates.SignedManifest
import com.aleksclark.primer.updates.TvReleaseAdapter
import com.aleksclark.primer.updates.TvReleaseDecision
import java.io.File
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.OkHttpClient
import okhttp3.Request

sealed interface UpdateState {
    data object UpToDate : UpdateState
    data class Available(val release: AppRelease, val manifest: SignedManifest) : UpdateState
    data object Downloading : UpdateState
    data object Installing : UpdateState
    data class Failed(val message: String) : UpdateState
}

class AppUpdater(
    private val context: Context,
    private val httpClient: OkHttpClient,
    private val trustRoot: String,
    private val session: SelfUpdateSession = SelfUpdateSession(
        context,
        ComponentName(context, UpdateInstallReceiver::class.java),
        storeName = "tv-self-update",
    ),
) {
    fun installedVersionCode(): Int = installedPackageInfo()?.longVersionCodeCompat()?.toInt() ?: 0

    fun stateFor(release: AppRelease): UpdateState =
        TvUpdateGate.stateFor(release, trustRoot, installedVersionCode().toLong())

    fun handleResult(intent: Intent) {
        session.handleResult(intent) { confirmation ->
            confirmation.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
            context.startActivity(confirmation)
            true
        }
    }

    suspend fun download(baseUrl: String, release: AppRelease, token: String?): UpdateState =
        withContext(Dispatchers.IO) {
            val decision = TvReleaseAdapter.decide(release.toPublished(), trustRoot, installedVersionCode().toLong())
            val ready = when (decision) {
                is TvReleaseDecision.Ready -> decision
                is TvReleaseDecision.UpgradeRequired -> return@withContext UpdateState.Failed(decision.reason)
                is TvReleaseDecision.Rejected -> return@withContext UpdateState.Failed(decision.reason)
            }
            val directory = File(context.cacheDir, "updates").apply { check(mkdirs() || isDirectory) }
            val target = File.createTempFile("primer-tv-", ".apk", directory)
            val url = baseUrl.trimEnd('/') + "/" + API_PATH_PREFIX +
                ready.downloadPath.removePrefix("/api/v1/").trimStart('/')
            val request = Request.Builder().url(url).apply {
                if (!token.isNullOrBlank()) header("Authorization", "Bearer $token")
            }.build()
            try {
                httpClient.newCall(request).execute().use { response ->
                    if (!response.isSuccessful) {
                        target.delete()
                        return@withContext UpdateState.Failed("The server refused the download (HTTP ${response.code}).")
                    }
                    val body = response.body
                        ?: run {
                            target.delete()
                            return@withContext UpdateState.Failed("The server sent an empty download.")
                        }
                    if (body.contentLength() > 0L && body.contentLength() != ready.manifest.byteSize) {
                        target.delete()
                        return@withContext UpdateState.Failed("The server reported an unexpected update size.")
                    }
                    target.outputStream().use { output ->
                        ArchiveChecks.copyVerified(
                            input = body.byteStream(),
                            output = output,
                            size = ready.manifest.byteSize,
                            sha256 = ready.manifest.sha256,
                        )
                    }
                }
            } catch (error: Exception) {
                target.delete()
                val message = if (error is ArchiveRejected) {
                    error.message ?: "The download was corrupted and has been discarded."
                } else {
                    "The update could not be downloaded."
                }
                return@withContext UpdateState.Failed(message)
            }
            try {
                val attempt = session.install(target, ready.manifest)
                if (attempt.status == "failed" || attempt.status == "blocked") {
                    UpdateState.Failed(attempt.error ?: "The update could not be installed.")
                } else {
                    UpdateState.Installing
                }
            } finally {
                target.delete()
            }
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

    private fun PackageInfo.longVersionCodeCompat(): Long =
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) longVersionCode else {
            @Suppress("DEPRECATION")
            versionCode.toLong()
        }
}
