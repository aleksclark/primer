package com.aleksclark.primer.tv.app.ui.components

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import com.aleksclark.primer.tv.app.ui.designsystem.PrimerTheme
import com.aleksclark.primer.tv.core.domain.MediaClass
import com.aleksclark.primer.tv.core.presentation.MediaLabels

/**
 * Consistent runtime / classification / availability rendering for cards and
 * heroes. Status is always text, never color-only.
 */
@Composable
fun MediaMetadataRow(
    mediaClass: MediaClass,
    runtimeSeconds: Int,
    modifier: Modifier = Modifier,
    oneViewing: Boolean = false,
    watched: Boolean = false,
    leavingSoon: Boolean = false,
    live: Boolean = false,
    extra: String? = null,
) {
    val colors = PrimerTheme.colors
    val parts = buildList {
        if (live) add(MediaLabels.LIVE)
        add(MediaLabels.classification(mediaClass))
        if (runtimeSeconds > 0) add(MediaLabels.runtime(runtimeSeconds))
        MediaLabels.statusLabels(oneViewing, watched, leavingSoon).forEach(::add)
        if (!extra.isNullOrBlank()) add(extra)
    }

    Row(
        modifier = modifier,
        horizontalArrangement = Arrangement.spacedBy(PrimerTheme.spacing.sm),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        parts.forEachIndexed { index, part ->
            val accent = when (part) {
                MediaLabels.LIVE -> colors.live
                MediaLabels.ONE_VIEWING -> colors.entertainment
                MediaLabels.WATCHED -> colors.onSurfaceMuted
                MediaLabels.LEAVING_SOON -> colors.onSurfaceMuted
                else -> null
            }
            if (accent != null && (part == MediaLabels.LIVE || part == MediaLabels.ONE_VIEWING || part == MediaLabels.WATCHED)) {
                StatusChip(text = part, container = accent.copy(alpha = 0.18f), content = accent)
            } else {
                Text(
                    text = if (index == 0 || accent != null) part else "·  $part",
                    style = PrimerTheme.typography.metadata,
                    color = colors.onSurfaceMuted,
                    maxLines = 1,
                )
            }
        }
    }
}

@Composable
fun OneViewingBadge(
    modifier: Modifier = Modifier,
    watched: Boolean = false,
) {
    val colors = PrimerTheme.colors
    val text = if (watched) MediaLabels.WATCHED else MediaLabels.ONE_VIEWING
    val accent = if (watched) colors.onSurfaceMuted else colors.entertainment
    StatusChip(
        text = text,
        container = accent.copy(alpha = 0.18f),
        content = accent,
        modifier = modifier,
    )
}

@Composable
fun LiveBadge(modifier: Modifier = Modifier) {
    val colors = PrimerTheme.colors
    StatusChip(
        text = MediaLabels.LIVE,
        container = colors.live,
        content = colors.onLive,
        modifier = modifier.semantics {
            contentDescription = "Live"
        },
    )
}

@Composable
private fun StatusChip(
    text: String,
    container: Color,
    content: Color,
    modifier: Modifier = Modifier,
) {
    Surface(
        modifier = modifier,
        color = container,
        contentColor = content,
        shape = RoundedCornerShape(999.dp),
    ) {
        Text(
            text = text,
            style = PrimerTheme.typography.label,
            modifier = Modifier.padding(horizontal = 10.dp, vertical = 4.dp),
            maxLines = 1,
        )
    }
}
