package com.aleksclark.primer.control.device

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class ControlUpdateDiscoveryTest {
    @Test
    fun defaultsDoNotAutoInstall() {
        val settings = ControlUpdateDiscoverySettings()
        assertFalse(
            ControlUpdateDiscovery.shouldRequestUnattendedInstall(
                settings,
                ControlSelfUpdatePhase.EligibleUnattended,
                canInstall = true,
                pendingConfirmation = false,
            ),
        )
        assertEquals(
            ControlDiscoveryAction.RefreshCatalog,
            ControlUpdateDiscovery.action(
                settings,
                nowMs = 1,
                phase = ControlSelfUpdatePhase.EligibleUnattended,
                canInstall = true,
                pendingConfirmation = false,
            ),
        )
    }

    @Test
    fun periodShorterThanFifteenMinutesIsSkipped() {
        val settings = ControlUpdateDiscoverySettings(
            checkOnResume = true,
            periodicEnabled = true,
            lastCatalogCheckAtMs = 1_000,
        )
        assertFalse(ControlUpdateDiscovery.shouldRefreshCatalog(settings, nowMs = 1_000 + ControlUpdateDiscovery.MIN_PERIOD_MS - 1))
        assertTrue(ControlUpdateDiscovery.shouldRefreshCatalog(settings, nowMs = 1_000 + ControlUpdateDiscovery.MIN_PERIOD_MS))
    }

    @Test
    fun unattendedCatchUpRequiresEligibleUnattendedAndInstallable() {
        val settings = ControlUpdateDiscoverySettings(unattendedCatchUp = true, lastCatalogCheckAtMs = Long.MAX_VALUE)
        assertTrue(
            ControlUpdateDiscovery.shouldRequestUnattendedInstall(
                settings,
                ControlSelfUpdatePhase.EligibleUnattended,
                canInstall = true,
                pendingConfirmation = false,
            ),
        )
        assertFalse(
            ControlUpdateDiscovery.shouldRequestUnattendedInstall(
                settings,
                ControlSelfUpdatePhase.EligibleConfirm,
                canInstall = true,
                pendingConfirmation = false,
            ),
        )
        assertFalse(
            ControlUpdateDiscovery.shouldRequestUnattendedInstall(
                settings,
                ControlSelfUpdatePhase.EligibleUnattended,
                canInstall = false,
                pendingConfirmation = false,
            ),
        )
        assertFalse(
            ControlUpdateDiscovery.shouldRequestUnattendedInstall(
                settings,
                ControlSelfUpdatePhase.EligibleUnattended,
                canInstall = true,
                pendingConfirmation = true,
            ),
        )
    }

    @Test
    fun pendingConfirmationIsResumeNotInstall() {
        val settings = ControlUpdateDiscoverySettings(unattendedCatchUp = true, checkOnResume = false, lastCatalogCheckAtMs = 1)
        assertEquals(
            ControlDiscoveryAction.None,
            ControlUpdateDiscovery.action(
                settings,
                nowMs = 1,
                phase = ControlSelfUpdatePhase.WaitingConfirmation,
                canInstall = true,
                pendingConfirmation = true,
            ),
        )
    }

    @Test
    fun failedOrDeferredNeverAutoInstall() {
        val settings = ControlUpdateDiscoverySettings(unattendedCatchUp = true)
        for (phase in listOf(ControlSelfUpdatePhase.Failed, ControlSelfUpdatePhase.Deferred, ControlSelfUpdatePhase.NeedsSettings)) {
            assertFalse(
                ControlUpdateDiscovery.shouldRequestUnattendedInstall(
                    settings,
                    phase,
                    canInstall = true,
                    pendingConfirmation = false,
                ),
            )
        }
    }
}
