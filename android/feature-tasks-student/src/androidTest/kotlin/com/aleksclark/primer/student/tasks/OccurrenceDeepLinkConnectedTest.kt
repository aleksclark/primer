package com.aleksclark.primer.student.tasks

import android.content.Intent
import android.net.Uri
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.uiautomator.By
import androidx.test.uiautomator.UiDevice
import androidx.test.uiautomator.Until
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/**
 * Connected promotion test for Primer Student. IDs and foreign title come from
 * the public parent/browser acceptance runner; no occurrence is fabricated here.
 *
 * Install with `adb install -r` so Tasks DataStore pairing is preserved. Device
 * owner / recovery files are never read or cleared by this test.
 */
class OccurrenceDeepLinkConnectedTest {
    private val instrumentation = InstrumentationRegistry.getInstrumentation()
    private val device = UiDevice.getInstance(instrumentation)
    private val arguments get() = InstrumentationRegistry.getArguments()

    @Before
    fun requirePublicAcceptanceInputs() {
        requireArgument("ownOccurrenceId")
        requireArgument("foreignOccurrenceId")
        requireArgument("foreignTitle")
    }

    @Test
    fun ownOccurrenceDeepLinkUsesMainActivityAndShowsOwnDetail() {
        val ownId = requireArgument("ownOccurrenceId")
        launchDeepLink(ownId)
        assertTrue("MainActivity did not render TASK DETAIL", device.wait(Until.hasObject(By.text("TASK DETAIL")), 15_000))
        assertTrue("own deep link did not load completed occurrence", device.wait(Until.hasObject(By.text("Status: completed")), 15_000))
        assertFalse("own deep link incorrectly rendered unavailable", device.hasObject(By.text("TASK UNAVAILABLE")))
    }

    @Test
    fun foreignOccurrenceDeepLinkUsesGenericUnavailableWithoutLeakingIdentity() {
        val foreignId = requireArgument("foreignOccurrenceId")
        val foreignTitle = requireArgument("foreignTitle")
        launchDeepLink(foreignId)
        assertTrue("MainActivity did not render generic unavailable screen", device.wait(Until.hasObject(By.text("TASK UNAVAILABLE")), 15_000))
        assertTrue(device.hasObject(By.text("This task is unavailable.")))
        assertTrue(device.hasObject(By.text("The requested task could not be opened.")))
        assertFalse("foreign occurrence ID leaked into the unavailable surface", device.hasObject(By.text(foreignId)))
        assertFalse("foreign task title leaked into the unavailable surface", device.hasObject(By.text(foreignTitle)))
    }

    private fun launchDeepLink(occurrenceId: String) {
        val intent = Intent(Intent.ACTION_VIEW, Uri.parse("primerstudent://occurrences/$occurrenceId")).apply {
            addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TASK)
            setPackage("com.aleksclark.primer.student")
        }
        instrumentation.targetContext.startActivity(intent)
        device.waitForIdle()
    }

    private fun requireArgument(name: String): String {
        val value = arguments.getString(name)
        return if (!value.isNullOrBlank()) value else error(
            "Missing instrumentation argument $name; provide the real ID/title from public acceptance",
        )
    }
}
