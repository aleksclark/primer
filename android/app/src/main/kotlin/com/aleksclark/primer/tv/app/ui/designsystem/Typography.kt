package com.aleksclark.primer.tv.app.ui.designsystem

import androidx.compose.runtime.Immutable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.sp
import com.aleksclark.primer.tv.core.domain.FormFactor

/**
 * Semantic type roles. Phone/tablet and TV keep separate sizes so TV is not
 * "tablet plus three".
 *
 * System C intent: content stays sans-serif (Instrument Sans stand-in);
 * labels/buttons use monospace + tracked letter-spacing as the system voice.
 * Full Instrument Sans / IBM Plex Mono packaging is a remaining gap — platform
 * defaults approximate the families without bundling font files.
 */
@Immutable
data class PrimerTypography(
    val heroTitle: TextStyle,
    val screenTitle: TextStyle,
    val railTitle: TextStyle,
    val cardTitle: TextStyle,
    val metadata: TextStyle,
    val body: TextStyle,
    val label: TextStyle,
    val button: TextStyle,
    val guideTime: TextStyle,
)

/** Content face — sans, approximating Instrument Sans. */
private val ContentFace = FontFamily.SansSerif

/** System voice — mono, approximating IBM Plex Mono (uppercase at call sites). */
private val SystemFace = FontFamily.Monospace

fun primerTypography(formFactor: FormFactor): PrimerTypography = when (formFactor) {
    FormFactor.TABLET -> PrimerTypography(
        heroTitle = TextStyle(
            fontFamily = ContentFace,
            fontWeight = FontWeight.Medium,
            fontSize = 32.sp,
            lineHeight = 38.sp,
            letterSpacing = (-0.02).sp,
        ),
        screenTitle = TextStyle(
            fontFamily = ContentFace,
            fontWeight = FontWeight.Medium,
            fontSize = 28.sp,
            lineHeight = 34.sp,
            letterSpacing = (-0.02).sp,
        ),
        railTitle = TextStyle(
            fontFamily = ContentFace,
            fontWeight = FontWeight.SemiBold,
            fontSize = 20.sp,
            lineHeight = 26.sp,
            letterSpacing = (-0.015).sp,
        ),
        cardTitle = TextStyle(
            fontFamily = ContentFace,
            fontWeight = FontWeight.SemiBold,
            fontSize = 16.sp,
            lineHeight = 20.sp,
            letterSpacing = (-0.015).sp,
        ),
        metadata = TextStyle(
            fontFamily = ContentFace,
            fontWeight = FontWeight.Normal,
            fontSize = 13.sp,
            lineHeight = 18.sp,
        ),
        body = TextStyle(
            fontFamily = ContentFace,
            fontWeight = FontWeight.Normal,
            fontSize = 16.sp,
            lineHeight = 22.sp,
        ),
        label = TextStyle(
            fontFamily = SystemFace,
            fontWeight = FontWeight.Normal,
            fontSize = 11.sp,
            lineHeight = 16.sp,
            letterSpacing = 0.8.sp,
        ),
        button = TextStyle(
            fontFamily = SystemFace,
            fontWeight = FontWeight.Medium,
            fontSize = 12.sp,
            lineHeight = 16.sp,
            letterSpacing = 0.6.sp,
        ),
        guideTime = TextStyle(
            fontFamily = SystemFace,
            fontWeight = FontWeight.Medium,
            fontSize = 14.sp,
            lineHeight = 18.sp,
            fontFeatureSettings = "tnum",
            letterSpacing = 0.4.sp,
        ),
    )

    FormFactor.TELEVISION -> PrimerTypography(
        heroTitle = TextStyle(
            fontFamily = ContentFace,
            fontWeight = FontWeight.Medium,
            fontSize = 48.sp,
            lineHeight = 56.sp,
            letterSpacing = (-0.024).sp,
        ),
        screenTitle = TextStyle(
            fontFamily = ContentFace,
            fontWeight = FontWeight.Medium,
            fontSize = 40.sp,
            lineHeight = 48.sp,
            letterSpacing = (-0.022).sp,
        ),
        railTitle = TextStyle(
            fontFamily = ContentFace,
            fontWeight = FontWeight.SemiBold,
            fontSize = 28.sp,
            lineHeight = 34.sp,
            letterSpacing = (-0.015).sp,
        ),
        cardTitle = TextStyle(
            fontFamily = ContentFace,
            fontWeight = FontWeight.SemiBold,
            fontSize = 22.sp,
            lineHeight = 28.sp,
            letterSpacing = (-0.015).sp,
        ),
        metadata = TextStyle(
            fontFamily = ContentFace,
            fontWeight = FontWeight.Normal,
            fontSize = 18.sp,
            lineHeight = 24.sp,
        ),
        body = TextStyle(
            fontFamily = ContentFace,
            fontWeight = FontWeight.Normal,
            fontSize = 22.sp,
            lineHeight = 30.sp,
        ),
        label = TextStyle(
            fontFamily = SystemFace,
            fontWeight = FontWeight.Normal,
            fontSize = 14.sp,
            lineHeight = 18.sp,
            letterSpacing = 1.0.sp,
        ),
        button = TextStyle(
            fontFamily = SystemFace,
            fontWeight = FontWeight.Medium,
            fontSize = 16.sp,
            lineHeight = 20.sp,
            letterSpacing = 0.8.sp,
        ),
        guideTime = TextStyle(
            fontFamily = SystemFace,
            fontWeight = FontWeight.Medium,
            fontSize = 20.sp,
            lineHeight = 24.sp,
            fontFeatureSettings = "tnum",
            letterSpacing = 0.4.sp,
        ),
    )
}

val LocalPrimerTypography = staticCompositionLocalOf {
    primerTypography(FormFactor.TABLET)
}
