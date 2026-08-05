package com.aleksclark.primer.tv.app.ui.home

import androidx.compose.foundation.focusGroup
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListState
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import com.aleksclark.primer.tv.app.ui.components.ContentRail
import com.aleksclark.primer.tv.app.ui.components.FeaturedHero
import com.aleksclark.primer.tv.app.ui.components.FeaturedHeroSkeleton
import com.aleksclark.primer.tv.app.ui.designsystem.PrimerTheme
import com.aleksclark.primer.tv.core.presentation.HeroAction
import com.aleksclark.primer.tv.core.presentation.HeroModel
import com.aleksclark.primer.tv.core.presentation.HomeUiState
import com.aleksclark.primer.tv.core.presentation.MediaId
import com.aleksclark.primer.tv.core.presentation.RailId

/** User intents from the Home surface. */
sealed interface HomeEvent {
    data object Refresh : HomeEvent
    data class SelectMedia(val id: String) : HomeEvent
    data object OpenChannel : HomeEvent
    data object PlayHero : HomeEvent
}

/**
 * Streaming-style Home: featured/on-now hero plus content rails.
 *
 * D-pad contract on television:
 * - Up/down moves between the LIVE/featured banner and each VOD category row.
 * - Left/right on a category row scrolls that row's posters.
 *
 * Rail [LazyListState] is remembered by [RailId] so refresh with stable IDs
 * keeps scroll/focus position. When [detailsOpen] flips false, [selectedMediaId]
 * is re-focused so TV returns to the originating card.
 */
