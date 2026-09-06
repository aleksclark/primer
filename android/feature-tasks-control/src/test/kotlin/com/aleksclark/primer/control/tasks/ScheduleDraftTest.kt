package com.aleksclark.primer.control.tasks

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
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
    fun fromSchedulePreservesOriginalInstantAndFractionalLocalTime() {
        val schedule = com.aleksclark.primertasks.client.Schedule(
            dueOffsetMinutes = 45,
            enabled = true,
            endAt = "2026-04-01T12:00:00Z",
            id = "sch-1",
            kind = "recurrence",
            revisionId = "r1",
            rrule = "FREQ=WEEKLY;COUNT=7",
            startAt = "2026-11-01T07:30:45.123Z",
            studentId = "st1",
            templateId = "t1",
            timezone = "America/Chicago",
            version = 1,
        )
        val draft = scheduleDraftFrom(schedule)
        assertEquals("weekly", draft.kind)
        assertEquals("2026-11-01", draft.date)
        assertEquals("01:30:45.123", draft.time)
        assertEquals("2026-11-01T07:30:45.123Z", draft.originalStartAt)
        assertEquals("2026-11-01T07:30:45.123Z", draft.toInput("st1", "t1", "r1").startAt)
        assertEquals("45", draft.dueOffsetText)
    }

    @Test
    fun oneOffClearsRecurrenceRule() {
        val input = ScheduleDraft(
            kind = "one_off",
            date = "2026-03-08",
            time = "07:00",
            timezone = "UTC",
            rrule = "FREQ=DAILY;COUNT=7",
        ).toInput("st1", "t1", "r1")
        assertEquals("one_off", input.kind)
        assertNull(input.rrule)
    }

    @Test
    fun dstGapAndOverlapAreObservable() {
        val gap = ScheduleDraft(kind = "one_off", date = "2026-03-08", time = "02:30", timezone = "America/Chicago")
        val overlap = ScheduleDraft(kind = "one_off", date = "2026-11-01", time = "01:30", timezone = "America/Chicago")
        assertTrue(assertThrows(IllegalArgumentException::class.java) { gap.toInput("st1", "t1", "r1") }.message!!.contains("DST gap"))
        assertTrue(assertThrows(IllegalArgumentException::class.java) { overlap.toInput("st1", "t1", "r1") }.message!!.contains("DST overlap"))
    }

    @Test
    fun invalidDueOffsetAndDateStayErrors() {
        assertTrue(
            assertThrows(IllegalArgumentException::class.java) {
                ScheduleDraft(dueOffsetText = "abc", date = "2026-03-08", time = "07:00", timezone = "UTC").toInput("st1", "t1", "r1")
            }.message!!.contains("whole number"),
        )
        assertTrue(
            assertThrows(IllegalArgumentException::class.java) {
                ScheduleDraft(date = "03/08/2026", time = "07:00", timezone = "UTC").toInput("st1", "t1", "r1")
            }.message!!.contains("YYYY-MM-DD"),
        )
    }

    @Test
    fun dailyUsesServerOwnedRrule() {
        val input = ScheduleDraft(kind = "daily", date = "2026-03-08", time = "07:00", timezone = "UTC")
            .toInput("st1", "t1", "r1")
        assertEquals("recurrence", input.kind)
        assertEquals("FREQ=DAILY;COUNT=7", input.rrule)
    }
}
