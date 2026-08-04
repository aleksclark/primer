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
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Shape
import com.aleksclark.primer.tv.core.domain.FormFactor

/**
 * Standard TV focus treatment: scale + accent rule ring + soft accent glow.
 * System C forbids elevation shadows — focus is contrast and outline only.
 * On phone/tablet it behaves as a plain clickable surface without focus chrome.
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
            .then(
                if (showFocusChrome) {
                    Modifier
                        .drawBehind {
                            // Soft accent wash behind the ring (no drop shadow).
                            val glowPad = spacing.focusOffset.toPx()
                            drawRoundRect(
                                color = colors.focusGlow,
                                topLeft = Offset(-glowPad, -glowPad),
                                size = Size(
                                    width = size.width + glowPad * 2,
                                    height = size.height + glowPad * 2,
                                ),
                                // Radius 0 — System C square language.
                                cornerRadius = CornerRadius(0f, 0f),
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
