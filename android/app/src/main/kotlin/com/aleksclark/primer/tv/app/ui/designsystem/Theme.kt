package com.aleksclark.primer.tv.app.ui.designsystem

import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.ReadOnlyComposable
import androidx.compose.runtime.remember
import androidx.compose.runtime.staticCompositionLocalOf
import com.aleksclark.primer.tv.core.domain.FormFactor
import com.aleksclark.primer.ui.primerContentFontFamily
import com.aleksclark.primer.ui.primerSystemFontFamily
import com.aleksclark.primer.ui.PrimerTheme as SharedPrimerTheme

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
 *
 * Dark System C is the product default; [darkTheme] selects the light-parity scheme.
 * Shared primitives come from :core-ui; TV form-factor tokens stay local.
 */
@Composable
fun PrimerTvTheme(
    formFactor: FormFactor,
    darkTheme: Boolean = true,
    content: @Composable () -> Unit,
) {
    val colors = if (darkTheme) PrimerDarkColors else PrimerLightColors
    val contentFace = primerContentFontFamily()
    val systemFace = primerSystemFontFamily()
    val typography = remember(formFactor, contentFace, systemFace) {
        primerTypography(formFactor, contentFace, systemFace)
    }
    val spacing = remember(formFactor) { primerSpacing(formFactor) }
    val shapes = remember(formFactor) { primerShapes(formFactor) }
    val reducedMotion = rememberReducedMotion()
    val motion = remember(reducedMotion) { primerMotion(reducedMotion) }

    val materialColors = if (darkTheme) {
        darkColorScheme(
            primary = colors.brand,
            onPrimary = colors.onBrand,
            primaryContainer = colors.brandHover,
            onPrimaryContainer = colors.onBrand,
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
            outlineVariant = colors.outlineStrong,
        )
    } else {
        lightColorScheme(
            primary = colors.brand,
            onPrimary = colors.onBrand,
            primaryContainer = colors.brandHover,
            onPrimaryContainer = colors.onBrand,
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
            outlineVariant = colors.outlineStrong,
        )
    }

    SharedPrimerTheme(darkTheme = darkTheme) {
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
}
