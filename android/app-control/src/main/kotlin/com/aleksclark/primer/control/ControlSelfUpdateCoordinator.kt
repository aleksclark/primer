package com.aleksclark.primer.control

import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.net.Uri
import android.provider.Settings
import com.aleksclark.primer.control.device.ControlSelfUpdate
import com.aleksclark.primer.control.device.ControlSelfUpdatePhase
import com.aleksclark.primer.control.device.ControlSelfUpdateUi
import com.aleksclark.primer.updates.SelfUpdateCommands
import com.aleksclark.primer.updates.SelfUpdateEligibility
import com.aleksclark.primer.updates.SelfUpdateSession
import com.aleksclark.primer.updates.SignedManifest
import com.aleksclark.primer.updates.SignedManifestCodec
import com.aleksclark.primertasks.client.Release
import java.io.File

class ControlSelfUpdateCoordinator(
    private val context: Context,
    private val trustRoot: String,
    private val session: SelfUpdateCommands = SelfUpdateSession(
        context,
        ComponentName(context, ControlSelfUpdateReceiver::class.java),
    ),
    private val presenter: ControlUserActionPresenter = ControlUserActionPresenter {
        UserActionPresentation.Deferred("Install confirmation presenter is not configured")
    },
    private val unknownSourcesAllowed: () -> Boolean = { context.packageManager.canRequestPackageInstalls() },
    private val installedVersion: () -> Long = {
        runCatching { context.packageManager.getPackageInfo(context.packageName, 0).longVersionCode }.getOrDefault(0L)
    },
) {
    @Volatile private var lastPresentation: UserActionPresentation? = null

    fun ui(candidate: Release? = null): ControlSelfUpdateUi {
        val installed = installedVersion()
        if (trustRoot.isBlank()) {
            return ControlSelfUpdateUi(
                phase = ControlSelfUpdatePhase.Failed,
                installedVersion = installed,
                status = "Release trust root is not configured.",
            )
        }
        session.reconcile()
        val state = session.snapshot()
        val presentation = lastPresentation
        val deferred = presentation is UserActionPresentation.Deferred
        val failed = state.lastOutcome.status == "failed" ||
            (state.lastOutcome.status == "blocked" && !state.pendingConfirmation)
        val sources = unknownSourcesAllowed()
        val plan = if (!sources) {
            ControlSelfUpdate.present(
                SelfUpdateEligibility(
                    unattendedEligible = false,
                    userActionRequired = true,
                    reason = "Unknown-source installs are not permitted; system confirmation is required",
                    mustHandlePendingUserAction = true,
                ),
                unknownSourcesAllowed = false,
            )
        } else {
            null
        }
        val verified = candidate?.let { runCatching { verify(it) }.getOrNull() }
        val unsigned = candidate != null && verified == null && !state.pendingConfirmation && !state.active
        val eligible = verified != null && ControlSelfUpdate.isNewer(verified.versionCode, installed)
        return ControlSelfUpdateUi(
            phase = ControlSelfUpdate.phase(
                plan = plan,
                pendingConfirmation = state.pendingConfirmation,
                sessionExists = state.installerSessionLive,
                failed = (failed || unsigned) && !state.pendingConfirmation,
                deferred = deferred && state.pendingConfirmation && state.installerSessionLive,
            ),
            installedVersion = installed,
            candidateVersion = verified?.versionCode ?: state.desiredVersion.takeIf { it > 0 },
            status = presentationStatus(state.status, presentation),
            plan = plan,
            canInstall = eligible && !state.active,
            canOpenSettings = !unknownSourcesAllowed(),
            canContinueConfirmation = state.pendingConfirmation && state.installerSessionLive && state.hasConfirmationIntent,
            presentation = when (presentation) {
                is UserActionPresentation.Deferred -> presentation.reason
                UserActionPresentation.ShownOnActivity -> "System confirmation is on screen."
                UserActionPresentation.Notified -> "Confirm the update from the notification."
                null -> null
            },
        )
    }

    fun install(download: File, expected: SignedManifest): ControlSelfUpdateUi {
        try {
            session.install(download, expected)
        } finally {
            download.delete()
        }
        lastPresentation = null
        return ui()
    }

    fun handleResult(intent: Intent): ControlSelfUpdateUi {
        lastPresentation = null
        session.handleResult(intent) { confirmation ->
            val presented = presenter.present(confirmation)
            lastPresentation = presented
            presented.shown
        }
        return ui()
    }

    fun continueConfirmation(): ControlSelfUpdateUi {
        lastPresentation = null
        session.resumeUserAction { confirmation ->
            val presented = presenter.present(confirmation)
            lastPresentation = presented
            presented.shown
        }
        return ui()
    }

    fun settingsIntent(): Intent =
        Intent(Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES, Uri.parse("package:${context.packageName}"))
            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)

    fun verify(release: Release): SignedManifest {
        check(trustRoot.isNotBlank()) { "Release trust root is not configured." }
        val decoded = SignedManifestCodec.verifyEnvelope(
            trustRoot = trustRoot,
            payloadBase64 = release.manifestPayloadBase64,
            signature = release.manifestSignature,
            signingKeyId = release.signingKeyId,
        )
        ControlSelfUpdate.requireMatchesOuter(decoded, release)
        return decoded
    }

    private fun presentationStatus(status: String, presentation: UserActionPresentation?): String = when (presentation) {
        is UserActionPresentation.Deferred -> "$status ${presentation.reason}."
        UserActionPresentation.Notified -> "$status Confirm from the notification."
        UserActionPresentation.ShownOnActivity -> status
        null -> status
    }
}
