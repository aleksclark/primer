package com.aleksclark.primer.tv.app.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import com.aleksclark.primer.tv.app.ui.designsystem.PrimerTheme
import com.aleksclark.primer.tv.core.presentation.SettingsSectionId
import com.aleksclark.primer.tv.core.presentation.title

/**
 * Reusable heading + content grouping for Settings (Device, Server, Updates,
 * Danger Zone). Danger Zone uses a slightly elevated surface so Unpair stays
 * visually separated from routine actions.
 */
@Composable
fun SettingsSection(
    id: SettingsSectionId,
    modifier: Modifier = Modifier,
    content: @Composable ColumnScope.() -> Unit,
) {
    val colors = PrimerTheme.colors
    val spacing = PrimerTheme.spacing
    val typography = PrimerTheme.typography
    val shapes = PrimerTheme.shapes
    val danger = id == SettingsSectionId.DANGER_ZONE
    val surface = if (danger) colors.surfaceRaised else colors.surface

    Column(
        modifier = modifier
            .fillMaxWidth()
            .background(surface, shapes.panel)
            .padding(spacing.md),
        verticalArrangement = Arrangement.spacedBy(spacing.sm),
    ) {
        Text(
            text = id.title(),
            style = typography.railTitle,
            color = if (danger) colors.error else colors.onSurface,
            modifier = Modifier.semantics { heading() },
        )
        content()
    }
}

/** Single labelled value row inside a [SettingsSection]. */
@Composable
fun SettingsInfoRow(
    label: String,
    value: String,
    modifier: Modifier = Modifier,
) {
    Column(
        modifier = modifier.fillMaxWidth(),
        verticalArrangement = Arrangement.spacedBy(PrimerTheme.spacing.xs),
    ) {
        Text(
            text = label,
            style = PrimerTheme.typography.label,
            color = PrimerTheme.colors.onSurfaceMuted,
        )
        Text(
            text = value,
            style = PrimerTheme.typography.body,
            color = PrimerTheme.colors.onSurface,
        )
    }
}
