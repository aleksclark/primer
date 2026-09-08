package com.aleksclark.primer.ui

import androidx.compose.material3.ColorScheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.ReadOnlyComposable
import androidx.compose.runtime.Stable
import androidx.compose.runtime.remember
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.Font
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.aleksclark.primer.ui.tokens.PrimerTokens

/** Semantic System C colors shared by Student, Control, and TV adapters. */
@Stable
data class PrimerColorScheme(
    val surface: Color,
    val surfaceRaised: Color,
    val rule: Color,
    val ruleStrong: Color,
    val textMuted: Color,
    val text: Color,
    val accent: Color,
    val accentHover: Color,
    val onAccent: Color,
    val attention: Color,
    val statusInProgress: Color,
    val statusSent: Color,
    val focus: Color = accent,
)

@Stable
data class PrimerSpacing(
    val xs: Dp = PrimerTokens.Space.Value1.dp,
    val sm: Dp = PrimerTokens.Space.Value2.dp,
    val md: Dp = PrimerTokens.Space.Value3.dp,
    val lg: Dp = PrimerTokens.Space.Value4.dp,
    val xl: Dp = PrimerTokens.Space.Value6.dp,
    val xxl: Dp = PrimerTokens.Space.Value8.dp,
    val rule: Dp = PrimerTokens.Rule.Default.dp,
    val activeRule: Dp = PrimerTokens.Rule.Active.dp,
    val progressRule: Dp = PrimerTokens.Rule.Progress.dp,
    val focusWidth: Dp = PrimerTokens.Focus.Width.dp,
    val focusOffset: Dp = PrimerTokens.Focus.Offset.dp,
)

@Stable
data class PrimerTypography(
    val display: TextStyle,
    val title: TextStyle,
    val sectionTitle: TextStyle,
    val body: TextStyle,
    val small: TextStyle,
    val label: TextStyle,
    val button: TextStyle,
    val mono: TextStyle,
)

val PrimerDarkColorScheme = PrimerColorScheme(
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
    statusInProgress = PrimerTokens.Dark.statusInProgress,
    statusSent = PrimerTokens.Dark.statusSent,
)

val PrimerLightColorScheme = PrimerColorScheme(
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
    statusInProgress = PrimerTokens.Light.statusInProgress,
    statusSent = PrimerTokens.Light.statusSent,
)

val PrimerDefaultSpacing = PrimerSpacing()

/** JVM-safe fallback used outside [PrimerTheme] and in unit tests. */
val PrimerFallbackTypography = primerTypography(FontFamily.SansSerif, FontFamily.Monospace)

private val LocalPrimerColorScheme = staticCompositionLocalOf { PrimerDarkColorScheme }
private val LocalPrimerSpacing = staticCompositionLocalOf { PrimerDefaultSpacing }
private val LocalPrimerTypography = staticCompositionLocalOf { PrimerFallbackTypography }

/**
 * Token accessors. Dark is the product default; light is a full-parity scheme
 * selected by [PrimerTheme].
 */
object PrimerTheme {
    val colors: PrimerColorScheme
        @Composable
        @ReadOnlyComposable
        get() = LocalPrimerColorScheme.current

    val spacing: PrimerSpacing
        @Composable
        @ReadOnlyComposable
        get() = LocalPrimerSpacing.current

    val typography: PrimerTypography
        @Composable
        @ReadOnlyComposable
        get() = LocalPrimerTypography.current
}

/**
 * Public System C theme. Bundles Instrument Sans / IBM Plex Mono and maps
 * generated tokens into Material 3 without exposing TV or device-policy types.
 */
