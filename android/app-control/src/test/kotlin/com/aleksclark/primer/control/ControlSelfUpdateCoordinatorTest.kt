package com.aleksclark.primer.control

import android.content.Intent
import com.aleksclark.primer.control.device.ControlSelfUpdatePhase
import com.aleksclark.primer.updates.InstallAttempt
import com.aleksclark.primer.updates.SelfUpdateCommands
import com.aleksclark.primer.updates.SelfUpdateSession
import com.aleksclark.primer.updates.SelfUpdateSessionState
import com.aleksclark.primer.updates.SignedManifest
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
    )

    private class FakeSession(
        var pending: Boolean = false,
        var live: Boolean = false,
        var hasConfirmation: Boolean = false,
        var active: Boolean = false,
        var outcome: String = "queued",
        var status: String = "No self-update attempted",
        var failOnResume: Boolean = false,
    ) : SelfUpdateCommands {
        var lastShown: Boolean? = null

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
        override fun install(apk: File, expected: SignedManifest) = snapshot().lastOutcome
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
