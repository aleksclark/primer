package com.aleksclark.primer.control

import android.content.Intent
import com.aleksclark.primer.control.device.ControlSelfUpdatePhase
import com.aleksclark.primer.control.device.ControlSelfUpdate
import com.aleksclark.primer.updates.InstallAttempt
import com.aleksclark.primer.updates.SelfUpdateCommands
import com.aleksclark.primer.updates.SelfUpdateEligibility
import com.aleksclark.primer.updates.SelfUpdateSession
import com.aleksclark.primer.updates.SelfUpdateSessionState
import com.aleksclark.primer.updates.SignedManifest
import com.aleksclark.primertasks.client.Release
import com.aleksclark.primertasks.client.ReleaseManifest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.junit.runners.JUnit4
import java.io.File

@RunWith(JUnit4::class)
class ControlSelfUpdateCoordinatorTest {
    @Test
    fun missingTrustRootFailsClosed() {
        val ui = coordinator(trustRoot = "").ui()
        assertEquals(ControlSelfUpdatePhase.Failed, ui.phase)
        assertTrue(ui.status.contains("trust root"))
        assertFalse(ui.canInstall)
        assertFalse(ui.canContinueConfirmation)
    }

    @Test
    fun silentStartActivityDoesNotClaimPresentation() {
        val session = FakeSession(pending = true, live = true, hasConfirmation = true)
        val presenter = ControlUserActionPresenter {
            UserActionDispatch.outcome(
                UserActionAttempt(
                    hasResumedActivity = false,
                    activityStarted = null,
                    notificationPermissionGranted = true,
                    notificationsEnabled = true,
                    channelEnabled = true,
                    notificationPosted = false,
                ),
            )
        }
        val ui = coordinator(session = session, presenter = presenter)
            .handleResult(Intent(SelfUpdateSession.ACTION_RESULT))
        assertEquals(ControlSelfUpdatePhase.Deferred, ui.phase)
        assertEquals("Install confirmation was not shown", ui.presentation)
        assertTrue(ui.canContinueConfirmation)
        assertFalse(session.lastShown == true)
        assertEquals(false, session.lastShown)
    }

    @Test
    fun notificationDeniedStaysObservableAndResumable() {
        val session = FakeSession(pending = true, live = true, hasConfirmation = true)
        val presenter = ControlUserActionPresenter {
            UserActionDispatch.outcome(
                UserActionAttempt(
                    hasResumedActivity = false,
                    notificationPermissionGranted = false,
                    notificationsEnabled = true,
                    channelEnabled = true,
                ),
            )
        }
        val coordinator = coordinator(session = session, presenter = presenter)
        val deferred = coordinator.handleResult(Intent(SelfUpdateSession.ACTION_RESULT))
        assertEquals(ControlSelfUpdatePhase.Deferred, deferred.phase)
        assertEquals("Notification permission is denied", deferred.presentation)
        assertTrue(deferred.canContinueConfirmation)
        session.lastShown = null
        val resumed = coordinator.continueConfirmation()
        assertEquals(ControlSelfUpdatePhase.Deferred, resumed.phase)
        assertEquals("Notification permission is denied", resumed.presentation)
        assertEquals(false, session.lastShown)
    }

    @Test
    fun resumedActivityPresentationIsWaitingNotDeferred() {
        val session = FakeSession(pending = true, live = true, hasConfirmation = true)
        val presenter = ControlUserActionPresenter { UserActionPresentation.ShownOnActivity }
        val ui = coordinator(session = session, presenter = presenter)
            .handleResult(Intent(SelfUpdateSession.ACTION_RESULT))
        assertEquals(ControlSelfUpdatePhase.WaitingConfirmation, ui.phase)
        assertEquals("System confirmation is on screen.", ui.presentation)
        assertTrue(session.lastShown == true)
    }

    @Test
    fun sourcesAllowedWithoutPreparedApkStaysIdle() {
        val ui = coordinator().ui(release())
        assertEquals(ControlSelfUpdatePhase.Idle, ui.phase)
        assertFalse(ui.canInstall)
        assertEquals(null, ui.plan)
    }

