package com.aleksclark.primer.tv.app.ui.player

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.unit.dp
import com.aleksclark.primer.tv.app.ui.components.LiveBadge
import com.aleksclark.primer.tv.app.ui.designsystem.PrimerTheme
import com.aleksclark.primer.tv.app.ui.designsystem.TvFocusSurface
import com.aleksclark.primer.tv.core.presentation.PlaybackMessageModel
import com.aleksclark.primer.tv.core.presentation.PlaybackOverlayKind
import com.aleksclark.primer.tv.core.presentation.PlaybackOverlayModel

/**
 * Policy-driven chrome layered above the video surface.
 *
 * Transport buttons themselves come from Media3's PlayerView when
 * [PlaybackOverlayModel.showTransportControls] is true. This overlay adds the
 * LIVE/title treatment for programmed playback and keeps a dim idle edge so
 * the student can still read the title without interactive seek chrome.
 *
 * Pause/seek are still enforced by PolicyPlayer — hiding UI is not the control.
 */
@Composable
fun PlaybackChromeOverlay(
    overlay: PlaybackOverlayModel,
    modifier: Modifier = Modifier,
) {
    when (overlay.kind) {
        PlaybackOverlayKind.LIVE_MINIMAL -> LiveMinimalOverlay(
            title = overlay.title,
            modifier = modifier,
        )
        PlaybackOverlayKind.FULL_TRANSPORT,
        PlaybackOverlayKind.PAUSE_PROGRESS,
        -> {
            // Media3 draws transport; optional top edge title for context.
            if (overlay.title.isNotBlank()) {
                Box(
                    modifier = modifier
                        .fillMaxSize()
                        .padding(PrimerTheme.spacing.screenHorizontal, PrimerTheme.spacing.screenVertical),
                ) {
                    Text(
                        text = overlay.title,
                        style = PrimerTheme.typography.metadata,
                        color = PrimerTheme.colors.onSurface.copy(alpha = 0.85f),
                        modifier = Modifier.align(Alignment.TopStart),
                    )
                }
            }
        }
    }
}

@Composable
private fun LiveMinimalOverlay(
    title: String,
    modifier: Modifier = Modifier,
) {
    val colors = PrimerTheme.colors
    val spacing = PrimerTheme.spacing
    Box(modifier = modifier.fillMaxSize()) {
        // Soft top gradient so LIVE + title stay readable on bright frames.
        Box(
            modifier = Modifier
                .fillMaxWidth()
                .height(120.dp)
                .align(Alignment.TopCenter)
                .background(
                    Brush.verticalGradient(
                        colors = listOf(colors.scrimStrong, colors.scrimSoft.copy(alpha = 0f)),
                    ),
                ),
        )
        Row(
            modifier = Modifier
                .align(Alignment.TopStart)
                .padding(horizontal = spacing.screenHorizontal, vertical = spacing.screenVertical),
            horizontalArrangement = Arrangement.spacedBy(spacing.md),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            LiveBadge()
            if (title.isNotBlank()) {
                Text(
                    text = title,
                    style = PrimerTheme.typography.railTitle,
                    color = colors.onSurface,
                    maxLines = 1,
                )
            }
        }
    }
}

/**
 * Full-screen player message / error surface. Replaces the former
 * full-page text utility layout for grant failures, finished sessions,
 * and start-up waits.
 */
@Composable
fun PlaybackErrorOverlay(
    message: PlaybackMessageModel,
    onPrimary: () -> Unit,
    modifier: Modifier = Modifier,
    onSecondary: (() -> Unit)? = null,
    secondaryLabel: String = "Back",
) {
    val colors = PrimerTheme.colors
    val spacing = PrimerTheme.spacing

    Box(
        modifier = modifier
            .fillMaxSize()
            .background(colors.background),
        contentAlignment = Alignment.Center,
    ) {
        Column(
            modifier = Modifier
                .widthIn(max = 560.dp)
                .padding(spacing.lg),
            verticalArrangement = Arrangement.spacedBy(spacing.md),
            horizontalAlignment = Alignment.Start,
        ) {
            Text(
                text = message.title,
                style = PrimerTheme.typography.screenTitle,
                color = if (message.isError) colors.error else colors.onSurface,
            )
            Text(
                text = message.body,
                style = PrimerTheme.typography.body,
                color = colors.onSurfaceMuted,
            )
            Spacer(Modifier.height(spacing.sm))
            Row(horizontalArrangement = Arrangement.spacedBy(spacing.md)) {
                OverlayActionButton(
                    label = message.primaryLabel,
                    emphasized = true,
                    onClick = onPrimary,
                    requestFocus = true,
                )
                if (message.canRetry && onSecondary != null) {
                    OverlayActionButton(
                        label = secondaryLabel,
                        emphasized = false,
                        onClick = onSecondary,
                        requestFocus = false,
                    )
                }
            }
        }
    }
}

/**
 * Finished-session treatment — same visual language as the error overlay but
 * without error coloring.
 */
@Composable
fun PlaybackFinishedOverlay(
    message: PlaybackMessageModel,
    onBack: () -> Unit,
    modifier: Modifier = Modifier,
) {
    PlaybackErrorOverlay(
        message = message,
        onPrimary = onBack,
        modifier = modifier,
    )
}

@Composable
private fun OverlayActionButton(
    label: String,
    emphasized: Boolean,
    onClick: () -> Unit,
    requestFocus: Boolean,
) {
    val colors = PrimerTheme.colors
    TvFocusSurface(
        onClick = onClick,
        requestFocus = requestFocus,
        shape = PrimerTheme.shapes.button,
        modifier = Modifier.widthIn(min = 140.dp),
    ) { focused ->
        Box(
            modifier = Modifier
                .background(
                    color = when {
                        focused -> colors.focusBorder
                        emphasized -> colors.brand
                        else -> colors.surface
                    },
                    shape = PrimerTheme.shapes.button,
                )
                .padding(horizontal = PrimerTheme.spacing.lg, vertical = PrimerTheme.spacing.sm),
            contentAlignment = Alignment.Center,
        ) {
            Text(
                text = label,
                style = PrimerTheme.typography.button,
                color = if (focused || emphasized) colors.onBrand else colors.onSurface,
            )
        }
    }
}
