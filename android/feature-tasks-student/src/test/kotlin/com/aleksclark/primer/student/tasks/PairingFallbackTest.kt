package com.aleksclark.primer.student.tasks

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class PairingFallbackTest {
    @Test
    fun pasteIsLabeledAsFallbackNotAScan() {
        assertEquals("Scan pairing QR", PairingActions.SCAN)
        assertEquals("Import pairing QR image", PairingActions.IMPORT_IMAGE)
        assertEquals("Paste pairing QR payload (fallback, not a scan)", PairingActions.PASTE_LABEL)
        assertEquals("Pair with pasted payload", PairingActions.PASTE_ACTION)
        assertTrue(PairingActions.PASTE_HELP.contains("fallback", ignoreCase = true))
        assertFalse(PairingActions.PASTE_LABEL.contains("scan", ignoreCase = false) && PairingActions.PASTE_LABEL.startsWith("Scan"))
        assertTrue(PairingActions.PASTE_LABEL.contains("not a scan"))
    }
}
