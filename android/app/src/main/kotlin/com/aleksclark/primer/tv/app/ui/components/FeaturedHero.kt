package com.aleksclark.primer.tv.app.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.border
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.aleksclark.primer.tv.app.ui.designsystem.PrimerTheme
import com.aleksclark.primer.tv.app.ui.designsystem.TvFocusSurface
import com.aleksclark.primer.tv.core.presentation.HeroAction
import com.aleksclark.primer.tv.core.presentation.HeroModel
import com.aleksclark.primer.tv.core.presentation.MediaLabels

/**
 * Home featured / on-now surface. Variants: live, curated catalog, empty/gap,
 * and loading skeleton. Does not fetch — the screen supplies [HeroModel].
 */
@Composable
fun FeaturedHero(
    hero: HeroModel,
    baseUrl: String,
    onPrimaryAction: () -> Unit,
    modifier: Modifier = Modifier,
) {
    when (hero) {
        HeroModel.Loading -> FeaturedHeroSkeleton(modifier = modifier)
        is HeroModel.Live -> LiveHeroContent(
            hero = hero,
            baseUrl = baseUrl,
            onPrimaryAction = onPrimaryAction,
            modifier = modifier,
        )
        is HeroModel.Featured -> FeaturedCatalogHero(
            hero = hero,
            baseUrl = baseUrl,
            onPrimaryAction = onPrimaryAction,
            modifier = modifier,
        )
        is HeroModel.Empty -> EmptyHeroContent(
            hero = hero,
            baseUrl = baseUrl,
            modifier = modifier,
        )
    }
}

@Composable
private fun LiveHeroContent(
    hero: HeroModel.Live,
    baseUrl: String,
    onPrimaryAction: () -> Unit,
    modifier: Modifier,
) {
    HeroFrame(
        baseUrl = baseUrl,
        imagePath = hero.imagePath,
        contentDescription = hero.title,
        modifier = modifier,
    ) {
        LiveBadge()
        Spacer(Modifier.height(PrimerTheme.spacing.sm))
        Text(
            text = hero.title,
            style = PrimerTheme.typography.heroTitle,
            color = PrimerTheme.colors.onSurface,
            maxLines = 2,
            overflow = TextOverflow.Ellipsis,
        )
        Spacer(Modifier.height(PrimerTheme.spacing.sm))
        MediaMetadataRow(
            mediaClass = hero.mediaClass,
            runtimeSeconds = 0,
            live = true,
            extra = hero.remainingLabel,
        )
        if (hero.overview.isNotBlank()) {
            Spacer(Modifier.height(PrimerTheme.spacing.sm))
            Text(
                text = hero.overview,
                style = PrimerTheme.typography.body,
                color = PrimerTheme.colors.onSurfaceMuted,
                maxLines = 3,
                overflow = TextOverflow.Ellipsis,
            )
        }
        Spacer(Modifier.height(PrimerTheme.spacing.md))
        HeroPrimaryButton(
            action = hero.primaryAction,
            enabled = hero.tunable,
            labelOverride = if (hero.joinInProgress) "${MediaLabels.WATCH_LIVE} — joins in progress" else null,
            onClick = onPrimaryAction,
        )
    }
}

