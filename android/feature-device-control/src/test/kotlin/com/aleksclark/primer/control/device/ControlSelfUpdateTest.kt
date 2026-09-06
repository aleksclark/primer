package com.aleksclark.primer.control.device

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class ControlSelfUpdateTest {
    @Test
    fun presentationMapsAdapterUnattendedWithoutDuplicatingValidators() {
        val plan = ControlSelfUpdate.present(
            AdapterEligibility(unattendedEligible = true, userActionRequired = false, reason = "Android may replace this package without a prompt"),
            unknownSourcesAllowed = true,
        )
        assertTrue(plan.unattendedEligible)
        assertFalse(plan.confirmationRequired)
        assertEquals(ControlSelfUpdatePhase.Held, ControlSelfUpdate.phase(plan, productionTrusted = false))
        assertEquals(ControlSelfUpdatePhase.EligibleUnattended, ControlSelfUpdate.phase(plan, productionTrusted = true))
    }

    @Test
    fun confirmationAndSettingsFallbacksComeFromAdapterUserAction() {
        val confirm = ControlSelfUpdate.present(
            AdapterEligibility(unattendedEligible = false, userActionRequired = true, reason = "Android requires a system install confirmation"),
            unknownSourcesAllowed = true,
        )
        assertTrue(confirm.confirmationRequired)
        assertTrue(confirm.notificationFallback)
        assertFalse(confirm.settingsRequired)
        assertEquals(ControlSelfUpdatePhase.EligibleConfirm, ControlSelfUpdate.phase(confirm, true))

        val settings = ControlSelfUpdate.present(
            AdapterEligibility(unattendedEligible = false, userActionRequired = true, reason = "Unknown-source installs are not permitted; system confirmation is required"),
            unknownSourcesAllowed = false,
        )
        assertTrue(settings.settingsRequired)
        assertEquals(ControlSelfUpdatePhase.NeedsSettings, ControlSelfUpdate.phase(settings, true))
    }

    @Test
    fun pendingConfirmationWithoutSessionMustNotWedge() {
        assertFalse(ControlSelfUpdate.pendingConfirmationStillLive(pendingConfirmation = true, sessionExists = false))
        assertTrue(ControlSelfUpdate.pendingConfirmationStillLive(pendingConfirmation = true, sessionExists = true))
    }

    @Test
    fun productionPhaseStaysHeldUntilSharedAdapterIsTrusted() {
        assertEquals(ControlSelfUpdatePhase.Held, ControlSelfUpdate.phase(null, productionTrusted = false))
    }
}
