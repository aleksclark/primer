package com.aleksclark.primertasks

import androidx.compose.material3.MaterialTheme
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
            MaterialTheme {
                PairingScreen(
                    message = null,
                    onScan = { cameraClicks++ },
                    onImportImage = { imageClicks++ },
                )
            }
        }

        composeRule.onNodeWithText("Scan pairing QR")
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
