package com.aleksclark.primer.tv.core.presentation

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class PairingPresenterTest {

    @Test
    fun `pairing code is normalized to uppercase`() {
        assertEquals("A3C9XY", PairingPresenter.normalizeCode("a3c9xy"))
        assertEquals("A3C9XY", PairingPresenter.normalizeCode("A3c9Xy"))
        assertEquals("ABCD", PairingPresenter.normalizeCode("ABCD"))
    }

    @Test
    fun `submit is blocked until both fields are filled and not submitting`() {
        assertFalse(PairingPresenter.canSubmit("", "ABCD", submitting = false))
        assertFalse(PairingPresenter.canSubmit("https://tv.local", "", submitting = false))
        assertFalse(PairingPresenter.canSubmit("https://tv.local", "ABCD", submitting = true))
        assertTrue(PairingPresenter.canSubmit("https://tv.local", "ABCD", submitting = false))
    }

    @Test
    fun `failure message prefers server detail and falls back to actionable copy`() {
        assertEquals(
            "pairing code already used",
            PairingPresenter.failureMessage("pairing code already used").value,
        )
        assertEquals(
            PairingPresenter.INLINE_ERROR_FALLBACK,
            PairingPresenter.failureMessage(null).value,
        )
        assertEquals(
            PairingPresenter.INLINE_ERROR_FALLBACK,
            PairingPresenter.failureMessage("   ").value,
        )
    }
}
