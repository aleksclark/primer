package com.aleksclark.primer.tv.app.ui.designsystem

import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Immutable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import com.aleksclark.primer.tv.core.domain.FormFactor

/** Spacing scale from System C tokens plus form-factor layout constants. */
@Immutable
data class PrimerSpacing(
    val xs: Dp = PrimerTokens.Space.Value1.dp,
    val sm: Dp = PrimerTokens.Space.Value2.dp,
    val md: Dp = PrimerTokens.Space.Value4.dp,
    val lg: Dp = PrimerTokens.Space.Value6.dp,
    val xl: Dp = PrimerTokens.Space.Value8.dp,
    val xxl: Dp = PrimerTokens.Space.Value12.dp,
    val screenHorizontal: Dp,
    val screenVertical: Dp,
    val railGutter: Dp,
    val cardWidth: Dp,
    val navCollapsedWidth: Dp,
    val navExpandedWidth: Dp,
    val focusScale: Float,
    val focusBorderWidth: Dp,
    val focusOffset: Dp = PrimerTokens.Focus.Offset.dp,
    val ruleWidth: Dp = PrimerTokens.Rule.Default.dp,
    val ruleActiveWidth: Dp = PrimerTokens.Rule.Active.dp,
)

/**
 * System C shapes are square (radius 0). Form factor no longer changes corner
 * radius — only layout metrics differ.
 */
@Immutable
data class PrimerShapes(
    val mediaCard: RoundedCornerShape,
    val button: RoundedCornerShape,
    val chip: RoundedCornerShape,
    val panel: RoundedCornerShape,
)

private val SystemCShape = RoundedCornerShape(PrimerTokens.Radius.Default.dp)

fun primerSpacing(formFactor: FormFactor): PrimerSpacing = when (formFactor) {
    FormFactor.TABLET -> PrimerSpacing(
        screenHorizontal = PrimerTokens.Space.Value6.dp,
        screenVertical = PrimerTokens.Space.Value6.dp,
        railGutter = PrimerTokens.Space.Value3.dp,
        cardWidth = 148.dp,
        navCollapsedWidth = 80.dp,
        navExpandedWidth = 220.dp,
        focusScale = 1.0f,
        // Touch surfaces do not need TV focus chrome.
        focusBorderWidth = 0.dp,
    )

    FormFactor.TELEVISION -> PrimerSpacing(
        screenHorizontal = PrimerTokens.Space.Value12.dp,
        screenVertical = PrimerTokens.Space.Value8.dp,
        railGutter = PrimerTokens.Space.Value4.dp,
        cardWidth = 200.dp,
        navCollapsedWidth = 72.dp,
        navExpandedWidth = 240.dp,
        focusScale = 1.08f,
        // 10-foot UI: slightly stronger than the 1px web focus ring.
        focusBorderWidth = PrimerTokens.Rule.Active.dp,
    )
}

@Suppress("UNUSED_PARAMETER")
fun primerShapes(formFactor: FormFactor): PrimerShapes = PrimerShapes(
    mediaCard = SystemCShape,
    button = SystemCShape,
    chip = SystemCShape,
    panel = SystemCShape,
)

val LocalPrimerSpacing = staticCompositionLocalOf {
    primerSpacing(FormFactor.TABLET)
}

val LocalPrimerShapes = staticCompositionLocalOf {
    primerShapes(FormFactor.TABLET)
}
