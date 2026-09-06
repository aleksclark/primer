package com.aleksclark.primer.control.device

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class ControlSelfUpdateTest {
    private val digest = "a".repeat(64)
    private val other = "b".repeat(64)

    @Test
    fun filenameOrMismatchedHashIsNotVerification() {
        val plan = ControlSelfUpdate.plan(
            runningPackage = ControlSelfUpdate.CONTROL_PACKAGE,
            installedPackage = ControlSelfUpdate.CONTROL_PACKAGE,
            installedVersion = 1,
            installedSigners = setOf("key-a"),
            archivePackage = ControlSelfUpdate.CONTROL_PACKAGE,
            archiveVersion = 2,
            archiveSigners = setOf("key-a"),
            expectedSha256 = digest,
            actualSha256 = other,
            sdk = 35,
            canRequestUnattended = true,
            unknownSourcesAllowed = true,
        )
        assertFalse(plan.verifiedBytes)
        assertFalse(plan.canAttempt)
        assertTrue(plan.reason.contains("Filename is not proof"))
        assertEquals(ControlSelfUpdatePhase.Held, ControlSelfUpdate.phase(plan, productionTrusted = false))
    }

    @Test
    fun unattendedOnlyWhenOrdinaryAndroidEligibilityHolds() {
        val eligible = verifiedPlan(sdk = 35, unattended = true, unknownSources = true)
        assertTrue(eligible.canAttempt)
        assertTrue(eligible.unattendedEligible)
        assertFalse(eligible.confirmationRequired)

        val oldSdk = verifiedPlan(sdk = 28, unattended = true, unknownSources = true)
        assertTrue(oldSdk.canAttempt)
        assertFalse(oldSdk.unattendedEligible)
        assertTrue(oldSdk.confirmationRequired)

        val noFlag = verifiedPlan(sdk = 35, unattended = false, unknownSources = true)
        assertFalse(noFlag.unattendedEligible)
        assertTrue(noFlag.confirmationRequired)
    }

    @Test
    fun confirmationAndSettingsFallbacksDoNotAssumeDeviceOwner() {
        val settings = verifiedPlan(sdk = 35, unattended = false, unknownSources = false)
        assertTrue(settings.settingsRequired)
        assertTrue(settings.confirmationRequired)
        assertTrue(settings.notificationFallback)
        assertEquals(ControlSelfUpdatePhase.NeedsSettings, ControlSelfUpdate.phase(settings, productionTrusted = true))
        assertEquals(ControlSelfUpdatePhase.EligibleConfirm, ControlSelfUpdate.phase(verifiedPlan(sdk = 35, unattended = false, unknownSources = true), true))
        assertEquals(ControlSelfUpdatePhase.EligibleUnattended, ControlSelfUpdate.phase(verifiedPlan(sdk = 35, unattended = true, unknownSources = true), true))
    }

    @Test
    fun foreignPackageOrSignerCannotAttempt() {
        val foreign = ControlSelfUpdate.plan(
            runningPackage = ControlSelfUpdate.CONTROL_PACKAGE,
            installedPackage = ControlSelfUpdate.CONTROL_PACKAGE,
            installedVersion = 1,
            installedSigners = setOf("key-a"),
            archivePackage = "com.aleksclark.primer.student",
            archiveVersion = 2,
            archiveSigners = setOf("key-a"),
            expectedSha256 = digest,
            actualSha256 = digest,
            sdk = 35,
            canRequestUnattended = true,
            unknownSourcesAllowed = true,
        )
        assertFalse(foreign.canAttempt)
        val signer = verifiedPlan().let {
            ControlSelfUpdate.plan(
                runningPackage = ControlSelfUpdate.CONTROL_PACKAGE,
                installedPackage = ControlSelfUpdate.CONTROL_PACKAGE,
                installedVersion = 1,
                installedSigners = setOf("key-a"),
                archivePackage = ControlSelfUpdate.CONTROL_PACKAGE,
                archiveVersion = 2,
                archiveSigners = setOf("key-b"),
                expectedSha256 = digest,
                actualSha256 = digest,
                sdk = 35,
                canRequestUnattended = true,
                unknownSourcesAllowed = true,
            )
        }
        assertFalse(signer.canAttempt)
    }

    @Test
    fun pendingConfirmationWithoutSessionMustNotWedge() {
        assertFalse(ControlSelfUpdate.pendingConfirmationStillLive(pendingConfirmation = true, sessionExists = false))
        assertTrue(ControlSelfUpdate.pendingConfirmationStillLive(pendingConfirmation = true, sessionExists = true))
        assertFalse(ControlSelfUpdate.pendingConfirmationStillLive(pendingConfirmation = false, sessionExists = true))
    }

    @Test
    fun productionPhaseStaysHeldUntilSharedAdapterIsTrusted() {
        assertEquals(ControlSelfUpdatePhase.Held, ControlSelfUpdate.phase(verifiedPlan(), productionTrusted = false))
    }

    private fun verifiedPlan(
        sdk: Int = 35,
        unattended: Boolean = true,
        unknownSources: Boolean = true,
    ) = ControlSelfUpdate.plan(
        runningPackage = ControlSelfUpdate.CONTROL_PACKAGE,
        installedPackage = ControlSelfUpdate.CONTROL_PACKAGE,
        installedVersion = 1,
        installedSigners = setOf("key-a"),
        archivePackage = ControlSelfUpdate.CONTROL_PACKAGE,
        archiveVersion = 2,
        archiveSigners = setOf("key-a"),
        expectedSha256 = digest,
        actualSha256 = digest,
        sdk = sdk,
        canRequestUnattended = unattended,
        unknownSourcesAllowed = unknownSources,
    )
}
