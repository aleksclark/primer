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
import com.aleksclark.primer.updates.SelfUpdateFlow
import com.aleksclark.primer.updates.SelfUpdateSession
import com.aleksclark.primer.updates.SelfUpdateSessionState
import com.aleksclark.primer.updates.SignedManifest
import com.aleksclark.primer.updates.SignedManifestCodec
import com.aleksclark.primertasks.client.Release
import java.io.File
import java.util.concurrent.atomic.AtomicLong
import java.util.concurrent.atomic.AtomicReference

data class PreparedControlUpdate(
    val expected: SignedManifest,
    val eligibility: SelfUpdateEligibility,
    val apk: File,
    val generation: Long,
)

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
    private val decode: (Release) -> SignedManifest = { release ->
        SignedManifestCodec.verifyEnvelope(
            trustRoot = trustRoot,
            payloadBase64 = release.manifestPayloadBase64,
            signature = release.manifestSignature,
            signingKeyId = release.signingKeyId,
        )
    },
) {
    private val lock = Any()
    private val lastPresentation = AtomicReference<UserActionPresentation?>(null)
    private val prepared = AtomicReference<PreparedControlUpdate?>(null)
    private val generation = AtomicLong(0)

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
        val presentation = lastPresentation.get()
        val deferred = presentation is UserActionPresentation.Deferred
        val failed = state.lastOutcome.status == "failed" ||
            (state.lastOutcome.status == "blocked" && !state.pendingConfirmation)
        val verified = candidate?.let { runCatching { verify(it) }.getOrNull() }
        val unsigned = candidate != null && verified == null && !state.pendingConfirmation && !state.active
        val current = publishedPrepared()?.takeIf { prepared ->
            prepared.apk.isFile && when {
                candidate == null -> true
                verified == null -> false
                else -> prepared.expected.sha256.equals(verified.sha256, true)
            }
        }
        val sources = unknownSourcesAllowed()
        val plan = when {
            !sources -> ControlSelfUpdate.present(
                SelfUpdateEligibility(
                    unattendedEligible = false,
                    userActionRequired = true,
                    reason = "Unknown-source installs are not permitted; system confirmation is required",
                    mustHandlePendingUserAction = true,
                ),
                unknownSourcesAllowed = false,
            )
            current != null -> ControlSelfUpdate.present(current.eligibility, unknownSourcesAllowed = true)
            else -> null
        }
        val version = verified?.versionCode ?: current?.expected?.versionCode
        val eligible = current != null && version != null && ControlSelfUpdate.isNewer(version, installed)
        return ControlSelfUpdateUi(
            phase = ControlSelfUpdate.phase(
                plan = plan,
                pendingConfirmation = state.pendingConfirmation,
                sessionExists = state.installerSessionLive,
                failed = (failed || unsigned) && !state.pendingConfirmation,
                deferred = deferred && state.pendingConfirmation && state.installerSessionLive,
            ),
            installedVersion = installed,
            candidateVersion = version ?: state.desiredVersion.takeIf { it > 0 },
            status = presentationStatus(state.status, presentation, current),
            plan = plan,
            canInstall = eligible && !state.active && sources && !state.pendingConfirmation && !blocksInstall(state, deferred),
            canOpenSettings = !sources,
            canContinueConfirmation = state.pendingConfirmation && state.installerSessionLive && state.hasConfirmationIntent,
            canCancel = SelfUpdateFlow.canCancel(state),
            canRetry = SelfUpdateFlow.canRetry(state) && trustRoot.isNotBlank(),
            presentation = when (presentation) {
                is UserActionPresentation.Deferred -> presentation.reason
                UserActionPresentation.ShownOnActivity -> "System confirmation is on screen."
                UserActionPresentation.Notified -> "Confirm the update from the notification."
                null -> null
            },
        )
    }

    fun prepare(download: File, expected: SignedManifest): PreparedControlUpdate {
        val token = generation.incrementAndGet()
        var kept: File? = null
        return try {
            val eligibility = session.evaluate(download, expected)
            val directory = download.parentFile ?: context.cacheDir
            kept = File.createTempFile("prepared-${expected.versionCode}-", ".apk", directory)
            check(download.renameTo(kept) || (download.copyTo(kept, overwrite = true).also { download.delete() }.exists())) {
                "Could not keep the verified Control APK"
            }
            val next = PreparedControlUpdate(expected, eligibility, kept, token)
            publishPrepared(next, token)
        } catch (error: Exception) {
            kept?.delete()
            download.delete()
            throw error
        } finally {
            if (download.exists()) download.delete()
        }
    }

    fun installPrepared(): ControlSelfUpdateUi {
        session.reconcile()
        val state = session.snapshot()
        val deferred = lastPresentation.get() is UserActionPresentation.Deferred
        if (blocksInstall(state, deferred)) {
            error("A failed or pending install confirmation blocks PackageInstaller dispatch.")
        }
        val current = publishedPrepared() ?: error("No verified Control update is prepared.")
        try {
            session.install(current.apk, current.expected)
        } finally {
            clearPrepared(current.generation)
        }
        lastPresentation.set(null)
        return ui()
    }

    fun handleResult(intent: Intent): ControlSelfUpdateUi {
        lastPresentation.set(null)
        session.handleResult(intent) { confirmation ->
            val presented = presenter.present(confirmation)
            lastPresentation.set(presented)
            presented.shown
        }
        return ui()
    }

    fun continueConfirmation(): ControlSelfUpdateUi {
        lastPresentation.set(null)
        session.resumeUserAction { confirmation ->
            val presented = presenter.present(confirmation)
            lastPresentation.set(presented)
            presented.shown
        }
        return ui()
    }

    fun cancelInstall(): ControlSelfUpdateUi {
        session.cancel()
        lastPresentation.set(null)
        clearPrepared()
        return ui()
    }

    fun retryFailed(): ControlSelfUpdateUi {
        session.reconcile()
        val state = session.snapshot()
        if (SelfUpdateFlow.canCancel(state)) {
            error("Cancel the live install confirmation before retrying.")
        }
        if (!SelfUpdateFlow.canRetry(state)) {
            error("There is no failed Control update to retry.")
        }
        session.clearFailed()
        lastPresentation.set(null)
        clearPrepared()
        return ui()
    }

    fun invalidatePrepared() {
        generation.incrementAndGet()
        clearPrepared()
        lastPresentation.set(null)
    }

    fun settingsIntent(): Intent =
        Intent(Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES, Uri.parse("package:${context.packageName}"))
            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)

    fun verify(release: Release): SignedManifest {
        check(trustRoot.isNotBlank()) { "Release trust root is not configured." }
        val decoded = decode(release)
        ControlSelfUpdate.requireMatchesOuter(decoded, release)
        return decoded
    }

    private fun blocksInstall(state: SelfUpdateSessionState, deferred: Boolean): Boolean {
        if (state.pendingConfirmation) return true
        if (deferred) return true
        if (!unknownSourcesAllowed()) return true
        val status = state.lastOutcome.status
        return status == "failed" || (status == "blocked" && !state.pendingConfirmation)
    }

    private fun publishedPrepared(): PreparedControlUpdate? {
        val current = prepared.get() ?: return null
        if (!current.apk.isFile) {
            prepared.compareAndSet(current, null)
            return null
        }
        return current
    }

    private fun publishPrepared(next: PreparedControlUpdate, token: Long): PreparedControlUpdate {
        synchronized(lock) {
            if (token != generation.get()) {
                next.apk.delete()
                error("Prepared Control update was cancelled.")
            }
            val previous = prepared.getAndSet(next)
            if (previous != null && previous.apk != next.apk) previous.apk.delete()
            return next
        }
    }

    private fun clearPrepared(expectedGeneration: Long? = null) {
        synchronized(lock) {
            val current = prepared.get() ?: return
            if (expectedGeneration != null && current.generation != expectedGeneration) return
            if (prepared.compareAndSet(current, null)) current.apk.delete()
        }
    }

    private fun presentationStatus(
        status: String,
        presentation: UserActionPresentation?,
        current: PreparedControlUpdate?,
    ): String {
        val base = when (presentation) {
            is UserActionPresentation.Deferred -> "$status ${presentation.reason}."
            UserActionPresentation.Notified -> "$status Confirm from the notification."
            UserActionPresentation.ShownOnActivity -> status
            null -> status
        }
        return if (current != null && presentation == null) current.eligibility.reason else base
    }
}