@Composable
fun HomeScreen(
    state: HomeUiState,
    baseUrl: String,
    onEvent: (HomeEvent) -> Unit,
    modifier: Modifier = Modifier,
    listState: LazyListState = rememberLazyListState(),
    selectedMediaId: String? = null,
    detailsOpen: Boolean = false,
) {
    val spacing = PrimerTheme.spacing
    val colors = PrimerTheme.colors
    val learnState = rememberLazyListState()
    val worthState = rememberLazyListState()
    val entertainmentState = rememberLazyListState()
    val unclassifiedState = rememberLazyListState()
    val leavingState = rememberLazyListState()

    fun railState(id: RailId): LazyListState = when (id) {
        RailId.LEARN -> learnState
        RailId.WORTH_WATCHING -> worthState
        RailId.ENTERTAINMENT -> entertainmentState
        RailId.UNCLASSIFIED -> unclassifiedState
        RailId.LEAVING_SOON -> leavingState
    }

    val heroFocus = remember { FocusRequester() }
    val railFocusRequesters = remember(state.rails.map { it.id }) {
        state.rails.associate { it.id to FocusRequester() }
    }

    var pendingFocusId by rememberSaveable { mutableStateOf<String?>(null) }
    var focusTarget by remember { mutableStateOf<String?>(null) }
    var bootstrapFocus by rememberSaveable { mutableStateOf(true) }

    LaunchedEffect(detailsOpen, selectedMediaId) {
        if (detailsOpen) {
            if (selectedMediaId != null) {
                pendingFocusId = selectedMediaId
            }
            focusTarget = null
            bootstrapFocus = false
        } else if (pendingFocusId != null) {
            focusTarget = pendingFocusId
            pendingFocusId = null
        }
    }

    LaunchedEffect(bootstrapFocus, state.hero, state.rails, detailsOpen) {
        if (!bootstrapFocus || detailsOpen || focusTarget != null) return@LaunchedEffect
        if (state.hero is HeroModel.Loading && state.rails.isEmpty()) return@LaunchedEffect
        runCatching { heroFocus.requestFocus() }
        bootstrapFocus = false
    }

    LaunchedEffect(focusTarget, state.rails) {
        val target = focusTarget ?: return@LaunchedEffect
        val railIndex = state.rails.indexOfFirst { r -> r.items.any { it.mediaItemId == target } }
        if (railIndex >= 0) {
            val rail = state.rails[railIndex]
            val index = rail.items.indexOfFirst { it.mediaItemId == target }
            if (index >= 0) {
                // +1 accounts for the hero item above the rails.
                listState.scrollToItem(railIndex + 1)
                railState(rail.id).scrollToItem(index)
            }
        }
        kotlinx.coroutines.delay(64)
        if (focusTarget == target) {
            focusTarget = null
        }
    }

    Column(
        modifier = modifier
            .fillMaxSize()
            .focusGroup(),
    ) {
        state.error?.let { err ->
            if (state.hasContent || state.rails.isNotEmpty() || state.hero !is HeroModel.Loading) {
                RowRefreshBanner(
                    message = err.value,
                    onRetry = { onEvent(HomeEvent.Refresh) },
                )
            }
        }

        when {
            state.showSkeletons -> HomeSkeleton(
                listState = listState,
                contentPadding = PaddingValues(bottom = spacing.xl),
            )

            state.error != null && !state.hasContent && state.rails.isEmpty() &&
                state.hero is HeroModel.Empty -> {
                HomeError(
                    message = state.error?.value ?: "Couldn't load Home",
                    onRetry = { onEvent(HomeEvent.Refresh) },
                )
            }

            else -> {
                LazyColumn(
                    state = listState,
                    modifier = Modifier.fillMaxSize(),
                    contentPadding = PaddingValues(bottom = spacing.xl),
                    verticalArrangement = Arrangement.spacedBy(spacing.lg),
                ) {
                    item(key = "hero") {
                        FeaturedHero(
                            hero = state.hero,
                            baseUrl = baseUrl,
                            onPrimaryAction = {
                                when (state.hero.primaryAction) {
                                    HeroAction.WATCH_LIVE,
                                    HeroAction.PLAY,
                                    HeroAction.VIEW_DETAILS,
                                    -> onEvent(HomeEvent.PlayHero)
                                    HeroAction.NONE -> Unit
                                }
                            },
                            focusRequester = heroFocus,
                            focusDown = state.rails.firstOrNull()?.let { railFocusRequesters[it.id] },
                            requestFocus = bootstrapFocus && focusTarget == null && !detailsOpen,
                            modifier = Modifier
                                .fillMaxWidth()
                                .padding(horizontal = spacing.screenHorizontal)
                                .padding(top = spacing.md),
                        )
                    }

                    itemsIndexed(state.rails, key = { _, rail -> rail.id.name }) { index, rail ->
                        val previous = if (index == 0) {
                            heroFocus
                        } else {
                            railFocusRequesters[state.rails[index - 1].id]
                        }
                        val next = state.rails.getOrNull(index + 1)?.let { railFocusRequesters[it.id] }
                        ContentRail(
                            rail = rail,
                            baseUrl = baseUrl,
                            onSelect = { id: MediaId -> onEvent(HomeEvent.SelectMedia(id.value)) },
                            state = railState(rail.id),
                            focusedMediaId = focusTarget,
                            firstItemFocusRequester = railFocusRequesters[rail.id],
                            focusUp = previous,
                            focusDown = next,
                            contentPadding = PaddingValues(horizontal = spacing.screenHorizontal),
                        )
                    }

                    if (state.isEmpty) {
                        item(key = "empty-copy") {
                            Text(
                                text = "New titles appear when the rotation changes. The channel guide stays available from navigation.",
                                style = PrimerTheme.typography.body,
                                color = colors.onSurfaceMuted,
                                modifier = Modifier.padding(horizontal = spacing.screenHorizontal),
                            )
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun HomeSkeleton(
    listState: LazyListState,
    contentPadding: PaddingValues,
) {
    val spacing = PrimerTheme.spacing
    LazyColumn(
        state = listState,
        modifier = Modifier.fillMaxSize(),
        contentPadding = contentPadding,
        verticalArrangement = Arrangement.spacedBy(spacing.lg),
    ) {
        item(key = "hero-skeleton") {
            FeaturedHeroSkeleton(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = spacing.screenHorizontal)
                    .padding(top = spacing.md),
            )
        }
        items(2, key = { "rail-skel-$it" }) {
            ContentRail(
                id = if (it == 0) RailId.LEARN else RailId.WORTH_WATCHING,
                title = if (it == 0) "Learn" else "Worth Watching",
                items = emptyList(),
                baseUrl = "",
                onSelect = {},
                skeletonCount = 5,
                contentPadding = PaddingValues(horizontal = spacing.screenHorizontal),
            )
        }
    }
}

@Composable
private fun HomeError(
    message: String,
    onRetry: () -> Unit,
) {
    val spacing = PrimerTheme.spacing
    Column(
        modifier = Modifier
            .fillMaxSize()
            .padding(spacing.lg),
        verticalArrangement = Arrangement.spacedBy(spacing.md, Alignment.CenterVertically),
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Text(
            text = message,
            style = PrimerTheme.typography.body,
            color = PrimerTheme.colors.error,
        )
        TextButton(onClick = onRetry) {
            Text("Retry", style = PrimerTheme.typography.button, color = PrimerTheme.colors.brand)
        }
    }
}

@Composable
private fun RowRefreshBanner(
    message: String,
    onRetry: () -> Unit,
) {
    val colors = PrimerTheme.colors
    val spacing = PrimerTheme.spacing
    Box(
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = spacing.screenHorizontal, vertical = spacing.sm),
    ) {
        androidx.compose.foundation.layout.Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(
                text = message,
                style = PrimerTheme.typography.metadata,
                color = colors.error,
                modifier = Modifier.weight(1f),
            )
            TextButton(onClick = onRetry) {
                Text("Retry", style = PrimerTheme.typography.button, color = colors.brand)
            }
        }
    }
}
