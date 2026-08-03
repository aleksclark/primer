package com.aleksclark.primer.tv.app.ui.designsystem

import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.interaction.collectIsFocusedAsState
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxScope
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.draw.scale
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.graphics.Shape
import androidx.compose.ui.graphics.graphicsLayer
import com.aleksclark.primer.tv.core.domain.FormFactor

/**
 * Standard TV focus treatment: scale, border, and soft glow. On phone/tablet
 * it behaves as a plain clickable surface without focus chrome.
 *
 * [selected] is exposed to [content] for styling but does not keep focus chrome
 * raised after D-pad focus moves away.
 *
 * When [requestFocus] is true on TV, focus is moved onto this surface once so
 * Details → Home restores the originating card.
 */
@Composable
fun TvFocusSurface(
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    selected: Boolean = false,
    enabled: Boolean = true,
    requestFocus: Boolean = false,
    shape: Shape = PrimerTheme.shapes.mediaCard,
    content: @Composable BoxScope.(focused: Boolean) -> Unit,
) {
    val colors = PrimerTheme.colors
    val spacing = PrimerTheme.spacing
    val formFactor = PrimerTheme.formFactor
    val interactionSource = remember { MutableInteractionSource() }
    val focusRequester = remember { FocusRequester() }
    val focused by interactionSource.collectIsFocusedAsState()
    val showFocusChrome = formFactor == FormFactor.TELEVISION && focused

    val motion = PrimerTheme.motion
    val scale by animateFloatAsState(
        targetValue = if (showFocusChrome) spacing.focusScale else 1f,
        animationSpec = motion.focus,
        label = "tv-focus-scale",
    )
    val elevation by animateFloatAsState(
        targetValue = if (showFocusChrome) 12f else 0f,
        animationSpec = motion.focus,
        label = "tv-focus-elevation",
    )

    // One-shot restore: requestFocus is true only until the caller clears it.
    LaunchedEffect(requestFocus) {
        if (requestFocus && formFactor == FormFactor.TELEVISION) {
            runCatching { focusRequester.requestFocus() }
        }
    }

    Box(
        modifier = modifier
            .focusRequester(focusRequester)
            .scale(scale)
            .graphicsLayer {
                shadowElevation = elevation
                this.shape = shape
                clip = false
            }
            .then(
                if (showFocusChrome) {
                    Modifier
                        .drawBehind {
                            // Glow is drawn before clip so it is not swallowed
                            // by the content shape.
                            val radius = size.minDimension * 0.08f
                            drawRoundRect(
                                color = colors.focusGlow,
                                cornerRadius = androidx.compose.ui.geometry.CornerRadius(radius, radius),
                            )
                        }
                        .border(
                            width = spacing.focusBorderWidth,
                            color = colors.focusBorder,
                            shape = shape,
                        )
                } else {
                    Modifier
                },
            )
            .clip(shape)
            .clickable(
                enabled = enabled,
                interactionSource = interactionSource,
                indication = null,
                onClick = onClick,
            ),
        // clickable already participates in focus; combine real focus with
        // selection so callers can style selected-unfocused states.
        content = { content(focused || selected) },
    )
}
