package com.aleksclark.primer.tv.app.ui.player

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.sizeIn
import androidx.compose.foundation.layout.widthIn
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.role
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import com.aleksclark.primer.tv.app.player.AudioOption
import com.aleksclark.primer.tv.app.player.SubtitleOption
import com.aleksclark.primer.tv.app.ui.designsystem.PrimerTheme
import com.aleksclark.primer.tv.app.ui.designsystem.TvFocusSurface

private enum class MediaMenu {
    NONE,
    SUBTITLES,
    AUDIO,
}

/**
 * Timed media chrome that sits with the transport overlay: volume, captions,
 * and audio-track selection. Hidden when the overlay is hidden.
 */
@Composable
fun PlaybackAccessibilityOverlay(
    volumePercent: Int,
    subtitles: List<SubtitleOption>,
    audioTracks: List<AudioOption>,
    onVolumeDown: () -> Unit,
    onVolumeUp: () -> Unit,
    onSubtitlesOff: () -> Unit,
    onSubtitleSelected: (SubtitleOption) -> Unit,
    onAudioSelected: (AudioOption) -> Unit,
    onUserInteraction: () -> Unit,
    modifier: Modifier = Modifier,
) {
    var menu by remember { mutableStateOf(MediaMenu.NONE) }
    val selectedSubtitle = subtitles.firstOrNull { it.selected }
    val selectedAudio = audioTracks.firstOrNull { it.selected }
    val spacing = PrimerTheme.spacing
    val colors = PrimerTheme.colors

    Box(modifier = modifier.fillMaxSize()) {
        Box(
            modifier = Modifier
                .fillMaxWidth()
                .align(Alignment.BottomCenter)
                .background(
                    Brush.verticalGradient(
                        colors = listOf(Color.Transparent, colors.scrimStrong.copy(alpha = 0.82f)),
                    ),
                )
                .padding(horizontal = spacing.screenHorizontal, vertical = spacing.screenVertical),
        ) {
            Column(
                modifier = Modifier.align(Alignment.BottomEnd),
                horizontalAlignment = Alignment.End,
                verticalArrangement = Arrangement.spacedBy(spacing.sm),
            ) {
                when (menu) {
                    MediaMenu.SUBTITLES -> TrackMenu(
                        title = "Subtitles",
                        offLabel = "Off",
                        offSelected = selectedSubtitle == null,
                        options = subtitles.map { it.label to it.selected },
                        onOff = {
                            onUserInteraction()
                            onSubtitlesOff()
                            menu = MediaMenu.NONE
                        },
                        onSelected = { index ->
                            onUserInteraction()
                            onSubtitleSelected(subtitles[index])
                            menu = MediaMenu.NONE
                        },
                    )
                    MediaMenu.AUDIO -> TrackMenu(
                        title = "Audio",
                        offLabel = null,
                        offSelected = false,
                        options = audioTracks.map { it.label to it.selected },
                        emptyLabel = "No alternate audio tracks",
                        onOff = {},
                        onSelected = { index ->
                            onUserInteraction()
                            onAudioSelected(audioTracks[index])
                            menu = MediaMenu.NONE
                        },
                    )
                    MediaMenu.NONE -> Unit
                }

                Row(horizontalArrangement = Arrangement.spacedBy(spacing.sm)) {
                    MediaChromeButton(
                        label = "Vol −",
                        description = "Lower playback volume",
                        requestFocus = true,
                        onClick = {
                            onUserInteraction()
                            onVolumeDown()
                        },
                    )
                    MediaChromeLabel(text = "$volumePercent%")
                    MediaChromeButton(
                        label = "Vol +",
                        description = "Raise playback volume",
                        onClick = {
                            onUserInteraction()
                            onVolumeUp()
                        },
                    )
                    MediaChromeButton(
                        label = "CC",
                        description = "Subtitles, ${selectedSubtitle?.label ?: "Off"}",
                        selected = menu == MediaMenu.SUBTITLES || selectedSubtitle != null,
                        onClick = {
                            onUserInteraction()
                            menu = if (menu == MediaMenu.SUBTITLES) MediaMenu.NONE else MediaMenu.SUBTITLES
                        },
                    )
                    if (audioTracks.size > 1) {
                        MediaChromeButton(
                            label = "Audio",
                            description = "Audio, ${selectedAudio?.label ?: "Default"}",
                            selected = menu == MediaMenu.AUDIO,
                            onClick = {
                                onUserInteraction()
                                menu = if (menu == MediaMenu.AUDIO) MediaMenu.NONE else MediaMenu.AUDIO
                            },
                        )
                    }
                }
            }
        }
    }
}

