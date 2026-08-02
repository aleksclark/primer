package com.aleksclark.primer.tv.app.ui

import androidx.compose.foundation.background
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
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import coil.compose.AsyncImage
import com.aleksclark.primer.tv.core.domain.CatalogPresenter
import com.aleksclark.primer.tv.core.domain.Programme
import com.aleksclark.primer.tv.core.net.resolveImageUrl
import java.time.ZoneId
import java.time.format.DateTimeFormatter

/**
 * The channel screens. Like the catalog they are written once and rendered by
 * both shells, with only [ShellMetrics] distinguishing the tablet from the box.
 */

/** clockTime renders an instant as a bare local clock time. */
private val clockFormatter: DateTimeFormatter = DateTimeFormatter.ofPattern("h:mm a")

private fun clockTime(instant: java.time.Instant): String =
    clockFormatter.format(instant.atZone(ZoneId.systemDefault()))

/** countdown renders a wait as "in 12 min" or "in 1h 5m". */
private fun countdown(seconds: Int): String {
    if (seconds <= 0) return "shortly"
    val minutes = seconds / 60
    if (minutes < 60) return "in ${minutes.coerceAtLeast(1)} min"
    return "in ${minutes / 60}h ${minutes % 60}m"
}

/**
 * ChannelScreen is the tune-in point: what is on now, and the button that joins
 * it in progress at the server's offset.
 */
@Composable
fun ChannelScreen(
    state: ChannelUiState,
    baseUrl: String,
    metrics: ShellMetrics,
    onTuneIn: () -> Unit,
    onOpenEpg: () -> Unit,
    onRefresh: () -> Unit,
    onBack: () -> Unit,
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
            Text("The channel", fontSize = metrics.titleSize.sp, fontWeight = FontWeight.SemiBold)
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                OutlinedButton(onClick = onOpenEpg) { Text("Today's schedule") }
                OutlinedButton(onClick = onRefresh) { Text("Refresh") }
                OutlinedButton(onClick = onBack) { Text("Back") }
            }
        }

        when {
            state.loading && state.now == null -> Box(
                modifier = Modifier.fillMaxSize(),
                contentAlignment = Alignment.Center,
            ) { CircularProgressIndicator() }

            state.error != null -> Text(
                text = state.error,
                color = MaterialTheme.colorScheme.error,
                fontSize = metrics.bodySize.sp,
            )

            else -> OnNow(state = state, baseUrl = baseUrl, metrics = metrics, onTuneIn = onTuneIn)
        }
    }
}

