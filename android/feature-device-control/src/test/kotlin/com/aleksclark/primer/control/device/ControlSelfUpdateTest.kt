package com.aleksclark.primer.control.device

import com.aleksclark.primer.updates.SelfUpdateEligibility
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class ControlSelfUpdateTest {
    @Test
    fun userActionRequiredMeansNotUnattended() {
        val plan = ControlSelfUpdate.present(
            SelfUpdateEligibility(
                unattendedEligible = false,
                userActionRequired = true,
                reason = "Android requires a system install confirmation",
                mustHandlePendingUserAction = true,
            ),
            unknownSourcesAllowed = true,
        )
        assertTrue(plan.confirmationRequired)
        assertTrue(plan.mustHandlePendingUserAction)
        assertFalse(plan.unattendedEligible)
        assertEquals(
            ControlSelfUpdatePhase.EligibleConfirm,
            ControlSelfUpdate.phase(plan, pendingConfirmation = false, sessionExists = false, failed = false),
        )
    }

    @Test
    fun unattendedStillRequiresPendingUserActionHandler() {
        val plan = ControlSelfUpdate.present(
            SelfUpdateEligibility(
                unattendedEligible = true,
                userActionRequired = false,
                reason = "Android may replace this package without a prompt",
                mustHandlePendingUserAction = true,
            ),
            unknownSourcesAllowed = true,
        )
        assertTrue(plan.unattendedEligible)
        assertTrue(plan.mustHandlePendingUserAction)
        assertTrue(plan.notificationFallback)
        assertEquals(
            ControlSelfUpdatePhase.EligibleUnattended,
            ControlSelfUpdate.phase(plan, pendingConfirmation = false, sessionExists = true, failed = false),
        )
    }

    @Test
    fun settingsFallbackWhenUnknownSourcesDisallowed() {
        val plan = ControlSelfUpdate.present(
            SelfUpdateEligibility(
                unattendedEligible = false,
                userActionRequired = true,
                reason = "Unknown-source installs are not permitted; system confirmation is required",
                mustHandlePendingUserAction = true,
            ),
            unknownSourcesAllowed = false,
        )
        assertTrue(plan.settingsRequired)
        assertEquals(
            ControlSelfUpdatePhase.NeedsSettings,
            ControlSelfUpdate.phase(plan, pendingConfirmation = false, sessionExists = false, failed = false),
        )
    }

    @Test
    fun pendingConfirmationWithoutSessionFailsClosed() {
        val plan = ControlSelfUpdate.present(
            SelfUpdateEligibility(false, true, "confirm", true),
            unknownSourcesAllowed = true,
        )
        assertEquals(
            ControlSelfUpdatePhase.Failed,
            ControlSelfUpdate.phase(plan, pendingConfirmation = true, sessionExists = false, failed = false),
        )
        assertEquals(
            ControlSelfUpdatePhase.WaitingConfirmation,
            ControlSelfUpdate.phase(plan, pendingConfirmation = true, sessionExists = true, failed = false),
        )
    }
}
