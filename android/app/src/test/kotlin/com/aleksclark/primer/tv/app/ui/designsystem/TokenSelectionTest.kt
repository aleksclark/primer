package com.aleksclark.primer.tv.app.ui.designsystem

import androidx.compose.ui.unit.dp
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
    fun `spacing scale matches System C base tokens`() {
        val tablet = primerSpacing(FormFactor.TABLET)
        assertEquals(PrimerTokens.Space.Value1.dp, tablet.xs)
        assertEquals(PrimerTokens.Space.Value2.dp, tablet.sm)
        assertEquals(PrimerTokens.Space.Value4.dp, tablet.md)
        assertEquals(PrimerTokens.Space.Value6.dp, tablet.lg)
        assertEquals(PrimerTokens.Space.Value8.dp, tablet.xl)
        assertEquals(PrimerTokens.Space.Value12.dp, tablet.xxl)
        assertEquals(PrimerTokens.Rule.Default.dp, tablet.ruleWidth)
        assertEquals(PrimerTokens.Focus.Offset.dp, tablet.focusOffset)
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
    fun `system labels and buttons use mono faces`() {
        val tablet = primerTypography(FormFactor.TABLET)
        assertEquals(androidx.compose.ui.text.font.FontFamily.Monospace, tablet.label.fontFamily)
        assertEquals(androidx.compose.ui.text.font.FontFamily.Monospace, tablet.button.fontFamily)
        assertEquals(androidx.compose.ui.text.font.FontFamily.SansSerif, tablet.body.fontFamily)
    }

    @Test
    fun `shapes are square System C radius zero for every form factor`() {
        val tablet = primerShapes(FormFactor.TABLET)
        val television = primerShapes(FormFactor.TELEVISION)
        val square = androidx.compose.foundation.shape.RoundedCornerShape(PrimerTokens.Radius.Default.dp)

        assertEquals(square, tablet.mediaCard)
        assertEquals(square, tablet.panel)
        assertEquals(square, tablet.button)
        assertEquals(square, tablet.chip)
        // Form factor no longer changes geometry — only layout metrics do.
        assertEquals(tablet.mediaCard, television.mediaCard)
        assertEquals(tablet.panel, television.panel)
        assertEquals(tablet.button, television.button)
        assertEquals(tablet.chip, television.chip)
    }

    @Test
    fun `dark colors map from System C tokens`() {
        val colors = PrimerDarkColors
        assertEquals(PrimerTokens.Dark.surface, colors.background)
        assertEquals(PrimerTokens.Dark.surface, colors.surface)
        assertEquals(PrimerTokens.Dark.surfaceRaised, colors.surfaceRaised)
        assertEquals(PrimerTokens.Dark.text, colors.onSurface)
        assertEquals(PrimerTokens.Dark.textMuted, colors.onSurfaceMuted)
        assertEquals(PrimerTokens.Dark.accent, colors.brand)
        assertEquals(PrimerTokens.Dark.accentHover, colors.brandHover)
        assertEquals(PrimerTokens.Dark.onAccent, colors.onBrand)
        assertEquals(PrimerTokens.Dark.attention, colors.error)
        assertEquals(PrimerTokens.Dark.attention, colors.live)
        assertEquals(PrimerTokens.Dark.rule, colors.outline)
        assertEquals(PrimerTokens.Dark.ruleStrong, colors.outlineStrong)
        assertEquals(PrimerTokens.Dark.accent, colors.focusBorder)
        assertEquals(0.40f, colors.focusGlow.alpha, 0.001f)
    }

    @Test
    fun `light colors map from System C tokens`() {
        val colors = PrimerLightColors
        assertEquals(PrimerTokens.Light.surface, colors.background)
        assertEquals(PrimerTokens.Light.accent, colors.brand)
        assertEquals(PrimerTokens.Light.attention, colors.error)
        assertEquals(PrimerTokens.Light.rule, colors.outline)
    }

    @Test
    fun `color roles expose brand live and classification accents`() {
        val colors = PrimerDarkColors
        // background and surface are both System C surface in dark (no separate page ground).
        assertEquals(colors.background, colors.surface)
        assertNotEquals(colors.surface, colors.surfaceRaised)
        assertNotEquals(colors.brand, colors.live)
        assertNotEquals(colors.educational, colors.entertainment)
        assertNotEquals(colors.onSurface, colors.onSurfaceMuted)
        // Classification accents are derived from System C, not free hex.
        assertEquals(PrimerTokens.Dark.accent, colors.educational)
        assertEquals(PrimerTokens.Dark.accentHover, colors.entertainment)
        assertEquals(colors.error, colors.live)
    }

    @Test
    fun `motion durations come from System C tokens`() {
        val motion = primerMotion(reduced = false)
        assertEquals(PrimerTokens.Motion.Fast, motion.focusMs)
        assertEquals(PrimerTokens.Motion.Standard, motion.fadeMs)
        assertEquals(PrimerTokens.Motion.Standard, motion.railExpandMs)
    }
}
