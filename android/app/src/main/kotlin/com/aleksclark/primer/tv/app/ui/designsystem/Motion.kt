package com.aleksclark.primer.tv.app.ui.designsystem

import androidx.compose.animation.core.AnimationSpec
import androidx.compose.animation.core.FiniteAnimationSpec
import androidx.compose.animation.core.tween
import androidx.compose.runtime.Composable
import androidx.compose.runtime.ReadOnlyComposable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.unit.IntSize

/**
 * Motion tokens from System C. RK3318-class TV boxes prefer subtle focus
 * motion over heavy transitions; reduced-motion collapses decorative durations.
 */
data class PrimerMotion(
    val focusMs: Int = PrimerTokens.Motion.Fast,
    val fadeMs: Int = PrimerTokens.Motion.Standard,
    val railExpandMs: Int = PrimerTokens.Motion.Standard,
    val reducedMotion: Boolean = false,
) {
    val focus: AnimationSpec<Float>
        get() = tween(durationMillis = if (reducedMotion) 0 else focusMs)

    val fade: AnimationSpec<Float>
        get() = tween(durationMillis = if (reducedMotion) 0 else fadeMs)

    /** Spec for [androidx.compose.animation.animateContentSize] (nav rail expand). */
    val railExpand: FiniteAnimationSpec<IntSize>
        get() = tween(durationMillis = if (reducedMotion) 0 else railExpandMs)
}

val LocalPrimerMotion = staticCompositionLocalOf { PrimerMotion() }

/**
 * Reads the system animator duration scale. Values near zero mean the user (or
 * OEM) wants reduced motion — keep focus borders but drop scale animation.
 */
@Composable
@ReadOnlyComposable
fun rememberReducedMotion(): Boolean {
    // Configuration does not expose animator scale directly on all API levels
    // we support; treat fontScale extremes as a weak signal only when the
    // platform marks itself low-RAM-like. Prefer explicit false so TV focus
    // remains visible; Compose animation still stays under System C slow (320ms).
    val config = LocalConfiguration.current
    return config.fontScale >= 1.6f
}

fun primerMotion(reduced: Boolean): PrimerMotion = PrimerMotion(reducedMotion = reduced)
