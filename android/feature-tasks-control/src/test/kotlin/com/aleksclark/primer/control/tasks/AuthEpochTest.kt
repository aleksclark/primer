package com.aleksclark.primer.control.tasks

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class AuthEpochTest {
    @Test
    fun lateResultAfterLogoutIsRejected() {
        val epoch = AuthEpoch()
        val started = epoch.current()
        epoch.bump()
        assertFalse(epoch.isCurrent(started))
        assertTrue(epoch.isCurrent(epoch.current()))
    }

    @Test
    fun mutationGateSerializesDoubleTap() {
        val gate = MutationGate()
        assertTrue(gate.tryBegin())
        assertFalse(gate.tryBegin())
        gate.end()
        assertTrue(gate.tryBegin())
    }

    @Test
    fun logoutStopsWhenServerRevocationFails() {
        val decision = FailClosedLogout.decide(serverRevoked = false, providerSignedOut = true, serverError = "Tasks down", providerError = null)
        assertTrue(decision is LogoutDecision.Incomplete)
        val incomplete = decision as LogoutDecision.Incomplete
        assertEquals("Tasks down", incomplete.message)
        assertFalse(incomplete.serverRevoked)
    }

    @Test
    fun logoutStopsWhenProviderSignOutFailsAfterServerRevoke() {
        val decision = FailClosedLogout.decide(serverRevoked = true, providerSignedOut = false, serverError = null, providerError = "Clerk down")
        assertTrue(decision is LogoutDecision.Incomplete)
        assertTrue((decision as LogoutDecision.Incomplete).serverRevoked)
    }

    @Test
    fun logoutClearsOnlyAfterBothRevocations() {
        assertEquals(LogoutDecision.ClearSession, FailClosedLogout.decide(true, true, null, null))
    }

    @Test
    fun scheduleIdentityDoesNotInventRevisions() {
        assertNull(ScheduleIdentity.fromPublishedTask(null, "r1"))
        assertNull(ScheduleIdentity.fromPublishedTask("t1", " "))
        assertEquals("t1", ScheduleIdentity.fromPublishedTask("t1", "r1")?.templateId)
        assertEquals("r1", ScheduleIdentity.fromPublishedTask("t1", "r1")?.revisionId)
    }

    @Test
    fun conflictRefreshIsScopedToMutatedResource() {
        assertTrue(ConflictRefresh.shouldRefreshOccurrence(ConflictResource.Occurrence))
        assertFalse(ConflictRefresh.shouldRefreshOccurrence(ConflictResource.Device))
        assertFalse(ConflictRefresh.shouldRefreshOccurrence(ConflictResource.Schedule))
        assertTrue(ConflictRefresh.shouldRefreshDevice(ConflictResource.Device))
    }
}