@Composable
fun PrimerTheme(
    darkTheme: Boolean = true,
    content: @Composable () -> Unit,
) {
    val colors = if (darkTheme) PrimerDarkColorScheme else PrimerLightColorScheme
    val typography = remember {
        primerTypography(primerContentFontFamily(), primerSystemFontFamily())
    }
    CompositionLocalProvider(
        LocalPrimerColorScheme provides colors,
        LocalPrimerSpacing provides PrimerDefaultSpacing,
        LocalPrimerTypography provides typography,
    ) {
        MaterialTheme(
            colorScheme = colors.toMaterial(darkTheme),
            typography = MaterialTheme.typography.copy(
                displayLarge = typography.display,
                headlineMedium = typography.title,
                titleLarge = typography.sectionTitle,
                bodyLarge = typography.body,
                bodyMedium = typography.body,
                bodySmall = typography.small,
                labelLarge = typography.label,
                labelMedium = typography.label,
            ),
            content = content,
        )
    }
}

internal fun primerTypography(content: FontFamily, system: FontFamily) = PrimerTypography(
    display = TextStyle(
        fontFamily = content,
        fontWeight = FontWeight.Medium,
        fontSize = 34.sp,
        lineHeight = 38.sp,
        letterSpacing = (-0.022).sp,
    ),
    title = TextStyle(
        fontFamily = content,
        fontWeight = FontWeight.Medium,
        fontSize = 26.sp,
        lineHeight = 32.sp,
        letterSpacing = (-0.022).sp,
    ),
    sectionTitle = TextStyle(
        fontFamily = content,
        fontWeight = FontWeight.SemiBold,
        fontSize = 18.sp,
        lineHeight = 24.sp,
        letterSpacing = (-0.015).sp,
    ),
    body = TextStyle(
        fontFamily = content,
        fontWeight = FontWeight.Normal,
        fontSize = 16.sp,
        lineHeight = 24.sp,
    ),
    small = TextStyle(
        fontFamily = content,
        fontWeight = FontWeight.Normal,
        fontSize = 13.sp,
        lineHeight = 18.sp,
    ),
    label = TextStyle(
        fontFamily = system,
        fontWeight = FontWeight.Normal,
        fontSize = 11.sp,
        lineHeight = 16.sp,
        letterSpacing = 0.8.sp,
    ),
    button = TextStyle(
        fontFamily = system,
        fontWeight = FontWeight.Medium,
        fontSize = 12.sp,
        lineHeight = 16.sp,
        letterSpacing = 0.6.sp,
    ),
    mono = TextStyle(
        fontFamily = system,
        fontWeight = FontWeight.Normal,
        fontSize = 12.sp,
        lineHeight = 18.sp,
        letterSpacing = 0.4.sp,
    ),
)

fun primerContentFontFamily(): FontFamily = FontFamily(
    Font(R.font.instrument_sans, FontWeight.Normal),
    Font(R.font.instrument_sans, FontWeight.Medium),
    Font(R.font.instrument_sans, FontWeight.SemiBold),
    Font(R.font.instrument_sans, FontWeight.Bold),
)

fun primerSystemFontFamily(): FontFamily = FontFamily(
    Font(R.font.ibm_plex_mono_regular, FontWeight.Normal),
    Font(R.font.ibm_plex_mono_medium, FontWeight.Medium),
)

private fun PrimerColorScheme.toMaterial(darkTheme: Boolean): ColorScheme {
    val scheme = if (darkTheme) {
        darkColorScheme(
            primary = accent,
            onPrimary = onAccent,
            primaryContainer = accentHover,
            onPrimaryContainer = onAccent,
            background = surface,
            onBackground = text,
            surface = surface,
            onSurface = text,
            surfaceVariant = surfaceRaised,
            onSurfaceVariant = textMuted,
            outline = rule,
            outlineVariant = ruleStrong,
            error = attention,
            onError = onAccent,
        )
    } else {
        lightColorScheme(
            primary = accent,
            onPrimary = onAccent,
            primaryContainer = accentHover,
            onPrimaryContainer = onAccent,
            background = surface,
            onBackground = text,
            surface = surface,
            onSurface = text,
            surfaceVariant = surfaceRaised,
            onSurfaceVariant = textMuted,
            outline = rule,
            outlineVariant = ruleStrong,
            error = attention,
            onError = onAccent,
        )
    }
    return scheme
}
