package com.aleksclark.primer.student.tasks

import androidx.compose.ui.test.assertHasClickAction
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class DashboardCardTest {
    @get:Rule
    val composeRule = createComposeRule()

    @Test
    fun cardShowsNamePendingWorkAndOpensTasks() {
        var opens = 0
        composeRule.setContent {
            com.aleksclark.primer.ui.PrimerTheme {
                StudentTasksDashboardCard(
                    snapshot = StudentDashboardSnapshot(
                        paired = true,
                        studentName = "Maya",
                        pendingToday = listOf(
                            PendingTodayTask("a", "Fractions", "Not started"),
                            PendingTodayTask("b", "Essay", "In progress"),
                        ),
                    ),
                    loading = false,
                    onOpenTasks = { opens++ },
                )
            }
        }

        composeRule.onNodeWithText("Maya").assertIsDisplayed()
        composeRule.onNodeWithText("2 pending today").assertIsDisplayed()
        composeRule.onNodeWithText("Fractions · Not started").assertIsDisplayed()
        composeRule.onNodeWithText("Essay · In progress").assertIsDisplayed()
        composeRule.onNodeWithContentDescription("Open today's tasks")
            .assertHasClickAction()
            .performClick()
        assertEquals(1, opens)
    }
}
