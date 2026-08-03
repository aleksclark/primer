package com.aleksclark.primer.tv.app.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
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
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.unit.dp
import com.aleksclark.primer.tv.app.ui.designsystem.PrimerTheme
import com.aleksclark.primer.tv.core.presentation.GuidePresenter
import com.aleksclark.primer.tv.core.presentation.GuideScreenModel
import com.aleksclark.primer.tv.core.presentation.GuideUiModel
import com.aleksclark.primer.tv.core.presentation.ProgrammeRowModel

/**
 * Day schedule list with day heading, skeleton/empty/error states, and
 * auto-scroll/focus onto the current or next programme.
 */
@Composable
fun ProgrammeGuide(
    state: GuideScreenModel,
    onRetry: () -> Unit,
    modifier: Modifier = Modifier,
    listState: LazyListState = rememberLazyListState(),
    onRowClick: ((ProgrammeRowModel) -> Unit)? = null,
    contentPadding: PaddingValues = PaddingValues(),
) {
    when {
        state.showSkeletons -> ProgrammeGuideSkeleton(
            modifier = modifier,
            contentPadding = contentPadding,
        )

        state.error != null && state.guide == null -> GuideMessage(
            message = state.error?.value ?: "Couldn't load the guide",
            error = true,
            onRetry = onRetry,
            modifier = modifier.padding(contentPadding),
        )

        state.isEmpty -> GuideMessage(
            message = GuidePresenter.EMPTY_MESSAGE,
            error = false,
            // Refresh stays available on an empty day without dominating the
            // schedule chrome (secondary action under the empty copy).
            onRetry = onRetry,
            retryLabel = "Refresh",
            modifier = modifier.padding(contentPadding),
        )

        state.guide != null -> GuideList(
            guide = state.guide!!,
            listState = listState,
            onRowClick = onRowClick,
            modifier = modifier,
            contentPadding = contentPadding,
            trailingError = state.error?.value,
        )
    }
}

@Composable
private fun GuideList(
    guide: GuideUiModel,
    listState: LazyListState,
    onRowClick: ((ProgrammeRowModel) -> Unit)?,
    modifier: Modifier,
    contentPadding: PaddingValues,
    trailingError: String?,
) {
    val spacing = PrimerTheme.spacing
    val colors = PrimerTheme.colors
    var focusTargetId by remember(guide.focusScheduleEntryId) {
        mutableStateOf(guide.focusScheduleEntryId)
    }

    LaunchedEffect(guide.focusIndex, guide.focusScheduleEntryId) {
        val index = guide.focusIndex
        if (index in guide.rows.indices) {
            // +1 accounts for the day heading item above the rows.
            listState.scrollToItem(index = index + 1)
            focusTargetId = guide.focusScheduleEntryId
            kotlinx.coroutines.delay(64)
            if (focusTargetId == guide.focusScheduleEntryId) {
                focusTargetId = null
            }
        }
    }

    LazyColumn(
        state = listState,
        modifier = modifier.fillMaxSize(),
        contentPadding = contentPadding,
        verticalArrangement = Arrangement.spacedBy(spacing.xs),
    ) {
        item(key = "day-heading") {
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(bottom = spacing.sm),
            ) {
                Text(
                    text = "Guide",
                    style = PrimerTheme.typography.screenTitle,
                    color = colors.onSurface,
                )
                Text(
                    text = guide.dayHeading,
                    style = PrimerTheme.typography.metadata,
                    color = colors.onSurfaceMuted,
                )
            }
        }

        trailingError?.let { message ->
            item(key = "refresh-error") {
                Text(
                    text = message,
                    style = PrimerTheme.typography.metadata,
                    color = colors.error,
                    modifier = Modifier.padding(bottom = spacing.sm),
                )
            }
        }

        itemsIndexed(
            items = guide.rows,
            key = { _, row -> row.scheduleEntryId },
        ) { _, row ->
            ProgrammeRow(
                model = row,
                requestFocus = focusTargetId == row.scheduleEntryId,
                onClick = onRowClick?.let { handler -> { handler(row) } },
            )
        }
    }
}

@Composable
fun ProgrammeGuideSkeleton(
    modifier: Modifier = Modifier,
    contentPadding: PaddingValues = PaddingValues(),
) {
    val spacing = PrimerTheme.spacing
    val colors = PrimerTheme.colors
    LazyColumn(
        modifier = modifier.fillMaxSize(),
        contentPadding = contentPadding,
        verticalArrangement = Arrangement.spacedBy(spacing.xs),
    ) {
        item(key = "day-heading-skel") {
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(bottom = spacing.sm),
                verticalArrangement = Arrangement.spacedBy(spacing.sm),
            ) {
                Text(
                    text = "Guide",
                    style = PrimerTheme.typography.screenTitle,
                    color = colors.onSurface,
                )
                Box(
                    Modifier
                        .fillMaxWidth(0.4f)
                        .height(16.dp)
                        .clip(PrimerTheme.shapes.chip)
                        .background(colors.surfaceRaised),
                )
            }
        }
        items(8, key = { "guide-skel-$it" }) {
            ProgrammeRowSkeleton()
        }
    }
}

@Composable
private fun GuideMessage(
    message: String,
    error: Boolean,
    onRetry: (() -> Unit)?,
    modifier: Modifier = Modifier,
    retryLabel: String = "Retry",
) {
    val spacing = PrimerTheme.spacing
    val colors = PrimerTheme.colors
    Column(
        modifier = modifier
            .fillMaxSize()
            .padding(spacing.lg),
        verticalArrangement = Arrangement.spacedBy(spacing.md, Alignment.CenterVertically),
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Text(
            text = "Guide",
            style = PrimerTheme.typography.screenTitle,
            color = colors.onSurface,
        )
        Text(
            text = message,
            style = PrimerTheme.typography.body,
            color = if (error) colors.error else colors.onSurfaceMuted,
        )
        if (onRetry != null) {
            TextButton(onClick = onRetry) {
                Text(
                    text = retryLabel,
                    style = PrimerTheme.typography.button,
                    color = if (error) colors.brand else colors.onSurfaceMuted,
                )
            }
        }
    }
}
