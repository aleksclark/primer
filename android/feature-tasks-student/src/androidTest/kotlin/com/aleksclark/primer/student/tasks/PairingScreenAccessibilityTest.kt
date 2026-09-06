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
class PairingScreenAccessibilityTest {
    @get:Rule
    val composeRule = createComposeRule()

    @Test
    fun cameraIsPrimaryAndImageImportIsASeparateAccessibleAction() {
        var cameraClicks = 0
        var imageClicks = 0
        composeRule.setContent {
            com.aleksclark.primer.ui.PrimerTheme {
                PairingScreen(
                    message = null,
                    onScan = { cameraClicks++ },
                    onImportImage = { imageClicks++ },
                )
            }
        }

        composeRule.onNodeWithText("SCAN PAIRING QR")
            .assertIsDisplayed()
            .assertHasClickAction()
            .performClick()
        composeRule.onNodeWithContentDescription("Import pairing QR image")
            .assertIsDisplayed()
            .assertHasClickAction()
            .performClick()

        assertEquals(1, cameraClicks)
        assertEquals(1, imageClicks)
    }
}
