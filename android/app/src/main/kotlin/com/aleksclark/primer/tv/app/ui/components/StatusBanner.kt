package com.aleksclark.primer.tv.app.ui.components

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.semantics
import com.aleksclark.primer.tv.app.ui.designsystem.PrimerTheme

/**
 * Non-blocking transient status strip for refresh/update messages. Prefer this
 * over replacing whole screens for short-lived feedback.
 */
@Composable
fun StatusBanner(
    message: String?,
    modifier: Modifier = Modifier,
    isError: Boolean = false,
) {
    val colors = PrimerTheme.colors
    val spacing = PrimerTheme.spacing
    AnimatedVisibility(
        visible = !message.isNullOrBlank(),
        enter = fadeIn(),
        exit = fadeOut(),
        modifier = modifier,
    ) {
        if (message.isNullOrBlank()) return@AnimatedVisibility
        Box(
            modifier = Modifier
                .fillMaxWidth()
                .background(if (isError) colors.error.copy(alpha = 0.18f) else colors.surfaceRaised)
                .padding(horizontal = spacing.md, vertical = spacing.sm)
                .semantics {
                    contentDescription = message
                    liveRegion = LiveRegionMode.Polite
                },
            contentAlignment = Alignment.CenterStart,
        ) {
            Text(
                text = message,
                style = PrimerTheme.typography.metadata,
                color = if (isError) colors.error else colors.onSurface,
            )
        }
    }
}
