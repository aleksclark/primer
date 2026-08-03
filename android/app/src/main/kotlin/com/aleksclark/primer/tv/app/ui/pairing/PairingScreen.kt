package com.aleksclark.primer.tv.app.ui.pairing

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import com.aleksclark.primer.tv.app.ui.PairingUiState
import com.aleksclark.primer.tv.app.ui.components.PairingCard
import com.aleksclark.primer.tv.app.ui.designsystem.PrimerTheme

/**
 * Full-screen pairing onboarding. Replaces the legacy utility form with a
 * centered branded [PairingCard]; no scaffold chrome while unpaired.
 */
@Composable
fun PairingScreen(
    state: PairingUiState,
    onBaseUrlChanged: (String) -> Unit,
    onCodeChanged: (String) -> Unit,
    onSubmit: () -> Unit,
    modifier: Modifier = Modifier,
) {
    Box(
        modifier = modifier
            .fillMaxSize()
            .background(PrimerTheme.colors.background),
    ) {
        PairingCard(
            baseUrl = state.baseUrlInput,
            code = state.codeInput,
            submitting = state.submitting,
            error = state.error,
            canSubmit = state.canSubmit,
            onBaseUrlChanged = onBaseUrlChanged,
            onCodeChanged = onCodeChanged,
            onSubmit = onSubmit,
            // After a failed attempt, return focus to the code field.
            requestCodeFocus = state.error != null && !state.submitting,
            modifier = Modifier.fillMaxSize(),
        )
    }
}