/** OnNow renders the current programme, or the gap before the next one. */
@Composable
private fun OnNow(
    state: ChannelUiState,
    baseUrl: String,
    metrics: ShellMetrics,
    onTuneIn: () -> Unit,
) {
    val now = state.now
    val onAir = state.onAir

    if (onAir == null) {
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(
                text = "Nothing is on the channel right now.",
                fontSize = metrics.bodySize.sp,
            )
            val next = now?.next
            if (next != null) {
                Text(
                    text = "Next: ${next.title} at ${clockTime(next.airsAt)}, ${countdown(now.nextStartsInSeconds)}.",
                    fontSize = metrics.bodySize.sp,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
        return
    }

    Row(horizontalArrangement = Arrangement.spacedBy(24.dp)) {
        AsyncImage(
            model = resolveImageUrl(baseUrl, onAir.imagePath),
            contentDescription = onAir.title,
            modifier = Modifier
                .width((metrics.cardWidth * 1.4f).dp)
                .aspectRatio(2f / 3f)
                .clip(RoundedCornerShape(12.dp))
                .background(Color(0xFF1B1F27)),
        )

        Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Text("On now", fontSize = (metrics.bodySize - 2).sp, color = MaterialTheme.colorScheme.tertiary)
            Text(onAir.title, fontSize = metrics.titleSize.sp, fontWeight = FontWeight.SemiBold)
            Text(
                text = "${clockTime(onAir.airsAt)} – ${clockTime(onAir.endsAt)} · ${CatalogPresenter.formatRuntime(onAir.runtimeSeconds)}",
                fontSize = metrics.bodySize.sp,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            if (now != null && now.offsetSeconds > 0) {
                Text(
                    text = "${now.offsetSeconds / 60} min in · ${(now.remainingSeconds / 60).coerceAtLeast(1)} min left",
                    fontSize = metrics.bodySize.sp,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            if (onAir.overview.isNotBlank()) {
                Text(onAir.overview, fontSize = metrics.bodySize.sp)
            }
            if (!onAir.directPlayOk) {
                Text(
                    text = "This programme cannot play on this device.",
                    fontSize = metrics.bodySize.sp,
                    color = MaterialTheme.colorScheme.error,
                )
            }
            Text(
                text = if (onAir.joinInProgress) {
                    "Tuning in joins the broadcast where it is now. There is no rewind."
                } else {
                    "This programme starts from the beginning."
                },
                fontSize = (metrics.bodySize - 2).sp,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )

            Button(onClick = onTuneIn, enabled = state.tunable) { Text("Watch") }

            now?.next?.let { next ->
                Text(
                    text = "Up next: ${next.title} at ${clockTime(next.airsAt)}",
                    fontSize = (metrics.bodySize - 2).sp,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}

/** EpgScreen lists today's grid as the server bucketed it. */
@Composable
fun EpgScreen(
    state: EpgUiState,
    metrics: ShellMetrics,
    onRefresh: () -> Unit,
    onBack: () -> Unit,
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
            Column {
                Text("Today", fontSize = metrics.titleSize.sp, fontWeight = FontWeight.SemiBold)
                state.day?.let { day ->
                    Text(
                        text = day.day,
                        fontSize = (metrics.bodySize - 2).sp,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                OutlinedButton(onClick = onRefresh) { Text("Refresh") }
                OutlinedButton(onClick = onBack) { Text("Back") }
            }
        }

        when {
            state.loading && state.day == null -> Box(
                modifier = Modifier.fillMaxSize(),
                contentAlignment = Alignment.Center,
            ) { CircularProgressIndicator() }

            state.error != null -> Text(
                text = state.error,
                color = MaterialTheme.colorScheme.error,
                fontSize = metrics.bodySize.sp,
            )

            state.isEmpty -> Text(
                text = "Nothing is scheduled today.",
                fontSize = metrics.bodySize.sp,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )

            else -> {
                val day = state.day!!
                val onAir = day.onAir()
                LazyColumn(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    items(day.programmes, key = { it.scheduleEntryId }) { programme ->
                        EpgRow(
                            programme = programme,
                            onAir = programme.scheduleEntryId == onAir?.scheduleEntryId,
                            past = programme.endsAt.isBefore(day.serverTime),
                            metrics = metrics,
                        )
                    }
                }
            }
        }
    }
}

/** EpgRow is one slot of the day's grid. */
@Composable
private fun EpgRow(
    programme: Programme,
    onAir: Boolean,
    past: Boolean,
    metrics: ShellMetrics,
) {
    Row(
        modifier = Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.spacedBy(16.dp),
    ) {
        Text(
            text = clockTime(programme.airsAt),
            fontSize = metrics.bodySize.sp,
            fontWeight = FontWeight.Medium,
            color = if (past && !onAir) {
                MaterialTheme.colorScheme.onSurfaceVariant
            } else {
                MaterialTheme.colorScheme.onSurface
            },
            modifier = Modifier.width((metrics.bodySize * 6).dp),
        )
        Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
            Text(
                text = programme.title,
                fontSize = metrics.bodySize.sp,
                fontWeight = if (onAir) FontWeight.SemiBold else FontWeight.Normal,
                color = if (onAir) MaterialTheme.colorScheme.tertiary else MaterialTheme.colorScheme.onSurface,
            )
            Text(
                text = buildString {
                    append(CatalogPresenter.formatRuntime(programme.runtimeSeconds))
                    append(" · ")
                    append(programme.mediaClass.label)
                    if (onAir) append(" · On now")
                    else if (past) append(" · Finished")
                },
                fontSize = (metrics.bodySize - 4).sp,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}
