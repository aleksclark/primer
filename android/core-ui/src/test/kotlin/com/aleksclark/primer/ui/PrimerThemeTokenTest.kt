package com.aleksclark.primer.ui

import com.aleksclark.primer.ui.tokens.PrimerTokens
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Test

class PrimerThemeTokenTest {
    @Test
    fun darkAndLightSchemesMapGeneratedSystemCTokens() {
        assertEquals(PrimerTokens.Dark.surface, PrimerDarkColorScheme.surface)
        assertEquals(PrimerTokens.Dark.accent, PrimerDarkColorScheme.accent)
        assertEquals(PrimerTokens.Dark.accent, PrimerDarkColorScheme.focus)
        assertEquals(PrimerTokens.Light.surface, PrimerLightColorScheme.surface)
        assertEquals(PrimerTokens.Light.accent, PrimerLightColorScheme.accent)
        assertEquals(PrimerTokens.Light.attention, PrimerLightColorScheme.attention)
        assertNotEquals(PrimerDarkColorScheme.surface, PrimerLightColorScheme.surface)
        assertNotEquals(PrimerDarkColorScheme.accent, PrimerLightColorScheme.accent)
    }

    @Test
    fun systemCGeometryIsSquareAndRuled() {
        assertEquals(0, PrimerTokens.Radius.Default)
        assertEquals(1, PrimerTokens.Rule.Default)
        assertEquals(2, PrimerTokens.Focus.Offset)
        assertEquals(1, PrimerTokens.Focus.Width)
        assertEquals(4, PrimerTokens.Space.Value1)
    }
}