@Composable
private fun FeaturedCatalogHero(
    hero: HeroModel.Featured,
    baseUrl: String,
    onPrimaryAction: () -> Unit,
    modifier: Modifier,
) {
    val card = hero.card
    HeroFrame(
        baseUrl = baseUrl,
        imagePath = card.imagePath,
        contentDescription = card.title,
        modifier = modifier,
    ) {
        Text(
            text = card.title,
            style = PrimerTheme.typography.heroTitle,
            color = PrimerTheme.colors.onSurface,
            maxLines = 2,
            overflow = TextOverflow.Ellipsis,
        )
        Spacer(Modifier.height(PrimerTheme.spacing.sm))
        MediaMetadataRow(
            mediaClass = card.mediaClass,
            runtimeSeconds = card.runtimeSeconds,
            oneViewing = card.oneViewing,
            watched = card.watched,
            leavingSoon = card.leavingSoon,
        )
        if (card.overview.isNotBlank()) {
            Spacer(Modifier.height(PrimerTheme.spacing.sm))
            Text(
                text = card.overview,
                style = PrimerTheme.typography.body,
                color = PrimerTheme.colors.onSurfaceMuted,
                maxLines = 3,
                overflow = TextOverflow.Ellipsis,
            )
        }
        hero.next?.let { next ->
            Spacer(Modifier.height(PrimerTheme.spacing.sm))
            Text(
                text = "Up next on the channel: ${next.title} · ${next.startsInLabel}",
                style = PrimerTheme.typography.metadata,
                color = PrimerTheme.colors.onSurfaceMuted,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
        }
        Spacer(Modifier.height(PrimerTheme.spacing.md))
        HeroPrimaryButton(
            action = hero.primaryAction,
            enabled = hero.primaryAction != HeroAction.NONE && (card.playable || hero.primaryAction == HeroAction.VIEW_DETAILS),
            onClick = onPrimaryAction,
        )
    }
}

@Composable
private fun EmptyHeroContent(
    hero: HeroModel.Empty,
    baseUrl: String,
    modifier: Modifier,
) {
    HeroFrame(
        baseUrl = baseUrl,
        imagePath = hero.imagePath,
        contentDescription = hero.title,
        modifier = modifier,
    ) {
        Text(
            text = hero.message.value,
            style = PrimerTheme.typography.heroTitle,
            color = PrimerTheme.colors.onSurface,
            maxLines = 2,
            overflow = TextOverflow.Ellipsis,
        )
        hero.next?.let { next ->
            Spacer(Modifier.height(PrimerTheme.spacing.sm))
            Text(
                text = "Next: ${next.title} · ${next.startsInLabel}",
                style = PrimerTheme.typography.body,
                color = PrimerTheme.colors.onSurfaceMuted,
                maxLines = 2,
                overflow = TextOverflow.Ellipsis,
            )
        }
    }
}

@Composable
fun FeaturedHeroSkeleton(modifier: Modifier = Modifier) {
    val colors = PrimerTheme.colors
    val spacing = PrimerTheme.spacing
    Box(
        modifier = modifier
            .fillMaxWidth()
            .aspectRatio(16f / 9f)
            .clip(PrimerTheme.shapes.panel)
            .background(colors.surfaceRaised)
            .border(width = spacing.ruleWidth, color = colors.outline, shape = PrimerTheme.shapes.panel)
            .padding(spacing.lg),
        contentAlignment = Alignment.BottomStart,
    ) {
        Column(verticalArrangement = Arrangement.spacedBy(spacing.sm)) {
            Box(
                Modifier
                    .fillMaxWidth(0.55f)
                    .height(28.dp)
                    .clip(PrimerTheme.shapes.mediaCard)
                    .background(colors.surface),
            )
            Box(
                Modifier
                    .fillMaxWidth(0.35f)
                    .height(16.dp)
                    .clip(PrimerTheme.shapes.mediaCard)
                    .background(colors.surface),
            )
            Box(
                Modifier
                    .widthIn(min = 120.dp)
                    .height(40.dp)
                    .clip(PrimerTheme.shapes.button)
                    .background(colors.surface),
            )
        }
    }
}

@Composable
private fun HeroFrame(
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
private fun HeroPrimaryButton(
    action: HeroAction,
    enabled: Boolean,
    onClick: () -> Unit,
    labelOverride: String? = null,
) {
    val label = labelOverride ?: when (action) {
        HeroAction.WATCH_LIVE -> MediaLabels.WATCH_LIVE
        HeroAction.PLAY -> MediaLabels.PLAY
        HeroAction.VIEW_DETAILS -> MediaLabels.VIEW_DETAILS
        HeroAction.NONE -> return
    }
    val colors = PrimerTheme.colors

    // TvFocusSurface provides the focusable surface; content is visual only so
    // phone and TV share one click target without nested clickables.
    TvFocusSurface(
        onClick = onClick,
        enabled = enabled,
        shape = PrimerTheme.shapes.button,
        modifier = Modifier.widthIn(min = 160.dp),
    ) { focused ->
        Box(
            modifier = Modifier
                .fillMaxWidth()
                .background(
                    color = when {
                        !enabled -> colors.surface
                        focused -> colors.brandHover
                        else -> colors.brand
                    },
                    shape = PrimerTheme.shapes.button,
                )
                .padding(horizontal = PrimerTheme.spacing.lg, vertical = PrimerTheme.spacing.sm),
            contentAlignment = Alignment.Center,
        ) {
            Text(
                text = label.uppercase(),
                style = PrimerTheme.typography.button,
                color = if (enabled) colors.onBrand else colors.outlineStrong,
            )
        }
    }
}
