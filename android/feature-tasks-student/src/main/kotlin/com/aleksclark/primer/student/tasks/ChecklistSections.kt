package com.aleksclark.primer.student.tasks

/** Server-backed checklist composition rules kept separate for deterministic UI tests. */
data class ChecklistSections(val showToday: Boolean, val showUpcoming: Boolean)

internal fun checklistSections(todayCount: Int, upcomingCount: Int): ChecklistSections =
    ChecklistSections(showToday = todayCount > 0, showUpcoming = upcomingCount > 0)
