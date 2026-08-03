package com.aleksclark.primer.tv.app.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.aleksclark.primer.tv.app.ui.designsystem.PrimerTheme
import com.aleksclark.primer.tv.app.ui.designsystem.TvFocusSurface
import com.aleksclark.primer.tv.core.presentation.MediaLabels
import com.aleksclark.primer.tv.core.presentation.OnNowHeroModel

/**
 * Channel-specific hero: on-air with Watch Live, gap/up-next, loading skeleton,
 * or error. Does not fetch — the screen supplies [OnNowHeroModel].
 */
@Composable
fun OnNowHero(
    model: OnNowHeroModel,
    baseUrl: String,
    onWatchLive: () -> Unit,
    modifier: Modifier = Modifier,
    requestWatchLiveFocus: Boolean = false,
) {
    when (model) {
        OnNowHeroModel.Loading -> OnNowHeroSkeleton(modifier = modifier)
        is OnNowHeroModel.OnAir -> OnAirHeroContent(
            model = model,
            baseUrl = baseUrl,
            onWatchLive = onWatchLive,
            requestWatchLiveFocus = requestWatchLiveFocus,
            modifier = modifier,
        )
        is OnNowHeroModel.Gap -> GapHeroContent(
            model = model,
            baseUrl = baseUrl,
            modifier = modifier,
        )
        is OnNowHeroModel.Error -> ErrorHeroContent(
            message = model.message.value,
            modifier = modifier,
        )
    }
}

@Composable
fun OnNowHeroSkeleton(modifier: Modifier = Modifier) {
    val colors = PrimerTheme.colors
    val spacing = PrimerTheme.spacing
    Box(
        modifier = modifier
            .fillMaxWidth()
            .aspectRatio(16f / 9f)
            .clip(PrimerTheme.shapes.panel)
            .background(colors.surfaceRaised)
            .padding(spacing.lg),
        contentAlignment = Alignment.BottomStart,
    ) {
        Column(verticalArrangement = Arrangement.spacedBy(spacing.sm)) {
            Box(
                Modifier
                    .widthIn(min = 64.dp)
                    .height(24.dp)
                    .clip(RoundedCornerShape(999.dp))
                    .background(colors.surface),
            )
            Box(
                Modifier
                    .fillMaxWidth(0.55f)
                    .height(28.dp)
                    .clip(RoundedCornerShape(6.dp))
                    .background(colors.surface),
            )
            Box(
                Modifier
                    .fillMaxWidth(0.35f)
                    .height(16.dp)
                    .clip(RoundedCornerShape(6.dp))
                    .background(colors.surface),
            )
            Box(
                Modifier
                    .widthIn(min = 140.dp)
                    .height(40.dp)
                    .clip(PrimerTheme.shapes.button)
                    .background(colors.surface),
            )
        }
    }
}

