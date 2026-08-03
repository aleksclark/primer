package com.aleksclark.primer.tv.app.ui.designsystem

import androidx.compose.runtime.Immutable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color

/**
 * Semantic color roles for Primer TV. Classification accents are reserved for
 * badges and metadata; artwork and surfaces stay neutral.
 */
@Immutable
data class PrimerColors(
    val background: Color,
    val surface: Color,
    val surfaceRaised: Color,
    val onSurface: Color,
    val onSurfaceMuted: Color,
    val brand: Color,
    val onBrand: Color,
    val live: Color,
    val onLive: Color,
    val educational: Color,
    val entertainment: Color,
    val error: Color,
    val onError: Color,
    val focusBorder: Color,
    val focusGlow: Color,
    val scrimStrong: Color,
    val scrimSoft: Color,
    val outline: Color,
)

val PrimerDarkColors = PrimerColors(
    background = Color(0xFF0B0F14),
    surface = Color(0xFF151B24),
    surfaceRaised = Color(0xFF1E2633),
    onSurface = Color(0xFFF2F5F8),
    onSurfaceMuted = Color(0xFFA7B0BD),
    brand = Color(0xFF5B8CFF),
    onBrand = Color(0xFF071018),
    live = Color(0xFFE4572E),
    onLive = Color(0xFFFFF7F4),
    educational = Color(0xFF3DBE8B),
    entertainment = Color(0xFFE0A458),
    error = Color(0xFFFF6B6B),
    onError = Color(0xFF1A0505),
    focusBorder = Color(0xFFF4F7FF),
    focusGlow = Color(0x665B8CFF),
    scrimStrong = Color(0xCC05070A),
    scrimSoft = Color(0x6605070A),
    outline = Color(0xFF2C3545),
)

val LocalPrimerColors = staticCompositionLocalOf { PrimerDarkColors }
