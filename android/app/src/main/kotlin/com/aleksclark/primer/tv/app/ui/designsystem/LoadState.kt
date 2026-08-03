package com.aleksclark.primer.tv.app.ui.designsystem

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import com.aleksclark.primer.tv.core.presentation.UiText

/**
 * Structure-preserving load model for screens that should keep chrome while
 * content arrives, fails, or is empty.
 */
sealed interface LoadState<out T> {
    data object Loading : LoadState<Nothing>
    data class Content<T>(val value: T, val refreshing: Boolean = false) : LoadState<T>
    data class Empty(val message: UiText) : LoadState<Nothing>
    data class Error(val message: UiText, val canRetry: Boolean = true) : LoadState<Nothing>
}

/**
 * Renders loading, empty, error, or content while callers decide the content
 * layout. Keeps chrome outside this composable.
 */
@Composable
fun <T> ScreenStateLayout(
    state: LoadState<T>,
    modifier: Modifier = Modifier,
    onRetry: (() -> Unit)? = null,
    loading: @Composable () -> Unit = { DefaultLoading() },
    empty: @Composable (UiText) -> Unit = { message -> DefaultMessage(message.value) },
    error: @Composable (UiText, Boolean) -> Unit = { message, canRetry ->
        DefaultError(message = message.value, canRetry = canRetry, onRetry = onRetry)
    },
    content: @Composable (value: T, refreshing: Boolean) -> Unit,
) {
    Box(modifier = modifier.fillMaxSize()) {
        when (state) {
            LoadState.Loading -> loading()
            is LoadState.Empty -> empty(state.message)
            is LoadState.Error -> error(state.message, state.canRetry)
            is LoadState.Content -> content(state.value, state.refreshing)
        }
    }
}

@Composable
private fun DefaultLoading() {
    Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        CircularProgressIndicator(color = PrimerTheme.colors.brand)
    }
}

@Composable
private fun DefaultMessage(message: String) {
    Box(
        modifier = Modifier
            .fillMaxSize()
            .padding(PrimerTheme.spacing.lg),
        contentAlignment = Alignment.Center,
    ) {
        Text(text = message, style = PrimerTheme.typography.body, color = PrimerTheme.colors.onSurfaceMuted)
    }
}

@Composable
private fun DefaultError(message: String, canRetry: Boolean, onRetry: (() -> Unit)?) {
    Column(
        modifier = Modifier
            .fillMaxSize()
            .padding(PrimerTheme.spacing.lg),
        verticalArrangement = Arrangement.spacedBy(PrimerTheme.spacing.md, Alignment.CenterVertically),
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Text(text = message, style = PrimerTheme.typography.body, color = PrimerTheme.colors.error)
        if (canRetry && onRetry != null) {
            Button(onClick = onRetry) {
                Text("Retry", style = PrimerTheme.typography.button)
            }
        }
    }
}
