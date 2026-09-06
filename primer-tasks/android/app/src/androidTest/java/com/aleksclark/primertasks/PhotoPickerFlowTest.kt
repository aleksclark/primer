package com.aleksclark.primertasks

import androidx.compose.ui.test.assertHasClickAction
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasContentDescription
import androidx.compose.ui.test.junit4.createEmptyComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.test.core.app.ActivityScenario
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.uiautomator.By
import androidx.test.uiautomator.UiDevice
import androidx.test.uiautomator.Until
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class PhotoPickerFlowTest {
    @get:Rule
    val composeRule = createEmptyComposeRule()

    private lateinit var scenario: ActivityScenario<MainActivity>
    private lateinit var device: UiDevice

    @Before
    fun launchFromCleanUnpairedState() {
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        val dataStoreDirectory = instrumentation.targetContext.filesDir.resolve("datastore")
        dataStoreDirectory.deleteRecursively()
        check(!dataStoreDirectory.exists()) {
            "Could not clear the target app DataStore before launching MainActivity"
        }
        device = UiDevice.getInstance(instrumentation)
        scenario = ActivityScenario.launch(MainActivity::class.java)
    }

    @After
    fun closeActivity() {
        scenario.close()
    }

    @Test
    fun secondaryImportOpensRealSystemPhotoPickerAndClosesThroughSystemUi() {
        composeRule.waitUntil(timeoutMillis = 10_000) {
            composeRule.onAllNodes(hasContentDescription(IMPORT_ACTION)).fetchSemanticsNodes().isNotEmpty()
        }

        // The filled CameraX action remains the primary pairing affordance.
        composeRule.onNodeWithText(SCAN_ACTION)
            .assertIsDisplayed()
            .assertHasClickAction()
        composeRule.onNodeWithContentDescription(IMPORT_ACTION)
            .assertIsDisplayed()
            .assertHasClickAction()
            .performClick()

        assertTrue(
            "Photo Picker did not become foreground",
            device.wait(Until.hasObject(By.pkg(PHOTO_PICKER_PACKAGE)), PICKER_TIMEOUT_MS),
        )
        assertEquals(PHOTO_PICKER_PACKAGE, device.currentPackageName)
        assertTrue(
            "Photo Picker privacy semantics were not visible",
            device.wait(Until.hasObject(By.text(PICKER_PRIVACY_TEXT)), PICKER_TIMEOUT_MS),
        )

        // Cancel is the picker-provided system action, not an app backdoor.
        assertTrue(
            "Photo Picker Cancel action was not visible",
            device.wait(Until.hasObject(By.desc(PICKER_CANCEL)), PICKER_TIMEOUT_MS),
        )
        device.findObject(By.desc(PICKER_CANCEL)).click()
        assertTrue(
            "App did not return after closing Photo Picker",
            device.wait(Until.hasObject(By.pkg(APP_PACKAGE)), PICKER_TIMEOUT_MS),
        )
        assertEquals(APP_PACKAGE, device.currentPackageName)
    }

    private companion object {
        const val APP_PACKAGE = "com.aleksclark.primertasks"
        const val PHOTO_PICKER_PACKAGE = "com.google.android.providers.media.module"
        const val PICKER_PRIVACY_TEXT = "This app can only access the photos you select"
        const val PICKER_CANCEL = "Cancel"
        const val SCAN_ACTION = "Scan pairing QR"
        const val IMPORT_ACTION = "Import pairing QR image"
        const val PICKER_TIMEOUT_MS = 5_000L
    }
}