    @Test
    fun prepareUsesSharedPolicyAndDoesNotInstall() {
        val session = FakeSession()
        session.eligibility = SelfUpdateEligibility(
            unattendedEligible = true,
            userActionRequired = false,
            reason = "Android may replace this package without a prompt",
        )
        val apk = File.createTempFile("control-", ".apk").apply { writeBytes(ByteArray(12) { 1 }) }
        val coordinator = coordinator(session = session)
        coordinator.prepare(apk, manifest())
        val ui = coordinator.ui(release())
        assertEquals(ControlSelfUpdatePhase.EligibleUnattended, ui.phase)
        assertTrue(ui.canInstall)
        assertTrue(ui.plan!!.unattendedEligible)
        assertEquals(1, session.evaluations)
        assertEquals(0, session.installs)
        assertFalse(apk.exists())
    }

    @Test
    fun preparedConfirmEligibilityDoesNotClaimUnattended() {
        val session = FakeSession()
        session.eligibility = SelfUpdateEligibility(
            unattendedEligible = false,
            userActionRequired = true,
            reason = "Android requires a system install confirmation",
        )
        val apk = File.createTempFile("control-", ".apk").apply { writeBytes(ByteArray(12) { 1 }) }
        val coordinator = coordinator(session = session)
        coordinator.prepare(apk, manifest())
        val ui = coordinator.ui(release())
        assertEquals(ControlSelfUpdatePhase.EligibleConfirm, ui.phase)
        assertTrue(ui.canInstall)
        assertTrue(ui.plan!!.confirmationRequired)
        assertEquals(0, session.installs)
    }

    @Test
    fun installPreparedIsSeparateFromEvaluate() {
        val session = FakeSession()
        session.eligibility = SelfUpdateEligibility(true, false, "unattended")
        val apk = File.createTempFile("control-", ".apk").apply { writeBytes(ByteArray(12) { 1 }) }
        val coordinator = coordinator(session = session)
        coordinator.prepare(apk, manifest())
        coordinator.installPrepared()
        assertEquals(1, session.evaluations)
        assertEquals(1, session.installs)
    }

    @Test
    fun preparingSameVersionTwiceKeepsTheSecondApk() {
        val dir = kotlin.io.path.createTempDirectory("control-repeat-prepare-").toFile()
        try {
            val coordinator = coordinator()
            val first = File(dir, "first.apk").apply { writeBytes(ByteArray(12) { 1 }) }
            coordinator.prepare(first, manifest())
            val second = File(dir, "second.apk").apply { writeBytes(ByteArray(12) { 1 }) }
            val prepared = coordinator.prepare(second, manifest())
            assertTrue("Replacing a prepared update must not delete its replacement", prepared.apk.isFile)
            assertTrue(coordinator.ui(release()).canInstall)
        } finally {
            dir.deleteRecursively()
        }
    }

    @Test
    fun failedSessionBlocksDirectInstallPreparedDispatch() {
        val dir = kotlin.io.path.createTempDirectory("control-failed-prepare-").toFile()
        try {
            val session = FakeSession()
            val coordinator = coordinator(session = session)
            val apk = File(dir, "candidate.apk").apply { writeBytes(ByteArray(12) { 1 }) }
            coordinator.prepare(apk, manifest())
            session.outcome = "failed"
            assertEquals(ControlSelfUpdatePhase.Failed, coordinator.ui(release()).phase)
            runCatching { coordinator.installPrepared() }
            assertEquals("A failed state must be guarded at the install boundary", 0, session.installs)
        } finally {
            dir.deleteRecursively()
        }
    }

    @Test
    fun missingInstallerSessionFailsClosedOnContinue() {
        val session = FakeSession(pending = true, live = false, hasConfirmation = true)
        session.failOnResume = true
        val ui = coordinator(session = session).continueConfirmation()
        assertEquals(ControlSelfUpdatePhase.Failed, ui.phase)
        assertFalse(ui.canContinueConfirmation)
        assertFalse(ui.canInstall)
    }

    private fun coordinator(
        trustRoot: String = "dGVzdA",
        session: FakeSession = FakeSession(),
        presenter: ControlUserActionPresenter = ControlUserActionPresenter {
            UserActionPresentation.Deferred("Install confirmation was not shown")
        },
    ) = ControlSelfUpdateCoordinator(
        context = object : android.content.ContextWrapper(null) {
            override fun getPackageName() = "com.aleksclark.primer.control"
        },
        trustRoot = trustRoot,
        session = session,
        presenter = presenter,
        unknownSourcesAllowed = { true },
        installedVersion = { 1L },
        decode = { manifest() },
    )

