package com.aleksclark.primer.ui

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.sizeIn
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.Checkbox
import androidx.compose.material3.CheckboxDefaults
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.composed
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.RectangleShape
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.unit.dp

/** System C button tone. Product-specific terms stay outside core-ui. */
enum class PrimerButtonVariant { Primary, Secondary, Quiet, Attention }

@Composable
fun PrimerButton(
    text: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    enabled: Boolean = true,
    variant: PrimerButtonVariant = PrimerButtonVariant.Primary,
) {
    val colors = PrimerTheme.colors
    val spacing = PrimerTheme.spacing
    val content = text.uppercase()
    val interaction = remember { MutableInteractionSource() }
    when (variant) {
        PrimerButtonVariant.Quiet -> TextButton(
            onClick = onClick,
            enabled = enabled,
            shape = RectangleShape,
            interactionSource = interaction,
            modifier = modifier
                .primerFocusOutline()
                .sizeIn(minHeight = 44.dp),
            colors = ButtonDefaults.textButtonColors(
                contentColor = colors.textMuted,
                disabledContentColor = colors.ruleStrong,
            ),
            contentPadding = PaddingValues(horizontal = 0.dp, vertical = spacing.md),
        ) {
            Text(content, style = PrimerTheme.typography.button)
        }

        else -> Button(
            onClick = onClick,
            enabled = enabled,
            shape = RectangleShape,
            interactionSource = interaction,
            elevation = ButtonDefaults.buttonElevation(0.dp, 0.dp, 0.dp, 0.dp, 0.dp),
            border = BorderStroke(
                width = spacing.rule,
                color = when {
                    !enabled -> colors.rule
                    variant == PrimerButtonVariant.Secondary -> colors.ruleStrong
                    variant == PrimerButtonVariant.Attention -> colors.attention
                    else -> colors.accent
                },
            ),
            colors = ButtonDefaults.buttonColors(
                containerColor = when (variant) {
                    PrimerButtonVariant.Primary -> colors.accent
                    PrimerButtonVariant.Attention -> colors.surfaceRaised
                    PrimerButtonVariant.Secondary -> Color.Transparent
                    PrimerButtonVariant.Quiet -> Color.Transparent
                },
                contentColor = when (variant) {
                    PrimerButtonVariant.Primary -> colors.onAccent
                    PrimerButtonVariant.Attention -> colors.attention
                    else -> colors.text
                },
                disabledContainerColor = colors.surfaceRaised,
                disabledContentColor = colors.ruleStrong,
            ),
            modifier = modifier
                .primerFocusOutline()
                .sizeIn(minHeight = 44.dp),
            contentPadding = PaddingValues(horizontal = spacing.lg, vertical = spacing.md),
        ) {
            Text(content, style = PrimerTheme.typography.button)
        }
    }
}

@Composable
fun PrimerTextField(
    value: String,
    onValueChange: (String) -> Unit,
    label: String,
    modifier: Modifier = Modifier,
    enabled: Boolean = true,
    singleLine: Boolean = true,
    isError: Boolean = false,
    supportingText: String? = null,
    keyboardOptions: KeyboardOptions = KeyboardOptions.Default,
    keyboardActions: KeyboardActions = KeyboardActions.Default,
    visualTransformation: VisualTransformation = VisualTransformation.None,
) {
    val colors = PrimerTheme.colors
    OutlinedTextField(
        value = value,
        onValueChange = onValueChange,
        enabled = enabled,
        singleLine = singleLine,
        isError = isError,
        keyboardOptions = keyboardOptions,
        keyboardActions = keyboardActions,
        visualTransformation = visualTransformation,
        label = { Text(label, style = PrimerTheme.typography.label) },
        supportingText = supportingText?.let { text ->
            { Text(text.uppercase(), style = PrimerTheme.typography.label) }
        },
        shape = RectangleShape,
        modifier = modifier
            .fillMaxWidth()
            .primerFocusOutline(),
        textStyle = PrimerTheme.typography.body,
        colors = OutlinedTextFieldDefaults.colors(
            focusedContainerColor = colors.surfaceRaised,
            unfocusedContainerColor = colors.surfaceRaised,
            disabledContainerColor = colors.surfaceRaised,
            errorContainerColor = colors.surfaceRaised,
            focusedTextColor = colors.text,
            unfocusedTextColor = colors.text,
            disabledTextColor = colors.ruleStrong,
            focusedLabelColor = colors.accent,
            unfocusedLabelColor = colors.textMuted,
            disabledLabelColor = colors.ruleStrong,
            errorLabelColor = colors.attention,
            focusedBorderColor = colors.accent,
            unfocusedBorderColor = colors.rule,
            disabledBorderColor = colors.rule,
            errorBorderColor = colors.attention,
            cursorColor = colors.accent,
            errorCursorColor = colors.attention,
            focusedSupportingTextColor = if (isError) colors.attention else colors.textMuted,
            unfocusedSupportingTextColor = if (isError) colors.attention else colors.textMuted,
            errorSupportingTextColor = colors.attention,
        ),
    )
}

enum class PrimerStatusTone { Neutral, Accent, Attention, Filled }

