package com.aleksclark.primer.tv.app.ui.components

import androidx.compose.foundation.focusGroup
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyListState
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import com.aleksclark.primer.tv.app.ui.designsystem.PrimerTheme
import com.aleksclark.primer.tv.core.presentation.MediaCardModel
import com.aleksclark.primer.tv.core.presentation.MediaId
import com.aleksclark.primer.tv.core.presentation.RailId
import com.aleksclark.primer.tv.core.presentation.RailModel

/**
 * Titled horizontal media row with optional skeleton placeholders and stable
 * [LazyListState] ownership for refresh/focus preservation.
 *
 * D-pad contract:
 * - Left/right moves between cards in this rail.
 * - Up/down leave the rail via [focusUp] / [focusDown] (usually the previous/next row).
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
    firstItemFocusRequester: FocusRequester? = null,
    focusUp: FocusRequester? = null,
    focusDown: FocusRequester? = null,
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
        firstItemFocusRequester = firstItemFocusRequester,
        focusUp = focusUp,
        focusDown = focusDown,
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
    firstItemFocusRequester: FocusRequester? = null,
    focusUp: FocusRequester? = null,
    focusDown: FocusRequester? = null,
) {
    val spacing = PrimerTheme.spacing
    val colors = PrimerTheme.colors
    val itemFocusRequesters = remember(id, items.map { it.mediaItemId }) {
        List(items.size) { index ->
            if (index == 0 && firstItemFocusRequester != null) firstItemFocusRequester
            else FocusRequester()
        }
    }

    Column(
        modifier = modifier
            .fillMaxWidth()
            .focusGroup(),
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
            modifier = Modifier.fillMaxWidth(),
        ) {
            if (items.isEmpty() && skeletonCount > 0) {
                items(skeletonCount, key = { "skeleton-$id-$it" }) {
                    MediaPosterCardSkeleton()
                }
            } else {
                itemsIndexed(items, key = { _, card -> card.mediaItemId }) { index, card ->
                    MediaPosterCard(
                        model = card,
                        baseUrl = baseUrl,
                        onClick = { onSelect(card.id) },
                        requestFocus = focusedMediaId != null && card.mediaItemId == focusedMediaId,
                        focusRequester = itemFocusRequesters.getOrNull(index),
                        focusLeft = itemFocusRequesters.getOrNull(index - 1),
                        focusRight = itemFocusRequesters.getOrNull(index + 1),
                        focusUp = focusUp,
                        focusDown = focusDown,
                    )
                }
            }
        }
    }
}
