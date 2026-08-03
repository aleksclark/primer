package com.aleksclark.primer.tv.app.ui.guide

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import com.aleksclark.primer.tv.app.ui.EpgUiState
import com.aleksclark.primer.tv.app.ui.components.ProgrammeGuide
import com.aleksclark.primer.tv.app.ui.designsystem.PrimerTheme
import com.aleksclark.primer.tv.core.presentation.GuidePresenter
import com.aleksclark.primer.tv.core.presentation.GuideScreenModel
import com.aleksclark.primer.tv.core.presentation.ProgrammeRowModel

/** User intents from the Guide surface. */
sealed interface GuideEvent {
    data object Refresh : GuideEvent
    data class SelectProgramme(val scheduleEntryId: String, val mediaItemId: String) : GuideEvent
}

/**
 * Guide destination backed by [ProgrammeGuide]. Opens scrolled to the current
 * or next programme; scaffold owns top-level navigation.
 */
@Composable
fun GuideScreen(
    state: EpgUiState,
    onEvent: (GuideEvent) -> Unit,
    modifier: Modifier = Modifier,
) {
    val model = remember(state.day, state.loading, state.error) {
        GuidePresenter.present(
            day = state.day,
            loading = state.loading,
            error = state.error,
        )
    }
    GuideScreenContent(
        model = model,
        onEvent = onEvent,
        modifier = modifier,
    )
}

@Composable
fun GuideScreenContent(
    model: GuideScreenModel,
    onEvent: (GuideEvent) -> Unit,
    modifier: Modifier = Modifier,
) {
    val spacing = PrimerTheme.spacing
    val colors = PrimerTheme.colors
    val listState = rememberLazyListState()

    Box(modifier = modifier.fillMaxSize()) {
        ProgrammeGuide(
            state = model,
            onRetry = { onEvent(GuideEvent.Refresh) },
            listState = listState,
            onRowClick = { row: ProgrammeRowModel ->
                onEvent(GuideEvent.SelectProgramme(row.scheduleEntryId, row.mediaItemId))
            },
            contentPadding = PaddingValues(
                horizontal = spacing.screenHorizontal,
                vertical = spacing.md,
            ),
            modifier = Modifier.fillMaxSize(),
        )

        // Subtle refresh when a non-empty schedule is mounted (including after a
        // failed refresh that kept the prior day). Empty/hard-error states
        // carry their own secondary Refresh/Retry inside ProgrammeGuide.
        if (!model.showSkeletons && model.guide != null && !model.guide!!.isEmpty) {
            TextButton(
                onClick = { onEvent(GuideEvent.Refresh) },
                modifier = Modifier
                    .align(Alignment.TopEnd)
                    .padding(top = spacing.sm, end = spacing.screenHorizontal),
            ) {
                Text(
                    text = if (model.refreshing) "Refreshing…" else "Refresh",
                    style = PrimerTheme.typography.metadata,
                    color = colors.onSurfaceMuted,
                )
            }
        }
    }
}
