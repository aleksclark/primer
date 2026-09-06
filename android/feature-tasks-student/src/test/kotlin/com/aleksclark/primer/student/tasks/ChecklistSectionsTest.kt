package com.aleksclark.primer.student.tasks

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class ChecklistSectionsTest {
    @Test fun emptyChecklistHasNoSections() {
        val sections = checklistSections(0, 0)
        assertFalse(sections.showToday)
        assertFalse(sections.showUpcoming)
    }

    @Test fun upcomingSectionIsPresentIndependentOfTodayRows() {
        val sections = checklistSections(todayCount = 10, upcomingCount = 1)
        assertTrue(sections.showToday)
        assertTrue(sections.showUpcoming)
    }
}
