package com.aleksclark.primer.tv.app.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.aleksclark.primer.tv.app.ui.designsystem.PrimerTheme
import com.aleksclark.primer.tv.app.ui.designsystem.TvFocusSurface
import com.aleksclark.primer.tv.core.presentation.ProgrammeRowModel
import com.aleksclark.primer.tv.core.presentation.ProgrammeTemporalState

/**
 * One guide slot: local clock time, title, runtime/class, and temporal state.
 * Optional [onClick] makes the row focusable for TV D-pad traversal.
 */
@Composable
fun ProgrammeRow(
    model: ProgrammeRowModel,
    modifier: Modifier = Modifier,
    requestFocus: Boolean = false,
    onClick: (() -> Unit)? = null,
) {
    val colors = PrimerTheme.colors
    val spacing = PrimerTheme.spacing
    val deEmphasized = model.temporal == ProgrammeTemporalState.PAST
    val titleColor = when (model.temporal) {
        ProgrammeTemporalState.CURRENT -> colors.brand
        ProgrammeTemporalState.PAST -> colors.onSurfaceMuted
        ProgrammeTemporalState.FUTURE -> colors.onSurface
    }
    val timeColor = if (deEmphasized) colors.onSurfaceMuted else colors.onSurface
    val titleWeight = if (model.isCurrent) FontWeight.SemiBold else FontWeight.Medium

    val body: @Composable () -> Unit = {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .then(
                    if (model.isCurrent) {
                        // Current row: raised surface + left accent rule (System C selected).
                        Modifier.background(colors.surfaceRaised)
                    } else {
                        Modifier
                    },
                )
                .padding(horizontal = spacing.md, vertical = spacing.sm),
            horizontalArrangement = Arrangement.spacedBy(spacing.md),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(
                text = model.timeLabel,
                style = PrimerTheme.typography.guideTime,
                color = timeColor,
                modifier = Modifier.widthIn(min = 72.dp).width(88.dp),
                maxLines = 1,
            )
            Column(
                modifier = Modifier.weight(1f),
                verticalArrangement = Arrangement.spacedBy(2.dp),
            ) {
                Text(
                    text = model.title,
                    style = PrimerTheme.typography.cardTitle.copy(fontWeight = titleWeight),
                    color = titleColor,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
                Text(
                    text = model.metadataLabel,
                    style = PrimerTheme.typography.metadata,
                    color = colors.onSurfaceMuted,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
            }
            if (model.isCurrent) {
                LiveBadge()
            }
        }
    }

    if (onClick != null) {
        TvFocusSurface(
            onClick = onClick,
            selected = model.isCurrent,
            requestFocus = requestFocus,
            shape = PrimerTheme.shapes.panel,
            modifier = modifier.fillMaxWidth(),
        ) { focused ->
            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .then(
                        if (focused) {
                            Modifier.background(colors.surfaceRaised)
                        } else {
                            Modifier
                        },
                    ),
            ) {
                body()
            }
        }
    } else {
        Box(modifier = modifier.fillMaxWidth()) {
            body()
        }
    }
}

@Composable
fun ProgrammeRowSkeleton(modifier: Modifier = Modifier) {
    val colors = PrimerTheme.colors
    val spacing = PrimerTheme.spacing
    Row(
        modifier = modifier
            .fillMaxWidth()
            .padding(horizontal = spacing.md, vertical = spacing.sm),
        horizontalArrangement = Arrangement.spacedBy(spacing.md),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Box(
            Modifier
                .width(72.dp)
                .padding(vertical = 4.dp)
                .background(colors.surfaceRaised, PrimerTheme.shapes.chip)
                .padding(vertical = 10.dp),
        )
        Column(
            modifier = Modifier.weight(1f),
            verticalArrangement = Arrangement.spacedBy(spacing.xs),
        ) {
            Box(
                Modifier
                    .fillMaxWidth(0.55f)
                    .background(colors.surfaceRaised, PrimerTheme.shapes.chip)
                    .padding(vertical = 10.dp),
            )
            Box(
                Modifier
                    .fillMaxWidth(0.35f)
                    .background(colors.surface, PrimerTheme.shapes.chip)
                    .padding(vertical = 8.dp),
            )
        }
    }
}
