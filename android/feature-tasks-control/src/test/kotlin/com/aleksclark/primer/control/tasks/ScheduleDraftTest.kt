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
    fun dailyUsesServerOwnedRrule() {
        val input = ScheduleDraft(kind = "daily", date = "2026-03-08", time = "07:00", timezone = "UTC")
            .toInput("st1", "t1", "r1")
        assertEquals("recurrence", input.kind)
        assertEquals("FREQ=DAILY;COUNT=7", input.rrule)
    }
}
