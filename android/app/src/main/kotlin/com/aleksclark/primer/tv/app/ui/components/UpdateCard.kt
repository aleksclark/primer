package com.aleksclark.primer.tv.app.ui.components

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.size
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import com.aleksclark.primer.tv.app.ui.designsystem.PrimerTheme
import com.aleksclark.primer.tv.core.domain.FormFactor
import com.aleksclark.primer.tv.core.presentation.UpdateCardModel

/**
 * Shows installed version, update status, progress, and install action.
 * Status is always conveyed with text (not color alone) for accessibility.
 */
@Composable
fun UpdateCard(
    installedVersionLabel: String,
    model: UpdateCardModel,
    onCheckForUpdate: () -> Unit,
    onInstallUpdate: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val colors = PrimerTheme.colors
    val spacing = PrimerTheme.spacing
    val typography = PrimerTheme.typography
    val minHeight = if (PrimerTheme.formFactor == FormFactor.TELEVISION) 56.dp else 48.dp

    Column(
        modifier = modifier.fillMaxWidth(),
        verticalArrangement = Arrangement.spacedBy(spacing.sm),
    ) {
        Text(
            text = installedVersionLabel,
            style = typography.body,
            color = colors.onSurface,
        )

        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(spacing.sm),
            modifier = Modifier.semantics(mergeDescendants = true) {
                contentDescription = model.statusLabel.value
                liveRegion = LiveRegionMode.Polite
            },
        ) {
            if (model.isBusy) {
                CircularProgressIndicator(
                    color = colors.brand,
                    strokeWidth = 2.dp,
                    modifier = Modifier.size(18.dp),
                )
            }
            Text(
                text = model.statusLabel.value.uppercase(),
                style = typography.label,
                color = when {
                    model.isError -> colors.error
                    model.canInstall -> colors.brand
                    else -> colors.onSurfaceMuted
                },
            )
        }

        Row(horizontalArrangement = Arrangement.spacedBy(spacing.sm)) {
            OutlinedButton(
                onClick = onCheckForUpdate,
                enabled = model.canRetryCheck,
                shape = PrimerTheme.shapes.button,
                border = androidx.compose.foundation.BorderStroke(
                    width = spacing.ruleWidth,
                    color = if (model.canRetryCheck) colors.outline else colors.outline.copy(alpha = 0.5f),
                ),
                colors = ButtonDefaults.outlinedButtonColors(
                    contentColor = colors.onSurface,
                    disabledContentColor = colors.outlineStrong,
                ),
                modifier = Modifier
                    .heightIn(min = minHeight)
                    .semantics { contentDescription = "Check for updates" },
            ) {
                Text("CHECK FOR UPDATES", style = typography.button)
            }

            if (model.canInstall) {
                Button(
                    onClick = onInstallUpdate,
                    shape = PrimerTheme.shapes.button,
                    elevation = ButtonDefaults.buttonElevation(
                        defaultElevation = 0.dp,
                        pressedElevation = 0.dp,
                        focusedElevation = 0.dp,
                        hoveredElevation = 0.dp,
                        disabledElevation = 0.dp,
                    ),
                    colors = ButtonDefaults.buttonColors(
                        containerColor = colors.brand,
                        contentColor = colors.onBrand,
                    ),
                    modifier = Modifier
                        .heightIn(min = minHeight)
                        .semantics { contentDescription = "Install update" },
                ) {
                    Text("INSTALL UPDATE", style = typography.button)
                }
            }
        }
    }
}
