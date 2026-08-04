package com.aleksclark.primer.tv.app.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.border
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.aleksclark.primer.tv.app.ui.designsystem.PrimerTheme
import com.aleksclark.primer.tv.app.ui.designsystem.TvFocusSurface
import com.aleksclark.primer.tv.core.domain.FormFactor
import com.aleksclark.primer.tv.core.presentation.MediaDetailModel

/**
 * Adaptive media detail surface.
 *
 * - Compact (phone portrait / narrow tablet): vertical scroll —
 *   backdrop/poster → title/meta/actions → synopsis.
 * - Expanded (wide tablet / TV): layered backdrop with scrim and content pane;
 *   TV requests initial focus on the primary action.
 */
@Composable
fun MediaDetailPane(
    detail: MediaDetailModel,
    baseUrl: String,
    onPlay: () -> Unit,
    onBack: () -> Unit,
    modifier: Modifier = Modifier,
    showBackButton: Boolean = true,
) {
    BoxWithConstraints(modifier = modifier.fillMaxSize()) {
        val wide = maxWidth >= 720.dp || PrimerTheme.formFactor == FormFactor.TELEVISION
        if (wide) {
            WideDetailLayout(
                detail = detail,
                baseUrl = baseUrl,
                onPlay = onPlay,
                onBack = onBack,
                showBackButton = showBackButton,
            )
        } else {
            CompactDetailLayout(
                detail = detail,
                baseUrl = baseUrl,
                onPlay = onPlay,
                onBack = onBack,
                showBackButton = showBackButton,
            )
        }
    }
}

@Composable
private fun CompactDetailLayout(
    detail: MediaDetailModel,
    baseUrl: String,
    onPlay: () -> Unit,
    onBack: () -> Unit,
    showBackButton: Boolean,
) {
    val spacing = PrimerTheme.spacing
    val scroll = rememberScrollState()
    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(scroll)
            .padding(horizontal = spacing.screenHorizontal, vertical = spacing.screenVertical),
        verticalArrangement = Arrangement.spacedBy(spacing.md),
    ) {
        AsyncMediaArtwork(
            baseUrl = baseUrl,
            imagePath = detail.backdropOrPosterPath,
            contentDescription = detail.title,
            kind = ArtworkKind.BACKDROP,
            shape = PrimerTheme.shapes.panel,
            showScrim = true,
            modifier = Modifier.fillMaxWidth(),
        )

        DetailIdentity(detail = detail)

        DetailActions(
            detail = detail,
            onPlay = onPlay,
            onBack = onBack,
            showBackButton = showBackButton,
            requestPrimaryFocus = false,
        )

        detail.oneViewingWarning?.let { warning ->
            OneViewingCallout(text = warning, watched = detail.watched)
        }

        if (detail.overview.isNotBlank()) {
            Text(
                text = detail.overview,
                style = PrimerTheme.typography.body,
                color = PrimerTheme.colors.onSurface,
            )
        }

        SubjectChipRow(tags = detail.subjectTags)
        Spacer(Modifier.height(spacing.lg))
    }
}