@Composable
private fun TrackMenu(
    title: String,
    offLabel: String?,
    offSelected: Boolean,
    options: List<Pair<String, Boolean>>,
    onOff: () -> Unit,
    onSelected: (Int) -> Unit,
    emptyLabel: String = "No subtitle tracks available",
) {
    val spacing = PrimerTheme.spacing
    val colors = PrimerTheme.colors
    Column(
        modifier = Modifier
            .widthIn(min = 220.dp, max = 360.dp)
            .background(colors.scrimStrong.copy(alpha = 0.92f), PrimerTheme.shapes.panel)
            .border(PrimerTheme.spacing.ruleWidth, colors.outlineStrong, PrimerTheme.shapes.panel)
            .padding(spacing.sm),
        verticalArrangement = Arrangement.spacedBy(spacing.xs),
    ) {
        Text(
            text = title.uppercase(),
            style = PrimerTheme.typography.label,
            color = colors.onSurfaceMuted,
            modifier = Modifier.padding(horizontal = spacing.sm, vertical = spacing.xs),
        )
        if (offLabel != null) {
            MediaChromeButton(
                label = offLabel,
                description = "$title off",
                selected = offSelected,
                onClick = onOff,
                modifier = Modifier.widthIn(min = 200.dp),
            )
        }
        options.forEachIndexed { index, (label, selected) ->
            MediaChromeButton(
                label = label,
                description = "Use $label $title",
                selected = selected,
                onClick = { onSelected(index) },
                modifier = Modifier.widthIn(min = 200.dp),
            )
        }
        if (options.isEmpty() && offLabel == null) {
            Text(
                text = emptyLabel,
                style = PrimerTheme.typography.metadata,
                color = colors.onSurfaceMuted,
                modifier = Modifier.padding(spacing.sm),
            )
        } else if (options.isEmpty() && offLabel != null) {
            Text(
                text = emptyLabel,
                style = PrimerTheme.typography.metadata,
                color = colors.onSurfaceMuted,
                modifier = Modifier.padding(spacing.sm),
            )
        }
    }
}

@Composable
private fun MediaChromeLabel(text: String) {
    val colors = PrimerTheme.colors
    Box(
        modifier = Modifier
            .background(colors.scrimStrong.copy(alpha = 0.72f), PrimerTheme.shapes.button)
            .sizeIn(minWidth = 64.dp, minHeight = 40.dp)
            .padding(horizontal = PrimerTheme.spacing.sm, vertical = PrimerTheme.spacing.xs),
        contentAlignment = Alignment.Center,
    ) {
        Text(
            text = text,
            style = PrimerTheme.typography.metadata,
            color = colors.onSurface,
        )
    }
}

@Composable
private fun MediaChromeButton(
    label: String,
    description: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    selected: Boolean = false,
    requestFocus: Boolean = false,
) {
    val colors = PrimerTheme.colors
    TvFocusSurface(
        onClick = onClick,
        selected = selected,
        requestFocus = requestFocus,
        shape = PrimerTheme.shapes.button,
        modifier = modifier.semantics {
            contentDescription = description
            role = Role.Button
        },
    ) { active ->
        val fill = when {
            active -> colors.onSurface.copy(alpha = 0.22f)
            selected -> colors.onSurface.copy(alpha = 0.14f)
            else -> colors.scrimStrong.copy(alpha = 0.55f)
        }
        Box(
            modifier = Modifier
                .background(fill, PrimerTheme.shapes.button)
                .sizeIn(minWidth = 48.dp, minHeight = 40.dp)
                .padding(horizontal = PrimerTheme.spacing.md, vertical = PrimerTheme.spacing.xs),
            contentAlignment = Alignment.Center,
        ) {
            Text(
                text = label,
                style = PrimerTheme.typography.button,
                color = colors.onSurface,
                maxLines = 1,
            )
        }
    }
}
