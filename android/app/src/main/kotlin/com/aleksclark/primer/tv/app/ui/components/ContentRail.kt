package com.aleksclark.primer.tv.app.ui.components

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyListState
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import com.aleksclark.primer.tv.app.ui.designsystem.PrimerTheme
import com.aleksclark.primer.tv.core.presentation.MediaCardModel
import com.aleksclark.primer.tv.core.presentation.MediaId
import com.aleksclark.primer.tv.core.presentation.RailId
import com.aleksclark.primer.tv.core.presentation.RailModel

/**
 * Titled horizontal media row with optional skeleton placeholders and stable
 * [LazyListState] ownership for refresh/focus preservation.
 */
@Composable
fun ContentRail(
    rail: RailModel,
    baseUrl: String,
    onSelect: (MediaId) -> Unit,
    modifier: Modifier = Modifier,
    state: LazyListState = rememberLazyListState(),
    skeletonCount: Int = 0,
    focusedMediaId: String? = null,
    contentPadding: PaddingValues = PaddingValues(horizontal = PrimerTheme.spacing.screenHorizontal),
) {
    ContentRail(
        id = rail.id,
        title = rail.title,
        items = rail.items,
        baseUrl = baseUrl,
        onSelect = onSelect,
        modifier = modifier,
        state = state,
        skeletonCount = skeletonCount,
        focusedMediaId = focusedMediaId,
        contentPadding = contentPadding,
    )
}

@Composable
fun ContentRail(
    id: RailId,
    title: String,
    items: List<MediaCardModel>,
    baseUrl: String,
    onSelect: (MediaId) -> Unit,
    modifier: Modifier = Modifier,
    state: LazyListState = rememberLazyListState(),
    skeletonCount: Int = 0,
    focusedMediaId: String? = null,
    contentPadding: PaddingValues = PaddingValues(horizontal = PrimerTheme.spacing.screenHorizontal),
) {
    val spacing = PrimerTheme.spacing
    val colors = PrimerTheme.colors

    Column(
        modifier = modifier.fillMaxWidth(),
        verticalArrangement = Arrangement.spacedBy(spacing.sm),
    ) {
        Text(
            text = title,
            style = PrimerTheme.typography.railTitle,
            color = colors.onSurface,
            modifier = Modifier.padding(horizontal = PrimerTheme.spacing.screenHorizontal),
        )

        LazyRow(
            state = state,
            contentPadding = contentPadding,
            horizontalArrangement = Arrangement.spacedBy(spacing.railGutter),
            // Key by rail so focus restoration can target a stable list identity.
            modifier = Modifier.fillMaxWidth(),
        ) {
            if (items.isEmpty() && skeletonCount > 0) {
                items(skeletonCount, key = { "skeleton-$id-$it" }) {
                    MediaPosterCardSkeleton()
                }
            } else {
                items(items, key = { it.mediaItemId }) { card ->
                    MediaPosterCard(
                        model = card,
                        baseUrl = baseUrl,
                        onClick = { onSelect(card.id) },
                        requestFocus = focusedMediaId != null && card.mediaItemId == focusedMediaId,
                    )
                }
            }
        }
    }
}