@Composable
private fun WideDetailLayout(
    detail: MediaDetailModel,
    baseUrl: String,
    onPlay: () -> Unit,
    onBack: () -> Unit,
    showBackButton: Boolean,
) {
    val spacing = PrimerTheme.spacing
    val colors = PrimerTheme.colors
    val isTv = PrimerTheme.formFactor == FormFactor.TELEVISION
    val scroll = rememberScrollState()

    Box(modifier = Modifier.fillMaxSize()) {
        // Full-bleed backdrop with readability scrim.
        AsyncMediaArtwork(
            baseUrl = baseUrl,
            imagePath = detail.backdropOrPosterPath,
            contentDescription = detail.title,
            kind = ArtworkKind.BACKDROP,
            shape = PrimerTheme.shapes.panel,
            showScrim = false,
            modifier = Modifier
                .fillMaxSize()
                .align(Alignment.Center),
        )
        Box(
            modifier = Modifier
                .fillMaxSize()
                .background(
                    Brush.horizontalGradient(
                        colors = listOf(
                            colors.scrimStrong,
                            colors.scrimStrong.copy(alpha = 0.85f),
                            colors.scrimSoft.copy(alpha = 0.45f),
                        ),
                    ),
                ),
        )
        Box(
            modifier = Modifier
                .fillMaxSize()
                .background(
                    Brush.verticalGradient(
                        colors = listOf(
                            colors.scrimSoft.copy(alpha = 0.2f),
                            colors.scrimStrong,
                        ),
                    ),
                ),
        )

        Row(
            modifier = Modifier
                .fillMaxSize()
                .padding(horizontal = spacing.screenHorizontal, vertical = spacing.screenVertical),
            horizontalArrangement = Arrangement.spacedBy(spacing.xl),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            // Poster keeps identity when the backdrop is a poster-crop fallback.
            AsyncMediaArtwork(
                baseUrl = baseUrl,
                imagePath = detail.imagePath,
                contentDescription = detail.title,
                kind = ArtworkKind.POSTER,
                shape = PrimerTheme.shapes.mediaCard,
                showScrim = false,
                modifier = Modifier
                    .widthIn(max = if (isTv) 280.dp else 220.dp)
                    .fillMaxWidth(0.28f)
                    .align(Alignment.CenterVertically),
            )

            Column(
                modifier = Modifier
                    .weight(1f)
                    .fillMaxHeight()
                    .verticalScroll(scroll)
                    .padding(vertical = spacing.md),
                verticalArrangement = Arrangement.spacedBy(spacing.md, Alignment.CenterVertically),
            ) {
                DetailIdentity(detail = detail)

                DetailActions(
                    detail = detail,
                    onPlay = onPlay,
                    onBack = onBack,
                    showBackButton = showBackButton,
                    requestPrimaryFocus = isTv,
                )

                detail.oneViewingWarning?.let { warning ->
                    OneViewingCallout(text = warning, watched = detail.watched)
                }

                if (detail.overview.isNotBlank()) {
                    Text(
                        text = detail.overview,
                        style = PrimerTheme.typography.body,
                        color = PrimerTheme.colors.onSurface,
                        modifier = Modifier.widthIn(max = 720.dp),
                    )
                }

                SubjectChipRow(tags = detail.subjectTags)
            }
        }
    }
}

@Composable
private fun DetailIdentity(detail: MediaDetailModel) {
    val spacing = PrimerTheme.spacing
    Column(verticalArrangement = Arrangement.spacedBy(spacing.sm)) {
        if (detail.oneViewing || detail.watched) {
            OneViewingBadge(watched = detail.watched)
        }
        Text(
            text = detail.title,
            style = PrimerTheme.typography.heroTitle,
            color = PrimerTheme.colors.onSurface,
            maxLines = 3,
            overflow = TextOverflow.Ellipsis,
        )
        MediaMetadataRow(
            mediaClass = detail.mediaClass,
            runtimeSeconds = detail.runtimeSeconds,
            oneViewing = detail.oneViewing && !detail.watched,
            watched = detail.watched,
            leavingSoon = detail.leavingSoon,
        )
    }
}

@Composable
private fun DetailActions(
    detail: MediaDetailModel,
    onPlay: () -> Unit,
    onBack: () -> Unit,
    showBackButton: Boolean,
    requestPrimaryFocus: Boolean,
) {
    val spacing = PrimerTheme.spacing
    // When Play is disabled (Watched), TV still needs an initial focus target
    // so D-pad landing is never a blank surface.
    val focusPrimary = requestPrimaryFocus && detail.primaryEnabled
    val focusBack = requestPrimaryFocus && !detail.primaryEnabled && showBackButton
    Row(
        horizontalArrangement = Arrangement.spacedBy(spacing.md),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        PrimaryDetailButton(
            label = detail.primaryActionLabel,
            enabled = detail.primaryEnabled,
            onClick = onPlay,
            requestFocus = focusPrimary,
        )
        if (showBackButton) {
            SecondaryDetailButton(
                label = "Back",
                onClick = onBack,
                requestFocus = focusBack,
            )
        }
    }
}