@Composable
private fun OnAirHeroContent(
    model: OnNowHeroModel.OnAir,
    baseUrl: String,
    onWatchLive: () -> Unit,
    requestWatchLiveFocus: Boolean,
    modifier: Modifier,
) {
    val spacing = PrimerTheme.spacing
    val colors = PrimerTheme.colors
    OnNowFrame(
        baseUrl = baseUrl,
        imagePath = model.imagePath,
        contentDescription = model.title,
        modifier = modifier,
    ) {
        LiveBadge()
        Spacer(Modifier.height(spacing.sm))
        Text(
            text = model.title,
            style = PrimerTheme.typography.heroTitle,
            color = colors.onSurface,
            maxLines = 2,
            overflow = TextOverflow.Ellipsis,
        )
        Spacer(Modifier.height(spacing.sm))
        MediaMetadataRow(
            mediaClass = model.mediaClass,
            runtimeSeconds = model.runtimeSeconds,
            live = true,
            extra = model.remainingLabel,
        )
        model.progressLabel?.let { progress ->
            Spacer(Modifier.height(spacing.xs))
            Text(
                text = progress,
                style = PrimerTheme.typography.metadata,
                color = colors.onSurfaceMuted,
                maxLines = 1,
            )
        }
        Text(
            text = "${model.airsAtLabel} – ${model.endsAtLabel}",
            style = PrimerTheme.typography.metadata,
            color = colors.onSurfaceMuted,
            maxLines = 1,
        )
        if (model.overview.isNotBlank()) {
            Spacer(Modifier.height(spacing.sm))
            Text(
                text = model.overview,
                style = PrimerTheme.typography.body,
                color = colors.onSurfaceMuted,
                maxLines = 3,
                overflow = TextOverflow.Ellipsis,
            )
        }
        model.notPlayableReason?.let { reason ->
            Spacer(Modifier.height(spacing.sm))
            Text(
                text = reason,
                style = PrimerTheme.typography.metadata,
                color = colors.error,
                maxLines = 2,
            )
        }
        Spacer(Modifier.height(spacing.xs))
        Text(
            text = model.joinHint,
            style = PrimerTheme.typography.metadata,
            color = colors.onSurfaceMuted,
            maxLines = 2,
        )
        Spacer(Modifier.height(spacing.md))
        WatchLiveButton(
            enabled = model.tunable,
            joinInProgress = model.joinInProgress,
            onClick = onWatchLive,
            requestFocus = requestWatchLiveFocus,
        )
        model.next?.let { next ->
            Spacer(Modifier.height(spacing.sm))
            Text(
                text = "Up next: ${next.title} · ${next.startsInLabel}",
                style = PrimerTheme.typography.metadata,
                color = colors.onSurfaceMuted,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
        }
    }
}

@Composable
private fun GapHeroContent(
    model: OnNowHeroModel.Gap,
    baseUrl: String,
    modifier: Modifier,
) {
    val spacing = PrimerTheme.spacing
    val colors = PrimerTheme.colors
    OnNowFrame(
        baseUrl = baseUrl,
        imagePath = model.next?.imagePath,
        contentDescription = model.next?.title,
        modifier = modifier,
    ) {
        Text(
            text = model.message.value,
            style = PrimerTheme.typography.heroTitle,
            color = colors.onSurface,
            maxLines = 2,
            overflow = TextOverflow.Ellipsis,
        )
        model.next?.let { next ->
            Spacer(Modifier.height(spacing.sm))
            Text(
                text = "Next: ${next.title}",
                style = PrimerTheme.typography.body,
                color = colors.onSurface,
                maxLines = 2,
                overflow = TextOverflow.Ellipsis,
            )
            Spacer(Modifier.height(spacing.xs))
            Text(
                text = next.startsInLabel,
                style = PrimerTheme.typography.metadata,
                color = colors.onSurfaceMuted,
                maxLines = 1,
            )
        }
        Spacer(Modifier.height(spacing.md))
        Text(
            text = "Watch Live is unavailable until the next programme starts.",
            style = PrimerTheme.typography.metadata,
            color = colors.onSurfaceMuted,
            maxLines = 2,
        )
    }
}

@Composable
private fun ErrorHeroContent(
    message: String,
    modifier: Modifier,
) {
    val spacing = PrimerTheme.spacing
    val colors = PrimerTheme.colors
    Box(
        modifier = modifier
            .fillMaxWidth()
            .aspectRatio(16f / 9f)
            .clip(PrimerTheme.shapes.panel)
            .background(colors.surfaceRaised)
            .padding(spacing.lg),
        contentAlignment = Alignment.CenterStart,
    ) {
        Text(
            text = message,
            style = PrimerTheme.typography.body,
            color = colors.error,
        )
    }
}

@Composable
private fun OnNowFrame(
    baseUrl: String,
    imagePath: String?,
    contentDescription: String?,
    modifier: Modifier = Modifier,
    content: @Composable () -> Unit,
) {
    val spacing = PrimerTheme.spacing
    Box(
        modifier = modifier
            .fillMaxWidth()
            .clip(PrimerTheme.shapes.panel),
    ) {
        AsyncMediaArtwork(
            baseUrl = baseUrl,
            imagePath = imagePath,
            contentDescription = contentDescription,
            kind = ArtworkKind.BACKDROP,
            shape = PrimerTheme.shapes.panel,
            showScrim = true,
            modifier = Modifier.fillMaxWidth(),
        )
        Column(
            modifier = Modifier
                .align(Alignment.BottomStart)
                .fillMaxWidth()
                .padding(spacing.lg)
                .widthIn(max = 720.dp),
        ) {
            content()
        }
    }
}

@Composable
private fun WatchLiveButton(
    enabled: Boolean,
    joinInProgress: Boolean,
    onClick: () -> Unit,
    requestFocus: Boolean,
) {
    val colors = PrimerTheme.colors
    val label = if (joinInProgress) {
        "${MediaLabels.WATCH_LIVE} — joins in progress"
    } else {
        MediaLabels.WATCH_LIVE
    }
    TvFocusSurface(
        onClick = onClick,
        enabled = enabled,
        requestFocus = requestFocus,
        shape = PrimerTheme.shapes.button,
        modifier = Modifier.widthIn(min = 160.dp),
    ) { focused ->
        Box(
            modifier = Modifier
                .fillMaxWidth()
                .background(
                    color = when {
                        !enabled -> colors.surface
                        focused -> colors.focusBorder
                        else -> colors.brand
                    },
                    shape = PrimerTheme.shapes.button,
                )
                .padding(horizontal = PrimerTheme.spacing.lg, vertical = PrimerTheme.spacing.sm),
            contentAlignment = Alignment.Center,
        ) {
            Text(
                text = label,
                style = PrimerTheme.typography.button,
                color = if (enabled) colors.onBrand else colors.onSurfaceMuted,
            )
        }
    }
}


