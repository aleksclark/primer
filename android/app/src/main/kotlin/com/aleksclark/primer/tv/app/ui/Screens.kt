package com.aleksclark.primer.tv.app.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import coil.compose.AsyncImage
import com.aleksclark.primer.tv.core.data.DeviceSettings
import com.aleksclark.primer.tv.app.update.UpdateState
import com.aleksclark.primer.tv.core.domain.CatalogCard
import com.aleksclark.primer.tv.core.domain.CatalogPresenter
import com.aleksclark.primer.tv.core.net.resolveImageUrl

/**
 * The screens are written once and rendered by both shells. The TV shell wraps
 * them in wider padding and larger type rather than reimplementing them, so
 * behaviour cannot drift between the tablet and the box.
 */

/** Layout constants that differ between the touch and 10-foot presentations. */
data class ShellMetrics(
    val screenPadding: Int,
    val titleSize: Int,
    val bodySize: Int,
    val cardWidth: Int,
) {
    companion object {
        val Tablet = ShellMetrics(screenPadding = 24, titleSize = 28, bodySize = 16, cardWidth = 180)

        /** Wide enough to clear TV overscan, with 10-foot legible type. */
        val Television = ShellMetrics(screenPadding = 48, titleSize = 40, bodySize = 22, cardWidth = 260)
    }
}

@Composable
fun PairingScreen(
    state: PairingUiState,
    metrics: ShellMetrics,
    onBaseUrlChanged: (String) -> Unit,
    onCodeChanged: (String) -> Unit,
    onSubmit: () -> Unit,
) {
    Column(
        modifier = Modifier
            .fillMaxSize()
            .padding(metrics.screenPadding.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp),
    ) {
        Text(
            text = "Pair this device",
            fontSize = metrics.titleSize.sp,
            fontWeight = FontWeight.SemiBold,
        )
        Text(
            text = "Enter the server address and the pairing code shown in the admin page.",
            fontSize = metrics.bodySize.sp,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )

        OutlinedTextField(
            value = state.baseUrlInput,
            onValueChange = onBaseUrlChanged,
            label = { Text("Server address") },
            singleLine = true,
            enabled = !state.submitting,
            modifier = Modifier.fillMaxWidth(),
        )
        OutlinedTextField(
            value = state.codeInput,
            onValueChange = onCodeChanged,
            label = { Text("Pairing code") },
            singleLine = true,
            enabled = !state.submitting,
            // Codes are uppercase-only; keep the software keyboard there too.
            keyboardOptions = androidx.compose.foundation.text.KeyboardOptions(
                capitalization = androidx.compose.ui.text.input.KeyboardCapitalization.Characters,
                autoCorrectEnabled = false,
            ),
            modifier = Modifier.fillMaxWidth(),
        )

        state.error?.let { message ->
            Text(text = message, color = MaterialTheme.colorScheme.error, fontSize = metrics.bodySize.sp)
        }

        Button(onClick = onSubmit, enabled = state.canSubmit) {
            Text(if (state.submitting) "Pairing…" else "Pair")
        }
    }
}

