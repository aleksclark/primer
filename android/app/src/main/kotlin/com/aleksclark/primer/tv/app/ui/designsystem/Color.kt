package com.aleksclark.primer.tv.app.ui.designsystem

import androidx.compose.runtime.Immutable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color

/**
 * Semantic color roles for Primer TV, mapped from System C ([PrimerTokens]).
 *
 * Classification accents (`live`, `educational`, `entertainment`) are product
 * extensions for media badges only. They are derived from System C tokens
 * (attention / accent / accentHover) — not a parallel palette.
 */
@Immutable
data class PrimerColors(
    val background: Color,
    val surface: Color,
    val surfaceRaised: Color,
    val onSurface: Color,
    val onSurfaceMuted: Color,
    val brand: Color,
    val brandHover: Color,
    val onBrand: Color,
    /** Live / on-air badge — derived from attention. */
    val live: Color,
    val onLive: Color,
    /** Educational classification — derived from accent (product extension). */
    val educational: Color,
    /** Entertainment / one-viewing — derived from accentHover (product extension). */
    val entertainment: Color,
    val error: Color,
    val onError: Color,
    val focusBorder: Color,
    val focusGlow: Color,
    val scrimStrong: Color,
    val scrimSoft: Color,
    /** Default rule / outline (System C `rule`). */
    val outline: Color,
    /** Strong rule / section divide (System C `ruleStrong`). */
    val outlineStrong: Color,
)

/**
 * Builds semantic colors from a System C theme object.
 * Dark is the product default.
 */
fun primerColorsFromTokens(
    surface: Color,
    surfaceRaised: Color,
    rule: Color,
    ruleStrong: Color,
    textMuted: Color,
    text: Color,
    accent: Color,
    accentHover: Color,
    onAccent: Color,
    attention: Color,
): PrimerColors = PrimerColors(
    background = surface,
    surface = surface,
    surfaceRaised = surfaceRaised,
    onSurface = text,
    onSurfaceMuted = textMuted,
    brand = accent,
    brandHover = accentHover,
    onBrand = onAccent,
    // Classification / live: derived from System C, not free palette hex.
    live = attention,
    onLive = onAccent,
    educational = accent,
    entertainment = accentHover,
    error = attention,
    onError = onAccent,
    focusBorder = accent,
    focusGlow = accent.copy(alpha = 0.40f),
    scrimStrong = surface.copy(alpha = 0.80f),
    scrimSoft = surface.copy(alpha = 0.40f),
    outline = rule,
    outlineStrong = ruleStrong,
)

val PrimerDarkColors: PrimerColors = primerColorsFromTokens(
    surface = PrimerTokens.Dark.surface,
    surfaceRaised = PrimerTokens.Dark.surfaceRaised,
    rule = PrimerTokens.Dark.rule,
    ruleStrong = PrimerTokens.Dark.ruleStrong,
    textMuted = PrimerTokens.Dark.textMuted,
    text = PrimerTokens.Dark.text,
    accent = PrimerTokens.Dark.accent,
    accentHover = PrimerTokens.Dark.accentHover,
    onAccent = PrimerTokens.Dark.onAccent,
    attention = PrimerTokens.Dark.attention,
)

val PrimerLightColors: PrimerColors = primerColorsFromTokens(
    surface = PrimerTokens.Light.surface,
    surfaceRaised = PrimerTokens.Light.surfaceRaised,
    rule = PrimerTokens.Light.rule,
    ruleStrong = PrimerTokens.Light.ruleStrong,
    textMuted = PrimerTokens.Light.textMuted,
    text = PrimerTokens.Light.text,
    accent = PrimerTokens.Light.accent,
    accentHover = PrimerTokens.Light.accentHover,
    onAccent = PrimerTokens.Light.onAccent,
    attention = PrimerTokens.Light.attention,
)

val LocalPrimerColors = staticCompositionLocalOf { PrimerDarkColors }
