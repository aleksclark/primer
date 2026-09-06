package com.aleksclark.primer.updates

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class SelfUpdateFlowTest {
    private val active = SelfUpdateRecord(active = true, sessionId = 7, desiredVersion = 2, outcomeStatus = "installing")

    @Test
    fun missingConfirmationFailsClosed() {
        val next = SelfUpdateFlow.onPendingUserAction(active, hasConfirmation = false, presented = false)
        assertFalse(next.active)
        assertFalse(next.pendingConfirmation)
        assertEquals("blocked", next.outcomeStatus)
        assertTrue(SelfUpdateFlow.shouldAbandon(next))
    }

    @Test
    fun unpresentedConfirmationStaysPending() {
        val next = SelfUpdateFlow.onPendingUserAction(active, hasConfirmation = true, presented = false)
        assertTrue(next.active)
        assertTrue(next.pendingConfirmation)
        assertTrue(next.hasConfirmation)
        assertEquals("blocked", next.outcomeStatus)
        assertEquals("Install confirmation was not shown", next.outcomeError)
        assertFalse(SelfUpdateFlow.shouldAbandon(next))
    }

    @Test
    fun presentedConfirmationWaitsWithoutAbandoning() {
        val next = SelfUpdateFlow.onPendingUserAction(active, hasConfirmation = true, presented = true)
        assertTrue(next.pendingConfirmation)
        assertEquals("Waiting for system install confirmation", next.status)
        assertEquals(null, next.outcomeError)
        assertFalse(SelfUpdateFlow.shouldAbandon(next))
    }

    @Test
    fun resumeWithoutLiveSessionFailsClosed() {
        val pending = active.copy(pendingConfirmation = true, hasConfirmation = true)
        val next = SelfUpdateFlow.onResumeUserAction(pending, sessionLive = false, presented = true)
        assertFalse(next.active)
        assertFalse(next.pendingConfirmation)
        assertEquals("failed", next.outcomeStatus)
    }

    @Test
    fun resumeWithoutStoredConfirmationFailsClosed() {
        val pending = active.copy(pendingConfirmation = true, hasConfirmation = false)
        val next = SelfUpdateFlow.onResumeUserAction(pending, sessionLive = true, presented = true)
        assertFalse(next.pendingConfirmation)
        assertEquals("blocked", next.outcomeStatus)
    }

    @Test
    fun resumeCanPresentAfterBackgroundDeferral() {
        val deferred = SelfUpdateFlow.onPendingUserAction(active, hasConfirmation = true, presented = false)
        val resumed = SelfUpdateFlow.onResumeUserAction(deferred, sessionLive = true, presented = true)
        assertTrue(resumed.pendingConfirmation)
        assertEquals("Waiting for system install confirmation", resumed.status)
        assertEquals(null, resumed.outcomeError)
    }

    @Test
    fun cancelAndMissingSessionAreTerminal() {
        assertFalse(SelfUpdateFlow.onCancel(active).active)
        val missing = SelfUpdateFlow.onMissingInstallerSession(active)
        assertFalse(missing.active)
        assertFalse(missing.pendingConfirmation)
        assertEquals("failed", missing.outcomeStatus)
        val idle = active.copy(active = false)
        assertEquals(idle, SelfUpdateFlow.onMissingInstallerSession(idle))
    }
}
