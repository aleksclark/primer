package com.aleksclark.primer.tv.app.ui.designsystem

import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.ReadOnlyComposable
import androidx.compose.runtime.remember
import androidx.compose.runtime.staticCompositionLocalOf
import com.aleksclark.primer.tv.core.domain.FormFactor

val LocalFormFactor = staticCompositionLocalOf { FormFactor.TABLET }

object PrimerTheme {
    val colors: PrimerColors
        @Composable
        @ReadOnlyComposable
        get() = LocalPrimerColors.current

    val typography: PrimerTypography
        @Composable
        @ReadOnlyComposable
        get() = LocalPrimerTypography.current

    val spacing: PrimerSpacing
        @Composable
        @ReadOnlyComposable
        get() = LocalPrimerSpacing.current

    val shapes: PrimerShapes
        @Composable
        @ReadOnlyComposable
        get() = LocalPrimerShapes.current

    val formFactor: FormFactor
        @Composable
        @ReadOnlyComposable
        get() = LocalFormFactor.current

    val motion: PrimerMotion
        @Composable
        @ReadOnlyComposable
        get() = LocalPrimerMotion.current
}

/**
 * Supplies Primer color, type, shape, and spacing tokens for the active
 * form factor, and bridges them into Material 3 components.
 */
@Composable
fun PrimerTvTheme(
    formFactor: FormFactor,
    content: @Composable () -> Unit,
) {
    val colors = PrimerDarkColors
    val typography = remember(formFactor) { primerTypography(formFactor) }
    val spacing = remember(formFactor) { primerSpacing(formFactor) }
    val shapes = remember(formFactor) { primerShapes(formFactor) }
    val reducedMotion = rememberReducedMotion()
    val motion = remember(reducedMotion) { primerMotion(reducedMotion) }

    val materialColors = darkColorScheme(
        primary = colors.brand,
        onPrimary = colors.onBrand,
        secondary = colors.educational,
        onSecondary = colors.onBrand,
        tertiary = colors.entertainment,
        onTertiary = colors.onBrand,
        background = colors.background,
        onBackground = colors.onSurface,
        surface = colors.surface,
        onSurface = colors.onSurface,
        surfaceVariant = colors.surfaceRaised,
        onSurfaceVariant = colors.onSurfaceMuted,
        error = colors.error,
        onError = colors.onError,
        outline = colors.outline,
    )

    CompositionLocalProvider(
        LocalFormFactor provides formFactor,
        LocalPrimerColors provides colors,
        LocalPrimerTypography provides typography,
        LocalPrimerSpacing provides spacing,
        LocalPrimerShapes provides shapes,
        LocalPrimerMotion provides motion,
    ) {
        MaterialTheme(
            colorScheme = materialColors,
            content = content,
        )
    }
}
