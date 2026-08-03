package com.aleksclark.primer.tv.app.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.aleksclark.primer.tv.app.ui.designsystem.PrimerTheme
import com.aleksclark.primer.tv.app.ui.designsystem.TvFocusSurface
import com.aleksclark.primer.tv.core.presentation.MediaCardModel

/**
 * Poster card with TV focus chrome, badges, and metadata. Contains no
 * navigation — [onClick] is supplied by the rail/screen.
 */
@Composable
fun MediaPosterCard(
    model: MediaCardModel,
    baseUrl: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    selected: Boolean = false,
    requestFocus: Boolean = false,
) {
    val spacing = PrimerTheme.spacing
    val colors = PrimerTheme.colors
    val typography = PrimerTheme.typography

    TvFocusSurface(
        onClick = onClick,
        selected = selected,
        requestFocus = requestFocus,
        modifier = modifier.width(spacing.cardWidth),
        shape = PrimerTheme.shapes.mediaCard,
    ) { focused ->
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .alpha(if (model.watched) 0.5f else 1f),
            verticalArrangement = Arrangement.spacedBy(spacing.sm),
        ) {
            Box {
                AsyncMediaArtwork(
                    baseUrl = baseUrl,
                    imagePath = model.imagePath,
                    contentDescription = model.title,
                    kind = ArtworkKind.POSTER,
                    modifier = Modifier.fillMaxWidth(),
                )
                if (model.oneViewing || model.watched) {
                    OneViewingBadge(
                        watched = model.watched,
                        modifier = Modifier
                            .align(Alignment.TopStart)
                            .padding(spacing.sm),
                    )
                }
            }
            Text(
                text = model.title,
                style = typography.cardTitle,
                color = colors.onSurface,
                maxLines = if (focused) 3 else 2,
                overflow = TextOverflow.Ellipsis,
            )
            Text(
                text = buildString {
                    append(model.runtimeLabel)
                    model.statusLabel?.let {
                        append(" · ")
                        append(it)
                    }
                },
                style = typography.metadata,
                color = colors.onSurfaceMuted,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
        }
    }
}

@Composable
fun MediaPosterCardSkeleton(
    modifier: Modifier = Modifier,
) {
    val spacing = PrimerTheme.spacing
    val colors = PrimerTheme.colors
    Column(
        modifier = modifier.width(spacing.cardWidth),
        verticalArrangement = Arrangement.spacedBy(spacing.sm),
    ) {
        Box(
            modifier = Modifier
                .fillMaxWidth()
                .aspectRatio(2f / 3f)
                .clip(PrimerTheme.shapes.mediaCard)
                .background(colors.surfaceRaised),
        )
        Box(
            modifier = Modifier
                .fillMaxWidth(0.9f)
                .height(14.dp)
                .clip(RoundedCornerShape(4.dp))
                .background(colors.surfaceRaised),
        )
        Box(
            modifier = Modifier
                .fillMaxWidth(0.55f)
                .height(12.dp)
                .clip(RoundedCornerShape(4.dp))
                .background(colors.surfaceRaised),
        )
    }
}
