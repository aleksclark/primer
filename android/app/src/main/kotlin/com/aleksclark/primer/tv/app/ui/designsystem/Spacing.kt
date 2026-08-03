package com.aleksclark.primer.tv.app.ui.designsystem

import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Immutable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import com.aleksclark.primer.tv.core.domain.FormFactor

/** 4/8dp spacing scale plus form-factor layout constants. */
@Immutable
data class PrimerSpacing(
    val xs: Dp = 4.dp,
    val sm: Dp = 8.dp,
    val md: Dp = 16.dp,
    val lg: Dp = 24.dp,
    val xl: Dp = 32.dp,
    val xxl: Dp = 48.dp,
    val screenHorizontal: Dp,
    val screenVertical: Dp,
    val railGutter: Dp,
    val cardWidth: Dp,
    val navCollapsedWidth: Dp,
    val navExpandedWidth: Dp,
    val focusScale: Float,
    val focusBorderWidth: Dp,
)

@Immutable
data class PrimerShapes(
    val mediaCard: RoundedCornerShape,
    val button: RoundedCornerShape,
    val chip: RoundedCornerShape,
    val panel: RoundedCornerShape,
)

fun primerSpacing(formFactor: FormFactor): PrimerSpacing = when (formFactor) {
    FormFactor.TABLET -> PrimerSpacing(
        screenHorizontal = 24.dp,
        screenVertical = 24.dp,
        railGutter = 12.dp,
        cardWidth = 148.dp,
        navCollapsedWidth = 80.dp,
        navExpandedWidth = 220.dp,
        focusScale = 1.0f,
        focusBorderWidth = 0.dp,
    )

    FormFactor.TELEVISION -> PrimerSpacing(
        screenHorizontal = 48.dp,
        screenVertical = 32.dp,
        railGutter = 16.dp,
        cardWidth = 200.dp,
        navCollapsedWidth = 72.dp,
        navExpandedWidth = 240.dp,
        focusScale = 1.08f,
        focusBorderWidth = 3.dp,
    )
}

fun primerShapes(formFactor: FormFactor): PrimerShapes = when (formFactor) {
    FormFactor.TABLET -> PrimerShapes(
        mediaCard = RoundedCornerShape(10.dp),
        button = RoundedCornerShape(10.dp),
        chip = RoundedCornerShape(999.dp),
        panel = RoundedCornerShape(16.dp),
    )

    FormFactor.TELEVISION -> PrimerShapes(
        mediaCard = RoundedCornerShape(12.dp),
        button = RoundedCornerShape(12.dp),
        chip = RoundedCornerShape(999.dp),
        panel = RoundedCornerShape(20.dp),
    )
}

val LocalPrimerSpacing = staticCompositionLocalOf {
    primerSpacing(FormFactor.TABLET)
}

val LocalPrimerShapes = staticCompositionLocalOf {
    primerShapes(FormFactor.TABLET)
}
