package com.aleksclark.primer.control.tasks

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class ScheduleDraftTest {
    @Test
    fun oneOffUsesZonedStartWithoutRrule() {
        val input = ScheduleDraft(kind = "one_off", date = "2026-03-08", time = "07:00", timezone = "America/Chicago")
            .toInput("st1", "t1", "r1")
        assertEquals("one_off", input.kind)
        assertEquals("st1", input.studentId)
        assertNull(input.rrule)
        assertEquals("2026-03-08T12:00:00Z", input.startAt)
    }

    @Test
    fun fromSchedulePreservesExistingDateTimeTimezoneAndDue() {
        val schedule = com.aleksclark.primertasks.client.Schedule(
            dueOffsetMinutes = 45,
            enabled = true,
            endAt = "2026-04-01T12:00:00Z",
            id = "sch-1",
            kind = "recurrence",
            revisionId = "r1",
            rrule = "FREQ=WEEKLY;COUNT=7",
            startAt = "2026-03-08T12:00:00Z",
            studentId = "st1",
            templateId = "t1",
            timezone = "America/Chicago",
            version = 1,
        )
        val draft = scheduleDraftFrom(schedule)
        assertEquals("weekly", draft.kind)
        assertEquals("2026-03-08", draft.date)
        assertEquals("07:00", draft.time)
        assertEquals("America/Chicago", draft.timezone)
        assertEquals("FREQ=WEEKLY;COUNT=7", draft.rrule)
        assertEquals(45L, draft.dueOffsetMinutes)
        assertEquals("2026-04-01T12:00:00Z", draft.endAt)
    }

    @Test
    fun dailyUsesServerOwnedRrule() {
        val input = ScheduleDraft(kind = "daily", date = "2026-03-08", time = "07:00", timezone = "UTC")
            .toInput("st1", "t1", "r1")
        assertEquals("recurrence", input.kind)
        assertEquals("FREQ=DAILY;COUNT=7", input.rrule)
    }
}
