package com.aleksclark.primer.control

import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import android.provider.Settings
import android.app.Notification
import com.aleksclark.primer.control.device.ControlSelfUpdate
import com.aleksclark.primer.control.device.ControlSelfUpdatePhase
import com.aleksclark.primer.control.device.ControlSelfUpdateUi
import com.aleksclark.primer.updates.SelfUpdateSession
import com.aleksclark.primer.updates.SignedManifest
import com.aleksclark.primer.updates.SignedManifestCodec
import com.aleksclark.primertasks.client.Release
import java.io.File

class ControlSelfUpdateCoordinator(
    private val context: Context,
    private val trustRoot: String,
    private val session: SelfUpdateSession = SelfUpdateSession(
        context,
        ComponentName(context, ControlSelfUpdateReceiver::class.java),
    ),
) {
    fun ui(candidate: Release? = null): ControlSelfUpdateUi {
        val installed = runCatching {
            context.packageManager.getPackageInfo(context.packageName, 0).longVersionCode
        }.getOrDefault(0L)
        if (trustRoot.isBlank()) {
            return ControlSelfUpdateUi(
                phase = ControlSelfUpdatePhase.Failed,
                installedVersion = installed,
                status = "Release trust root is not configured.",
            )
        }
        session.reconcile()
        val outcome = session.lastOutcome
        val pending = session.pendingConfirmation
        val live = pending && runCatching {
            context.packageManager.packageInstaller.getSessionInfo(
                context.getSharedPreferences("self-update", Context.MODE_PRIVATE).getInt("session", -1),
            ) != null
        }.getOrDefault(false)
        val failed = outcome.status == "failed" || outcome.status == "blocked" && !pending
        return ControlSelfUpdateUi(
            phase = ControlSelfUpdate.phase(null, pending, live, failed && !pending),
            installedVersion = installed,
            candidateVersion = candidate?.versionCode ?: session.desiredVersion.takeIf { it > 0 },
            status = session.status,
            canInstall = candidate != null && candidate.packageName == ControlSelfUpdate.CONTROL_PACKAGE && !session.active,
            canOpenSettings = !context.packageManager.canRequestPackageInstalls(),
        )
    }

    fun install(download: File, release: Release): ControlSelfUpdateUi {
        val expected = verify(release)
        try {
            session.install(download, expected)
        } finally {
            download.delete()
        }
        return ui(release)
    }

    fun handleResult(intent: Intent): ControlSelfUpdateUi {
        session.handleResult(intent) { confirmation ->
            confirmation.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
            runCatching { context.startActivity(confirmation) }.isSuccess || notifyConfirmation(confirmation)
        }
        return ui()
    }

    fun settingsIntent(): Intent =
        Intent(Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES, Uri.parse("package:${context.packageName}"))
            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)

    fun verify(release: Release): SignedManifest {
        check(release.packageName == ControlSelfUpdate.CONTROL_PACKAGE) { "Self-update can only replace the running Control package." }
        check(trustRoot.isNotBlank()) { "Release trust root is not configured." }
        return SignedManifestCodec.verifyEnvelope(
            trustRoot = trustRoot,
            payloadBase64 = release.manifestPayloadBase64,
            signature = release.manifestSignature,
            signingKeyId = release.signingKeyId,
        )
    }

    private fun notifyConfirmation(confirmation: Intent): Boolean {
        val manager = context.getSystemService(NotificationManager::class.java) ?: return false
        if (Build.VERSION.SDK_INT >= 26) {
            manager.createNotificationChannel(
                NotificationChannel(CHANNEL, "Control updates", NotificationManager.IMPORTANCE_HIGH),
            )
        }
        val pending = PendingIntent.getActivity(
            context,
            1,
            confirmation,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )
        manager.notify(
            41,
            Notification.Builder(context, CHANNEL)
                .setSmallIcon(android.R.drawable.stat_sys_download_done)
                .setContentTitle("Confirm Control update")
                .setContentText("Android needs approval to replace Primer Control.")
                .setContentIntent(pending)
                .setAutoCancel(true)
                .build(),
        )
        return true
    }

    companion object {
        private const val CHANNEL = "control-self-update"
    }
}