@Composable
fun PrimerStatus(
    text: String,
    modifier: Modifier = Modifier,
    tone: PrimerStatusTone = PrimerStatusTone.Neutral,
) {
    val colors = PrimerTheme.colors
    val foreground = when (tone) {
        PrimerStatusTone.Neutral -> colors.textMuted
        PrimerStatusTone.Accent -> colors.accent
        PrimerStatusTone.Attention -> colors.attention
        PrimerStatusTone.Filled -> colors.onAccent
    }
    val background = if (tone == PrimerStatusTone.Filled) colors.accent else Color.Transparent
    val border = when (tone) {
        PrimerStatusTone.Attention -> colors.attention
        PrimerStatusTone.Accent -> colors.accent
        PrimerStatusTone.Filled -> colors.accent
        PrimerStatusTone.Neutral -> colors.rule
    }
    Text(
        text = text.uppercase(),
        style = PrimerTheme.typography.label,
        color = foreground,
        modifier = modifier
            .background(background)
            .border(PrimerTheme.spacing.rule, border, RectangleShape)
            .padding(horizontal = PrimerTheme.spacing.sm, vertical = PrimerTheme.spacing.xs)
            .semantics { contentDescription = text },
    )
}

@Composable
fun PrimerRule(modifier: Modifier = Modifier, strong: Boolean = false) {
    HorizontalDivider(
        modifier = modifier,
        thickness = PrimerTheme.spacing.rule,
        color = if (strong) PrimerTheme.colors.ruleStrong else PrimerTheme.colors.rule,
    )
}

@Composable
fun PrimerRuledSurface(
    modifier: Modifier = Modifier,
    contentPadding: PaddingValues = PaddingValues(PrimerTheme.spacing.lg),
    content: @Composable () -> Unit,
) {
    Surface(
        color = PrimerTheme.colors.surfaceRaised,
        contentColor = PrimerTheme.colors.text,
        shape = RectangleShape,
        border = BorderStroke(PrimerTheme.spacing.rule, PrimerTheme.colors.rule),
        modifier = modifier,
    ) {
        Box(Modifier.padding(contentPadding)) { content() }
    }
}

@Composable
fun PrimerRecordRow(
    label: String,
    value: String,
    modifier: Modifier = Modifier,
    status: String? = null,
    statusTone: PrimerStatusTone = PrimerStatusTone.Neutral,
) {
    Column(modifier.fillMaxWidth()) {
        PrimerRule()
        Row(
            Modifier
                .fillMaxWidth()
                .padding(vertical = PrimerTheme.spacing.md),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(PrimerTheme.spacing.md),
        ) {
            Column(Modifier.weight(1f)) {
                Text(label.uppercase(), style = PrimerTheme.typography.label, color = PrimerTheme.colors.textMuted)
                Text(value, style = PrimerTheme.typography.body, color = PrimerTheme.colors.text)
            }
            if (status != null) PrimerStatus(status, tone = statusTone)
        }
    }
}

@Composable
fun PrimerSectionHeader(
    label: String,
    title: String,
    modifier: Modifier = Modifier,
    description: String? = null,
    trailing: (@Composable RowScope.() -> Unit)? = null,
) {
    Column(modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(PrimerTheme.spacing.sm)) {
        Row(verticalAlignment = Alignment.Bottom, horizontalArrangement = Arrangement.spacedBy(PrimerTheme.spacing.md)) {
            Column(Modifier.weight(1f)) {
                Text(label.uppercase(), style = PrimerTheme.typography.label, color = PrimerTheme.colors.textMuted)
                Text(
                    title,
                    style = PrimerTheme.typography.title,
                    color = PrimerTheme.colors.text,
                    modifier = Modifier.semantics { heading() },
                )
            }
            trailing?.invoke(this)
        }
        if (description != null) {
            Text(description, style = PrimerTheme.typography.body, color = PrimerTheme.colors.textMuted)
        }
        PrimerRule(strong = true)
    }
}

@Composable
fun PrimerCheckboxRow(
    text: String,
    checked: Boolean,
    onCheckedChange: (Boolean) -> Unit,
    modifier: Modifier = Modifier,
    enabled: Boolean = true,
) {
    Row(
        modifier
            .fillMaxWidth()
            .toggleable(value = checked, enabled = enabled, role = Role.Checkbox, onValueChange = onCheckedChange)
            .primerFocusOutline()
            .padding(vertical = PrimerTheme.spacing.sm),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(PrimerTheme.spacing.sm),
    ) {
        Checkbox(
            checked = checked,
            onCheckedChange = null,
            enabled = enabled,
            colors = CheckboxDefaults.colors(
                checkedColor = PrimerTheme.colors.accent,
                uncheckedColor = PrimerTheme.colors.ruleStrong,
                checkmarkColor = PrimerTheme.colors.onAccent,
                disabledCheckedColor = PrimerTheme.colors.rule,
                disabledUncheckedColor = PrimerTheme.colors.rule,
            ),
        )
        Text(
            text,
            modifier = Modifier.weight(1f),
            style = PrimerTheme.typography.body,
            color = if (enabled) PrimerTheme.colors.text else PrimerTheme.colors.ruleStrong,
        )
    }
}

@Composable
fun PrimerEmptyState(
    title: String,
    message: String,
    modifier: Modifier = Modifier,
) {
    PrimerRuledSurface(modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(PrimerTheme.spacing.sm)) {
            Text(title, style = PrimerTheme.typography.sectionTitle, color = PrimerTheme.colors.text)
            Text(message, style = PrimerTheme.typography.body, color = PrimerTheme.colors.textMuted)
        }
    }
}

fun Modifier.primerFocusOutline(): Modifier = composed {
    var focused by remember { mutableStateOf(false) }
    val colors = PrimerTheme.colors
    val spacing = PrimerTheme.spacing
    onFocusChanged { focused = it.isFocused }.then(
        if (focused) {
            Modifier.border(spacing.focusWidth, colors.focus, RectangleShape)
        } else {
            Modifier
        },
    )
}