@Composable
fun CatalogScreen(
    state: CatalogUiState,
    baseUrl: String,
    metrics: ShellMetrics,
    onSelect: (String) -> Unit,
    onRefresh: () -> Unit,
    onOpenSettings: () -> Unit,
    onOpenChannel: () -> Unit,
) {
    Column(
        modifier = Modifier
            .fillMaxSize()
            .padding(metrics.screenPadding.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp),
    ) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(text = "Available now", fontSize = metrics.titleSize.sp, fontWeight = FontWeight.SemiBold)
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Button(onClick = onOpenChannel) { Text("The channel") }
                OutlinedButton(onClick = onRefresh) { Text("Refresh") }
                OutlinedButton(onClick = onOpenSettings) { Text("Settings") }
            }
        }

        when {
            state.loading && state.view == null -> Box(
                modifier = Modifier.fillMaxSize(),
                contentAlignment = Alignment.Center,
            ) { CircularProgressIndicator() }

            state.error != null -> Text(
                text = state.error,
                color = MaterialTheme.colorScheme.error,
                fontSize = metrics.bodySize.sp,
            )

            state.isEmpty -> Text(
                text = "Nothing is available right now. New titles appear when the rotation changes.",
                fontSize = metrics.bodySize.sp,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )

            else -> LazyColumn(verticalArrangement = Arrangement.spacedBy(20.dp)) {
                items(state.view!!.rails, key = { it.title }) { rail ->
                    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                        Text(
                            text = rail.title,
                            fontSize = (metrics.bodySize + 4).sp,
                            fontWeight = FontWeight.Medium,
                        )
                        LazyRow(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                            items(rail.cards, key = { it.entry.mediaItemId }) { card ->
                                CatalogCardView(
                                    card = card,
                                    baseUrl = baseUrl,
                                    metrics = metrics,
                                    onClick = { onSelect(card.entry.mediaItemId) },
                                )
                            }
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun CatalogCardView(
    card: CatalogCard,
    baseUrl: String,
    metrics: ShellMetrics,
    onClick: () -> Unit,
) {
    Column(
        modifier = Modifier
            .width(metrics.cardWidth.dp)
            .clickable(onClick = onClick)
            // A title watched during this session stays visible but dimmed until
            // the next refresh drops it, so the row does not vanish mid-gesture.
            .alpha(if (card.alreadyWatched) 0.45f else 1f),
        verticalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        AsyncImage(
            model = resolveImageUrl(baseUrl, card.entry.imagePath),
            contentDescription = card.entry.title,
            modifier = Modifier
                .fillMaxWidth()
                .aspectRatio(2f / 3f)
                .clip(RoundedCornerShape(8.dp))
                .background(Color(0xFF1B1F27)),
        )
        Text(
            text = card.entry.title,
            fontSize = metrics.bodySize.sp,
            maxLines = 2,
            fontWeight = FontWeight.Medium,
        )
        Text(
            text = buildString {
                append(CatalogPresenter.formatRuntime(card.entry.runtimeSeconds))
                if (card.alreadyWatched) append(" · Already watched")
                else if (card.consumesPlay) append(" · One viewing")
                if (card.expiringSoon) append(" · Leaving soon")
            },
            fontSize = (metrics.bodySize - 3).sp,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

@Composable
fun DetailScreen(
    card: CatalogCard?,
    baseUrl: String,
    metrics: ShellMetrics,
    onPlay: () -> Unit,
    onBack: () -> Unit,
) {
    if (card == null) {
        Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
            Text("That title is no longer available.", fontSize = metrics.bodySize.sp)
        }
        return
    }

    Row(
        modifier = Modifier
            .fillMaxSize()
            .padding(metrics.screenPadding.dp),
        horizontalArrangement = Arrangement.spacedBy(24.dp),
    ) {
        AsyncImage(
            model = resolveImageUrl(baseUrl, card.entry.imagePath),
            contentDescription = card.entry.title,
            modifier = Modifier
                .width((metrics.cardWidth * 1.4f).dp)
                .aspectRatio(2f / 3f)
                .clip(RoundedCornerShape(12.dp))
                .background(Color(0xFF1B1F27)),
        )

        Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Text(card.entry.title, fontSize = metrics.titleSize.sp, fontWeight = FontWeight.SemiBold)
            Text(
                text = "${card.entry.mediaClass.label} · ${CatalogPresenter.formatRuntime(card.entry.runtimeSeconds)}",
                fontSize = metrics.bodySize.sp,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            if (card.entry.overview.isNotBlank()) {
                Text(card.entry.overview, fontSize = metrics.bodySize.sp)
            }
            if (card.entry.subjectTags.isNotEmpty()) {
                Text(
                    text = "Subjects: ${card.entry.subjectTags.joinToString(", ")}",
                    fontSize = (metrics.bodySize - 2).sp,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            if (card.consumesPlay && !card.alreadyWatched) {
                Text(
                    text = "This title may be watched once.",
                    fontSize = metrics.bodySize.sp,
                    color = MaterialTheme.colorScheme.tertiary,
                )
            }

            Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                Button(onClick = onPlay, enabled = card.playable) {
                    Text(if (card.alreadyWatched) "Already watched" else "Play")
                }
                OutlinedButton(onClick = onBack) { Text("Back") }
            }
        }
    }
}

@Composable
fun SettingsScreen(
    settings: DeviceSettings,
    metrics: ShellMetrics,
    installedVersion: Int,
    update: UpdateState,
    onCheckForUpdate: () -> Unit,
    onInstallUpdate: () -> Unit,
    onUnpair: () -> Unit,
    onBack: () -> Unit,
) {
    Column(
        modifier = Modifier
            .fillMaxSize()
            .padding(metrics.screenPadding.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Text("Settings", fontSize = metrics.titleSize.sp, fontWeight = FontWeight.SemiBold)
        Text("Server: ${settings.baseUrl ?: "not set"}", fontSize = metrics.bodySize.sp)
        Text("Device: ${settings.deviceName ?: "unpaired"}", fontSize = metrics.bodySize.sp)
        Text("Kind: ${settings.deviceKind ?: "unknown"}", fontSize = metrics.bodySize.sp)
        Text("App version: $installedVersion", fontSize = metrics.bodySize.sp)

        when (update) {
            is UpdateState.UpToDate -> Text(
                "This device is up to date.",
                fontSize = metrics.bodySize.sp,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            is UpdateState.Available -> Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(
                    "Version ${update.release.versionCode} is available.",
                    fontSize = metrics.bodySize.sp,
                    color = MaterialTheme.colorScheme.tertiary,
                )
                Button(onClick = onInstallUpdate) { Text("Install update") }
            }
            is UpdateState.Downloading -> Text("Downloading…", fontSize = metrics.bodySize.sp)
            is UpdateState.Installing -> Text("Starting the installer…", fontSize = metrics.bodySize.sp)
            is UpdateState.Failed -> Text(
                update.message,
                fontSize = metrics.bodySize.sp,
                color = MaterialTheme.colorScheme.error,
            )
        }

        Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            OutlinedButton(onClick = onCheckForUpdate) { Text("Check for updates") }
            Button(onClick = onUnpair) { Text("Unpair") }
            OutlinedButton(onClick = onBack) { Text("Back") }
        }
    }
}

/** Full-screen message used for grant refusals and playback failures. */
@Composable
fun PlaybackMessage(message: String, metrics: ShellMetrics, onBack: () -> Unit) {
    Column(
        modifier = Modifier
            .fillMaxSize()
            .padding(metrics.screenPadding.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp),
    ) {
        Text(message, fontSize = metrics.bodySize.sp)
        Button(onClick = onBack) { Text("Back") }
    }
}
