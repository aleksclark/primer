package com.aleksclark.primer.tv.app.ui.channel

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import com.aleksclark.primer.tv.app.ui.ChannelUiState
import com.aleksclark.primer.tv.app.ui.components.OnNowHero
import com.aleksclark.primer.tv.app.ui.designsystem.PrimerTheme
import com.aleksclark.primer.tv.core.presentation.ChannelPresenter
import com.aleksclark.primer.tv.core.presentation.ChannelScreenModel
import com.aleksclark.primer.tv.core.presentation.OnNowHeroModel

/** User intents from the Channel surface. */
sealed interface ChannelEvent {
    data object WatchLive : ChannelEvent
    data object Refresh : ChannelEvent
}

/**
 * Channel destination: OnNowHero plus a subtle refresh action. Navigation
 * chrome (Guide, Back) lives on the scaffold — not as header buttons.
 */
@Composable
fun ChannelScreen(
    state: ChannelUiState,
    baseUrl: String,
    onEvent: (ChannelEvent) -> Unit,
    modifier: Modifier = Modifier,
) {
    val model = remember(state.now, state.loading, state.error) {
        ChannelPresenter.present(
            now = state.now,
            loading = state.loading,
            error = state.error,
        )
    }
    ChannelScreenContent(
        model = model,
        baseUrl = baseUrl,
        onEvent = onEvent,
        modifier = modifier,
    )
}

@Composable
fun ChannelScreenContent(
    model: ChannelScreenModel,
    baseUrl: String,
    onEvent: (ChannelEvent) -> Unit,
    modifier: Modifier = Modifier,
) {
    val spacing = PrimerTheme.spacing
    val colors = PrimerTheme.colors
    val scroll = rememberScrollState()

    Column(
        modifier = modifier
            .fillMaxSize()
            .verticalScroll(scroll)
            .padding(horizontal = spacing.screenHorizontal)
            .padding(top = spacing.md, bottom = spacing.xl),
        verticalArrangement = Arrangement.spacedBy(spacing.md),
    ) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(
                text = "Channel",
                style = PrimerTheme.typography.screenTitle,
                color = colors.onSurface,
            )
            // Secondary refresh — does not compete with the title or Watch Live.
            TextButton(onClick = { onEvent(ChannelEvent.Refresh) }) {
                Text(
                    text = if (model.refreshing) "Refreshing…" else "Refresh",
                    style = PrimerTheme.typography.metadata,
                    color = colors.onSurfaceMuted,
                )
            }
        }

        // Non-blocking error when a previous snapshot is still on screen.
        model.error?.let { err ->
            if (model.hero !is OnNowHeroModel.Error && model.hero !is OnNowHeroModel.Loading) {
                Text(
                    text = err.value,
                    style = PrimerTheme.typography.metadata,
                    color = colors.error,
                )
            }
        }

        OnNowHero(
            model = model.hero,
            baseUrl = baseUrl,
            onWatchLive = { onEvent(ChannelEvent.WatchLive) },
            requestWatchLiveFocus = model.hero is OnNowHeroModel.OnAir &&
                (model.hero as OnNowHeroModel.OnAir).tunable,
            modifier = Modifier.fillMaxWidth(),
        )

        if (model.hero is OnNowHeroModel.Error) {
            Box(modifier = Modifier.fillMaxWidth(), contentAlignment = Alignment.Center) {
                TextButton(onClick = { onEvent(ChannelEvent.Refresh) }) {
                    Text("Retry", style = PrimerTheme.typography.button, color = colors.brand)
                }
            }
        }
    }
}
