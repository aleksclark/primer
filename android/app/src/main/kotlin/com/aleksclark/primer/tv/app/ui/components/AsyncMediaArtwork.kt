package com.aleksclark.primer.tv.app.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxScope
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Shape
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.text.style.TextAlign
import coil.compose.AsyncImage
import coil.compose.AsyncImagePainter
import com.aleksclark.primer.tv.app.ui.designsystem.PrimerTheme
import com.aleksclark.primer.tv.core.net.resolveImageUrl

enum class ArtworkKind {
    /** 2:3 poster. */
    POSTER,

    /** 16:9 backdrop / hero. */
    BACKDROP,

    /** 16:9 thumbnail. */
    THUMBNAIL,
}

/**
 * Image loading with fixed aspect ratio, placeholder, error fallback, and
 * optional readability scrim. Poster paths are used as backdrop fallbacks when
 * a dedicated backdrop URL is unavailable.
 */
@Composable
fun AsyncMediaArtwork(
    baseUrl: String,
    imagePath: String?,
    contentDescription: String?,
    kind: ArtworkKind,
    modifier: Modifier = Modifier,
    shape: Shape = PrimerTheme.shapes.mediaCard,
    showScrim: Boolean = kind == ArtworkKind.BACKDROP,
    crossfade: Boolean = true,
) {
    val colors = PrimerTheme.colors
    val ratio = when (kind) {
        ArtworkKind.POSTER -> 2f / 3f
        ArtworkKind.BACKDROP, ArtworkKind.THUMBNAIL -> 16f / 9f
    }
    // Backdrop requests fall back to the poster path the API already returns.
    // A dedicated /Backdrop variant is a backend follow-up; crop/scale handles it.
    val model = imagePath
        ?.takeIf { it.isNotBlank() }
        ?.let { resolveImageUrl(baseUrl, it) }

    var painterState by remember(model) { mutableStateOf<AsyncImagePainter.State>(AsyncImagePainter.State.Empty) }
    val showPlaceholder = model == null ||
        painterState is AsyncImagePainter.State.Loading ||
        painterState is AsyncImagePainter.State.Empty ||
        painterState is AsyncImagePainter.State.Error

    Box(
        modifier = modifier
            .fillMaxWidth()
            .aspectRatio(ratio)
            .clip(shape)
            .background(colors.surfaceRaised),
    ) {
        if (model != null) {
            AsyncImage(
                model = model,
                contentDescription = contentDescription,
                contentScale = if (kind == ArtworkKind.BACKDROP) ContentScale.Crop else ContentScale.Crop,
                modifier = Modifier.fillMaxSize(),
                onState = { painterState = it },
            )
        }

        if (showPlaceholder) {
            ArtworkPlaceholder(
                label = contentDescription?.takeIf { it.isNotBlank() }?.take(1)?.uppercase() ?: "·",
            )
        }

        if (showScrim) {
            Box(
                modifier = Modifier
                    .fillMaxSize()
                    .background(
                        Brush.verticalGradient(
                            colors = listOf(
                                colors.scrimSoft.copy(alpha = 0.15f),
                                colors.scrimStrong,
                            ),
                        ),
                    ),
            )
        }
    }
}

@Composable
private fun BoxScope.ArtworkPlaceholder(label: String) {
    val colors = PrimerTheme.colors
    Box(
        modifier = Modifier
            .fillMaxSize()
            .background(colors.surfaceRaised)
            .align(Alignment.Center),
        contentAlignment = Alignment.Center,
    ) {
        Text(
            text = label,
            style = PrimerTheme.typography.heroTitle,
            color = colors.onSurfaceMuted.copy(alpha = 0.35f),
            textAlign = TextAlign.Center,
        )
    }
}
