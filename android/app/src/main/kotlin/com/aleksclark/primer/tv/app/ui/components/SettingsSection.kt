package com.aleksclark.primer.tv.app.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
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
 * Danger Zone). Square, ruled System C panel — no shadow elevation.
 * Danger Zone uses attention color on the title and a stronger rule.
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
    val ruleColor = if (danger) colors.error else colors.outline

    Column(
        modifier = modifier
            .fillMaxWidth()
            .background(surface, shapes.panel)
            .border(width = spacing.ruleWidth, color = ruleColor, shape = shapes.panel)
            .padding(spacing.md),
        verticalArrangement = Arrangement.spacedBy(spacing.sm),
    ) {
        Text(
            text = id.title().uppercase(),
            style = typography.label,
            color = if (danger) colors.error else colors.onSurfaceMuted,
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
            text = label.uppercase(),
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
