package com.aleksclark.primer.tv.app.ui.designsystem

import com.aleksclark.primer.tv.core.domain.FormFactor
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class TokenSelectionTest {

    @Test
    fun `TV spacing is overscan-safe and larger than tablet`() {
        val tablet = primerSpacing(FormFactor.TABLET)
        val television = primerSpacing(FormFactor.TELEVISION)

        assertTrue(television.screenHorizontal > tablet.screenHorizontal)
        assertTrue(television.screenVertical >= tablet.screenVertical)
        assertTrue(television.cardWidth > tablet.cardWidth)
        assertEquals(1.08f, television.focusScale)
        assertEquals(1.0f, tablet.focusScale)
        assertTrue(television.focusBorderWidth > tablet.focusBorderWidth)
        assertTrue(television.navExpandedWidth > television.navCollapsedWidth)
    }

    @Test
    fun `TV typography is a distinct token set not tablet arithmetic`() {
        val tablet = primerTypography(FormFactor.TABLET)
        val television = primerTypography(FormFactor.TELEVISION)

        assertNotEquals(tablet.screenTitle.fontSize, television.screenTitle.fontSize)
        assertNotEquals(tablet.body.fontSize, television.body.fontSize)
        assertTrue(television.heroTitle.fontSize > tablet.heroTitle.fontSize)
        assertTrue(television.cardTitle.fontSize > tablet.cardTitle.fontSize)
        assertTrue(television.guideTime.fontSize > tablet.guideTime.fontSize)
    }

    @Test
    fun `shape families differ by form factor while remaining rounded`() {
        val tablet = primerShapes(FormFactor.TABLET)
        val television = primerShapes(FormFactor.TELEVISION)

        assertNotEquals(tablet.mediaCard, television.mediaCard)
        assertNotEquals(tablet.panel, television.panel)
    }

    @Test
    fun `color roles expose brand live and classification accents`() {
        val colors = PrimerDarkColors
        assertNotEquals(colors.background, colors.surface)
        assertNotEquals(colors.brand, colors.live)
        assertNotEquals(colors.educational, colors.entertainment)
        assertNotEquals(colors.onSurface, colors.onSurfaceMuted)
        assertNotEquals(colors.error, colors.entertainment)
    }
}
