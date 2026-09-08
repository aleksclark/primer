package com.aleksclark.primer.student.tasks

import androidx.compose.ui.test.assertDoesNotExist
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class ChecklistBackNavigationTest {
    @get:Rule
    val composeRule = createComposeRule()

    @Test
    fun headerBackReturnsToHomeWithoutOneDeviceCopy() {
        var leaves = 0
        composeRule.setContent {
            com.aleksclark.primer.ui.PrimerTheme {
                PairingScreen(
                    message = null,
                    onScan = {},
                    onImportImage = {},
                    onBack = { leaves++ },
                )
            }
        }

        composeRule.onNodeWithText("ONE STUDENT · ONE DEVICE").assertDoesNotExist()
        composeRule.onNodeWithText("BACK").assertIsDisplayed().performClick()
        assertEquals(1, leaves)
    }
}
