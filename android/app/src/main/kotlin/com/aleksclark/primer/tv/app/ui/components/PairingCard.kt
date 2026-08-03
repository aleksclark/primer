package com.aleksclark.primer.tv.app.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.focusGroup
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusProperties
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.unit.dp
import com.aleksclark.primer.tv.app.ui.designsystem.PrimerTheme
import com.aleksclark.primer.tv.core.domain.FormFactor

/**
 * Centered onboarding surface: server URL, uppercase pairing code, inline error,
 * and Pair action. On TV the focus order is fixed URL → code → Pair and does not
 * escape the card while tabbing down through the fields.
 */
@Composable
fun PairingCard(
    baseUrl: String,
    code: String,
    submitting: Boolean,
    error: String?,
    canSubmit: Boolean,
    onBaseUrlChanged: (String) -> Unit,
    onCodeChanged: (String) -> Unit,
    onSubmit: () -> Unit,
    modifier: Modifier = Modifier,
    requestCodeFocus: Boolean = false,
) {
    val colors = PrimerTheme.colors
    val spacing = PrimerTheme.spacing
    val typography = PrimerTheme.typography
    val shapes = PrimerTheme.shapes
    val formFactor = PrimerTheme.formFactor

    val urlFocus = remember { FocusRequester() }
    val codeFocus = remember { FocusRequester() }
    val pairFocus = remember { FocusRequester() }

    LaunchedEffect(Unit) {
        if (formFactor == FormFactor.TELEVISION) {
            runCatching { urlFocus.requestFocus() }
        }
    }
    LaunchedEffect(requestCodeFocus, error) {
        if (requestCodeFocus || error != null) {
            runCatching { codeFocus.requestFocus() }
        }
    }

    Box(
        modifier = modifier
            .fillMaxWidth()
            .padding(horizontal = spacing.screenHorizontal, vertical = spacing.screenVertical),
        contentAlignment = Alignment.Center,
    ) {
        Column(
            modifier = Modifier
                .widthIn(max = if (formFactor == FormFactor.TELEVISION) 720.dp else 480.dp)
                .fillMaxWidth()
                .background(colors.surface, shapes.panel)
                .padding(spacing.lg)
                .focusGroup()
                .semantics { contentDescription = "Pair this device" }
                .testTag("pairing_card"),
            verticalArrangement = Arrangement.spacedBy(spacing.md),
        ) {
            Text(
                text = "Pair this device",
                style = typography.screenTitle,
                color = colors.onSurface,
            )
            Text(
                text = "Enter the server address and the pairing code shown in the admin page.",
                style = typography.body,
                color = colors.onSurfaceMuted,
            )

            OutlinedTextField(
                value = baseUrl,
                onValueChange = onBaseUrlChanged,
                label = { Text("Server address") },
                singleLine = true,
                enabled = !submitting,
                keyboardOptions = KeyboardOptions(imeAction = ImeAction.Next),
                keyboardActions = KeyboardActions(onNext = { codeFocus.requestFocus() }),
                colors = pairingFieldColors(),
                modifier = Modifier
                    .fillMaxWidth()
                    .heightIn(min = if (formFactor == FormFactor.TELEVISION) 56.dp else 48.dp)
                    .focusRequester(urlFocus)
                    .focusProperties {
                        next = codeFocus
                        down = codeFocus
                    }
                    .testTag("pairing_server_url")
                    .semantics { contentDescription = "Server address" },
            )

            OutlinedTextField(
                value = code,
                onValueChange = onCodeChanged,
                label = { Text("Pairing code") },
                singleLine = true,
                enabled = !submitting,
                // Codes are uppercase-only; keep the software keyboard there too.
                keyboardOptions = KeyboardOptions(
                    capitalization = KeyboardCapitalization.Characters,
                    autoCorrectEnabled = false,
                    imeAction = ImeAction.Done,
                ),
                keyboardActions = KeyboardActions(
                    onDone = { if (canSubmit) onSubmit() else pairFocus.requestFocus() },
                ),
                colors = pairingFieldColors(),
                modifier = Modifier
                    .fillMaxWidth()
                    .heightIn(min = if (formFactor == FormFactor.TELEVISION) 56.dp else 48.dp)
                    .focusRequester(codeFocus)
                    .focusProperties {
                        previous = urlFocus
                        up = urlFocus
                        next = pairFocus
                        down = pairFocus
                    }
                    .testTag("pairing_code")
                    .semantics { contentDescription = "Pairing code" },
            )

            error?.let { message ->
                Text(
                    text = message,
                    style = typography.body,
                    color = colors.error,
                    modifier = Modifier
                        .testTag("pairing_error")
                        .semantics { contentDescription = "Pairing error: $message" },
                )
            }

            Button(
                onClick = onSubmit,
                enabled = canSubmit,
                shape = shapes.button,
                colors = ButtonDefaults.buttonColors(
                    containerColor = colors.brand,
                    contentColor = colors.onBrand,
                    disabledContainerColor = colors.surfaceRaised,
                    disabledContentColor = colors.onSurfaceMuted,
                ),
                modifier = Modifier
                    .heightIn(min = if (formFactor == FormFactor.TELEVISION) 56.dp else 48.dp)
                    .focusRequester(pairFocus)
                    .focusProperties {
                        previous = codeFocus
                        up = codeFocus
                        // Keep Down from leaving the card into empty chrome.
                        down = pairFocus
                        next = pairFocus
                    }
                    .testTag("pairing_submit")
                    .semantics {
                        contentDescription = if (submitting) "Pairing in progress" else "Pair"
                    },
            ) {
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(spacing.sm),
                ) {
                    if (submitting) {
                        CircularProgressIndicator(
                            color = colors.onBrand,
                            strokeWidth = 2.dp,
                            modifier = Modifier.size(18.dp),
                        )
                    }
                    Text(
                        text = if (submitting) "Pairing…" else "Pair",
                        style = typography.button,
                    )
                }
            }
        }
    }
}

@Composable
private fun pairingFieldColors() = OutlinedTextFieldDefaults.colors(
    focusedTextColor = PrimerTheme.colors.onSurface,
    unfocusedTextColor = PrimerTheme.colors.onSurface,
    disabledTextColor = PrimerTheme.colors.onSurfaceMuted,
    focusedBorderColor = PrimerTheme.colors.focusBorder,
    unfocusedBorderColor = PrimerTheme.colors.outline,
    focusedLabelColor = PrimerTheme.colors.brand,
    unfocusedLabelColor = PrimerTheme.colors.onSurfaceMuted,
    cursorColor = PrimerTheme.colors.brand,
    focusedContainerColor = PrimerTheme.colors.surfaceRaised,
    unfocusedContainerColor = PrimerTheme.colors.surfaceRaised,
    disabledContainerColor = PrimerTheme.colors.surface,
)
