package com.aleksclark.primer.student.tasks

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class PairingFallbackTest {
    @Test
    fun pasteIsLabeledAsFallbackNotAScan() {
        assertEquals("Scan parent’s QR code", PairingActions.SCAN)
        assertEquals("Use a saved QR image", PairingActions.IMPORT_IMAGE)
        assertEquals("Can’t scan the code?", PairingActions.FALLBACK)
        assertEquals("Pairing information", PairingActions.PASTE_LABEL)
        assertEquals("Connect this device", PairingActions.PASTE_ACTION)
        assertTrue(PairingActions.PASTE_HELP.contains("unavailable", ignoreCase = true))
        assertFalse(PairingActions.PASTE_LABEL.contains("payload", ignoreCase = true))
    }
}