@Composable
private fun PrimaryDetailButton(
    label: String,
    enabled: Boolean,
    onClick: () -> Unit,
    requestFocus: Boolean,
) {
    val colors = PrimerTheme.colors
    TvFocusSurface(
        onClick = onClick,
        enabled = enabled,
        requestFocus = requestFocus,
        shape = PrimerTheme.shapes.button,
        modifier = Modifier.widthIn(min = 160.dp),
    ) { focused ->
        Box(
            modifier = Modifier
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

@Composable
private fun SecondaryDetailButton(
    label: String,
    onClick: () -> Unit,
    requestFocus: Boolean = false,
) {
    val colors = PrimerTheme.colors
    val spacing = PrimerTheme.spacing
    TvFocusSurface(
        onClick = onClick,
        requestFocus = requestFocus,
        shape = PrimerTheme.shapes.button,
        modifier = Modifier.widthIn(min = 120.dp),
    ) { focused ->
        Box(
            modifier = Modifier
                .background(
                    color = if (focused) colors.surfaceRaised else Color.Transparent,
                    shape = PrimerTheme.shapes.button,
                )
                .border(
                    width = spacing.ruleWidth,
                    color = if (focused) colors.onSurfaceMuted else colors.outline,
                    shape = PrimerTheme.shapes.button,
                )
                .padding(horizontal = PrimerTheme.spacing.lg, vertical = PrimerTheme.spacing.sm),
            contentAlignment = Alignment.Center,
        ) {
            Text(
                text = label.uppercase(),
                style = PrimerTheme.typography.button,
                color = colors.onSurface,
            )
        }
    }
}

@Composable
private fun OneViewingCallout(text: String, watched: Boolean) {
    val colors = PrimerTheme.colors
    val spacing = PrimerTheme.spacing
    // Watched is muted; active one-viewing uses attention (System C review/error role).
    val accent = if (watched) colors.onSurfaceMuted else colors.error
    Surface(
        color = Color.Transparent,
        contentColor = accent,
        shape = PrimerTheme.shapes.panel,
        shadowElevation = 0.dp,
        tonalElevation = 0.dp,
        modifier = Modifier.border(
            width = spacing.ruleWidth,
            color = accent,
            shape = PrimerTheme.shapes.panel,
        ),
    ) {
        Text(
            text = text,
            style = PrimerTheme.typography.body,
            color = accent,
            modifier = Modifier.padding(
                horizontal = PrimerTheme.spacing.md,
                vertical = PrimerTheme.spacing.sm,
            ),
        )
    }
}

@Composable
private fun SubjectChipRow(tags: List<String>) {
    if (tags.isEmpty()) return
    val colors = PrimerTheme.colors
    val spacing = PrimerTheme.spacing
    val scroll = rememberScrollState()
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .horizontalScroll(scroll),
        horizontalArrangement = Arrangement.spacedBy(spacing.sm),
    ) {
        tags.forEach { tag ->
            Surface(
                color = Color.Transparent,
                contentColor = colors.onSurfaceMuted,
                shape = PrimerTheme.shapes.chip,
                shadowElevation = 0.dp,
                tonalElevation = 0.dp,
                modifier = Modifier.border(
                    width = spacing.ruleWidth,
                    color = colors.outline,
                    shape = PrimerTheme.shapes.chip,
                ),
            ) {
                Text(
                    text = tag.uppercase(),
                    style = PrimerTheme.typography.label,
                    modifier = Modifier.padding(horizontal = 12.dp, vertical = 6.dp),
                    maxLines = 1,
                )
            }
        }
    }
}

/**
 * Empty / unavailable detail surface when the catalog no longer has the item.
 */
@Composable
fun MediaDetailUnavailable(
    onBack: () -> Unit,
    modifier: Modifier = Modifier,
    showBackButton: Boolean = true,
) {
    val spacing = PrimerTheme.spacing
    Column(
        modifier = modifier
            .fillMaxSize()
            .padding(horizontal = spacing.screenHorizontal, vertical = spacing.screenVertical),
        verticalArrangement = Arrangement.spacedBy(spacing.md, Alignment.CenterVertically),
        horizontalAlignment = Alignment.Start,
    ) {
        Text(
            text = "That title is no longer available.",
            style = PrimerTheme.typography.body,
            color = PrimerTheme.colors.onSurfaceMuted,
        )
        if (showBackButton) {
            SecondaryDetailButton(label = "Back", onClick = onBack)
        }
    }
}