    private fun manifest() = SignedManifest(
        packageName = ControlSelfUpdate.CONTROL_PACKAGE,
        channel = ControlSelfUpdate.CONTROL_CHANNEL,
        versionCode = 2,
        versionName = "0.2.0",
        minSdk = 28,
        supportedAbis = listOf("arm64-v8a"),
        signerSha256 = "a".repeat(64),
        sha256 = "b".repeat(64),
        byteSize = 12,
    )

    private fun release() = Release(
        byteSize = 12,
        channel = ControlSelfUpdate.CONTROL_CHANNEL,
        id = "rel-2",
        manifest = ReleaseManifest(
            byteSize = 12,
            channel = ControlSelfUpdate.CONTROL_CHANNEL,
            minSdk = 28,
            packageName = ControlSelfUpdate.CONTROL_PACKAGE,
            sha256 = "b".repeat(64),
            signerSha256 = "a".repeat(64),
            supportedAbis = listOf("arm64-v8a"),
            versionCode = 2,
            versionName = "0.2.0",
        ),
        manifestPayloadBase64 = "payload",
        manifestSignature = "sig",
        minSdk = 28,
        packageName = ControlSelfUpdate.CONTROL_PACKAGE,
        publishedAt = "2026-01-01T00:00:00Z",
        sha256 = "b".repeat(64),
        signerSha256 = "a".repeat(64),
        signingKeyId = "ed25519-v1",
        status = "published",
        supportedAbis = listOf("arm64-v8a"),
        versionCode = 2,
        versionName = "0.2.0",
    )

    private class FakeSession(
        var pending: Boolean = false,
        var live: Boolean = false,
        var hasConfirmation: Boolean = false,
        var active: Boolean = false,
        var outcome: String = "queued",
        var status: String = "No self-update attempted",
        var failOnResume: Boolean = false,
        var eligibility: SelfUpdateEligibility = SelfUpdateEligibility(false, true, "confirm"),
    ) : SelfUpdateCommands {
        var lastShown: Boolean? = null
        var evaluations = 0
        var installs = 0

        override fun snapshot() = SelfUpdateSessionState(
            status = status,
            active = active || pending,
            pendingConfirmation = pending,
            installerSessionLive = live,
            desiredVersion = if (pending) 2 else 0,
            lastOutcome = InstallAttempt(outcome, null, if (outcome == "failed") status else null),
            hasConfirmationIntent = hasConfirmation,
        )

        override fun reconcile() = snapshot().lastOutcome
        override fun evaluate(apk: File, expected: SignedManifest): SelfUpdateEligibility {
            evaluations += 1
            return eligibility
        }
        override fun install(apk: File, expected: SignedManifest): InstallAttempt {
            installs += 1
            return snapshot().lastOutcome
        }
        override fun handleResult(intent: Intent, onUserAction: ((Intent) -> Boolean)?): InstallAttempt {
            lastShown = onUserAction?.invoke(Intent())
            pending = true
            live = true
            hasConfirmation = true
            active = true
            outcome = "blocked"
            status = if (lastShown == true) "Waiting for system install confirmation" else "Install confirmation is waiting"
            return snapshot().lastOutcome
        }

        override fun resumeUserAction(onUserAction: (Intent) -> Boolean): InstallAttempt {
            if (failOnResume || !live) {
                pending = false
                active = false
                hasConfirmation = false
                outcome = "failed"
                status = "Update failed: Installation interrupted or rejected"
                return snapshot().lastOutcome
            }
            lastShown = onUserAction(Intent())
            status = if (lastShown == true) "Waiting for system install confirmation" else "Install confirmation is waiting"
            outcome = "blocked"
            return snapshot().lastOutcome
        }

        override fun cancel(): InstallAttempt {
            pending = false
            active = false
            live = false
            hasConfirmation = false
            outcome = "failed"
            status = "Update failed: Installation cancelled"
            return snapshot().lastOutcome
        }
    }
}
